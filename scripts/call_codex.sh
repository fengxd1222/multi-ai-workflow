#!/bin/bash
# Codex 调用封装脚本 - 适配 multi-ai-workflow-cli v2.0
# 用途: 调用 Codex CLI 进行代码生成

set -e  # 遇到错误立即退出

PROMPT=$1
OUTPUT_DIR=$2

# 参数验证
if [ -z "$PROMPT" ] || [ -z "$OUTPUT_DIR" ]; then
    echo "错误: 缺少必需参数"
    echo "用法: $0 <提示词> <输出目录>"
    exit 1
fi

# 确保输出目录存在
if [ ! -d "$OUTPUT_DIR" ]; then
    echo "错误: 输出目录不存在: $OUTPUT_DIR"
    exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "开始调用 Codex 进行代码实现..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "输出目录: $OUTPUT_DIR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 保存当前目录
ORIGINAL_DIR=$(pwd)

# 禁用 set -e 以捕获退出码
set +e

# 调用 Codex CLI
echo "调用 Codex..."
codex exec "$PROMPT" -C "$OUTPUT_DIR" --full-auto

# 捕获退出码
EXIT_CODE=$?

# 返回原始目录
cd "$ORIGINAL_DIR" || true

set -e

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $EXIT_CODE -eq 0 ]; then
    echo "✓ Codex 代码生成完成!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit 0
else
    echo "✗ Codex 执行失败! 错误码: $EXIT_CODE"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    echo ""
    echo "可能的原因:"
    echo "1. Codex CLI 未安装或不在 PATH 中"
    echo "2. API 密钥未配置"
    echo "3. 提示词格式错误"
    echo "4. 输出目录权限问题"
    echo ""
    echo "建议:"
    echo "- 检查 Codex CLI: which codex"
    echo "- 检查配置: codex --version"
    echo "- 手动测试: codex exec 'hello world'"
    echo "- 或使用 Claude: /workflow-start --ai=claude"
    exit 1
fi
