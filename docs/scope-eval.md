# Scope 求值算法（rev3 §9 落地 · 语言无关伪代码）

> 单一确定性算法，**预防层（PreToolUse path guard）与检测层（PostToolUse / 迁移点 diff review）共用**。
> 设计目标：堵住对抗审计发现的六种绕过——新建 untracked（N14）、.gitignore 命中（N15）、rename 双端（N18）、glob 优先级/语义（N24）、symlink+`../` 穿越（N25）、大小写不敏感 FS（N32）。
> 所有判定零 LLM 参与（rev3 D7）。worker 自报 `changed_files` 不参与（rev3 C3）。

---

## 0. 数据结构

```
Scope     = { allowed: [glob], denied: [glob] }          # per-job 不可变快照
Reserved  = { patterns:[glob], pre_existing_untracked:[path],
              pre_existing_ignored:[path], reserved_by_human:[glob] }   # .harness/reserved.json
Decision  = ALLOW | DENY_SCOPE | DENY_RESERVED | GATE
Verdict   = { path: realpath, decision: Decision, rule: glob|null }
```

`base_root`（判定基准实根）：
- 写 job（worktree）：`job.workdir`（即 `.worktrees/<jid>`）
- read-only job：`repo_root`（只读，不应产生改动）

---

## 1. 路径规范化 `normalize(raw, base_root) -> realpath | REJECT`

> 修 N25（symlink/`../`）、N32（大小写）。**预防层和检测层都必须先过这一步**，否则字符串匹配可被绕过。

```
function normalize(raw, base_root):
    # 1. 解析为绝对路径（不跟随末段 symlink 之前先做语法规范化）
    abs = raw if is_absolute(raw) else join(base_root, raw)

    # 2. 逐段 lstat：禁止路径中任一段是 symlink（防经 symlink 写到树外，N25）
    for seg_prefix in ancestors(abs):           # base_root/a, base_root/a/b, ...
        if lexists(seg_prefix) and is_symlink(seg_prefix):
            return REJECT("symlink-in-path", seg_prefix)

    # 3. 规范化 '..' 与 '.'（纯语法），得到 canonical
    canon = lexical_normalize(abs)               # 不访问 fs，纯字符串折叠 ../

    # 4. 逃逸检查：canon 必须在 base_root 实根内
    if commonpath([realpath(base_root), canon]) != realpath(base_root):
        return REJECT("escapes-base-root", canon)

    # 5. 大小写：仓库卷大小写不敏感时（git config core.ignorecase=true）统一 casefold
    if repo_ignorecase:
        canon = casefold(canon)
    return canon
```

- `REJECT` 在**预防层**等价于 `GATE`（更严格可直接 `DENY_RESERVED`/`blocked-policy 10`）。
- 对「worker 新建一个指向 scope 外的 symlink」这个动作本身：预防层 `GATE`（创建 symlink 视为高风险）。

---

## 2. glob 匹配 `glob_match(path, pattern) -> bool`

> 修 N24（语义未定）。**钉死 gitignore / minimatch 风格**，实现必须照此，禁止用各语言默认 glob：

```
规则:
  '**'  跨任意层目录（含 0 层）
  '*'   匹配单层内任意字符，不跨 '/'
  '?'   单字符，不跨 '/'
  前缀目录 glob 'p/**' 同时匹配 'p' 自身与 'p/x/y'    # 关键：避免漏匹配目录本身
  尾部 '/' 表示仅目录
  匹配前 path 与 pattern 都已按 §1 casefold（若 ignorecase）
实现建议: 编译为锚定正则；'p/**' => ^p(/.*)?$
```

---

## 3. 分类 `classify(realpath, scope, reserved) -> Verdict`

> 修 N24 优先级未定。**优先级写死，顺序不可换**：

```
function classify(rp, scope, reserved):
    # 3.1 reserved 最高优先级硬拦（命中即拒，连 gate 都不给，N15/N27）
    reserved_all = reserved.patterns
                 + reserved.pre_existing_untracked
                 + reserved.pre_existing_ignored
                 + reserved.reserved_by_human
    for g in reserved_all:
        if glob_match(rp, g): return Verdict(rp, DENY_RESERVED, g)

    # 3.2 denied 优先于 allowed（denied 命中即拒，无视 allowed，N24）
    for g in scope.denied:
        if glob_match(rp, g): return Verdict(rp, DENY_SCOPE, g)

    # 3.3 allowed 命中放行
    for g in scope.allowed:
        if glob_match(rp, g): return Verdict(rp, ALLOW, g)

    # 3.4 default-deny：既不在 allowed 也不在 denied -> 一律 GATE，绝不隐式放行（N24）
    return Verdict(rp, GATE, null)
```

映射到错误码：`DENY_RESERVED -> blocked-policy(10)`；`DENY_SCOPE -> 越权处置（§5）`；`GATE -> needs-human(12)`。

---

## 4. 真实改动集 `changed_set(workdir, base_commit) -> {path: status}`

> 修 N14（新建 untracked）、N15（.gitignore 命中）、N18（rename 双端）。
> **绝不只用 `git diff --name-only`**——它看不到新建文件、看不到 ignored、把 rename 拆成 delete+add 丢失关联。

