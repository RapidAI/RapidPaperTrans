#!/bin/bash

# AI布局检测环境设置脚本

set -e

echo "=========================================="
echo "AI布局检测环境设置"
echo "=========================================="

# 检测操作系统
OS="$(uname -s)"
case "${OS}" in
    Linux*)     PLATFORM=linux;;
    Darwin*)    PLATFORM=macos;;
    MINGW*|MSYS*|CYGWIN*)    PLATFORM=windows;;
    *)          PLATFORM="UNKNOWN:${OS}"
esac

echo "检测到操作系统: ${PLATFORM}"

# 创建必要的目录
echo ""
echo "创建目录结构..."
mkdir -p models
mkdir -p fonts
mkdir -p output

# 下载DocLayout-YOLO模型
MODEL_PATH="models/doclayout_yolo_docstructbench_imgsz1024.onnx"
if [ -f "$MODEL_PATH" ]; then
    echo ""
    echo "模型已存在: $MODEL_PATH"
else
    echo ""
    echo "下载DocLayout-YOLO模型..."
    echo "模型大小约200MB，请耐心等待..."
    
    # 使用HuggingFace镜像（国内用户）
    if [ -n "$USE_HF_MIRROR" ]; then
        MODEL_URL="https://hf-mirror.com/wybxc/DocLayout-YOLO-DocStructBench-onnx/resolve/main/doclayout_yolo_docstructbench_imgsz1024.onnx"
    else
        MODEL_URL="https://huggingface.co/wybxc/DocLayout-YOLO-DocStructBench-onnx/resolve/main/doclayout_yolo_docstructbench_imgsz1024.onnx"
    fi
    
    if command -v wget &> /dev/null; then
        wget -O "$MODEL_PATH" "$MODEL_URL"
    elif command -v curl &> /dev/null; then
        curl -L -o "$MODEL_PATH" "$MODEL_URL"
    else
        echo "错误: 需要wget或curl来下载模型"
        echo "请手动下载模型到: $MODEL_PATH"
        echo "下载地址: $MODEL_URL"
        exit 1
    fi
    
    echo "模型下载完成！"
fi

# 下载思源字体
FONT_PATH="fonts/SourceHanSerifCN-Regular.ttf"
if [ -f "$FONT_PATH" ]; then
    echo ""
    echo "字体已存在: $FONT_PATH"
else
    echo ""
    echo "下载思源宋体..."
    FONT_URL="https://github.com/adobe-fonts/source-han-serif/raw/release/OTF/SimplifiedChinese/SourceHanSerifCN-Regular.otf"
    
    if command -v wget &> /dev/null; then
        wget -O "$FONT_PATH" "$FONT_URL"
    elif command -v curl &> /dev/null; then
        curl -L -o "$FONT_PATH" "$FONT_URL"
    else
        echo "警告: 无法下载字体，请手动下载"
        echo "下载地址: $FONT_URL"
    fi
fi

# 安装系统依赖
echo ""
echo "检查系统依赖..."

case "${PLATFORM}" in
    linux)
        echo "Linux系统，检查依赖..."
        
        # 检查ONNX Runtime
        if ! ldconfig -p | grep -q libonnxruntime; then
            echo "需要安装ONNX Runtime:"
            echo "  Ubuntu/Debian: sudo apt-get install libonnxruntime-dev"
            echo "  或从 https://github.com/microsoft/onnxruntime/releases 下载"
        fi
        
        # 检查poppler-utils (pdftoppm)
        if ! command -v pdftoppm &> /dev/null; then
            echo "需要安装poppler-utils:"
            echo "  sudo apt-get install poppler-utils"
        fi
        ;;
        
    macos)
        echo "macOS系统，检查依赖..."
        
        # 检查Homebrew
        if ! command -v brew &> /dev/null; then
            echo "建议安装Homebrew: https://brew.sh"
        else
            # 检查ONNX Runtime
            if ! brew list onnxruntime &> /dev/null; then
                echo "安装ONNX Runtime:"
                echo "  brew install onnxruntime"
            fi
            
            # 检查poppler
            if ! brew list poppler &> /dev/null; then
                echo "安装poppler:"
                echo "  brew install poppler"
            fi
        fi
        ;;
        
    windows)
        echo "Windows系统，请手动安装依赖:"
        echo "1. ONNX Runtime: https://github.com/microsoft/onnxruntime/releases"
        echo "2. Poppler for Windows: https://github.com/oschwartz10612/poppler-windows/releases"
        ;;
esac

# 安装Go依赖
echo ""
echo "安装Go依赖..."
go get github.com/yalue/onnxruntime_go
go get github.com/disintegration/imaging

echo ""
echo "=========================================="
echo "环境设置完成！"
echo "=========================================="
echo ""
echo "下一步:"
echo "1. 确保已安装系统依赖（ONNX Runtime, poppler-utils）"
echo "2. 设置环境变量（如需要）:"
echo "   export OPENAI_API_KEY='your-api-key'"
echo "3. 运行测试:"
echo "   go run cmd/test_ai_layout/main.go -pdf your.pdf -ai"
echo ""
