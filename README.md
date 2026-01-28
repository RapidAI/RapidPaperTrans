# LaTeX Translator

将英文 LaTeX 文档翻译成中文并自动编译生成 PDF 的桌面应用程序。

## 功能特性

- 📥 **多种源码获取方式**：支持 arXiv URL、arXiv ID 或本地 zip 文件导入
- 🔄 **智能翻译**：使用 OpenAI API 将英文内容翻译为中文，同时保留 LaTeX 命令和数学公式
- 📝 **语法检测与修正**：自动检测翻译后的 LaTeX 语法错误并修正
- 📄 **自动编译**：自动编译原始和翻译后的文档生成 PDF
- 👀 **双栏预览**：左右对比显示原始英文 PDF 和翻译后的中文 PDF
- 💻 **命令行支持**：支持通过命令行参数启动并自动处理
- 🔍 **GitHub 搜索**：根据 arXiv ID 搜索已分享的翻译并下载
- 📤 **GitHub 分享**：将翻译结果上传到 GitHub 仓库与他人分享

## 系统要求

### 必需依赖

- **Go 1.21+**：用于编译后端代码
- **Node.js 18+**：用于前端构建
- **Wails CLI v2**：用于构建桌面应用
- **LaTeX 发行版**（以下任选其一）：
  - [TeX Live](https://www.tug.org/texlive/)（推荐，跨平台）
  - [MiKTeX](https://miktex.org/)（Windows）
  - [MacTeX](https://www.tug.org/mactex/)（macOS）
- **OpenAI API 密钥**：用于翻译功能

### 安装 Wails CLI

```bash
go install github.com/wailsapp/wails/v2/cmd/wails@latest
```

### 验证 LaTeX 安装

```bash
# 检查 pdflatex
pdflatex --version

# 检查 xelatex（中文编译需要）
xelatex --version
```

## 安装

### 克隆项目

```bash
git clone <repository-url>
cd latex-translator
```

### 安装依赖

```bash
# 安装前端依赖
cd frontend
npm install
cd ..
```

## 配置

### OpenAI API 密钥配置

有两种方式配置 API 密钥：

#### 方式一：环境变量（推荐）

```bash
# Linux/macOS
export OPENAI_API_KEY="your-api-key-here"

# Windows (PowerShell)
$env:OPENAI_API_KEY="your-api-key-here"

# Windows (CMD)
set OPENAI_API_KEY=your-api-key-here
```

#### 方式二：配置文件

配置文件位置：`~/.config/latex-translator/latex-translator-config.json`

```json
{
  "openai_api_key": "your-api-key-here",
  "openai_model": "gpt-4",
  "default_compiler": "pdflatex",
  "work_directory": ""
}
```

### 配置选项说明

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `openai_api_key` | OpenAI API 密钥 | 空（必须配置） |
| `openai_model` | 使用的 OpenAI 模型 | `gpt-4` |
| `default_compiler` | 默认 LaTeX 编译器 | `pdflatex` |
| `work_directory` | 工作目录 | 系统临时目录 |

> **注意**：环境变量 `OPENAI_API_KEY` 的优先级高于配置文件。

## 使用方法

### GitHub 分享和搜索

详细的 GitHub 分享和搜索功能说明，请参阅 [GITHUB_SHARING.md](GITHUB_SHARING.md)。

**快速开始**：
1. 在主界面顶部搜索框输入 arXiv ID，搜索已翻译的论文
2. 翻译完成后，点击"分享到 GitHub"按钮上传结果
3. 在设置中配置自己的 GitHub 仓库和 Token

### GUI 模式

#### 开发模式

```bash
wails dev
```

这将启动开发服务器，支持热重载。

#### 构建生产版本

```bash
wails build
```

构建完成后，可执行文件位于 `build/bin/` 目录。

### 命令行模式

```bash
# 显示帮助信息
./latex-translator --help

# 通过 arXiv URL 处理
./latex-translator --url https://arxiv.org/abs/2301.00001

# 通过 arXiv ID 处理
./latex-translator --id 2301.00001

# 处理本地 zip 文件
./latex-translator --file /path/to/paper.zip
```

### 命令行参数

| 参数 | 说明 | 示例 |
|------|------|------|
| `--url` | arXiv 论文 URL | `--url https://arxiv.org/abs/2301.00001` |
| `--id` | arXiv 论文 ID | `--id 2301.00001` 或 `--id hep-th/9901001` |
| `--file` | 本地 zip 文件路径 | `--file /path/to/paper.zip` |
| `-h, --help` | 显示帮助信息 | |

> **注意**：只能同时指定一个输入源。

## 开发

### 项目结构

```
latex-translator/
├── app.go                 # Wails 应用主控制器
├── main.go                # 程序入口，命令行解析
├── internal/
│   ├── compiler/          # LaTeX 编译器封装
│   ├── config/            # 配置管理
│   ├── downloader/        # arXiv 源码下载
│   ├── logger/            # 日志模块
│   ├── parser/            # 输入解析
│   ├── translator/        # OpenAI 翻译引擎
│   ├── types/             # 数据类型定义
│   └── validator/         # LaTeX 语法验证
├── frontend/
│   ├── src/               # 前端源码
│   └── index.html         # 入口页面
├── build/                 # 构建配置和输出
└── wails.json             # Wails 配置文件
```

### 运行测试

```bash
# 运行所有测试
go test ./...

# 运行特定包的测试
go test ./internal/compiler/...
go test ./internal/config/...

# 运行测试并显示详细输出
go test -v ./...
```

### 代码风格

```bash
# 格式化代码
go fmt ./...

# 静态检查
go vet ./...
```

## 工作流程

1. **源码获取**：从 arXiv 下载或导入本地 zip 文件
2. **解压提取**：解压源码并识别主 tex 文件
3. **原始编译**：编译原始英文文档生成 PDF
4. **翻译处理**：调用 OpenAI API 翻译文档内容
5. **语法修正**：检测并修正翻译后的 LaTeX 语法错误
6. **中文编译**：使用 xelatex 编译中文文档生成 PDF
7. **双栏预览**：在界面中同时显示原始和翻译后的 PDF

## 常见问题

### Q: arXiv 下载失败，提示"论文源码不可用"？

并非所有 arXiv 论文都提供 LaTeX 源码。有些作者只上传 PDF 文件。

**检查论文是否有源码：**

```bash
# 使用检查工具
cd latex-translator
go run cmd/check_arxiv_source/main.go 2501.17160

# 或手动访问
https://arxiv.org/e-print/ARXIV_ID
```

**解决方案：**
- 如果论文有源码：正常使用 arXiv ID 下载
- 如果论文无源码：下载 PDF 文件，使用 PDF 翻译功能

详细说明请参考：[arXiv 下载问题文档](docs/ARXIV_DOWNLOAD_ISSUE.md)

### Q: 编译中文文档失败？

确保安装了支持中文的 LaTeX 包：
- TeX Live：`sudo tlmgr install ctex`
- MiKTeX：通过 MiKTeX Console 安装 `ctex` 包

### Q: API 调用失败？

1. 检查 API 密钥是否正确配置
2. 检查网络连接
3. 确认 API 配额是否充足

### Q: 找不到主 tex 文件？

确保 zip 文件中包含带有 `\documentclass` 命令的 tex 文件。

## 许可证

MIT License
