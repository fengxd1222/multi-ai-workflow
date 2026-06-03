# Codex 与 Claude Code 双向对等本地多 Agent Harness 架构蓝图

## 研究基线与设计判断

这套 harness 完全可以做成**本地、无 UI、无数据库、无后台服务**的形式，而且不必把 Codex 或 Claude Code 绑定成单向主从。原因在于，两边现在都已经具备了足够成熟的本地 CLI、非交互执行能力、权限控制与生命周期 hook：Codex CLI 已提供稳定的 `codex exec` 非交互模式、JSON 事件输出、输出 schema 校验、沙箱模式与生命周期 hooks；Claude Code 已提供 `claude -p` 非交互模式、`json` / `stream-json` 输出、JSON Schema、permission mode、`allowedTools`，以及覆盖更完整生命周期的 hooks。两者都能通过本地配置文件从仓库目录加载项目级行为，因此最合理的做法不是再造一个“第三个智能中枢”，而是做一个**协议层 + 状态层 + 本地脚本层**，让 Codex 和 Claude Code 共享同一套 session / task / job / artifact / lease 语义。citeturn24view1turn24view3turn9view0turn23view0turn23view1turn23view4turn13view4turn9view2

官方能力边界也决定了架构取舍。Codex 的 hooks 目前覆盖 `SessionStart`、`PreToolUse`、`PermissionRequest`、`PostToolUse`、`PreCompact`、`PostCompact`、`UserPromptSubmit`、`SubagentStart`、`SubagentStop`、`Stop`，并且多个匹配 hook 会并发运行；`transcript_path` 只是便利字段，并非稳定的 hooks 接口。Claude Code 的 hook 面更广，除上述核心事件外，还有 `PostToolUseFailure`、`Notification`、`TaskCreated`、`TaskCompleted`、`StopFailure`、`TeammateIdle`、`SessionEnd`、`CwdChanged`、`FileChanged`、`WorktreeCreate` 等，并且所有匹配 hook 也会并发执行；另外 Claude 支持原生 async hooks，但 Codex 文档当前聚焦同步 command hooks。因此，**跨运行时的统一 MVP 不应依赖“更宽的那个平台”的特性**，而应把“Claude 更强的 hook 面”视作增强，把“Codex 可移植的最小 hook 面”视作协议下限。citeturn29view1turn29view2turn15view2turn15view3turn15view5turn15view7turn14view7turn30view0turn16view3turn13view5

你的“默认共享工作区，worktree 只做可选增强”的偏好是可行的，但它要求状态层足够严谨。Claude Code 官方文档明确把 worktree 定位为并行会话的隔离手段，Codex 与 Git 也都把 worktree 视为在同一仓库中并存多个工作树的标准机制；另一方面，Claude 的 agent teams 目前仍是**实验特性**，并且文档直接承认任务协调、恢复与关停存在已知限制。因此，MVP 不应该押注 Claude agent teams，也不应该默认使用 worktree；正确路线是：**先用共享工作区 + 文件 ownership + lease + gate + verify 实现一个可恢复内核，再把 worktree 作为“冲突升级策略”加上去**。citeturn28search0turn10search1turn19view0turn28search4turn10search8

从生态样例看，这条路线也合理。Trellis 的核心价值不在 UI，而在把大而全的提示拆成可渐进加载的 spec、task、workspace 和记忆层；OpenRig 证明了“Claude Code 与 Codex 同时跑在一个 rig 中”是现实需求，但它采用了本地 daemon、SQLite 和 UI，这与你的“低依赖、零后端、零数据库”目标相反。你的 harness 最适合借 Trellis 的**workflow / state / context / handoff / recovery**思想，但把承载形式缩到 `.harness/` 目录和本地脚本，不引入 OpenRig 这种额外运行时。citeturn32view0turn33view0

基于这些事实，我的结论是：**最佳方案不是“让 Codex 或 Claude Code 某一方成为固定大脑”，而是让它们都成为“运行时”，再由一个文件协议定义 orchestrator、worker、job、lease、artifact、verify、handoff 的行为边界。** 这样你能保留双向对等、共享工作区、高自动、低依赖、可恢复这些偏好，同时避免把系统耦死在某个 CLI 的私有机制上。这个判断与官方可用能力是一致的。citeturn24view0turn9view1turn7search17turn8search2

## 总体架构

建议把系统拆成六层，但**都落在仓库内和本机进程里**：

```text
User / CI / Shell
   │
   ├─ harness CLI wrapper
   │    ├─ task / job / lock / verify / recover commands
   │    └─ runtime adapters
   │         ├─ codex adapter
   │         └─ claude adapter
   │
   ├─ hook adapters
   │    ├─ codex hooks
   │    └─ claude hooks
   │
   ├─ state store
   │    ├─ append-only events
   │    ├─ sharded JSON state
   │    └─ human-readable Markdown
   │
   ├─ conflict guard
   │    ├─ ownership / lease
   │    ├─ dangerous command policy
   │    └─ diff / stage / rollback guard
   │
   └─ verification & handoff
        ├─ test / review / diff checks
        ├─ aggregation
        └─ recovery / handoff notes
```

