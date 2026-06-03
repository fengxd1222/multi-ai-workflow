# IMPLEMENTATION_PLAN

> 实现 `docs/harness-v1-spec.md`（rev3）。技术栈 Go 1.26；交付分支模型；CLI+hooks+adapters（无自动 orchestrator，v2 再做）。
> 里程碑 M1–M6 见 spec §19。本文件只追踪**当前在做的 stage** 的细分与状态，完成即更新。

## 已锁定决策（不再动）
push 模型 · worktree-per-write-job · events 单一真相 · 信任边界(worker 自报不可信) · LLM 提议/CLI 裁决 · git 硬前置 · 交付分支(harness 永不写主工作树) · 仅 macOS/Linux · Go · 当前目录同仓(docs/ 放设计)。
默认值: max_delegation_depth=3 · max_recover_retries=2 · stale grace=60s · MCP v1 不做。

---

## Stage M1: 状态基质 + init/session（进行中）

**Goal**: 把 events-as-truth 的写协议、CAS、视图重建跑通，并能 `harness init` / `harness session start` / `harness recover`(重建视图)。这是整个系统地基。

**模块**（internal/）:
- `store`: 原子写(tmp+fsync+rename)、flock(state.lock/recover.lock)、路径布局。
- `model`: Event/Session/Task/Job/Gate 结构（M1 用 Event/Session/Task）。
- `event`: ULID(单 actor 单调) · append(O_APPEND 单次写+fsync) · fold/replay((ts,actor,event_id) 全序) · torn-tail 容忍。
- `state`: 迁移协议(lock→校验→append event→更新视图→rev++) · CAS · 从 events 重建视图。
- `cli`: init(git 检查 + .harness 未 track 校验 + 写 schemas/reserved/contract) · session start(baseline 捕获 + per-session active-task) · recover(replay→重建视图)。
- `schemas/`: go:embed 内嵌，init 时写入 .harness/schemas/。

**Success Criteria**:
- [x] `harness init` 在非 git repo 报错(exit 64)；在 .harness 已被 track 时报 state-corrupt(30)。
- [x] `harness init` 写出 .harness/ 全套 schema + reserved.json + workflow-contract.md，并把 .harness/ 加入 .gitignore。
- [x] `harness session start` 建 session、捕获 session-baseline(HEAD + porcelain -uall --ignored)、建 per-session active-task。
- [x] 状态迁移走 state.lock + CAS；CAS 不符返回 ErrCASRetry(32) 且不覆盖。
- [x] `harness recover` 重建视图 == events 重放(rebuild==incremental 测试)；events 末行 torn 被容忍跳过，非末行损坏报 30。
- [x] 同 actor 多进程并发 append 不丢事件、event_id 单调；折叠全序确定(-race 通过)。

**实测**: 全包测试绿；覆盖率 cli 81% / event 91% / state 80% / store 83.5%；`go vet` 干净；二进制端到端冒烟(init/session/recover/非 git 拒绝)通过。

**Tests**（测试先行）:
- store: 原子写中断不留半文件可见；flock 串行化两并发写。
- event: append 后 fold 还原；torn 末行容忍 vs 中间行报错；并发 append 计数守恒；ULID 单调。
- state: 迁移 CAS 成功/失败(32)；非法迁移拒绝+写 policy.violation；视图重建幂等。
- cli: init 非 git 报错 / .harness tracked 报 30 / 产物齐全；session start 基线正确。

**Status**: Complete

---

## Stage M2: scope 求值 + Runtime 接口/Mock（进行中）

**Goal**: 落地 scope-eval.md 的确定性算法（路径规范化/glob/分类/真实改动集），配 8 条越权用例；建立 Runtime 抽象与 Mock（崩溃/越权/僵死/坏 schema/非零退出注入），作为 M3/M4 的可注入测试缝。job/task 状态机 CAS 已在 M1 完成。

