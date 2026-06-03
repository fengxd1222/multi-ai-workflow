# 对调研报告的架构级意见（补充评审）

> 说明：目录下已有一份 `deep-research-review.md`，它从「收缩 v1 范围、补状态机权限、包装 adapter、定义 human gate」的角度评审，结论扎实。
> 本文不重复那些点，只针对**我认为报告里更深、更要命、且会决定整套系统是否优雅的架构假设**提出异议和取舍建议。
> 一句话立场：报告的方向（协议先于运行时、`.harness/` 为真相、双向对等）是对的，但它把一个**单机本地协作工具**写成了一套**分布式系统协议**，引入了大量本可消除的边界情况。下面按「该砍的复杂度」和「该补的硬决策」两类展开。

---

## 一、报告最对的三个判断（不展开，已认同）

1. **协议先于运行时**：Codex / Claude Code 都只当 runtime，协调权在 `.harness/` 的文件协议里。这是双向对等成立的唯一正确抽象。
2. **`.harness/` 是真相，会话只是缓存**：不靠 transcript、不靠会话记忆恢复状态。
3. **needs-human 与 failed 分开**：人工 gate 是独立语义而非失败。

这三点是整份报告的地基，值得保留。我的异议都建立在「认同地基」之上。

---

## 二、我的核心异议（报告应当重新决策的地方）

### 1. `claim` / 队列模型和「无 daemon、无后台服务」前提自相矛盾 —— 这是最该先拍板的一处

报告同时存在两套互相打架的委派模型：

- **A. push（orchestrator 直接派发）**：`harness delegate` 创建 job → 拉起 runtime adapter → 子进程就是这个 job 的 worker。
- **B. pull（worker 抢单）**：job 生命周期里有 `queued → claimed`，worker「启动后 claim job 并申请 lease」。

`claimed` 这个状态只有在 **多个长生命周期 worker 从同一个共享队列里抢任务** 时才有意义——那是 daemon / 消息队列的语义。但报告的前提恰恰是「无后台服务、无数据库、orchestrator 直接 spawn 子进程」。在 push 模型里，orchestrator **派发的瞬间就知道哪个 job 给了哪个 worker**，根本不存在「抢」这个动作，`queued/claimed` 是多余状态。

**我的建议**：v1 明确选 push 模型。job 状态机砍成 `created → running → (completed | failed | needs-human | timeout)`，去掉 `queued / claimed / blocked` 里和「抢单」相关的语义。`blocked` 如果保留，应只表示「等待另一个 job 的产物」，而不是「等待被认领」。pull / 队列只在未来真要做「常驻 worker 池」时再引入，而那已经违背了当前的低依赖目标。

> 这一刀下去，lease 的「acquire 前先 claim」一整套流程都能简化掉一半。

### 2. 用 lease + heartbeat 保护共享工作区，本质是在重新发明 git worktree 已经免费解决的问题

报告为「两个 agent 同时想写同一文件」设计了：ownership map + 独占 write lease + 60–120s heartbeat + 2–4min stale 回收 + PreToolUse 拦截 + PostToolUse diff 复核 + pathspec 回滚。这是一套**自研的文件级并发控制 + 分布式租约**，复杂度极高，而且每条边都有失败模式（heartbeat 抖动、进程僵死但未退出、lease 过期但工作仍在跑、torn write……）。

而 git worktree 本就是为「同仓库多工作树并行、互不脏写」设计的，merge 冲突由 git 解决——这是几十年打磨过的正确数据结构。报告把 worktree 降级为「冲突升级策略」，是为了迁就「默认共享工作区」这个**偏好**；但从「好品味 = 消除边界情况」的角度看，结论恰恰相反：

- **并行 implementation job 之间真要并发写**：那就 worktree-per-job（或 branch-per-job），让冲突在结构上不可能发生，根本不需要 lease。
- **共享工作区**：只在 job **串行执行、或各 job scope 物理不重叠**时才用，此时连 lease 都不太需要——一个文件级的简单「占用标记 + 进程存活检查」就够，根本上不到 TTL/heartbeat/租约这种重武器。

**我的建议**：把规则翻过来——
- **预防优于检测**：并行写优先用 worktree 隔离；shared workspace 只服务「无重叠 scope 的并行」和「全程串行」。
- 真要在 shared workspace 做并行写时，**不要造 lease 协议**，直接升级 worktree。lease/heartbeat 这套东西如果保留，也应标成 v2，而不是 v1 内核。
- 报告里「先做 lease 内核，worktree 作为增强」的实施顺序，我认为应当反过来。

### 3. orchestrator 到底是 LLM 还是确定性 CLI 代码？报告没划线，但这条线决定一切

报告说 orchestrator 负责 intake / 拆任务 / 分配 scope / 汇总 / 推进 phase / 做 gate / integrate。但它既可能指「一个跑着长 prompt 的 Claude/Codex 会话」，也可能指「`harness` 这个确定性 CLI」。这两者混在一起是危险的：如果让 LLM 来决定「lease 是否过期、状态能否迁移、gate 是否放行」，那协议的确定性就被一个会幻觉的东西接管了。