核心原则是把**状态与执行解耦**。Codex 和 Claude Code 都只是“执行器”；真正的协调权来自 `.harness/` 中的规范文件和脚本：谁是当前 orchestrator、当前 active task 是什么、允许改哪些文件、某个 job 的状态是什么、某个文件的 lease 是否还有效、verify 是否通过、handoff 是否写完，全部由本地状态协议定义，而不是由某个会话的上下文记忆隐式决定。这样做的直接收益是：即使某个 agent 会话中断、compact、resume，甚至整个终端被杀掉，你仍能从状态文件恢复。这个思路也与 Trellis 的“把知识和流程从单一大 prompt 中拆出来”高度一致。citeturn32view0

我建议把**append-only event log**当作最终事实来源，把各类 JSON 视为可重建快照。原因有三。第一，Codex 与 Claude 的 hooks 都可能并发触发多个脚本，因此大而全的单文件 JSON 容易成为损坏热点；而 append-only 事件文件配合小颗粒快照文件，更容易做到幂等和恢复。第二，Codex 官方明确提示 transcript 并不是 hooks 的稳定接口，因此不应靠解析 transcript 来恢复系统状态。第三，Claude Code 的 hook 额外上下文会被写入会话语境，并在恢复时重复回放，过多的动态状态如果直接塞进会话，反而会过期；把动态事实落在 `.harness/` 并在会话启动时重新生成 compact summary，才是稳妥做法。citeturn29view1turn30view1

这个系统里的同步与异步边界应该很清楚。**必须同步**的只有五类操作：读取 active task、校验写权限、申请或续期 lease、提交状态变更、做高风险 gate。其余如测试、索引、diff 汇总、artifact 清点、handoff 渲染，都可以异步化为后台脚本或后续 job。这里要特别强调：Claude 虽然支持 async hooks，但 Codex 当前文档没有对等的 async hook 机制，所以“跨运行时协议”不应建立在 native async hook 之上；可移植做法是让 hook 只做快速判断与轻量写入，然后由 hook 触发一个后台脚本处理长任务。citeturn16view3turn30view1turn17view3turn13view5

最终的 orchestrator / worker 边界建议这样划：orchestrator 负责 intake、拆任务、分配 file scope、汇总结果、推进 phase、做人工 gate、执行 integrate；worker 只负责在授权边界内完成一个 job，并产出结构化 `job.result.json`。这样无论 orchestrator 是 Codex 还是 Claude Code，协议都不变；只是 runtime adapter 不同。这个“协议先于运行时”的设计，是双向对等真正成立的前提。citeturn24view0turn9view0

## 本地状态目录与文件协议

推荐目录采用 `.harness/`，并把**会话级状态、任务级状态、job 级状态、协调状态、事件、人工可读文档**分开，尽量避免多个 agent 高频写同一个文件。

```text
.harness/
  workflow-contract.md
  schemas/
    job-request.schema.json
    job-result.schema.json
    context-packet.schema.json
  current/
    session.json
    active-task.json
    state-summary.md
  sessions/
    S-20260531-001/
      session.json
      events.jsonl
      recovery.json
      decisions.md
      agents/
        registry.json
        codex-main.json
        claude-reviewer.json
      heartbeats/
        codex-main.json
        claude-reviewer.json
      tasks/
        T-001/
          task.json
          brief.md
          plan.md
          handoff.md
          verification.json
          recovery.json
          context-packet.json
          state-summary.md
      jobs/
        J-001.json
        J-001.result.json
        J-002.json
        J-002.result.json
      coordination/
        ownership.json
        locks/
          state.lock
          files/
            src__auth__service_ts.lock.json
        leases/
          src__auth__service_ts.lease.json
          tests__auth__service_test_ts.lease.json
      artifacts/
        J-001/
          stdout.log
          stderr.log
          process.json
          files-changed.json
          diff.patch
          verify/
            unit-test.json
            lint.json
```

为什么要这样切。第一，Codex 和 Claude 的项目级配置都可以从仓库目录加载：Codex 读 `.codex/config.toml` / `hooks.json`；Claude 读当前工作目录的 `.claude/settings.json`，而且 Claude 的 hook 配置**没有父目录回退**。把 `.harness/` 固定在仓库根，可以让两个 runtime 的 hooks 与脚本总能定位到同一状态根。第二，Codex `AGENTS.md` 是启动前读取，Claude 更适合用 `CLAUDE.md` + `SessionStart` 注入；因此 repo 的长期规范应保存在静态指导文件里，而**高频流转状态必须放在 `.harness/`**，不能寄希望于会话 prompt。citeturn13view4turn29view2turn18view4turn22view0turn15view0

建议采用下面这几个关键文件作为“机器真相”：

```json
{
  "protocol_version": "1.0",
  "session_id": "S-20260531-001",
  "repo_root": "/repo",
  "created_at": "2026-05-31T14:10:00Z",
  "updated_at": "2026-05-31T14:28:22Z",
  "orchestrator": {
    "agent_id": "codex-main",
    "runtime": "codex",
    "session_ref": "codex:local:abc123"
  },
  "active_task_id": "T-001",
  "phase": "delegate",
  "event_seq": 183,
  "status": "running",
  "recovery_cursor": {
    "last_good_event_seq": 182,
    "snapshot_rev": "r23"
  }
}
```

```json
{
  "task_id": "T-001",
  "title": "实现 auth 模块重构并补齐验证",
  "goal": "重构 auth service，保持 API 兼容并补齐测试",
  "phase": "implement",
  "status": "running",
  "constraints": {
    "must_keep_api": true,
    "max_scope": ["src/auth/**", "tests/auth/**"],
    "deny_paths": [".github/**", "infra/**"]
  },
  "acceptance": [
    "现有 API 契约不变",
    "新增测试全部通过",
    "lint 和 typecheck 通过"
  ],
  "open_questions": [],
  "job_ids": ["J-001", "J-002"],
  "owned_path_prefixes": ["src/auth/**", "tests/auth/**"]
}
```