**模块**（internal/）:
- `scope`: Normalize(realpath+逐段 lstat 防 symlink+逃逸+casefold) · Match(gitignore/minimatch，`p/**` 匹配 `p`) · Classify(reserved>denied>allowed>default-deny) · ChangedSet(`status --porcelain -uall --ignored` ∪ `diff --name-status -M`) · LoadReserved。
- `runtime`: Request/Result · Runtime 接口 · Mock(脚本化场景：Normal/BadSchema/NonZeroExit/Zombie/ScopeViolation)。

**Success Criteria**:
- [x] 8 条越权用例(scope-eval.md §8)全绿：新建 untracked / .env ignored / rename 双端 / 大小写 / `../` 逃逸 / symlink 穿越 / deny>allow / default-deny。
- [x] Match 语义正确：`**` 跨目录、`*` 不跨 `/`、`p/**` 同时匹配 `p` 与 `p/x`。
- [x] Classify 优先级 reserved>denied>allowed>default-deny。
- [x] ChangedSet 含新建 untracked、ignored、rename 双端。
- [x] Mock 场景行为正确：Zombie 响应 ctx 取消并标 KilledByWatchdog；NonZeroExit 退出码≠0；Normal/Torn/NoUsage/ScopeViolation。
- [x] 覆盖率 scope 81.9% / runtime 83.3%，`-race` 干净。

**Status**: Complete

## Stage M3: adapter 编排 + worktree 写隔离 + 真实 runtime（进行中）

**Goal**: 把 state/scope/runtime 拼成 adapter：写 job 起 worktree → 经注入 Runtime 跑 → 抓 artifacts → result 抽取+schema 校验(修复回路) → §8.4 求真(git 算 changed_files、CLI 重跑 verify) → CAS 迁终态。看门狗超时 SIGKILL。真实 codex/claude 薄封装。

**模块**（internal/）:
- `worktree`: Add(`git worktree add -b job/<jid>`，base=HEAD) · Remove(remove --force + prune + branch -D，幂等)。
- `runtime`: 进程运行器(Setpgid + 看门狗 kill -pgid) + Codex/Claude argv 构造 + result 抽取(§8.2)。
- `adapter`: Run 编排；packet 组装(≤4KiB assert) + verify 实跑 + 求真 + CAS 迁移。
- `model`: JobResult + 校验；state 扩展 TransitionJobRunning(写 worker/worktree 字段)。

**Success Criteria**:
- [x] worktree Add/Remove 幂等：Remove 后再 Add 不因分支残留 fatal（N16）。
- [x] Mock 驱动：Normal→completed；ScopeViolation→needs-human(求真 git diff 抓越权，不信自报)；BadSchema→修复重试→completed；Zombie→timeout(看门狗)；NonZeroExit/Torn→runtime-exec-failed→failed；verify 失败→failed。
- [x] result 求真：completed 由 CLI 重跑 verify 判定 + git 算 changed_files，非 worker 自报（C3）。
- [x] 进程运行器看门狗：超时 SIGKILL 子进程组，快速返回。
- [x] 覆盖率 adapter 82.9 / runtime 90.1 / worktree 94.1 / cli 80.3 / state 83.1，`-race` 干净；真实 codex/claude 用 missing-bin 确定性测试替代 skip。
- [x] `harness run --job` CLI 接通整链；runtime bin 可经 HARNESS_{CODEX,CLAUDE}_BIN 覆盖。

**Status**: Complete

## Stage M4: hooks 闸门 + 危险命令硬 deny + 注入隔离（进行中）

**Goal**: PreToolUse path guard(§9 算法)、PostToolUse 迁移点 diff review、task-stop 拆 worker/orchestrator；危险命令(不可逆副作用)PreToolUse 硬 deny、混淆/不可解析 gate；reserved 硬拦层 + 注入隔离。

