# Harness v1 实现 Spec（可开工版 · rev3）

> 收敛自蓝图 + 两轮架构异议 + 两轮评审（rev2 的 7 findings + 对抗审计 42 findings，全部确认为真）。
> rev3 三个结构性变化（把 42 条坍缩成根因修复，而非 42 个补丁）：
> 1. **写隔离 = worktree-per-write-job**（你选定）：所有写 job 在独立 worktree+分支里改并 commit；read-only job 跑共享主树。崩溃恢复=丢弃 worktree。**harness 永不写人类的主工作树**。
> 2. **状态 = 单一 append-only 事件日志 + 物化视图**：events 是唯一真相，JSON 是可重建投影；所有迁移走 `state.lock` 下「校验→append event(fsync)→更新视图」。
> 3. **信任边界**：worker 自报（changed_files/verification/usage）一律 informational-only，由确定性 CLI 用 git/重跑独立求真。
> 逐条处置见 §21。

---

## 0. v1 决策表（可推翻）

| # | 决策 | 取值 | 来源 |
|---|---|---|---|
| D1 | 委派模型 | **push**：orchestrator spawn 即派定，无队列/抢单 | — |
| D2 | 写隔离 | **worktree-per-write-job**：写 job 各自 worktree+分支；read-only job 共享主树 | 你选定（C2） |
| D3 | 主工作树 | **harness 永不写人类主工作树**；写/集成都在 worktree；结果以 harness 分支交付 | D2 推论 |
| D4 | 状态真相 | **events 为单一真相源**，JSON 视图可从 events 重建；迁移先 append event 再更新视图 | 审计 C1 |
| D5 | 信任边界 | worker 自报仅 informational；CLI 用 git/重跑独立求真 | 审计 C3 |
| D6 | git | 硬前置；非 git repo 报错提示；`.harness/` 必须 untracked（init 校验） | — |
| D7 | orchestrator | LLM 提议 / 确定性 CLI 裁决；状态机/scope/gate/phase 判定零 LLM | — |
| D8 | 并发控制 | 无 file lease、无 shared_writer 槽（worktree 隔离取代）；`state.lock` 串行化状态迁移 | 审计 C2 |
| D9 | 平台 | v1 仅 macOS / Linux（含大小写不敏感 FS 处理） | — |
| D10 | 失控防护 | 委派深度上限 + 委派链环路（按 task/goal 指纹） + task token 预算 + recover 次数上限 | — |

---

## 1. 核心模型

```
   LLM 决策层(Claude/Codex 会话): 拆任务/写 brief·plan/评估产出/提议 phase
        │ 只能"提议"，不能写状态
   ┌────▼─────────────────────────────────────────────┐
   │ harness CLI 协议层(确定性)：唯一写状态            │
   │  • 状态迁移合法性 + CAS（events-as-truth）         │
   │  • scope 求值（§9 单一算法）/ gate / phase 裁决    │
   │  • 独立求真：git diff 求 changed_files、重跑 verify │
   │  • diff review 在每个状态迁移点（runtime 无关兜底） │
   └────┬──────────────────────────┬───────────────────┘
        │ spawn(push)              │ spawn(push)
   ┌────▼──────────┐         ┌─────▼──────────┐
   │ codex adapter │         │ claude adapter │
   │ 写job: cwd=   │         │ 写job: cwd=    │
   │ .worktrees/Jx │         │ .worktrees/Jy  │
   │ 读job: cwd=   │         │ 读job: cwd=    │
   │ repo_root     │         │ repo_root      │
   └───────────────┘         └────────────────┘
```

