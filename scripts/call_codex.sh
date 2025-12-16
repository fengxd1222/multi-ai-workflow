#!/bin/bash
# Codex 调用封装脚本
# 用途: 调用 Codex CLI 进行代码实现,并处理执行结果

set -e  # 遇到错误立即退出

PROJECT_NAME=$1
REQUIREMENTS_DOC=$2
OUTPUT_DIR=$3

# 参数验证
if [ -z "$PROJECT_NAME" ] || [ -z "$REQUIREMENTS_DOC" ] || [ -z "$OUTPUT_DIR" ]; then
    echo "错误: 缺少必需参数"
    echo "用法: $0 <项目名称> <需求文档路径> <输出目录>"
    exit 1
fi

# 检查需求文档是否存在
if [ ! -f "$REQUIREMENTS_DOC" ]; then
    echo "错误: 需求文档不存在: $REQUIREMENTS_DOC"
    exit 1
fi

# 检查输出目录是否存在
if [ ! -d "$OUTPUT_DIR" ]; then
    echo "错误: 输出目录不存在: $OUTPUT_DIR"
    exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "开始调用 Codex 进行编码..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "项目名称: $PROJECT_NAME"
echo "需求文档: $REQUIREMENTS_DOC"
echo "设计文档: requirements/$PROJECT_NAME/详细设计.md"
echo "输出目录: $OUTPUT_DIR"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# 构建 Codex 提示词
DESIGN_DOC="requirements/$PROJECT_NAME/详细设计.md"

PROMPT="根据以下文档在当前目录实现功能:

文档位置:
- 需求文档: $REQUIREMENTS_DOC
- 设计文档: $DESIGN_DOC

实现要求:
1. 代码结构清晰,采用模块化设计
2. 为所有公共函数和类添加完整的注释和文档字符串
3. 严格遵循所选语言的最佳实践和编码规范
4. 实现基础的错误处理和异常捕获
5. 如适用,编写单元测试以确保代码质量
6. 生成必要的配置文件(如 requirements.txt, package.json 等)
7. 创建 README.md 说明项目结构和使用方法

请仔细阅读需求文档和设计文档,确保实现所有功能点。"

# 调用 Codex (禁用 set -e 以捕获退出码)
set +e
codex exec "$PROMPT" -C "$OUTPUT_DIR" --full-auto

# 捕获退出码
EXIT_CODE=$?
set -e

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ $EXIT_CODE -eq 0 ]; then
    echo "✓ Codex 执行成功!"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit 0
else
    echo "✗ Codex 执行失败! 错误码: $EXIT_CODE"
    echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
    exit $EXIT_CODE
fi
