# PDF 覆盖翻译设置指南

PDF 覆盖翻译功能可以将翻译后的中文文本精确覆盖在原始 PDF 的英文文本位置上，保持原生的视觉效果。

## 方案选择

系统支持两种覆盖方案，按优先级排序：

### 1. Python + PyMuPDF 方案（推荐）✨

**优点：**
- 精确的坐标定位
- 可靠的文本覆盖
- 更好的中文字体支持
- 跨平台兼容性好

**安装步骤：**

```bash
# 安装 Python（如果还没有）
# Windows: 从 python.org 下载安装
# macOS: brew install python
# Linux: sudo apt install python3

# 安装 PyMuPDF
pip install PyMuPDF

# 验证安装
python -c "import fitz; print('PyMuPDF installed successfully')"
```

### 2. LaTeX 方案（备用）

**优点：**
- 高质量的排版
- 适合学术文档

**缺点：**
- 坐标转换复杂
- 可能出现位置偏差
- 需要安装完整的 LaTeX 环境

**安装步骤：**

```bash
# Windows
# 下载并安装 MiKTeX: https://miktex.org/download

# macOS
brew install --cask mactex

# Linux
sudo apt install texlive-full
```

## 使用方法

### 测试覆盖效果

```bash
cd latex-translator/cmd/test_pdf_overlay
go run main.go ../../ICML_SX_2026_1_25.pdf
```

这会：
1. 解析 PDF 并提取文本
2. 模拟翻译前 10 个文本块
3. 生成覆盖后的 PDF 到 `test_output/overlay_test.pdf`

### 检查结果

打开生成的 PDF，检查：
- ✓ 白色矩形是否完全覆盖了原文
- ✓ 中文文本位置是否准确
- ✓ 字体大小是否合适
- ✓ 是否有文本溢出

## 常见问题

### Q: 提示 "Python script not found"

**A:** 确保 Python 脚本在正确的位置：
```
latex-translator/internal/pdf/overlay_pdf.py
```

### Q: 提示 "No module named 'fitz'"

**A:** 安装 PyMuPDF：
```bash
pip install PyMuPDF
```

### Q: 中文显示为方块或乱码

**A:** PyMuPDF 会尝试使用内置的中文字体。如果失败，可以：
1. 安装系统中文字体
2. 或者使用 LaTeX 方案（需要配置 ctex）

### Q: 文本位置不准确

**A:** 这通常是坐标系统转换的问题。Python 方案已经处理了：
- PDF 坐标系：原点在左下角，Y 轴向上
- PyMuPDF 坐标系：原点在左上角，Y 轴向下

如果仍有问题，请提供示例 PDF 以便调试。

### Q: 白色矩形盖不住原文

**A:** 检查：
1. 文本块的宽度和高度是否正确提取
2. 是否需要增加矩形的边距（当前是 ±2 像素）

可以在 `overlay_pdf.py` 中调整：
```python
cover_rect = fitz.Rect(
    rect.x0 - 5,  # 增加左边距
    rect.y0 - 5,  # 增加上边距
    rect.x1 + 5,  # 增加右边距
    rect.y1 + 5   # 增加下边距
)
```

## 调试技巧

### 1. 查看提取的文本块信息

```bash
cd latex-translator/cmd/test_pdf_overlay
go run main.go your_file.pdf
```

会显示每个文本块的：
- 位置 (X, Y)
- 尺寸 (Width, Height)
- 字体大小

### 2. 测试单页

修改 `test_pdf_overlay/main.go`，只处理第一页：
```go
// 只处理第一页的文本块
for _, block := range blocks {
    if block.Page == 1 {
        // ... 翻译逻辑
    }
}
```

### 3. 可视化调试

在 `overlay_pdf.py` 中添加调试矩形：
```python
# 画红色边框显示覆盖区域
page.draw_rect(cover_rect, color=(1, 0, 0), width=1)
```

## 性能优化

对于大型 PDF：
1. 使用批量翻译减少 API 调用
2. 启用翻译缓存
3. 考虑并发处理多页

## 下一步

- 集成到主应用的 UI
- 添加字体选择选项
- 支持自定义覆盖样式
- 添加预览功能
