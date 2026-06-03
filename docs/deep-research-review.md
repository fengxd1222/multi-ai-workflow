# 对 `deep-research-report (1).md` 的评审意见

## 总体判断

这份调研报告已经具备完整架构蓝图的雏形，核心方向是正确的：用 `.harness/` 作为协议中心，用 JSON / Markdown 做共享状态，用 hooks 做同步闸门，用 wrapper 做 Codex 与 Claude Code 的双向委派，用 ownership / lease 管住共享工作区风险。这条路线符合“无 UI、无数据库、无后台服务、高自动、双向对等”的目标。

但它目前更像一份“研究结论 + 架构草案”，还不是可以直接交给工程师实现的设计文档。主要问题不是覆盖面不够，而是缺少更硬的决策边界、协议细节、MVP 收缩和失败场景验证。下一步应把它压成一套可实现的 v1 规格，而不是继续扩展概念范围。

## 主要优点

- 架构定位清楚：没有把 Codex 或 Claude Code 固定成唯一主控，而是把两者都视为 runtime，通过本地协议协调，这是双向对等的正确抽象。
- 对共享工作区风险有清醒判断：报告没有轻描淡写并发写入，而是提出 ownership、lease、PreToolUse、PostToolUse、diff review、pathspec rollback 等组合策略。
- 状态设计方向合理：append-only event log + 分片 JSON + Markdown handoff 的组合，比单个全局状态文件更适合多 hook 并发写入。
- Hook 设计贴近现实能力差异：报告意识到 Claude hook 面更宽、Codex hook 面更窄，因此把 Claude 能力作为增强，把 Codex 最小 hook 面作为协议下限。
- Trellis 借鉴点把握准确：重点落在 workflow、state、context、handoff、recovery，而不是 UI 或任务看板。

## 关键风险与缺口

### 1. 引用来源不可复核

报告里大量使用 `cite...` 形式的引用占位，但当前 Markdown 没有 source appendix、URL 列表或脚注解析。作为调研结果，这会削弱可信度，也不利于后续验证 Codex / Claude Code hooks 的真实接口。

建议：补一个“资料来源与接口事实表”，至少列出每个关键判断对应的官方文档链接、版本日期和验证状态。尤其是 Codex hooks、Claude hooks、`codex exec` 参数、`claude -p` 参数、JSON schema 输出能力、permission mode 等。

### 2. v1 范围仍然偏大

报告同时覆盖状态协议、hooks、CLI、wrapper、delegate、lease、event replay、recovery、verify、integrate、handoff、worktree 升级策略。作为完整蓝图可以，但作为第一版实现会过重。

建议把 v1 明确收缩为：

- 单仓库、单 active session、单 active task。
- 支持 Codex / Claude 双向调用，但一次只允许一个 orchestrator。
- 支持并行 job，但 implementation job 默认不能写同一文件。
- 实现 file lease，但暂不实现完整 event replay。
- 实现 PreToolUse / PostToolUse / Stop 三类关键 hook，其他 hook 先记录为增强。
- 先做 job result contract 与 handoff 闭环，再做高级 recovery。

### 3. 共享工作区的“真实执行拦截”还不够具体

报告提出 lease 和 ownership，但没有完全说明如何从 hook 输入中可靠提取“将要修改哪些文件”。在真实环境里，很多写入来自 Bash、formatter、codegen、test snapshot 更新，PreToolUse 未必能提前知道完整影响面。

建议明确采用“双阶段防护”：

- PreToolUse 只拦显式路径和明显危险命令。
- PostToolUse 必须以 `git diff --name-only` 作为真实修改来源。
- 任何无法提前预测写范围的 Bash 命令，必须声明 expected write scope。
- 对 formatter / codegen 命令默认进入 stricter mode：运行后若 diff 超出 lease，立即转 `needs-human`。

### 4. 状态一致性机制需要更可执行

报告提到 atomic write、lock、lease、heartbeat、event log，但还缺少具体写入协议。例如：锁超时如何处理、事件序号如何分配、并发 append 如何保证、快照损坏如何恢复、JSON schema 校验失败如何处置。

建议补充一份 state write protocol：

- 所有 JSON 写入必须经过 `validate -> write tmp -> fsync -> atomic rename`。
- `events.jsonl` 只允许追加，不允许重写。
- 每个事件必须有 `event_id`、`seq`、`actor`、`timestamp`、`type`、`payload_hash`。
- `seq` 分配必须在 `state.lock` 下完成。
- 快照文件带 `rev` 和 `last_event_seq`。
- 发现快照损坏时，从最近有效快照 + event log 重建。

### 5. Job 状态机需要加非法迁移规则

报告列出了 `created -> queued -> claimed -> running -> ...`，但没有规定哪些迁移是非法的，也没有说明谁有权迁移。多 agent 系统里，状态机的权限模型比状态枚举更重要。

建议补充：

- 只有 orchestrator 可从 `completed` 推进 task phase。
- worker 只能把自己 claim 的 job 从 `running` 写到 `completed` / `failed` / `blocked` / `needs-human`。
- stale job 只能由 orchestrator 或 recovery command 回收。
- `cancelled`、`needs-human`、`integrated` 应为 terminal 或 semi-terminal 状态，不能被 worker 自行恢复。
- 所有状态迁移必须写 event。

