#!/bin/bash
# 创建项目目录结构
# 用途: 为多 AI 协作工作流创建必要的目录结构

PROJECT_NAME=$1

# 参数验证
if [ -z "$PROJECT_NAME" ]; then
    echo "错误: 缺少项目名称参数"
    echo "用法: $0 <项目名称>"
    exit 1
fi

# 验证项目名称格式 (只允许字母、数字、连字符、下划线)
if ! [[ "$PROJECT_NAME" =~ ^[a-zA-Z0-9_-]+$ ]]; then
    echo "错误: 项目名称格式不正确"
    echo "项目名称只能包含字母、数字、连字符(-)和下划线(_)"
    echo "示例: user-api, my_project, project123"
    exit 1
fi

echo "创建项目目录结构..."
echo ""

# 创建文档目录
REQUIREMENTS_DIR="requirements/$PROJECT_NAME"

if [ -d "$REQUIREMENTS_DIR" ]; then
    echo "警告: 目录已存在: $REQUIREMENTS_DIR"
    echo "将使用现有目录,可能会覆盖已有文件"
    echo ""
else
    mkdir -p "$REQUIREMENTS_DIR"
    if [ $? -ne 0 ]; then
        echo "✗ 目录创建失败: $REQUIREMENTS_DIR"
        echo ""
        echo "可能原因:"
        echo "- 权限不足"
        echo "- 磁盘空间不足"
        echo "- 父目录不可写"
        exit 1
    fi
fi

# 确认目录创建成功
if [ -d "$REQUIREMENTS_DIR" ]; then
    echo "✓ 项目目录创建成功"
    echo ""
    echo "目录结构:"
    echo "  📁 $REQUIREMENTS_DIR/  - 存放需求文档、设计文档和审查报告"
    echo "  📁 $(pwd)              - 当前工作目录 (代码将生成在此)"
    echo ""
    exit 0
else
    echo "✗ 目录创建失败"
    exit 1
fi
