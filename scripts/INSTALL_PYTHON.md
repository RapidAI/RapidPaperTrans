# Python 安装指南

## 快速安装步骤

### 1. 安装 Python

从官网下载并安装 Python:
- 访问: https://www.python.org/downloads/
- 下载最新版本 (推荐 Python 3.11 或 3.12)
- 安装时 **勾选 "Add Python to PATH"**

或者使用 winget (Windows 包管理器):
```powershell
winget install Python.Python.3.11
```

### 2. 安装 PyMuPDF

打开命令提示符或 PowerShell，运行:
```powershell
pip install PyMuPDF
```

### 3. 验证安装

```powershell
python --version
python -c "import fitz; print(fitz.version)"
```

### 4. 运行翻译脚本

```powershell
cd latex-translator
python scripts/translate_pdf.py single-test.pdf output_single_test/translation_cache.json output_single_test/python_translated.pdf
```

## 常见问题

### "Python was not found" 错误

这是因为 Windows Store 的 Python 占位符。解决方法:
1. 打开 Windows 设置
2. 搜索 "应用执行别名" 或 "App execution aliases"
3. 关闭 "python.exe" 和 "python3.exe" 的开关
4. 安装真正的 Python

### pip 命令找不到

尝试:
```powershell
python -m pip install PyMuPDF
```

### 中文字体问题

PyMuPDF 内置了中文字体 (china-ss)，通常不需要额外配置。
如果遇到字体问题，可以指定系统字体:
- 微软雅黑: C:/Windows/Fonts/msyh.ttc
- 宋体: C:/Windows/Fonts/simsun.ttc