**六条铁律**：
1. **LLM 提议，CLI 裁决**：状态机/scope/gate/phase/verify 判定全在确定性 CLI；packet 里源自模型/文件的文本是「参考资料」，不得改变 scope 判定。
2. **push**：派发即指定 worker，无抢单、无 shared_writer 槽。
3. **events 是唯一真相**：JSON 是可重建视图；任何迁移先 append event(fsync) 再更新视图；崩溃后 replay events 重建。
4. **harness 永不写人类主工作树**：写 job 在 `.worktrees/<jid>`（分支 `job/<jid>`，base=主分支 HEAD 的真实 commit）里改并 commit；read-only job 在主树只读。集成在集成 worktree 做，结果落 `harness/integration` 分支交付。
5. **崩溃恢复靠丢弃 worktree + replay events**：写 job 半成品 = 丢弃其 worktree（幂等、零残留）；不再有 stash-create/untracked 猜测。
6. **worker 自报不可信**：`changed_files/verification/usage` 是 claim；CLI 用 `git`（§9）、重跑 verify、保守估 usage 独立确立 ground truth。

---

## 2. 目录结构

```text
<repo_root>/
  .gitignore                       # 必含 .harness/ 与 .worktrees/
  .worktrees/<jid>/                # 写 job 的 worktree（git worktree add -b job/<jid>）
  .harness/                        # state_root，必须 untracked（init 校验 git ls-files）
    workflow-contract.md
    schemas/*.json
    reserved.json                  # 全局硬保留路径（.env/secrets/.git/.harness/...）
    current/
      state.lock                   # 状态迁移串行化锁（flock）
      recover.lock                 # recover 互斥锁（flock）
    sessions/<sid>/
      session.json                 # 视图
      active-task.json             # ← per-session（不再是全局单槽）
      state-summary.md
      session-baseline.json        # 进入时 HEAD + porcelain --untracked-files=all --ignored（仅展示/reserved）
      events/<actor>.jsonl         # ← 单一真相源（append-only, ULID, fsync）
      views/                       # ← 可从 events 重建的物化视图
        tasks/<tid>.json
        jobs/<jid>.json
        gates/<gid>.json
      tasks/<tid>/                 # 人类可读产物
        brief.md plan.md handoff.md verification.json
      artifacts/<jid>/
        stdout.log stderr.log process.json   # process.json 含 exit_code + final_json_ok
        final.json                            # Codex --output-last-message 落点
        diff.patch
        verify/*.json
    .trash/<jid>/                  # 删除 untracked 改为移入此处（可人工恢复，不 rm）
```

---

## 3. 数据契约（关键字段）

### 3.1 job 视图（views/jobs/<jid>.json，由 events 折叠重建）
```json
{
  "job_id": "J-002", "task_id": "T-001", "rev": 7,
  "created_by": "codex-main", "target_runtime": "claude",
  "role": "implementation", "writes": true,
  "status": "running",
  "mode": "worktree",
  "state_root": "/repo/.harness", "repo_root": "/repo",
  "workdir": "/repo/.worktrees/J-002", "branch": "job/J-002",
  "base_commit": "a1b2c3d",
  "worker": { "pid": 48219, "boot_id": "…" },
  "scope": { "allowed": ["src/auth/**","tests/auth/**"], "denied": [".github/**","package.json"] },
  "verification_requirements": ["npm test -- auth","npm run lint"],
  "delegation": { "depth": 1, "chain_fingerprints": ["T-001:auth-refactor"] },
  "budget": { "max_tokens": 200000, "timeout_s": 1800 },
  "recover_count": 0,
  "result_contract": "job-result.schema.json"
}
```
- `writes`：role∈{implementation,test,integration} → true（worktree）；analysis/review/verification → false（共享主树只读）。
- `base_commit`：worktree 分支起点的**真实 commit**（非 stash）。`worker.pid/boot_id`：recover 探活用。`rev`：CAS 版本。

