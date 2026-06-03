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

## Stage M3–M6（未开始）
- M3: 真实 adapter(codex/claude) + worktree 写隔离 + result 求真(§8.4) + 看门狗 + schema 修复回路。
- M4: hooks(path guard/迁移点 diff review/task-stop 拆分) + 危险命令硬 deny + 注入隔离。
- M5: verify 分层(CLI 实跑) + integrate(集成 worktree merge+abort → harness/integration 分支) + handoff + gate。
- M6: recover 全套(孤儿扫描/prune/branch -D/熔断) + 委派深度·环路·budget。