**我的建议**：硬性划线——**LLM 提议，CLI 裁决（LLM proposes, CLI disposes）**。
- **确定性 CLI 代码**负责：状态迁移合法性、lease/占用判定、scope 越权检测、event 写入、gate 触发条件、verify 命令的执行与判定、错误码。这些**绝不能**交给模型。
- **LLM** 只负责需要判断力的部分：任务分解、写 brief/plan、评估 worker 产出的语义质量、生成 handoff 文字。
- 因此「orchestrator」其实是**两层**：一个 LLM 会话（决策层）+ 一组确定性命令（协议层）。报告应把这两层显式拆开，否则实现时一定会把协议逻辑塞进 prompt，导致不可复现。

### 4. event replay 能重建「逻辑状态」，但重建不了「工作树」——报告把两种恢复混为一谈

报告把 append-only event log 当「最终事实来源 + 可重放快照」，暗示崩溃后能 replay 恢复。但要分清两件事：

- **逻辑状态恢复**（session/task/job 处于什么状态）：event replay 有效。
- **工作树恢复**（一个 worker 被 kill 在写到一半，文件残缺、改动半成品）：event replay **完全无能为力**，replay 不会撤销文件系统的副作用。

真正能回滚工作树的只有 git（`git restore --source=<tree> -- <pathspec>` / stash）。所以恢复的真实公式是：**git 负责回滚工作树，event log 只负责审计与重新派发决策**。报告里 event log 承担的「恢复」职责被夸大了，应当降级为「可观测性 + 决策依据」，把工作树回滚的责任明确交给 git baseline + pathspec restore。

> 推论：**这个 harness 在 v1 应当硬性要求是 git repo**。没有 git，baseline / diff 复核 / pathspec 回滚全部失效，报告里大半安全机制会落空。这点必须写进前置约束，而不是留作开放问题。

### 5. 「无锁 append」和「全局 event seq」是自相矛盾的，二选一

报告一边说「event 单独 append，避免重写大快照」（追求免锁并发），一边又给 event 设计了全局 `event_seq`、`recovery_cursor.last_good_event_seq`（要求全局有序）。这两个目标无法同时成立：

- 全局单调 `seq` 必须由**单一写者**或**全局锁**分配——那 append 就不再是免锁的，`state.lock` 反而成了所有 hook 并发写入的瓶颈（而 sharding 本来就是为了避开这个瓶颈）。
- 而且多进程向同一个 `events.jsonl` 并发 append，**并非所有文件系统都保证单条记录原子写入**：POSIX 的 `O_APPEND` 只在「单次 `write()` 调用 + 本地文件系统」下大致安全，记录较大或在网络文件系统上可能出现 torn / 交错写。

**我的建议**：放弃全局 seq，改用**每个 actor 一个事件文件**（`events/<agent_id>.jsonl`），各写各的、零共享锁；读取时按 `(timestamp, actor, local_seq)` 归并出一个视图。需要因果序的地方用 `caused_by: <event_id>` 显式串联，而不是依赖一个全局计数器。这样既真正免锁，又不会有「seq 分配点变成全局瓶颈」的隐藏矛盾。

### 6. hooks 作为主防护过于脆弱；安全的主力应是 spawn 时的 scope 约束

报告把 PreToolUse lease 检查当主防护。问题：(1) 每次工具调用都要读/解析 JSON 做 lease 判定，给每次 edit 加延迟；(2) Codex / Claude 的 hook 输入结构、可阻塞行为不一致，「统一 adapter」吞掉差异的同时也吞掉了失败；(3) 很多写入来自 Bash/formatter/codegen，PreToolUse 根本预测不到影响面（这点已有 review 提过）。

**我的建议**：把安全主力前移到**派发时就给死边界**——
- Claude 用 `--allowedTools` 把可用工具和 `Bash(...)` 模式收窄；Codex 用 `--sandbox workspace-write` + execpolicy。让 worker **结构上就碰不到 scope 外的东西**。
- hook 退为**纵深防御的第二层**（事后用 `git diff --name-only` 复核越权），而不是唯一闸门。
- 一句话：**预防（spawn 约束）> 检测（hook 复核）> 回滚（git pathspec）**，三层都要，但顺序别搞反。

### 7. 完全缺失：成本 / 递归深度 / 预算控制（多 agent 系统的头号失控源）

报告有 `retry_policy`、`timeout_at`，但**没有任何全局预算、委派深度上限、环路检测**。多 agent harness 最容易出两种事故：(1) token 成本指数级膨胀；(2) A 委派 B、B 又委派回 A 的委派环。这在「双向对等、互相能调用对方」的设计里尤其危险——双向对等放大了成环风险。

**我的建议**，v1 就要有：
- `max_delegation_depth`（建议默认 2–3），context-packet 里带 `delegation_chain`，超限直接 `needs-human`。
- 环路检测：派发前检查 `delegation_chain` 是否已含目标 runtime+role。
- task 级 token / 墙钟预算上限，由确定性 CLI 统计（adapter 能从 `codex exec --json` / `claude --output-format json` 的用量字段里抓到），超预算停止并 gate。

