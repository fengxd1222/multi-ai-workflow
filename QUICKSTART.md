# 快速入门指南 - Multi-AI Workflow CLI

## 🎉 安装已完成！

multi-ai-workflow-cli 已经成功安装并测试。

## 📁 安装位置

```
~/.claude/skills/multi-ai-workflow-cli/  # Skill 主目录
~/.claude/commands/workflow-*.md          # Slash 命令
~/.claude/data/workflow-cli.db            # SQLite 数据库
```

## ⚡ 快速开始

### 第一步：初始化项目

```bash
/workflow-init myproject
cd myproject
```

### 第二步：编写需求

编辑 `docs/requirements.md` 文件，描述你的项目需求。

或者让 AI 帮你生成：

```bash
/workflow-start
# 然后直接描述你的需求，AI 会生成文档
```

### 第三步：执行工作流

```bash
/workflow-start
```

工作流将自动执行以下步骤：
1. **需求分析** (AI: claude) - 生成 requirements.md 和 design.md
2. **代码实现** (AI: codex) - 生成完整项目代码
3. **代码审查** (AI: gemini) - 生成审查报告
4. **优化迭代** (可选) - 根据审查结果优化代码

## 🎮 常用命令

```bash
# 查看工作流状态
/workflow-status

# 暂停工作流
/workflow-pause

# 恢复工作流
/workflow-resume

# 列出所有工作流
/workflow-list

# 配置管理
/workflow-config show
/workflow-config set step1_ai claude
```

## 🔧 配置 AI 类型

### 方法 1: 命令行参数（最高优先级）

```bash
/workflow-start --step1=claude --step2=codex --step3=gemini
```

### 方法 2: 项目配置文件

编辑项目中的 `.workflow-config.yaml`:

```yaml
ai:
  step1: claude    # 需求分析
  step2: codex     # 代码实现
  step3: gemini    # 代码审查
```

### 方法 3: 全局配置

```bash
/workflow-config set step1_ai claude
/workflow-config set step2_ai codex
/workflow-config set step3_ai gemini
```

## 📊 项目结构

每个项目都遵循标准结构：

```
myproject/
├── docs/                    # 文档目录
│   ├── requirements.md      # 需求文档
│   ├── design.md            # 设计文档
│   └── reviews/             # 审查报告
├── code/                    # 代码目录
│   └── ...                  # 由 AI 根据技术栈生成
├── .workflow/               # 工作流元数据
│   ├── config.yaml
│   └── workflow-id.txt
├── README.md
├── .gitignore
└── .workflow-config.yaml
```

## 🚀 高级功能

### 暂停和恢复

在任何用户确认点暂停：

```bash
/workflow-pause
```

稍后恢复：

```bash
/workflow-resume
```

### 回滚到之前的步骤

```bash
# 回滚到步骤 2（代码实现）
/workflow-rollback 2
```

### 查看所有工作流

```bash
# 列出所有工作流
/workflow-list

# 按项目筛选
/workflow-list --project=myproject

# 按状态筛选
/workflow-list --status=paused
```

### 清理旧工作流

```bash
# 清理 7 天前的已完成工作流
/workflow-clean

# 清理 30 天前的工作流
/workflow-clean --older-than=30

# 预览将要清理的内容
/workflow-clean --dry-run
```

## 🔍 测试安装

运行测试脚本验证安装：

```bash
~/.claude/skills/multi-ai-workflow-cli/scripts/test_workflow.sh
```

## 📚 完整文档

查看完整文档：

```bash
cat ~/.claude/skills/multi-ai-workflow-cli/README.md
```

## 💡 示例：创建一个 Python 计算器

```bash
# 1. 初始化项目
/workflow-init python-calculator
cd python-calculator

# 2. 启动工作流
/workflow-start

# 3. 当 AI 询问需求时，回答：
"创建一个命令行计算器程序，支持基本的加减乘除运算，
要求使用 Python 实现，有完整的错误处理和单元测试。"

# 4. AI 会自动：
#    - 生成需求文档和设计文档
#    - 实现完整的 Python 代码
#    - 进行代码审查
#    - 提供优化建议

# 5. 完成后查看结果
ls -la code/
cat docs/reviews/review-*.md
```

## 🛠️ 故障排除

### 数据库问题

```bash
# 重新初始化数据库（警告：会清空历史记录）
rm ~/.claude/data/workflow-cli.db
~/.claude/skills/multi-ai-workflow-cli/scripts/db_manager.sh init
```

### 工作流卡住

```bash
# 查看状态
/workflow-status

# 回滚到之前的步骤
/workflow-rollback 1

# 或启动新的工作流
/workflow-start
```

### 外部 CLI 工具问题

如果 Codex 或 Gemini CLI 不可用，可以使用 Claude 执行所有步骤：

```bash
/workflow-start --ai=claude
```

## 📖 更多帮助

- 完整 README: `~/.claude/skills/multi-ai-workflow-cli/README.md`
- 命令文档: `~/.claude/commands/workflow-*.md`
- 数据库工具: `~/.claude/skills/multi-ai-workflow-cli/scripts/db_manager.sh`
- 状态管理: `~/.claude/skills/multi-ai-workflow-cli/scripts/state_manager.sh`
- 配置管理: `~/.claude/skills/multi-ai-workflow-cli/scripts/config_manager.sh`

## 🎯 下一步

1. 创建你的第一个项目：`/workflow-init my-first-project`
2. 阅读完整文档了解所有功能
3. 探索不同的 AI 配置组合
4. 尝试暂停/恢复功能
5. 使用回滚功能优化工作流

祝你使用愉快！🚀
