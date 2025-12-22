#!/bin/zsh
# Gemini 调用封装脚本 - 适配 multi-ai-workflow-cli v2.0
# 用途: 调用 Gemini CLI 进行代码审查，并生成审查报告
#
# 注意:
# - 使用 zsh 以便加载用户的 gemini 函数配置
# - 自动检测 gemini 是函数还是命令：
#   * 如果是函数（定义在 ~/.zshrc）：使用函数调用（包含用户的代理配置等）
#   * 如果是命令（系统安装的 CLI）：使用 command 调用

set -e  # 遇到错误立即退出

PROMPT=$1
PROJECT_DIR=$2

# 参数验证
if [ -z "$PROMPT" ] || [ -z "$PROJECT_DIR" ]; then
    echo "错误: 缺少必需参数"
    echo "用法: $0 <提示词> <项目目录>"
    exit 1
fi

# 检查项目目录是否存在
if [ ! -d "$PROJECT_DIR" ]; then
    echo "错误: 项目目录不存在: $PROJECT_DIR"
    exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "开始调用 Gemini 进行代码审查..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "项目目录: $PROJECT_DIR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 保存当前目录
ORIGINAL_DIR=$(pwd)

# 切换到项目目录
cd "$PROJECT_DIR" || exit 1

# 禁用 set -e 以捕获退出码
set +e

# 尝试加载 ~/.zshrc（如果用户在其中定义了 gemini 函数）
if [ -f ~/.zshrc ]; then
    source ~/.zshrc 2>/dev/null || true
fi

# 检查 gemini 是否是 zsh 函数
if typeset -f gemini > /dev/null 2>&1; then
    # gemini 是函数（用户在 ~/.zshrc 中定义，可能包含代理配置）
    echo "调用 Gemini (使用 ~/.zshrc 中的函数)..."
    gemini --yolo --output-format text "$PROMPT"
else
    # gemini 不是函数，作为普通命令调用
    echo "调用 Gemini (使用系统命令)..."
    command gemini --yolo --output-format text "$PROMPT"
fi

# 捕获退出码
EXIT_CODE=$?

# 返回原始目录
cd "$ORIGINAL_DIR" || true

set -e

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $EXIT_CODE -eq 0 ]; then
    echo "✓ Gemini 审查完成!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit 0
else
    echo "✗ Gemini 执行失败! 错误码: $EXIT_CODE"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "可能的原因:"
    echo "1. Gemini CLI 未安装或不在 PATH 中"
    echo "2. 网络连接问题（需要代理或 VPN）"
    echo "3. API 密钥未配置"
    echo "4. 提示词格式错误"
    echo ""
    echo "建议:"
    echo "- 检查 Gemini 是否可用:"
    echo "  * 如果是函数: 检查 ~/.zshrc 中的 gemini 函数定义"
    echo "  * 如果是命令: which gemini"
    echo "- 检查代理设置: echo \$HTTP_PROXY"
    echo "- 检查网络连接: curl -I https://cloudcode-pa.googleapis.com"
    echo "- 手动测试: gemini --yolo 'hello'"
    echo "- 或使用 Claude: /workflow-start --step3=claude"
    exit 1
fi