### 8. schema 校验失败的处置缺一条明确回路

`codex exec --output-schema` / `claude --json-schema` 在真实使用中**会**产出不合 schema 的结果（模型偶发跑偏）。报告默认结构化 result 总能拿到。必须定义失败回路：

**建议**：`result schema 校验失败 → 自动一次「修复重试」（把 schema error 回灌给同一会话要求重出）→ 仍失败则 job 置 `failed` 并附原始 stdout 供人工`。同时严格区分「进程层失败」（exit code≠0 / 认证 / 网络）与「模型层失败」（result 不合 schema），这两类的恢复动作不同（前者重启进程，后者修复输出）。报告已强调进程层与业务层分离，这里要把「schema 失败」明确归到业务层并给出重试策略。

### 9. 怎么测试 harness 本身？—— 协议层必须能在不烧 API 的前提下被测

一套多 agent 协调层，最该有却最常被忽略的是**确定性可测性**。状态机、lease 过期、恢复、越权检测、环路检测，这些都不该靠真实调用 Codex/Claude 才能验证。

**建议**：把 runtime adapter 设计成**可注入的接口**，提供一个 `mock` runtime（按脚本返回预设 result / 故意返回坏 schema / 故意越权写文件 / 故意僵死不退出）。协议层的所有分支用 mock runtime 做确定性测试，真实 CLI 只在少量端到端冒烟测试里跑。这条如果不在一开始就定，后面整个状态机都将无法回归测试。

---

## 三、与已有 `deep-research-review.md` 的关系

已有 review 的 8 条（引用不可核验、v1 偏大、真实拦截不具体、状态写协议、状态机非法迁移、adapter 包装、gate 语义、job/task 级 verify 分层）我都认同，不重复。

它和本文的分工：
- **已有 review**：在「报告这套设计成立」的前提下，教你**怎么把它收紧成可实现的 v1 spec**。
- **本文**：质疑「这套设计本身是不是太重了」，主张**砍掉一批分布式语义、用 git 顶替自研并发控制、把 orchestrator 拆成 LLM+CLI 两层**。

如果两份意见有冲突，冲突点就是真正需要你拍板的地方（见下）。

---

## 四、我会怎么砍 v1（比已有 review 更激进）

已有 review 把 v1 收到「单 session / 单 task / file lease / 三类 hook」。我会再砍一刀：

1. **push 模型，去掉抢单**：orchestrator spawn 即派发；job 状态机砍到 5 个状态。
2. **v1 不做 lease/heartbeat**：并行写一律走 worktree-per-job + git merge；shared workspace 只用于串行或物理不重叠的并行。把 lease 整套推到 v2。
3. **orchestrator 两层显式拆分**：LLM 决策层 + 确定性协议 CLI；协议判定零 LLM 参与。
4. **git 作为硬前置**：非 git repo 直接拒绝运行（或 `harness init` 时 `git init`）。
5. **event log 降级为审计**：每 actor 一个 `events/<agent>.jsonl`，无全局 seq；恢复工作树只用 git。
6. **预防式 scope**：靠 `--allowedTools` / `--sandbox` 给死边界，hook 只做事后 diff 复核。
7. **预算与环路**：`max_delegation_depth` + 委派链 + task 级 token 预算，v1 必备。
8. **mock runtime**：协议层全部可在不烧 API 下测试。

闭环目标和已有 review 一致：**一个 task 从 create 到 handoff 能在双向 runtime 上跑通**，但路径更短、边界情况更少。

---

## 五、需要你拍板的关键决策（这些答案会改变架构）

1. **委派模型**：push（orchestrator 直接 spawn+派发，简单）还是 pull（队列+抢单，需要更重的协调）？我强烈建议 push。
2. **并行写隔离**：worktree-per-job（git 兜底，边界情况少）还是坚持 shared workspace + 自研 lease（贴合偏好但复杂）？我建议并行写默认 worktree，shared workspace 只做串行/不重叠。
3. **是否硬性要求 git repo**？我建议是——否则一半安全机制失效。
4. **orchestrator 的协议判定**是否允许 LLM 参与？我建议完全不允许，只让 LLM 做语义判断。
5. **目标平台**：v1 是否要管 Windows？atomic rename / flock / shell 策略在 Windows 上要单独设计，建议 v1 先只保 macOS/Linux。
6. **并发规模**：实际同时跑几个 agent？2–4 个的话，很多分布式语义（全局 seq、TTL 租约）都可以不做。

---

## 六、结论

报告的**方向**对，但**重量**偏。它把一个单机本地工具按分布式系统去设计，引入了 lease、heartbeat、全局 event seq、queue/claim 这些会带来大量边界情况的机制——而其中相当一部分，git worktree + git restore + 派发即派定的 push 模型，已经用更成熟的数据结构免费解决了。

下一步我建议不是「把蓝图收紧成 v1 spec」（那是已有 review 的方向，也对），而是先回答第五节那 6 个决策。其中第 1、2、4 条一旦定了，报告里至少 1/3 的复杂度可以直接删掉。**真正的好品味，是让这套 harness 小到不需要恢复协议就几乎不会坏。**