```json
{
  "job_id": "J-002",
  "task_id": "T-001",
  "parent_job_id": null,
  "created_by": "codex-main",
  "target_runtime": "claude",
  "role": "implementation",
  "status": "claimed",
  "status_detail": "worker launched and acquiring leases",
  "priority": "high",
  "goal": "在授权范围内完成 auth service 重构",
  "allowed_files": ["src/auth/**", "tests/auth/**"],
  "denied_files": [".github/**", "package.json"],
  "verification_requirements": ["npm test -- auth", "npm run lint"],
  "timeout_at": "2026-05-31T15:00:00Z",
  "retry_policy": {
    "max_attempts": 2,
    "backoff_seconds": 30
  },
  "result_contract": "job-result.schema.json"
}
```

```json
{
  "job_id": "J-002",
  "worker": {
    "agent_id": "claude-impl-01",
    "runtime": "claude",
    "session_ref": "claude:local:550e8400-e29b-41d4-a716-446655440000"
  },
  "status": "completed",
  "started_at": "2026-05-31T14:31:00Z",
  "completed_at": "2026-05-31T14:42:18Z",
  "summary": "已完成 auth service 重构并补齐 4 个测试",
  "changed_files": [
    "src/auth/service.ts",
    "tests/auth/service.test.ts"
  ],
  "claimed_files": [
    "src/auth/service.ts",
    "tests/auth/service.test.ts"
  ],
  "artifacts": [
    ".harness/sessions/S-20260531-001/artifacts/J-002/diff.patch",
    ".harness/sessions/S-20260531-001/artifacts/J-002/verify/unit-test.json"
  ],
  "verification": {
    "unit_test": "passed",
    "lint": "passed",
    "typecheck": "not-run"
  },
  "stdout_ref": ".harness/sessions/S-20260531-001/artifacts/J-002/stdout.log",
  "stderr_ref": ".harness/sessions/S-20260531-001/artifacts/J-002/stderr.log",
  "process_exit_code": 0,
  "needs_human": false,
  "followups": []
}
```

```json
{
  "resource": "src/auth/service.ts",
  "resource_type": "file",
  "task_id": "T-001",
  "job_id": "J-002",
  "agent_id": "claude-impl-01",
  "mode": "exclusive-write",
  "owner": "claude-impl-01",
  "lease_token": "lease_01JXYZ",
  "acquired_at": "2026-05-31T14:31:12Z",
  "expires_at": "2026-05-31T14:34:12Z",
  "heartbeat_at": "2026-05-31T14:33:41Z",
  "stale_after_seconds": 240,
  "status": "active"
}
```

```json
{
  "agent_id": "claude-impl-01",
  "runtime": "claude",
  "task_id": "T-001",
  "job_id": "J-002",
  "pid": 48219,
  "status": "running",
  "cwd": "/repo",
  "started_at": "2026-05-31T14:31:00Z",
  "heartbeat_at": "2026-05-31T14:33:41Z",
  "last_hook": "PostToolUse",
  "last_event_seq": 172
}
```

Markdown 文件则保持人和 agent 都能读懂，但不承担并发热点职责：