### 3.2 job.result.json（worker 回写——**全部 informational-only**）
```json
{
  "job_id":"J-002","status":"completed",
  "summary":"重构 auth service 并补 4 个测试",
  "_informational": true,
  "changed_files":["src/auth/service.ts"],
  "verification":{"unit_test":"passed","lint":"passed"},
  "needs_human":false,"followups":[]
}
```
> schema 注释钉死：`changed_files`/`verification`/`usage` 为 worker 自述，**CLI 不采信**，仅作人读摘要。真相由 §9 git 求值 + §13 CLI 重跑 verify 确立。

### 3.3 task 视图
```json
{
  "task_id":"T-001","rev":12,"title":"auth 模块重构","status":"active","phase":"implement",
  "acceptance":["API 契约不变","新增测试通过","lint/typecheck 通过"],
  "job_ids":["J-001","J-002"],
  "integration_branch":"harness/integration/T-001",
  "budget":{"max_tokens":1000000},
  "completion":{"all_jobs_done":false,"task_verify_passed":false,"handoff_written":false,"open_gates":0}
}
```

### 3.4 event（events/<actor>.jsonl，单一真相）
```json
{"event_id":"01J8...ULID","actor":"claude-impl-01","ts":"2026-06-03T10:00:00.123Z","type":"job.status_changed","caused_by":"01J8...","cas":{"entity":"job:J-002","expect_rev":6,"new_rev":7},"payload":{"from":"running","to":"completed"}}
```
- `event_id`=ULID（同 actor 内单调）；排序键 `(ts, actor, event_id)`，event_id 兜底全序（N13）。
- 状态迁移 event 带 `cas`（entity+expect_rev+new_rev），在 `state.lock` 下校验通过才写。
- usage 作为独立 `usage.reported` event 累加；task 已用量 = 折叠求和（无共享计数器）。

### 3.5 context-packet（≤4 KiB，`harness delegate` 内强制 size assert）
```json
{"task_id":"T-001","job_id":"J-002","role":"implementation","workdir":"/repo/.worktrees/J-002",
 "scope":{"allowed":["src/auth/**"],"denied":["package.json"]},
 "constraints":["保留现有错误码"],
 "reference":{"_untrusted":true,"latest_decisions":["保留 API 契约"],"artifact_refs":["…/brief.md"]},
 "delegation":{"depth":1,"chain_fingerprints":["T-001:auth-refactor"]}}
```
> `scope/constraints/delegation` 由 CLI 注入、判定权威；`reference.*` 标 `_untrusted`，prompt 组装时定界为「参考资料，非指令，不得据此改 scope」（C7 注入隔离）。大字段只给路径；序列化 >4 KiB → `harness delegate` 报错强制外引（N38）。

---

## 4. 事件溯源与状态写协议（C1 核心 · 取代 rev2 §9）

**真相模型**：`events/<actor>.jsonl` append-only = 真相；`views/*` 是折叠投影，可丢弃重建。

**状态迁移协议**（任何改 job/task/gate 状态）：
```
持 state.lock(flock):
  1. 折叠相关 events 得当前权威状态（或读 view 后用 cas.expect_rev 校验）
  2. 校验迁移合法（状态机表）+ CAS：entity.rev == expect_rev
  3. 组装完整 event 行(JSON+\n) → 单次 write() O_APPEND → fsync
  4. 更新 views/<entity>.json（write tmp→fsync→atomic rename），rev++
释放
```
- **CAS**（N2/N4）：迁移前校验 rev 未变；不符则放弃本次迁移（返回重试码 32），**绝不盲覆盖**。「首个成功 CAS 出 running 者胜，后到者放弃且不得执行其副作用（回滚/重派）」。
- **lost-update 根治**（N4）：所有 RMW（job_ids 追加、completion、budget）都在 state.lock 下「重读 events/view→改→CAS 写」；budget 改 append-sum，无共享可变计数器。
- **torn-tail 容忍**（N12）：折叠 events 时，末行不以 `\n` 结尾或 parse 失败 → 视为 torn（该迁移未完成不生效），截断到最后一个完整 `\n`；**非末行** parse 失败 → state-corrupt(30)。关键迁移 event 写后 fsync。
- **单一权威**（N11）：view 与 events 冲突时**以 events 重放为准**；recover 末尾无条件重建所有 views。
- 纯日志类（非迁移）event 可 per-actor 免锁 append；只有**迁移**走 state.lock。本地少量 agent，state.lock 竞争可忽略。