**模块**（internal/）:
- `guard`: ClassifyCommand(网络出站/push/装包/destructive/写保护→deny；eval/base64/管道→gate) · EvaluatePreTool(write 工具路径过 §9 Normalize+Classify+命令危险性) · PostToolReview(ChangedSet 求真) · hook I/O 解析(codex/claude PreToolUse→ToolCall) + 决策格式化。
- `cli`: guard pretool/posttool · hook task-stop(worker→job result 已产出 / orchestrator→completion contract) · ContractStatus 计算。

**Success Criteria**:
- [x] ClassifyCommand：rm -rf/curl/git push/npm install/chmod/写 .git→deny；eval/`$(`/base64 -d/管道 sh→gate；echo/npm test→allow。
- [x] EvaluatePreTool：write 工具 reserved/denied/越界/symlink→deny；default-deny→gate；allowed→allow；read-only 工具→allow。
- [x] PostToolReview：worktree 内越权(走 ChangedSet，含新建/ignored/rename)→违规并标记。
- [x] task-stop：worker 缺 result/非终态→block；orchestrator contract(4 条)未满足→block，满足→放行。
- [x] hook I/O：claude/codex PreToolUse 解析为 ToolCall；决策按平台格式化(claude permissionDecision / codex decision)；CLI 经 stdin 驱动可测；不可解析→gate(fail-safe)。
- [x] 覆盖率 guard 81.2 / cli 82.9 / store 84.4，`-race` 干净。

**Status**: Complete

## Stage M5: 闭环 delegate→verify→integrate→handoff + gate（进行中）

**Goal**: 一个 task 从 create 到 handoff/交付分支跑通闭环。delegate 建 job；verify 分层 CLI 实跑写 verification.json；integrate 在集成 worktree 合并 job 分支→harness/integration/<tid>（merge 前 denied 复核 R4，冲突 --abort+gate N17）；handoff 渲染；gate 事件溯源 + list/approve/reject（approve_extra_files 扩 scope N30）。

**模块**:
- `state`/`model`: gate 事件溯源(gate.opened/resolved + Gates 视图) · ExtendJobScope(job.scope_extended)。
- `internal/verify`: Run(workdir, cmds) → model.Verification + allPassed。
- `internal/integrate`: 集成 worktree + 逐 job 分支 merge + denied 复核 + --abort+gate。
- `cli`: delegate · verify · integrate · handoff · gate list/show/approve/reject；ComputeContract 改为按 TaskID 扫 job 视图。

**Success Criteria**:
- [x] delegate 建 job(role→writes/mode)，adapter 可消费；委派深度上限→ExitDelegationLoop。
- [x] verify --task 实跑命令写 verification.json(verify 包)，contract 条2 据此判定；空集非 vacuous-true。
- [x] integrate：无冲突合并 job 分支→harness/integration/<tid>；denied 改动→拒绝 merge+gate；冲突→--abort(主树不留半合并)+gate。
- [x] handoff 渲染 handoff.md(jobs/验证/交付分支)。
- [x] gate：open/resolve 事件溯源(reducer Gates)；approve_extra_files 扩 job scope CAS 持久化(N30)；reject 记录。
- [x] adapter 在 worker 成功后把 worktree 改动 commit 到 job 分支(供 integrate 合并)。
- [x] 闭环集成测试：create→delegate→run(Mock completed)→verify→integrate→handoff→contract satisfied→交付分支含改动。
- [x] 覆盖率 cli 82.4 / verify 96 / integrate 87.5 / state 80.5，`-race` 干净；二进制端到端冒烟通过。

**Status**: Complete

## Stage M6（未开始）
- M3: 真实 adapter(codex/claude) + worktree 写隔离 + result 求真(§8.4) + 看门狗 + schema 修复回路。
- M4: hooks(path guard/迁移点 diff review/task-stop 拆分) + 危险命令硬 deny + 注入隔离。
- M5: verify 分层(CLI 实跑) + integrate(集成 worktree merge+abort → harness/integration 分支) + handoff + gate。
- M6: recover 全套(孤儿扫描/prune/branch -D/熔断) + 委派深度·环路·budget。
