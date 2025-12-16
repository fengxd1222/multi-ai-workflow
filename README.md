# Multi-AI Workflow - 多 AI 协作开发工作流

一个完整的软件开发自动化工作流,通过协调 Claude、Codex 和 Gemini 三个 AI,实现从需求分析到代码审查的全流程自动化。

## ✨ 核心特性

- 🤖 **三 AI 协作**: Claude 负责需求分析,Codex 负责代码实现,Gemini 负责代码审查
- 📋 **完整工作流**: 需求分析 → 文档生成 → 代码实现 → 代码审查 → 迭代优化
- 🔄 **自动化流程**: 主流程自动执行,关键节点用户确认
- 📊 **文档驱动**: 自动生成需求文档和详细设计文档
- ✅ **质量保证**: Gemini 自动审查代码质量、安全性和性能
- 🛡️ **错误处理**: 完善的错误处理和恢复机制

## 🚀 工作流程

```
用户输入需求
    ↓
[Claude] 需求分析 + 生成文档
    ↓ (自动保存到 requirements/项目名/)
    ↓
[Codex] 执行编码
    ↓ (同步阻塞,检测结果)
    ├─ 成功 → 展示代码 → 询问用户
    └─ 失败 → 展示错误 → [重试/修改/终止]
    ↓ (用户确认)
[Gemini] 代码审查
    ↓ (生成报告)
展示审查结果 → 询问用户 [优化/接受/结束]
```

## 📦 使用场景

- ✅ 开发新功能模块
- ✅ 创建新项目脚手架
- ✅ 实现完整的业务逻辑
- ✅ 多 AI 协作开发
- ✅ 需求分析和技术设计

## 🔧 前置要求

### 必需的 CLI 工具

1. **Claude Code CLI** - 用于运行此 skill
2. **Codex CLI** - 用于代码生成
3. **Gemini CLI** - 用于代码审查

### 环境配置

```bash
# 确保以下工具已安装并配置好 API 密钥
which codex
which gemini

# 配置环境变量(根据实际 CLI 要求)
export CODEX_API_KEY="your-api-key"
export GEMINI_API_KEY="your-api-key"
```

## 📥 安装

1. 克隆此仓库到 Claude Code 的 skills 目录:

```bash
cd ~/.claude/skills
git clone https://github.com/fengxd1222/multi-ai-workflow.git
```

2. 确保脚本有执行权限:

```bash
chmod +x multi-ai-workflow/scripts/*.sh
```

3. 在 Claude Code 中激活 skill(如果需要)

## 🎯 使用方法

### 启动工作流

在 Claude Code 中调用此 skill,并描述你的需求:

```
创建一个用户管理的 REST API,使用 Python Flask,
包含用户注册、登录、信息修改功能,使用 JWT 认证。
```

### 工作流步骤

1. **项目初始化**: 输入项目名称,创建目录结构
2. **需求分析**: Claude 自动分析需求并生成文档
3. **代码实现**: Codex 根据文档生成代码
4. **代码审查**: Gemini 审查代码并提供改进建议
5. **迭代优化**: 可选的优化迭代循环

## 📁 目录结构

执行后会创建以下结构:

```
当前工作目录/
├── [生成的代码文件]
│   ├── src/
│   ├── tests/
│   ├── requirements.txt
│   └── README.md
└── requirements/
    └── 项目名/
        ├── 需求文档.md
        ├── 详细设计.md
        └── 代码审查报告.md
```

## 🔍 核心功能详解

### 1. 需求分析 (Claude)

- 深入理解用户需求
- 生成完整的需求文档
- 生成详细的技术设计文档
- 包含系统架构、技术栈、接口设计等

### 2. 代码实现 (Codex)

- 根据需求和设计文档生成代码
- 模块化、注释完整
- 包含错误处理和测试
- 输出到当前工作目录

### 3. 代码审查 (Gemini)

- 分析代码质量和安全性
- 识别严重问题和潜在风险
- 提供优化建议
- 生成详细审查报告

## ⚙️ 配置选项

### 脚本配置

脚本位于 `scripts/` 目录:

- `create_project_dir.sh` - 创建项目目录
- `call_codex.sh` - 调用 Codex CLI
- `call_gemini.sh` - 调用 Gemini CLI

### 文档模板

参考模板位于 `references/`:

- `document_templates.md` - 需求和设计文档模板
- `error_troubleshooting.md` - 错误处理指南

## 🛠️ 故障排除

### 常见问题

**Q: Codex 调用失败?**
- 检查 Codex CLI 是否正确安装
- 验证 API 密钥是否配置
- 查看网络连接是否正常

**Q: 生成的代码不符合预期?**
- 检查需求文档是否足够详细
- 修改需求文档后重新生成
- 查看 Codex CLI 的错误输出

**Q: Gemini 审查报告在哪?**
- 报告保存在 `requirements/项目名/代码审查报告.md`

详细的故障排除指南请查看 `references/error_troubleshooting.md`

## 📝 开发指南

### 修改工作流

如需自定义工作流,编辑 `SKILL.md`:

- 修改各步骤的执行逻辑
- 调整用户交互提示
- 自定义错误处理策略

### 扩展功能

- 添加新的 AI 工具集成
- 自定义文档模板
- 添加额外的验证步骤

## 🤝 贡献

欢迎贡献代码和反馈!

1. Fork 此仓库
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 提交 Pull Request

## 📄 许可证

本项目采用 Apache 2.0 许可证 - 详见 [LICENSE](LICENSE) 文件

## 🙏 致谢

- Claude Code - 提供 skill 框架
- Anthropic Claude - 需求分析和流程协调
- OpenAI Codex - 代码生成
- Google Gemini - 代码审查

## 📮 联系方式

- Issues: [GitHub Issues](https://github.com/fengxd1222/multi-ai-workflow/issues)
- Discussions: [GitHub Discussions](https://github.com/fengxd1222/multi-ai-workflow/discussions)

---

**版本**: 1.1.0
**更新时间**: 2025-12-16