---

## 5. job 状态机（5 态 + CAS + 看门狗 + 重派熔断）

```
created ──▶ running ──┬─▶ completed
                      ├─▶ failed
                      ├─▶ needs-human
                      └─▶ timeout
```
| from→to | 执行者 | 约束 |
|---|---|---|
| created→running | adapter | spawn 成功后、worker 动手前，CAS 写 running+event+fsync（§7 顺序） |
| running→{completed,failed,needs-human} | adapter | CAS（status 仍 running）；completed 需 CLI 重跑 required verify 全过（§13） |
| running→timeout | **adapter 看门狗** | adapter 内置 wall-clock，超 `timeout_s` 主动 `SIGKILL` 子进程组 → CAS 迁 timeout（N3） |
| 回收 stale running | 仅 `harness recover`（持 recover.lock） | 探活：`kill(pid,0)` 失败或 boot_id 不符 → 死亡；或 `now-running_since>timeout_s+grace`（N5） |

- **timeout 双路径**（N3）：正常由 adapter 看门狗；adapter 自身死亡由 recover 二分（有 base 且超时→timeout 非 created）。
- **重派熔断**（N3/N30）：`recover_count`/gate 重派计数超 `max_recover_retries`(=2) → `needs-human`，不再自动重派。
- 非法迁移 / CAS 失败 → 拒绝 + 写 `policy.violation` event。

---

## 6. task 状态机 + phase 命令 + completion contract（C6）

```
intake→research→plan→delegate→implement→verify→integrate→handoff→done
                              ▲           │   (verify 失败 → followup → implement)
            任一 → blocked-human / failed 旁路
```
- **phase 推进是确定性的**（N22）：新增 `harness task phase --to <phase>`，CLI 持合法转移表（禁 implement 直跳 handoff、禁 done 回退）；LLM 只能**提议**，CLI 裁决并写 event；非法迁移拒绝 + `policy.violation`。从执行者里删去「orchestrator 直接写」。
- **completion contract**（CLI 唯一判定 task→done，四条全真）：
  1. 所有 `job_ids` 状态=completed；
  2. task 级 verify 中**至少一条 required** 且全部 passed——**空集判 false**（非 vacuous-true，N23）；`required` 缺省=**true**（fail-safe）；
  3. `handoff.md` + `verification.json` 存在且非空；
  4. 关联本 task 的 open gate=0。
- **task-stop 拆分**（N21）：**worker** 会话的 Stop → 只校验本 job result 已写（job 级）；**orchestrator** 会话的 Stop 或显式 `harness task done` → 才校验 task contract。hook 按 actor 角色绑定不同行为。
- **active-task per-session**（N33）：`sessions/<sid>/active-task.json`；session-start/task-stop 以本会话 sid 取，不读全局单槽。

---

## 7. 工作目录与隔离模型（C2 落地 · worktree-per-write-job）

| | 写 job（implementation/test/integration） | read-only job（analysis/review/verification） |
|---|---|---|
| 隔离 | `git worktree add -b job/<jid> .worktrees/<jid>`，base=主分支 HEAD（真实 commit） | cwd=`repo_root`，只读主树 |
| 改动 | worker 在 worktree 内改 + commit（**不碰人类主树 HEAD/index**） | 不写 |
| 归因 | worktree 独立，`git -C <wt> diff --name-status -M <base_commit>`（含 untracked/rename，§9） | 无 |
| 崩溃恢复 | `git worktree remove --force` + `prune` + `branch -D`（幂等，§15） | 无 |
| 集成 | orchestrator 在**集成 worktree** `git merge job/<jid>`→`harness/integration/<tid>`；冲突→`git merge --abort`+gate（N17） | — |