```markdown
# brief

目标：
- 重构 auth service，避免重复逻辑
- 不改公开 API
- 补齐回归测试

边界：
- 只允许修改 `src/auth/**` 和 `tests/auth/**`
- 不允许修改 CI、依赖清单、部署脚本

最新决策：
- token 校验逻辑抽到 `validateToken()`
- 保留旧错误码，避免破坏客户端兼容

交接：
- 若单测失败，先看 `tests/auth/service.test.ts`
- 若需要新增依赖，转为 `needs-human`
```

避免写冲突的关键不是“全局大锁”，而是**三件事同时做**。第一，状态文件分片：每个 job、每个 heartbeat、每个 lease 单独成文件，降低冲突概率。第二，所有快照文件都通过“tmp 文件写入 + 原子替换”更新；在 POSIX 语义下，`rename()` / `os.replace()` 对同文件系统内替换是原子的，`flock` 是 advisory lock；Windows 侧应使用等价的 replace 语义而不是直接覆写。第三，事件日志单独 append，避免不同脚本反复重写一个大全快照。citeturn35search1turn35search0turn35search2turn35search7turn35search9

具体到共享工作区下“谁有权修改某个文件”，建议把**ownership**定义为“当前任务的责任边界”，把**lease**定义为“当前写入窗口的临时独占权”。`ownership.json` 记录路径前缀或文件模式到 `task_id / job_id / responsible_role` 的长期映射；而某次真实改写前，worker 必须拿到精确到文件的 `exclusive-write` lease。这样设计有两个好处：一是 plan 阶段就能分配大范围职责，二是 implement 阶段仍能在文件级阻止撞车。若某 agent 越权改了未授权路径，PreToolUse 可以先拦，PostToolUse 还能用 git diff 与 claimed files 复核并把 job 直接转 `needs-human`。Claude 的 deny-first permission 规则和 Codex 的 `PreToolUse` / `PermissionRequest` 也都支持这种双重闸门思路。citeturn18view1turn18view3turn5view2turn5view3

新启动 agent 不应灌整份 session，而应读取**最小必要状态**。Codex 会在启动前读 `AGENTS.md`，且项目指导合并后默认上限是 32 KiB；Claude 的 `SessionStart` 适合注入动态上下文，且 hook 输出和 `additionalContext` 都有大小上限，过大还会被落盘为文件路径。最省 token 的做法是：把长期规则放在 `AGENTS.md` / `CLAUDE.md`；把 phase 相关的任务摘要压成 `state-summary.md`；把 job 相关细节压成 `context-packet.json`；把大文件、diff、长日志都只给路径引用，不内联全文。citeturn22view0turn22view1turn17view1turn30view1

## 委派协议与 Context Packet

统一委派协议建议采用“**job request + context packet + structured result + artifact directory**”四件套。orchestrator 不给 worker 自由发挥式自然语言任务，而是创建一个**有明确边界的 job**；worker 必须回写机器可读结果，而不是只回一句“我做完了”。这样才能做到自动聚合、自动验证、自动恢复。

我建议角色定义收敛成六类即可：`analysis`、`implementation`、`test`、`review`、`verification`、`integration`。这与 Codex、Claude Code 的内建能力是相容的：Codex 官方文档明确把其用途覆盖到 research、write code、review code、understand codebase；Claude Code 的 headless 模式和工具控制也天然适合这些角色。真正需要单独协议化的不是“模型能力”，而是**输出合同**。citeturn7search6turn8search18turn9view0

推荐 job 生命周期如下：

```text
created
  -> queued
  -> claimed
  -> running
  -> blocked
  -> completed
  -> failed
  -> cancelled
  -> expired
  -> needs-human
```

对外可以再做 alias：`pending = created|queued`，`done = completed`，`timeout = expired`。其中 `needs-human` 不等于失败，它表示 job 进入了人工确认状态，例如：需要越权写文件、需要安装新依赖、发现与其他 agent 的 lease 冲突、需要破坏性命令、验证结果互相矛盾。这个状态要和 `failed` 分开，否则 orchestrator 无法区分“应该重试”还是“应该 gate”。这一点与 Codex 的 `PermissionRequest`、Claude 的 `PermissionRequest` / `PreToolUse defer` 机制是兼容的。citeturn5view3turn14view6turn17view4

统一的 `context-packet.json` 建议如下：

```json
{
  "protocol_version": "1.0",
  "task_id": "T-001",
  "job_id": "J-002",
  "parent_job_id": null,
  "orchestrator": {
    "agent_id": "codex-main",
    "runtime": "codex"
  },
  "worker": {
    "runtime": "claude",
    "role": "implementation"
  },
  "goal": "重构 auth service，保持外部 API 不变，并补齐测试",
  "current_phase": "implement",
  "constraints": [
    "不得修改 package.json",
    "不得修改 .github/**",
    "不得新增第三方依赖",
    "必须保留现有错误码"
  ],
  "allowed_files": [
    "src/auth/**",
    "tests/auth/**"
  ],
  "denied_files": [
    ".github/**",
    "infra/**",
    "package.json"
  ],
  "claimed_files": [
    "src/auth/service.ts",
    "tests/auth/service.test.ts"
  ],
  "relevant_artifacts": [
    ".harness/sessions/S-20260531-001/tasks/T-001/brief.md",
    ".harness/sessions/S-20260531-001/tasks/T-001/plan.md"
  ],
  "latest_decisions": [
    "保留 API 契约，不改 controller 层签名",
    "错误码兼容优先于内部实现整洁"
  ],
  "open_questions": [],
  "verification_requirements": [
    "npm test -- auth",
    "npm run lint"
  ],
  "expected_output_contract": {
    "must_write_result_json": true,
    "must_list_changed_files": true,
    "must_attach_verify_artifacts": true
  }
}
```

这里最重要的不是字段多少，而是**严格限制体积**。我建议把 `context packet` 控制在 2–4 KiB，把 `state-summary.md` 控制在 1–2 屏以内。原因是官方文档已经揭示了两个现实：Codex 对指导文件和 skill 列表都有明确的上下文预算；Claude 的 hook 上下文也会写入会话并在恢复时继续生效，过大或过时内容都会污染后续回合。换句话说，**packet 负责“启动任务”，artifact path 负责“延迟取用”**。citeturn22view0turn22view1turn30view1

artifact 约定建议固定下来，不随 job role 摇摆：

- `stdout.log`：CLI 子进程标准输出完整记录  
- `stderr.log`：CLI 子进程标准错误完整记录  
- `process.json`：`pid / exit_code / started_at / ended_at / duration_ms`  
- `files-changed.json`：真实文件修改列表  
- `diff.patch`：该 job 对授权文件的差异  
- `verify/*.json`：测试、lint、typecheck、review 结果  
- `notes.md`：可选，人类可读补充说明  

这样 orchestrator 既能看结构化结果，也能在需要时追溯原始日志。重要的是：**stdout / stderr / exit code 属于“进程执行层”产物，不能和模型的结构化结果混为一谈。** `job.result.json` 应只承载“该 job 的业务结果”；进程层日志由 wrapper 捕获。这样当 Claude / Codex 因 CLI 参数、权限、网络、认证错误而失败时，你仍能区分“模型没完成”与“进程没成功启动”。这一点在 Claude 的 `claude -p --output-format json|stream-json` 和 Codex 的 `codex exec --json --output-last-message --output-schema` 模式下都很容易实现。citeturn23view0turn23view4turn24view1turn24view4

下面这两组命令足够作为双向调用模板。第一组是 **Claude Code 调 Codex**：

```bash
codex exec \
  --json \
  --sandbox workspace-write \
  --output-schema .harness/schemas/job-result.schema.json \
  --output-last-message .harness/sessions/S-20260531-001/artifacts/J-002/final.md \
  "$(cat .harness/sessions/S-20260531-001/tasks/T-001/context-packet.prompt.txt)" \
  > .harness/sessions/S-20260531-001/artifacts/J-002/stdout.log \
  2> .harness/sessions/S-20260531-001/artifacts/J-002/stderr.log
```

第二组是 **Codex 调 Claude Code**：

```bash
claude -p \
  "$(cat .harness/sessions/S-20260531-001/tasks/T-001/context-packet.prompt.txt)" \
  --output-format json \
  --json-schema '{"type":"object","properties":{"summary":{"type":"string"},"changed_files":{"type":"array","items":{"type":"string"}},"verification":{"type":"object"}},"required":["summary","changed_files","verification"]}' \
  --allowedTools "Read,Edit,Bash(npm test *),Bash(npm run lint)" \
  --permission-mode acceptEdits \
  > .harness/sessions/S-20260531-001/artifacts/J-003/stdout.log \
  2> .harness/sessions/S-20260531-001/artifacts/J-003/stderr.log
```

这些命令用到的关键能力——Codex 的非交互 `exec`、JSON 事件流、输出 schema、沙箱模式，以及 Claude 的 `-p`、`--output-format json|stream-json`、`--json-schema`、`--allowedTools`、`--permission-mode`——都已经在官方文档里明确给出。citeturn24view0turn24view1turn24view3turn24view5turn9view0turn23view0turn23view1turn23view4turn23view6

结果聚合时，orchestrator 不应该“读两段自然语言摘要然后自己猜”，而应走三步：先合并 `changed_files` 与 `claimed_files`，确认没有越权写；再检查 `verification` 是否满足任务 acceptance；最后再读取 `summary` 与 `notes.md` 生成最终 handoff。若多个子 job 对同一文件都有变更而没有共享 lease，则直接把 task 标成 `blocked` 或 `needs-human`，不要尝试自动三方合并。共享工作区默认不是为了“聪明地合并冲突”，而是为了“在严格边界内减少切换成本”。

## Hook 生命周期与守卫

这里真正要做的是一个“**统一 hook 适配层**”：Codex 和 Claude 的原生事件名不同、可阻塞行为不同、输出结构也不完全一样，但它们都可以最终落到一组本地脚本，例如 `harness hook session-start`、`harness guard pretool`、`harness guard posttool`、`harness task complete`、`harness recover hint`。换句话说，**平台差异由 adapter 吞掉，`.harness/` 协议不感知平台差异。**

先说 Claude。它的 hook 面更完整，适合做“全量运行时观察 + 精细 gate”。`SessionStart` 能注入 `additionalContext`，还可设置 session title、watch paths；`UserPromptSubmit` 可加上下文或阻断；`PreToolUse` 可 `allow` / `deny` / `ask` / `defer`；`PermissionRequest` 可代替用户做 allow/deny，并更新权限；`PostToolUse` / `PostToolUseFailure` 能把工具结果与失败详情反馈给模型；`Stop` / `SubagentStop` / `TaskCompleted` / `TeammateIdle` 都可以阻止停止或完成；`Notification` 与 `SessionEnd` 则适合做日志、通知和清理。所有匹配 hook 会并发运行，默认同步阻塞，只有显式 `async` 才后台执行。citeturn17view1turn16view4turn15view1turn17view4turn14view6turn15view2turn17view5turn15view6turn15view7turn15view3turn14view7turn30view0turn16view3

再说 Codex。它更适合做“最小但可靠的控制环”。`SessionStart` 可注入 developer context；`PreToolUse` 能拦截 Bash、`apply_patch` 和 MCP 调用，还能 deny 或 rewrite 输入；`PermissionRequest` 处理即将弹出的批准请求；`PostToolUse` 可对已完成工具结果施加反馈；`UserPromptSubmit` 可注入补充上下文；`PreCompact` / `PostCompact` 适合做 compact 前后状态摘要；`SubagentStop` / `Stop` 能要求继续执行。Codex 当前文档没有 Claude 那种 `Notification`、`StopFailure`、`TaskCompleted`、`SessionEnd` 级别的完整事件面，因此你在 Codex 侧需要**额外的 wrapper 进程**去记录 CLI 退出码、stderr、异常与完成态。citeturn5view1turn5view2turn5view3turn6view0turn6view1turn6view2turn5view4turn5view5turn6view3turn6view4turn29view1turn29view3

我建议统一成下面这张映射表。它不是“官方事件名对照表”，而是你的脚本职责表。

| 统一脚本 | Claude 入口 | Codex 入口 | 是否阻塞 | 主要职责 |
|---|---|---|---|---|
| `harness hook session-start` | `SessionStart` | `SessionStart` | 是 | 读取 workflow contract、active task、生成 compact summary、注入最小上下文 |
| `harness hook prompt-submit` | `UserPromptSubmit` | `UserPromptSubmit` | 是 | 记录用户输入、关联 active task、注入 breadcrumb |
| `harness guard pretool` | `PreToolUse` | `PreToolUse` | 是 | 校验 lease / ownership / denied scope / risky command |
| `harness guard approval` | `PermissionRequest` | `PermissionRequest` | 是 | 高风险操作自动 allow/deny 或转 needs-human |
| `harness guard posttool` | `PostToolUse` / `PostToolUseFailure` | `PostToolUse` | 建议轻阻塞 | 记录工具调度、diff、测试结果、artifact、异常 |
| `harness hook compact` | `PreCompact` / `PostCompact` | `PreCompact` / `PostCompact` | 是 | 压缩前写摘要，压缩后恢复最小状态 |
| `harness hook task-stop` | `Stop` / `SubagentStop` / `TaskCompleted` / `TeammateIdle` | `Stop` / `SubagentStop` | 是 | 写 handoff、卡未完成验证、推动继续执行 |
| `harness hook notify` | `Notification` / `StopFailure` / `SessionEnd` | wrapper 进程 | 否 | 通知、日志、恢复建议、尾部清理 |

用法上有三个重点。第一，**PreToolUse 必须只做轻量决策**：例如检查路径是否在 `allowed_files`、是否命中 deny 规则、该文件 lease 是否还有效、是否是高风险命令。不要在这里跑长测试，否则会把每次工具调用都拖慢。第二，**重验证放在 PostToolUse 和 Stop**：例如某次 `Write` 之后启动异步测试，或者在 `Stop` 时检查“是否已产出结构化 result”“verify 是否还缺失”。第三，**不要把 hook 当数据库**：hook 只写小状态、事件和指针，大对象由 artifacts 保存。Claude 文档明确提示 hook 输出会进入会话上下文，并可能在 resume 后变陈旧；Codex 也没有把 transcript 当稳定接口承诺。citeturn30view1turn17view3turn29view1

针对“每次 agent 启动自动注入当前 workflow contract、active task、状态摘要、约束和 handoff”，建议做成以下层级，而不是一次性大包塞给模型：

```text
静态层
- AGENTS.md / CLAUDE.md
- workflow-contract.md

动态层
- current/state-summary.md
- tasks/<task_id>/brief.md
- tasks/<task_id>/context-packet.json

延迟层
- artifacts 路径
- decisions.md / handoff.md / verification.json
```

静态层放长期不变的 repo 约定；动态层放当前任务的精简事实；延迟层只给路径引用，模型需要时再读。这样既尊重了 Codex/Claude 的上下文预算，也减少了 hook 注入导致的 token 浪费。citeturn22view0turn22view1turn17view1turn30view1

## 共享工作区冲突控制与恢复

共享工作区模式里，最危险的不是“两个 agent 同时存在”，而是**两个 agent 同时相信自己有权写同一个文件**。因此安全策略要从“权限声明”提升到“修改前先 claim，修改后再核验”。

第一步是保护 pre-existing uncommitted changes。进入 session 时，先用 `git status --porcelain` 建立一份**基线快照**，因为 porcelain 格式是 Git 明确保证适合脚本解析且兼容性稳定的；把这份快照存到 `.harness/sessions/<sid>/repo-baseline.json`。如果仓库一开始就有未提交变更，默认把这些路径加入 `reserved_by_human`，除非用户显式把它们分配给某个 task/job。这样可避免 agent 把人类尚未提交的本地修改误当成自己的起始状态。citeturn36search0turn36search16

第二步是 ownership map。`ownership.json` 不要按“当前文件属于谁”做全量写死，而应该是三层信息叠加：

```json
{
  "path_rules": [
    {
      "pattern": "src/auth/**",
      "task_id": "T-001",
      "preferred_role": "implementation",
      "responsible_agent": "claude-impl-01"
    },
    {
      "pattern": "tests/auth/**",
      "task_id": "T-001",
      "preferred_role": "test",
      "responsible_agent": "codex-test-01"
    }
  ],
  "reserved_by_human": [
    "src/legacy/auth_adapter.ts"
  ],
  "last_updated_at": "2026-05-31T14:30:10Z"
}
```

这不是绝对写锁，只是“谁优先认领”的责任地图。真实写保护仍靠 lease。一个文件的 lease 建议是**独占写**，而不是共享写；目录级 lease 只允许在“批量生成文件且不涉及现存文件”的情况下使用。续租策略建议 60–120 秒 heartbeat 一次，过期阈值 2–4 分钟；只要 `heartbeat_at` 超时且进程已退出，就把 lease 视为 stale，可由 orchestrator 回收。这个状态不需要依赖 OS 层 lease，只要你在 `.harness/` 中有明确的 TTL 和回收逻辑即可。之所以仍建议外层再加文件锁，是因为 Claude 和 Codex 的多个 hook 都会并发运行，状态更新仍需串行化。citeturn30view0turn29view1turn35search2

第三步是越权修改检测。仅靠 PreToolUse 不够，因为格式化器、编译器、代码生成器可能借 Bash 或编辑工具带出额外改动。做法是：

- 修改前：`PreToolUse` 检查命令意图与目标路径  
- 修改后：`PostToolUse` 或 wrapper 运行 `git diff --name-only` / `--name-status`，比对真实改动路径与 `claimed_files`  
- 若发现越权路径：  
  - 未命中任何 allow：转 `needs-human`  
  - 命中他人 active lease：标记 `blocked_conflict`  
  - 命中格式化器扩散：若只是同一 ownership 前缀内，可允许；否则回滚越权部分并要求 worker 缩小范围  

Git 官方文档说明了 `--name-only` / `--name-status` 的稳定用途；你还可以用 `git restore --source=<tree>` 或 `git restore` 恢复指定路径。换言之，**回滚永远按 pathspec 做，不做“整仓回滚”**。这样才能在失败时只撤销某个子任务的改动，而不伤到其他 agent 的文件。citeturn36search9turn36search20turn36search2turn36search10

第四步是 stage 和 integrate 约束。建议强制每个 worker 只对自己 `claimed_files` 执行 stage；integrate 前必须过“三关”：

- **scope review**：stage 集合必须是 `claimed_files` 子集  
- **diff review**：对每个 job 生成 `diff.patch`，由 review/verification job 过一遍  
- **verification gate**：task 级 acceptance 必须全部满足，否则 `TaskCompleted` / `Stop` 拦下  

Claude 的 `TaskCompleted`、`TeammateIdle`、`Stop` 非常适合拦“自我宣布完成但验证没过”的情况；Codex 没有对等的 task hook，因此在 Codex 模式下这道 gate 应由 wrapper + `Stop` + 后置 verify 脚本共同完成。citeturn15view5turn15view7turn15view6turn6view4

第五步是危险命令 gate。以下几类建议一律进入 `needs-human` 或至少 `PermissionRequest`：

- 删除或覆盖授权范围外路径  
- 改 `package.json`、锁文件、CI、部署文件  
- `git add .`、`git commit -a`、批量格式化全仓  
- 可能触发网络写入或远端副作用的命令  
- 任何“跨 scope 修改 + 高风险命令”组合  

Claude 权限系统是 deny-first，且 deny/ask 规则与 sandbox 会共同约束 Bash；Codex 则把审批与沙箱分层处理，并支持用 rules 和 `execpolicy check` 做额外命令策略。建议两边都做：**静态 deny 规则拦“绝不能自动做”的事，动态 PreToolUse / PermissionRequest 拦“这次不该做”的事。**citeturn18view1turn18view3turn9view6turn25search3

什么时候升级成 worktree 模式。我的建议很明确：满足任一条件就升级，而不是继续硬扛共享工作区。

- 两个以上并行 job 必须改同一文件或同一路径簇  
- 任务需要大规模格式化、代码生成、迁移脚本  
- 仓库已有大量人类未提交修改  
- 需要对比两条独立实现路线  
- 需要在并行 session 间保证“零交叉脏写”  

因为 Git 官方把 worktree 定义为同仓库多工作树，Claude 也明确把它当成并行隔离手段，Codex app 文档同样建议用 worktree 隔离独立任务。所以 worktree 非常适合作为**冲突升级策略**，但不必成为默认前提。citeturn10search1turn28search0turn28search4turn10search8

## Trellis 风格工作流与 CLI / MVP

这套 harness 的 workflow 建议固定为八个阶段，但实际只需要 orchestrator 驱动一个很轻的状态机。阶段不要做“看板工具语义”，而要做“执行协议语义”。

**Intake**  
输入是用户任务、仓库当前基线、静态项目规则。输出是 `task.json`、`brief.md`、初始 acceptance、初始 deny scope。这个阶段必须由 orchestrator 串行决策，不能并行。若发现仓库已有大面积未提交改动，先保护 path，再决定要不要升级 worktree。验收标准是“任务边界能落成机器可读文件”。

**Research**  
输入是 `brief` 和受限读权限。输出是事实记录、风险点、候选方案。这个阶段非常适合并行：可以同时派一个 Codex worker 做代码地形勘测，一个 Claude worker 做约束/兼容性研究。验收标准不是“给了很多文字”，而是给出结构化发现、引用文件路径、列出 open questions。

**Plan**  
输入是 research 结果。输出是 `plan.md`、ownership 初稿、job 划分、verify 计划。这个阶段必须由 orchestrator 汇总，因为只有它知道全局冲突面。验收标准是每个 job 都有明确 role、scope、acceptance、timeout 和 result contract。

**Delegate**  
输入是 plan。输出是 `jobs/*.json`、`context-packet.json`、agent claim 状态。可以并行派发，但**claim 必须串行落锁**。恢复策略是：未 claim 的 job 直接重派，claimed 但 heartbeat stale 的 job 回收 lease 后重派。

**Implement**  
输入是 claimed jobs。输出是代码改动、`job.result.json`、artifact。最适合并行，但只允许在不冲突的 scope 内并行。验收标准是 worker 回写结构化结果，并通过最小 verify。失败恢复策略是 pathspec 粒度回滚，不做整仓撤销。

**Verify**  
输入是 worker 结果与 diff。输出是测试、lint、review、verification verdict。可并行运行多个 verify job，但“是否放行到 integrate”必须由 orchestrator 串行判定。若 verify 失败，可以生成 follow-up job 回到 implement，或转 `needs-human`。

**Integrate**  
输入是全部通过的 job 结果。输出是 task 级合并视图、最终 diff、最终验证结论。这里必须串行，且建议只由 orchestrator 做。共享工作区模式下，integrate 本质上不是 merge branch，而是做“最终收口与边界复核”。

**Handoff**  
输入是最终状态。输出是 `handoff.md`、`verification.json`、`recovery.json`、task closeout。要求写清：做了什么、哪些文件变了、哪些验证通过、还有什么风险、下一个 agent 恢复时该从哪里接。这个阶段既服务人，也服务下一次 resume。Treillis 的“workspace handoff / journals / decisions”值得借鉴，但要缩成极简格式。citeturn32view0

为了让这套工作流真正能落地，我建议 CLI 先做下面这一组最小命令。它们都不需要数据库，也不需要 daemon。

```text
harness init
harness session start
harness task create
harness state summary
harness delegate
harness job claim
harness job complete
harness lock acquire
harness lock release
harness guard pretool
harness guard posttool
harness verify
harness integrate
harness handoff
harness recover
```

这些命令的职责建议非常朴素：

- `harness init`：创建 `.harness/`、schema、合同模板  
- `harness session start`：建立 session、记录 repo baseline、产出 current summary  
- `harness task create`：从用户任务生成 `task.json` 与 `brief.md`  
- `harness state summary`：输出给 hook 注入的 compact summary  
- `harness delegate`：创建 job、产出 context packet、拉起 runtime adapter  
- `harness job claim`：worker 启动后 claim job，并申请 lease  
- `harness job complete`：写回结构化结果、释放 lease、更新状态  
- `harness lock acquire/release`：协调状态写锁或文件 lease  
- `harness guard pretool/posttool`：hook 入口  
- `harness verify`：跑 task 级或 job 级验证  
- `harness integrate`：执行最终汇总与收口  
- `harness handoff`：渲染最终交接  
- `harness recover`：按 event log + heartbeat + stale lease 恢复现场  

MVP 的错误码也应该标准化，例如：`0=ok`，`10=blocked-policy`，`11=blocked-lease`，`12=needs-human`，`20=verify-failed`，`30=state-corrupt`，`31=lock-timeout`，`40=runtime-exec-failed`。这样 hooks、wrapper、CI 和 orchestrator 都能用同一套语义判断后续动作。

第一版最少文件，我建议只上这些：

```text
.harness/workflow-contract.md
.harness/current/session.json
.harness/current/active-task.json
.harness/current/state-summary.md
.harness/sessions/<sid>/events.jsonl
.harness/sessions/<sid>/tasks/<tid>/task.json
.harness/sessions/<sid>/tasks/<tid>/brief.md
.harness/sessions/<sid>/jobs/<jid>.json
.harness/sessions/<sid>/jobs/<jid>.result.json
.harness/sessions/<sid>/coordination/ownership.json
.harness/sessions/<sid>/coordination/leases/*.json
.harness/sessions/<sid>/heartbeats/*.json
```

第一版最少 hooks，我建议只接：

- Codex：`SessionStart`、`PreToolUse`、`PostToolUse`、`UserPromptSubmit`、`Stop`、`PermissionRequest`、`PreCompact`、`PostCompact`  
- Claude：同上再加 `PostToolUseFailure`、`TaskCompleted`、`Notification`、`SessionEnd`  

第一版先**不做**这些：自动 worktree 管理、跨 repo 协同、复杂 team mailbox、可视化 UI、语义记忆检索、自动 merge、插件分发。原因不是这些不重要，而是它们会把 MVP 从“协议层”带偏成“平台工程”。

最务实的实施顺序是：

1. **先把状态目录和 schema 跑通**。验收标准：能手工创建 session/task/job，并能通过 `recover` 重建 summary。  
2. **再把 hooks 接起来**。验收标准：启动会话能自动注入 summary，改文件会写事件，危险 Bash 会被 block。  
3. **再做双向 delegate wrapper**。验收标准：Codex 能成功调用 Claude，Claude 也能成功调用 Codex，并都回写 `job.result.json`。  
4. **再做 verify / integrate / handoff**。验收标准：至少一个 task 可以从创建到完成闭环运行。  
5. **最后再补 worktree 与高级恢复**。验收标准：冲突 task 能自动升级为 worktree 模式，或至少给出清晰 gate。  

如果你按这个顺序做，v2 可以扩展的方向会非常自然：更细粒度的 lease、自动 reviewer job、watch paths 驱动的状态热更新、WorktreeCreate 适配、技能库、恢复向导、以及更丰富的 artifact 索引。

## 开放问题与限制

当前资料下，最需要正视的限制有三点。

第一，**Codex 的 hook 面仍比 Claude 窄**。Claude 有 `Notification`、`TaskCompleted`、`SessionEnd`、`StopFailure` 等事件，而 Codex 官方文档当前不提供对等面，因此真正的“Error / Notification / cleanup”在 Codex 侧必须依赖 wrapper 进程和后置脚本，而不是寄希望于原生 hook parity。citeturn29view1turn29view3turn15view5turn15view7turn14view7

第二，**Claude 的 hook 上下文会进入会话语境并在 resume 时继续回放**，而 Codex 也不鼓励把 transcript 当稳定接口。因此，这个 harness 必须坚持“`.harness/` 是真相，会话只是缓存”的原则；任何把动态状态硬塞进长上下文的设计，后面都会遇到 stale context 问题。citeturn30view1turn29view1

第三，**共享工作区模式的上限是真实存在的**。官方文档已经把 worktree 作为并行隔离的标准手段，这意味着当任务进入“同文件并行修改”“大规模生成与格式化”“高风险变更”这些区间时，继续坚持共享工作区只会增加复杂度，而不是体现架构优雅。最好的设计不是死守共享工作区，而是把它做成默认模式，再明确一条升级到 worktree 的自动/半自动路径。citeturn28search0turn28search4turn10search8turn10search1

综合来看，最稳妥、最工程化的答案是：**用 `.harness/` 做协议中心，用事件日志 + 分片 JSON 做状态中心，用 hooks 做同步闸门，用 wrapper 做双向委派，用 lease/ownership 做共享工作区控制，用 verify/integrate/handoff 做闭环。** 这就是一个既符合你偏好、又能直接开始实现的本地多 agent harness。