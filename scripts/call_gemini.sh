#!/bin/zsh
# Gemini 调用封装脚本
# 用途: 调用 Gemini CLI 进行代码审查,并生成审查报告
# 注意: 使用 zsh 以便加载用户的 gemini 函数配置

set -e  # 遇到错误立即退出

PROJECT_NAME=$1
CODE_DIR=$2
OUTPUT_REPORT=$3

# 参数验证
if [ -z "$PROJECT_NAME" ] || [ -z "$CODE_DIR" ] || [ -z "$OUTPUT_REPORT" ]; then
    echo "错误: 缺少必需参数"
    echo "用法: $0 <项目名称> <代码目录> <报告输出路径>"
    exit 1
fi

# 检查代码目录是否存在
if [ ! -d "$CODE_DIR" ]; then
    echo "错误: 代码目录不存在: $CODE_DIR"
    exit 1
fi

# 确保报告输出目录存在（相对于代码目录）
OUTPUT_DIR=$(dirname "$OUTPUT_REPORT")
FULL_OUTPUT_DIR="$CODE_DIR/$OUTPUT_DIR"
if [ ! -d "$FULL_OUTPUT_DIR" ]; then
    echo "警告: 报告输出目录不存在，尝试创建: $FULL_OUTPUT_DIR"
    mkdir -p "$FULL_OUTPUT_DIR" || {
        echo "错误: 无法创建报告输出目录: $FULL_OUTPUT_DIR"
        exit 1
    }
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "开始调用 Gemini 进行代码审查..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "项目名称: $PROJECT_NAME"
echo "代码目录: $CODE_DIR"
echo "报告输出: $OUTPUT_REPORT"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 构建 Gemini 提示词（简洁版，让 Gemini 自己读取文档和代码）
DESIGN_DOC="requirements/$PROJECT_NAME/详细设计.md"
REQUIREMENTS_DOC="requirements/$PROJECT_NAME/需求文档.md"

PROMPT="请审查这个项目的代码质量。

项目信息:
- 项目名称: $PROJECT_NAME
- 需求文档: $REQUIREMENTS_DOC
- 设计文档: $DESIGN_DOC
- 代码目录: $CODE_DIR (当前目录)

任务:
1. 阅读需求文档和设计文档,理解项目需求
2. 分析代码目录中的所有代码文件
3. 从代码质量、安全性、性能、测试覆盖等维度进行审查
4. 生成详细的代码审查报告,包含问题描述、严重等级、具体位置和修改建议
5. 将审查报告保存到: $OUTPUT_REPORT

请使用 Markdown 格式输出报告。"

# 调用 Gemini (禁用 set -e 以捕获退出码)
# 注意: 使用 zsh 加载 .zshrc 中的 gemini 函数，该函数包含代理配置
set +e

# 加载 zsh 配置（包含 gemini 函数）
source ~/.zshrc

# 保存当前目录
ORIGINAL_DIR=$(pwd)

# 切换到代码目录并调用 Gemini
cd "$CODE_DIR" || exit 1

# 使用 gemini 函数（自动包含代理配置）
gemini --yolo --output-format text "$PROMPT"

# 捕获退出码
EXIT_CODE=$?

# 返回原始目录
cd "$ORIGINAL_DIR" || true

set -e

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $EXIT_CODE -eq 0 ]; then
    echo "✓ Gemini 审查完成!"
    echo "报告已保存: $OUTPUT_REPORT"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit 0
else
    echo "✗ Gemini 执行失败! 错误码: $EXIT_CODE"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit $EXIT_CODE
fi