**关键不变量与后果**：
- **harness 永不在人类主工作树执行写或 merge**（D3）。集成结果落 `harness/integration/<tid>` 分支，handoff 交付该分支供人类合并——**不是就地改你的工作树**。
- 写 job base=主分支 HEAD 的**已提交**状态：人类未提交改动**不被写 job 看见也不被触碰**（安全）；read-only job 在主树能看到未提交状态（供分析）。若某写任务必须基于人类未提交工作 → `harness session start` 可（gate 后）把它快照成 base commit，默认不做。
- adapter spawn 前的 git 副作用顺序见 §8（意图先落盘，防孤儿 worktree N9）。
- 这一模型**溶解** N1/N5/N6/N7/N8/N19/N34/N37（无 shared_writer 槽、无 stash、回滚=丢 worktree=天然 per-job 隔离）。

---

## 8. 委派协议 + adapter（push + 信任边界 + 容错）

`harness run codex|claude --job <jid>`。adapter 步骤（严格 happens-before，N8/N9）：
```
1. CAS 写 job 视图(mode/writes/scope/budget) + base_commit 落盘 fsync
2. 写 job: git worktree add -b job/<jid>（add 成功后立即写 worktree.created event）
3. 组 prompt：CLI 注入 scope/constraints(权威) + reference(_untrusted 定界)；size assert ≤4 KiB
4. spawn(cwd=workdir)；spawn 成功、worker 动手前 → CAS 迁 running + fsync（§5）
5. 内置看门狗(timeout_s)；捕获 stdout/stderr/exit → artifacts/，写 process.json{exit_code,final_json_ok}
6. 抽取 result(§8.2) + 完整性校验 + schema 校验(§8.3)
7. **独立求真**(§8.4) → CAS 迁终态 + event(fsync)
8. 写 job: 不在此 merge；交 orchestrator 集成阶段
```

### 8.2 result 抽取来源（N36/F6）
| runtime | result 来源 | usage 来源 | 完整性 |
|---|---|---|---|
| Claude | stdout 解析为单 JSON | 同对象 usage | parse 失败/torn |
| Codex | 读 `final.json`（--output-last-message） | stdout JSONL 事件 | `final.json` 末尾完整 JSON |
- `final.json` torn（N36）→ 判 `runtime-exec-failed(40)`（进程层，重启进程），**不**进 schema 修复回路（不是 result-invalid）。

### 8.3 schema 校验失败回路
`合 schema→用；fail 1 次→resume 回灌 schema error 要求只重出 JSON→再校验；fail 2 次→failed(22) 留原始候选`。区分进程层(40,重启) vs 模型层(22,修复)。

### 8.4 信任边界：CLI 独立求真（C3 · N20/N23/N28/R5）
- `changed_files`：**弃用 worker 自报**，用 §9 scope 求值算出真实改动集。
- `verification`：completed 前 **CLI 独立重跑** `verification_requirements`，以实跑 exit code 为准，覆盖/否决 result.verification。
- `usage`：取 runtime 自报；**缺失则保守估**（prompt 字节/4 + max_output），缺失视为「预算不可核算」→ gate；累加+判定在 state.lock 内原子（N4/N28）。budget 是**软门禁**（只阻下次派发，不硬截已 running），文档写明。
- `expected_write_scope`（Bash，R5）：必须 ⊆ scope.allowed，CLI 求交、worker 只能缩不能扩，超出即 gate。

### 8.5 委派深度 + 环路（N29）
- `depth`=chain 长度，硬上限 `max_delegation_depth`(=3)，超→拒绝(42)。
- 环 key=**task_id+归一化 goal 指纹**（非 (runtime,role)）：祖先链出现同一指纹→判环拒绝(42)。删除含糊的「超阈」。

