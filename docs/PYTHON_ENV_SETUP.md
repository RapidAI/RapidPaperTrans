# Python 环境自动安装

本应用程序会自动安装和管理一个独立的 Python 环境，用于 PDF 翻译功能。用户无需手动安装 Python 或任何依赖包。

## 工作原理

1. **自动下载 uv**: 程序会自动从 GitHub 下载 [uv](https://github.com/astral-sh/uv) - 一个快速的 Python 包管理器
2. **创建虚拟环境**: 使用 uv 创建一个独立的 Python 3.11 虚拟环境
3. **安装依赖**: 自动安装 PDF 处理所需的 PyMuPDF 包

## 环境位置

所有文件都安装在用户目录下的 `.RapidPaperTrans` 目录中，避免程序目录权限问题：

```
~/.RapidPaperTrans/          # Windows: C:\Users\用户名\.RapidPaperTrans
├── .tools/
│   └── uv.exe               # uv 包管理器
└── .venv/
    └── Scripts/
        └── python.exe       # Python 解释器
```

## 优点

- **完全隔离**: 不影响系统 Python 环境，也不受系统环境影响
- **自动管理**: 用户无需手动安装任何东西
- **快速安装**: uv 比 pip 快 10-100 倍
- **小巧**: 只安装必要的包，不会占用太多空间

## 支持的平台

- Windows (x64, ARM64)
- macOS (Intel, Apple Silicon)
- Linux (x64, ARM64)

## 首次运行

首次使用 PDF 翻译功能时，程序会自动：

1. 下载 uv (~10MB)
2. 创建 Python 虚拟环境 (~50MB)
3. 安装 PyMuPDF (~30MB)

整个过程通常需要 1-2 分钟，取决于网络速度。

## 故障排除

如果遇到问题，可以尝试：

1. 删除 `.tools` 和 `.venv` 目录，让程序重新安装
2. 检查网络连接是否正常
3. 确保有足够的磁盘空间（至少 200MB）

## 开发者信息

Python 环境管理代码位于 `internal/python/` 目录：

- `env.go`: 环境管理核心逻辑
- `global.go`: 全局环境实例和便捷函数
- `env_test.go`: 单元测试

使用示例：

```go
import "latex-translator/internal/python"

// 确保环境已设置
err := python.EnsureGlobalEnv(func(msg string) {
    fmt.Println(msg)
})

// 运行 Python 脚本
output, err := python.RunScript("script.py", "arg1", "arg2")
```