### 6. 双向调用示例需要包装成 adapter，而不是直接裸调 CLI

报告给了 `codex exec` 和 `claude -p` 示例，这对说明可行性足够，但工程实现不应让 orchestrator 直接拼这些命令。否则 stdout / stderr / exit code、schema 校验、状态写回、超时、重试、日志路径都会散落在不同 prompt 或脚本中。

建议新增两个稳定 adapter：

- `harness run codex --job J-002`
- `harness run claude --job J-003`

adapter 负责读取 job、生成 prompt、调用 runtime、捕获日志、校验结构化输出、写 result、更新 events。orchestrator 只调用 adapter，不直接管理底层 CLI 参数。

### 7. Human gate 的语义需要细化

报告多次提到 `needs-human`，但没有定义人类如何恢复任务。高自动系统里，人工 gate 不能只是一个状态标签，否则会变成新的阻塞点。

建议定义 gate file：

```json
{
  "gate_id": "G-001",
  "task_id": "T-001",
  "job_id": "J-002",
  "reason": "formatter modified files outside lease",
  "requested_action": "approve_extra_files | reject_and_rollback | reassign_scope",
  "affected_files": ["src/shared/format.ts"],
  "recommended_action": "reject_and_rollback",
  "created_at": "2026-06-02T19:45:00+08:00",
  "status": "open"
}
```

并提供 `harness gate list`、`harness gate approve`、`harness gate reject` 之类命令。

### 8. Verification gate 要区分 job 级和 task 级

报告把 verify 描述得比较完整，但还需要明确 job 级验证和 task 级验证的不同职责。否则每个子 agent 都可能重复跑全量测试，浪费时间；或者只跑局部测试，最后遗漏集成风险。

建议：

- job 级 verify：只验证该 job 的局部 acceptance，例如相关单测、scope 检查、result schema。
- task 级 verify：由 orchestrator 在 integrate 前统一执行，例如全量测试、lint、typecheck、diff review。
- `verification.json` 应标记每个 check 的层级、命令、结果、耗时、日志路径和是否 required。

## 建议的重构方向

建议把报告改成两层文档：

1. `architecture.md`：保留完整蓝图，说明设计原则、组件、状态协议、hook 生命周期、共享工作区策略。
2. `v1-spec.md`：只写第一版实现所需的硬规格，包括目录、schema、命令、状态机、hook 映射、验收流程。

`v1-spec.md` 应避免继续讨论大而全的长期演进，而是让工程师能直接开始实现。它至少需要明确：

- v1 只支持哪些 hook。
- v1 只支持哪些 job role。
- v1 是否允许并行 implementation。
- v1 如何落锁。
- v1 如何校验越权改动。
- v1 如何生成和校验 `job.result.json`。
- v1 如何从失败中恢复。
- v1 如何判断 task 完成。

## 我建议的 v1 落地优先级

1. 建立 `.harness/` 目录与 schema：先能创建 session、task、job、event。
2. 实现 adapter：`harness run codex --job` 和 `harness run claude --job`。
3. 实现结构化 job result：强制每个 worker 写 `job.result.json`，并做 schema 校验。
4. 实现最小 hook：SessionStart 注入 summary，PreToolUse 做危险命令和显式路径拦截，PostToolUse 做 diff 复核，Stop 阻止未 handoff 的完成。
5. 实现共享工作区 guard：baseline、reserved files、ownership、file lease、越权 diff 检测。
6. 实现 verify / integrate / handoff：先做确定性脚本闭环，再考虑复杂 recovery。
7. 最后补高级能力：event replay、gate 管理、worktree 升级、并行 conflict resolver。

## 需要补充的关键设计问题

- 当前 harness 是否假设项目一定是 Git repo？如果不是，baseline、diff、rollback 需要替代机制。
- Windows 是否需要作为 v1 支持目标？如果支持，atomic rename、file lock、shell command 策略需要单独设计。
- hooks 的配置文件由 harness 自动生成，还是由用户手工复制？
- `AGENTS.md` 和 `CLAUDE.md` 的职责如何与 `.harness/workflow-contract.md` 分离？
- Codex / Claude Code 的 CLI 输出 schema 失败时，是否允许模型二次修复 result，还是直接 job failed？
- token budget 如何硬性控制？例如 `state-summary.md` 最大多少字符，artifact 摘要最大多少行。
- 多 orchestrator 同时启动时谁是 leader？是否允许抢占？如果不允许，如何提示用户？

## 结论

这份调研报告的方向值得继续推进。它最强的部分是架构取舍：协议中心、共享状态、双向 runtime、hook guard、handoff 闭环，这些判断都合理。

下一步不应继续扩大功能，而应把它压实为 v1 规格：减少状态文件数量，明确状态迁移权限，包装 runtime adapter，定义 human gate，落实 PostToolUse 的真实 diff 复核，并把所有“完成”都绑定到结构化 result 和 verification gate。做到这些之后，它就能从架构蓝图进入可实现阶段。