---

## 9. scope 求值算法（C4 · 单一确定性算法，预防与检测共用）

**输入规范化**（N25/N32）：每个目标路径先 `realpath`（解 symlink + 规范化 `..`）→ 必须落在判定基准实根内（`commonpath` 校验），否则 gate(10)；拒绝目标为 symlink 或经 symlink 的写（逐段 `lstat`）；大小写不敏感 FS（`core.ignorecase=true`）统一 `casefold` 比对。

**真实改动集来源**（不止 `--name-only`，N14/N15/N18）：
```
changed = (git status --porcelain --untracked-files=all --ignored)   # 含新建 untracked + .gitignore 命中
        ∪ (git diff --name-status -M <base_commit>)                  # 含 rename old+new 两端
```
（写 job 在其 worktree 内求；read-only 不产生改动）

**判定优先级**（N24，写死）：
```
reserved-deny（reserved.json，最高，命中即 blocked-policy 10，不给 gate）
  → denied（命中即拒）
    → allowed（命中即放行）
      → default-deny（未命中 allowed 一律 gate，绝不隐式放行）
```
glob 引擎钉死：gitignore/minimatch 风格，`**` 跨目录，`p/**` 同时匹配 `p` 与 `p/x`，`*` 不跨 `/`。

**reserved.json 默认**：`**/.env*`、`**/secrets/**`、`**/*.pem`、`**/id_rsa`、`.git/**`、`.harness/**`、`.worktrees/**` + session 进入时的 pre-existing untracked/ignored（N15）+ 用户 `reserved_by_human`。

---

## 10. scope 安全三层（预防 > 检测 > 回滚）

| 层 | 写 job（worktree） | read-only job |
|---|---|---|
| 预防 | spawn 时 sandbox-first：Claude `--allowedTools` 白名单 + `Bash(...)` 模式、Codex `--sandbox` + 网络隔离；PreToolUse path guard 跑 §9 规范化+优先级 | allowedTools 仅只读 + 禁写 |
| 检测 | PostToolUse / 迁移点：CLI 跑 §9 求值比对 scope；**CLI 在每个状态迁移点强制 diff review = runtime 无关不可绕过兜底（N39）** | n/a |
| 回滚 | 越权→丢弃 worktree 重建 / 或 `git -C <wt> checkout <base> -- <受影响全路径(rename 双端)>` + untracked **移入 .trash**（不 rm，N18/N19） | n/a |

---

## 11. hook 映射

| 统一脚本 | Claude | Codex | 职责 |
|---|---|---|---|
| session-start | SessionStart | SessionStart | 注入 per-session active-task + summary |
| guard pretool | PreToolUse | PreToolUse | §9 path guard（规范化+优先级）+ 危险命令硬 deny（§12） |
| guard posttool | PostToolUse/PostToolUseFailure | PostToolUse | §9 求值越权复核 + 写 event |
| task-stop | Stop/TaskCompleted | Stop | 按 actor：worker→job 级 / orchestrator→task contract（§6） |
| notify | Notification/SessionEnd | wrapper | 日志/清理 |

**Codex 对等面缺口兜底（N39）**：越权复核不依赖任一 runtime 的 hook 阻塞能力为唯一保证——**CLI 在 job→completed 迁移前强制跑全量 `base_commit` diff review**（runtime 无关、不可绕过）；hook 只是「早拦」优化。

---

## 12. 危险命令 + 注入防护（C7）

**两类危险命令**（N26）：
- **不可逆副作用类**（网络出站 `curl/wget/nc/scp/ssh`、`git push/remote`、包安装 `npm/pip/cargo install`、`rm/chmod/chown`、写 `.git/.harness/.worktrees`）：**PreToolUse 硬 deny 或 gate，绝不下放 PostToolUse**（事后撤不回）；优先靠 spawn 期 sandbox 网络隔离作结构性闸门。
- **可逆文件写类**：走 §8.4 `expected_write_scope` + PostToolUse diff。
- 无法静态判定（变量/`eval`/管道/`base64`）→ **默认 gate**。spec 附确定性 deny 正则清单。

