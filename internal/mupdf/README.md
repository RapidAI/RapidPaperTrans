# MuPDF Go Binding

这是一个 MuPDF 的 Go 封装，提供 PDF 读取和写入功能。

## 功能

- PDF 文档读取
- 文本提取（带位置信息）
- 结构化文本块提取（类似 BabelDOC）
- PDF 文本写入/覆盖
- 白色矩形覆盖（用于遮盖原文）
- 页面尺寸获取

## API

### Context
```go
ctx, err := mupdf.NewContext()
defer ctx.Close()
```

### 读取文档
```go
// 打开任意文档（PDF, XPS, EPUB 等）
doc, err := ctx.OpenDocument("file.pdf")
defer doc.Close()

// 获取页数
pageCount := doc.PageCount()

// 获取页面尺寸
x0, y0, x1, y1 := doc.PageBounds(0)

// 提取文本
text, err := doc.ExtractText(0)

// 提取结构化文本块（带位置信息）
blocks, err := doc.ExtractTextBlocks(0)
for _, block := range blocks {
    fmt.Printf("Text: %s, Pos: (%.1f, %.1f), Size: %.1f x %.1f\n",
        block.Text, block.X, block.Y, block.Width, block.Height)
}
```

### 修改 PDF
```go
// 打开 PDF 文档（支持写入）
doc, err := ctx.OpenPDFDocument("file.pdf")
defer doc.Close()

// 添加文本
err = doc.AddText(pageNum, "Hello", x, y, fontSize, fontName)

// 添加矩形（用于覆盖原文）
err = doc.AddRect(pageNum, x, y, width, height, r, g, b)

// 覆盖原文并添加翻译（组合操作）
err = doc.CoverAndAddText(pageNum, x, y, width, height, "翻译文本", fontSize)

// 保存
err = doc.Save("output.pdf")
```

## 依赖

需要安装 MuPDF 开发库：

### Windows (MSYS2/MinGW)
```bash
pacman -S mingw-w64-x86_64-mupdf
```

### macOS
```bash
brew install mupdf
```

### Ubuntu/Debian
```bash
sudo apt-get install libmupdf-dev
```

### Fedora/RHEL
```bash
sudo dnf install mupdf-devel
```

## 构建

```bash
# 仅在安装了 MuPDF 的情况下构建
go build -tags mupdf ./...

# 运行测试
go test -tags mupdf ./internal/mupdf/...
```

## 与 BabelDocTranslator 集成

MuPDF binding 可以替代 pdfcpu 用于更精确的文本提取和覆盖：

```go
// 使用 MuPDF 提取文本块
ctx, _ := mupdf.NewContext()
defer ctx.Close()

doc, _ := ctx.OpenDocument("paper.pdf")
defer doc.Close()

for page := 0; page < doc.PageCount(); page++ {
    blocks, _ := doc.ExtractTextBlocks(page)
    // 处理每个文本块...
}

// 使用 MuPDF 生成翻译后的 PDF
pdfDoc, _ := ctx.OpenPDFDocument("paper.pdf")
defer pdfDoc.Close()

for _, block := range translatedBlocks {
    pdfDoc.CoverAndAddText(block.Page, block.X, block.Y, 
        block.Width, block.Height, block.TranslatedText, block.FontSize)
}
pdfDoc.Save("translated.pdf")
```

## 许可证

MuPDF 使用 AGPL-3.0 许可证。如需商业使用（闭源项目），请联系 Artifex Software 获取商业许可。

## 限制

- 当前实现使用 Base14 字体，中文字体支持需要额外工作
- CGO 依赖，需要 C 编译器
- 跨平台编译需要对应平台的 MuPDF 库