```
function changed_set(workdir, base_commit):
    result = {}   # realpath -> status ∈ {modified, added, deleted, renamed_from, renamed_to, untracked, ignored}

    # 4.1 tracked 改动（含 rename 检测；-M 保留 old+new 两端）
    for line in `git -C {workdir} diff --name-status -M --no-renames=false {base_commit}`:
        # 形如:  M path | A path | D path | R100 old new
        if line.status startswith 'R':
            add(result, old, renamed_from); add(result, new, renamed_to)   # 两端都进集合
        else:
            add(result, path, map(status))   # M/A/D

    # 4.2 工作区 untracked + ignored（覆盖 diff 的盲区）
    for line in `git -C {workdir} status --porcelain=v1 --untracked-files=all --ignored`:
        # '?? path' = untracked ; '!! path' = ignored
        if code == '??': add(result, path, untracked)
        if code == '!!': add(result, path, ignored)

    # 4.3 untracked 目录折叠项递归展开到文件级（porcelain 可能只给目录名）
    expand_untracked_dirs(result)

    # 4.4 全部经 §1 normalize（统一基准与大小写），丢弃 normalize=REJECT 的项前先记 scope.violation
    return { normalize(p, workdir): st for p, st in result }
```

> read-only job 的 `changed_set` 应为空；非空即 worker 越权写，直接 `scope.violation` + gate。

---

## 5. 越权检测与处置 `review(job) -> [Verdict]`

> 检测层（PostToolUse / job→completed 迁移点）。**CLI 在 job→completed 前强制跑一次全量 review，runtime 无关、不可绕过（N39）**。

```
function review(job):
    changed = changed_set(job.workdir, job.base_commit)
    verdicts = [ classify(rp, job.scope, reserved) for rp in changed.keys ]

    violations = [ v for v in verdicts if v.decision != ALLOW ]
    if violations is empty: return OK

    for v in violations:
        switch v.decision:
          DENY_RESERVED: open_gate(job, reason="reserved-path", paths=[v.path]); status=needs-human
          DENY_SCOPE, GATE:
             # 越权处置（rev3 §10）
             if formatter_spread_within_allowed_prefix(v.path):   # 同 ownership 前缀内的格式化扩散
                 allow_with_note(v.path)
             else:
                 rollback(job, v.path)                            # §6
                 open_gate(job, reason="scope-violation", paths=[v.path])
    write_event("scope.violation", job, violations)
```

`formatter_spread_within_allowed_prefix`：改动落在某条 `allowed` glob 的前缀目录内、且仅是既有文件的格式化（非新增 denied 文件）时放行；否则一律 gate。

---

## 6. 回滚 `rollback(job, paths)`

> 修 N18（rename 单侧回滚不全）、N19（误删人类 untracked）。**只回越权 pathspec，绝不整仓**。

```
function rollback(job, paths):
    if job.mode == worktree and rollback_whole_job:
        # 写 job 崩溃恢复的首选：直接丢弃整个 worktree（天然 per-job 隔离，rev3 §15）
        git worktree remove --force {job.workdir}
        git worktree prune
        git branch -D {job.branch}
        return

    # 局部回滚（gate reject / 单路径越权）：基于 name-status -M 拿完整改动集
    full = `git -C {job.workdir} diff --name-status -M {job.base_commit}`
    for p in affected_closure(paths, full):     # rename 的 old+new 两端都纳入
        if p is tracked-change:
            git -C {job.workdir} checkout {job.base_commit} -- {p}   # 含被删恢复 + 新增端回到 base(不存在=删)
        if p is untracked-added-by-worker:
            move_to_trash(p, ".harness/.trash/{job_id}/")            # 不 rm，可人工恢复（N19）
    # 绝不删「不在 base_snapshot 但也非本 worker 所写」的 untracked —— 见 §7 归因
```

---

## 7. per-tool-call 增量基准（R5 · 精确归因与最小回滚）

> 单条 Bash 的 formatter/codegen 扩散，需精确定位「这条命令新增了什么」，job 级 `base_commit` 会把之前所有改动算进来。维护两级基准：

```
job 级:   base_commit         # 整体归因 / worktree 丢弃恢复
call 级:  PreToolUse 记录该次调用前的 `git stash create`（或 status 指纹）
          PostToolUse 与之 diff，得本次调用精确增量 -> 比对 scope -> 最小回滚
```

- `expected_write_scope`（无法静态解析写范围的 Bash）：worker 声明值必须 `⊆ scope.allowed`，CLI 求交、只能缩不能扩，超出即 gate（R5）。

---

## 8. 测试矩阵（mock，对应 §18 崩溃/越权注入）

| 用例 | 期望 |
|---|---|
| worker 新建 `infra/x.sh`（untracked，scope 外） | §4.2 捕获 → DENY/GATE，非放行（N14） |
| worker 写 `.env`（.gitignore 命中） | §4.2 `!!` 捕获 + reserved 命中 → DENY_RESERVED（N15） |
| `git mv src/auth/a.ts src/payments/b.ts` | §4.1 rename 双端入集合，b.ts 越权 → 回滚两端（N18） |
| `denied=[package.json]`，写 `Package.json`（macOS） | §1 casefold → 命中 denied（N32） |
| 写 `src/auth/../../package.json` | §1 逃逸检查 REJECT（N25） |
| `ln -s ../../package.json src/auth/l` 后写 `l` | §1 symlink-in-path REJECT（N25） |
| `allowed=[**]`，`denied=[package.json]`，写 package.json | §3.2 deny 优先 → DENY_SCOPE（N24） |
| 写 `src/new.ts`，既不在 allow 也不在 deny | §3.4 default-deny → GATE（N24） |
| read-only job 产生任何改动 | `changed_set` 非空 → scope.violation + gate |