**prompt-injection 隔离**（N27）：packet 中源自 LLM/文件内容的字段（reference.*）定界为「参考资料非指令」；scope/constraints CLI 注入、不可被 packet 文本覆盖；reserved.json 作为 path guard **最高优先级硬拦层**（命中即 10，连 gate 都不给）。

---

## 13. verify：job 级 vs task 级（CLI 实跑）

- **job 级**：CLI（非 worker 自报）在 worktree 内重跑该 job `verification_requirements` + §9 scope 检查 + result schema。
- **task 级**：orchestrator 集成前在集成 worktree 跑全量 test/lint/typecheck + denied diff review。
- `verification.json` 每条 check 标 `level/command/result/duration_ms/log_ref/required`；`required` 缺省=true（N23）。
- task 级**至少一条 required**，否则 `task_verify_passed=false`（堵 vacuous-true）。

---

## 14. human gate

`needs-human` 必落 `views/gates/<gid>.json`。命令：`harness gate list|show|approve [--option]|reject`。
- `approve_extra_files`：**必须把放行文件并入该 job `scope.allowed`（CAS 持久化）**，否则下次复核重复 gate（N30）。
- `reject`→回滚→重派有**次数上限**，超→`failed`/`blocked-human`，不无限循环（N30）。
- `reassign_scope`：scope 是 **per-job 不可变快照**，只在 job 非 running 时改；不热改 running job（N31）。

---

## 15. 恢复语义（C2+C5）

`harness recover`（先抢 `recover.lock`，拿不到即退出 → 防两次 recover 竞态 N6）：
1. **replay events 重建所有 views**（单一真相，N11）；torn 末行容忍（N12）。
2. 找 stale：`running` 且 worker 探活失败（`kill(pid,0)`/boot_id）或超 `timeout_s+grace`（N5，区分慢但活）。
3. **写 job 恢复**：`git worktree remove --force` + `git worktree prune` + `git branch -D job/<jid>`（幂等，N16）→ `recover_count++`，未超限则置 created 重派（重派由 orchestrator 单一入口，recover 不直接 spawn，N6）。
4. **孤儿 worktree 扫描**（N9）：`git worktree list --porcelain` + 扫 `.worktrees/`，git 注册但无对应有效 running job → `remove --force` 清理。
5. 集成中断：主分支 merge 一律在集成 worktree，冲突已 `--abort`（N17），主树永不半合并。
6. `.harness` 被误 track 检查（N35）：`git ls-files --error-unmatch .harness` 命中 → state-corrupt(30) 提示 `git rm -r --cached`。
7. 重生成 state-summary。

> 因 worktree 隔离：写 job 恢复 = 丢弃 worktree，**天然只影响该 job**（根除 N34「串行≠回滚隔离」），不再有 untracked 误删（N19）。

---

## 16. CLI 命令清单

```text
harness init                      # 建 .harness/+schema+reserved；非 git/已 track .harness → 报错
harness session start             # 建 session + session-baseline + per-session active-task
harness task create | phase --to  # 建 task / CLI 裁决 phase 迁移
harness state summary
harness delegate                  # 建 job + 选 writes→worktree / readonly→shared + packet(size assert)
harness run codex|claude --job    # adapter：worktree/spawn/看门狗/求真/CAS 迁移
harness guard pretool|posttool
harness verify [--job|--task]     # CLI 实跑
harness integrate                 # 集成 worktree merge → harness/integration；冲突 --abort+gate
harness handoff
harness gate list|show|approve|reject
harness recover                   # recover.lock + replay + 丢 worktree + 孤儿扫描
```

---

## 17. 错误码

`0=ok` · `10=blocked-policy` · `12=needs-human` · `20=verify-failed` · `22=result-invalid` · `30=state-corrupt` · `31=lock-timeout` · `32=cas-retry` · `40=runtime-exec-failed` · `41=budget-exceeded` · `42=delegation-loop`

---

## 18. 可测性：mock runtime（崩溃注入一等公民）

`Runtime` 可注入接口 + `mock`：正常/坏 schema/越权写(新建文件·ignored·symlink·大小写·rename)/僵死/非零退出/自报 usage=0/伪造 changed_files。**崩溃注入**：在 §4 协议每步、§8 adapter 每步之间 SIGKILL，断言 recover 后 views==events 重放、worktree 无孤儿、无数据丢失。CAS 竞态、torn-tail、env-injection、reserved 硬拦全部 mock 可测。**不先定这条，状态机无法回归**。

---

## 19. 落地顺序（M1–M6）

| M | 内容 | 验收 |
|---|---|---|
| M1 | events-as-truth + §4 协议 + views 重建 + init/session/task + reserved.json | 崩溃注入下 recover 后 views==events 重放 |
| M2 | Runtime 接口 + mock + job/task 状态机(CAS) + §9 scope 算法 | mock 全分支(越权5型/CAS/torn/孤儿)绿 |
| M3 | 真实 adapter + worktree 写隔离 + result 求真(§8.4) + 看门狗 | 双向 Codex↔Claude；completed 由 CLI 实跑 verify 判定 |
| M4 | hook：path guard(§9) + 迁移点 diff review + task-stop 拆分 + 危险命令硬 deny | 越权(含新建/ignored/symlink)被拦；伪造 done 被堵 |
| M5 | verify 分层(CLI 实跑) + integrate(集成 worktree merge+abort) + handoff + gate | 一个 task create→handoff/integration 分支闭环 |
| M6 | recover 全套(孤儿扫描/prune/branch -D/熔断) + 委派深度·环路·budget | kill 写 job 后 recover 幂等恢复，无孤儿 |

---

## 20. v1 明确不做

shared-write 模式（worktree 取代）· file lease/heartbeat/shared_writer 槽 · 全局 event seq · 队列/抢单 · 就地改人类主工作树 · 跨 repo · team mailbox · UI · 语义记忆 · 自动三方合并 · 插件分发 · Windows。

---

## 21. 处置表（rev2 7 findings + 对抗审计 42 findings）

**rev2**：F1→§7 worktree 取代串行写(不再需占用槽)·F2→§7 worktree 隔离+§9 ignored 纳入·F3→§3.4 ULID+§4 torn 容忍·F4→§7/§13 worktree 也跑 scope+merge 前 denied 复核·F5→§8.4 expected_write_scope⊆allowed·F6→§8.2·F7→§6。

**审计 42（全部确认为真）**：

| 簇 | findings | 落点 |
|---|---|---|
| C1 事件溯源 | N4 N10 N11 N12 N13 N28 N40 | §4 §3.4 §8.4 |
| C2 写隔离(worktree) | N1 N3 N5 N6 N7 N8 N19 N34 N37 | §5 §7 §15 |
| C3 信任边界 | N20 N23 N28 R5 | §3.2 §8.4 §13 |
| C4 scope 求值 | N14 N15 N18 N24 N25 N32 | §9 §10 |
| C5 git/worktree 生命周期 | N9 N16 N17 N35 N36 | §8 §15 §16 |
| C6 task 级确定性 | N21 N22 N33 | §6 |
| C7 危险/注入 | N26 N27 N29 N30 N31 N38 N39 R4 | §8.5 §12 §14 |
| N2 双写 CAS | N2 | §4 §5 |
```
注：N29(环路)归 §8.5；N30(gate 回环)归 §14；N31(scope TOCTOU)归 §14；N38(packet 4KiB)归 §3.5/§8。
```
