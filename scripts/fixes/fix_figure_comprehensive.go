package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	inputFile := "testdata/arxiv_test/2601.22156_extracted/translated_example_paper_complete.tex"
	
	content, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}
	
	text := string(content)
	
	fmt.Println("Applying comprehensive figure layout fixes...")
	
	// 1. 在导言区添加更好的图片控制包
	preambleAddition := `
% 图片布局优化设置
\usepackage{placeins}  % 提供 \FloatBarrier 命令
\setlength{\textfloatsep}{10pt plus 2pt minus 2pt}  % 减小图片与文本的间距
\setlength{\floatsep}{10pt plus 2pt minus 2pt}
\setlength{\intextsep}{10pt plus 2pt minus 2pt}

% 设置浮动体参数以获得更好的布局
\renewcommand{\topfraction}{0.9}       % 页面顶部最多90%可以是浮动体
\renewcommand{\bottomfraction}{0.8}    % 页面底部最多80%可以是浮动体
\renewcommand{\textfraction}{0.1}      % 页面至少10%必须是文本
\renewcommand{\floatpagefraction}{0.7} % 浮动页至少70%必须是浮动体
`
	
	// 在 \begin{document} 之前插入
	docBeginPattern := regexp.MustCompile(`(\\begin\{document\})`)
	text = docBeginPattern.ReplaceAllString(text, preambleAddition+"\n$1")
	
	// 2. 修复所有 figure* 环境的结束标签
	figureStarPattern := regexp.MustCompile(`(\\begin\{figure\*\}[^\\]*(?:\\[^e][^\\]*)*?)\\end\{figure\}`)
	text = figureStarPattern.ReplaceAllString(text, `$1\end{figure*}`)
	
	// 3. 处理图片宽度
	lines := strings.Split(text, "\n")
	inFigureStar := false
	inFigure := false
	
	for i, line := range lines {
		// 检测进入 figure* 环境
		if strings.Contains(line, `\begin{figure*}`) {
			inFigureStar = true
			inFigure = false
			// 改进位置参数
			lines[i] = strings.Replace(line, `\begin{figure*}[!t]`, `\begin{figure*}[!htbp]`, 1)
			lines[i] = strings.Replace(lines[i], `\begin{figure*}[t]`, `\begin{figure*}[!htbp]`, 1)
			if !strings.Contains(lines[i], `[`) {
				lines[i] = strings.Replace(lines[i], `\begin{figure*}`, `\begin{figure*}[!htbp]`, 1)
			}
		}
		
		// 检测进入 figure 环境
		if strings.Contains(line, `\begin{figure}`) && !strings.Contains(line, `\begin{figure*}`) {
			inFigure = true
			inFigureStar = false
			// 改进位置参数
			lines[i] = strings.Replace(line, `\begin{figure}[!t]`, `\begin{figure}[htbp]`, 1)
			lines[i] = strings.Replace(lines[i], `\begin{figure}[!h]`, `\begin{figure}[htbp]`, 1)
			lines[i] = strings.Replace(lines[i], `\begin{figure}[t]`, `\begin{figure}[htbp]`, 1)
			if !strings.Contains(lines[i], `[`) {
				lines[i] = strings.Replace(lines[i], `\begin{figure}`, `\begin{figure}[htbp]`, 1)
			}
		}
		
		// 在 figure* 环境中调整图片宽度
		if inFigureStar && strings.Contains(line, `\includegraphics`) {
			// 使用 0.9\textwidth 而不是 \linewidth，避免超出边界
			if strings.Contains(line, `width=\linewidth`) {
				lines[i] = strings.Replace(line, `width=\linewidth`, `width=0.9\textwidth`, 1)
			} else if strings.Contains(line, `width=0.9\linewidth`) {
				lines[i] = strings.Replace(line, `width=0.9\linewidth`, `width=0.85\textwidth`, 1)
			} else if !strings.Contains(line, `width=`) {
				// 如果没有指定宽度，添加一个
				lines[i] = strings.Replace(line, `\includegraphics{`, `\includegraphics[width=0.9\textwidth]{`, 1)
				lines[i] = strings.Replace(lines[i], `\includegraphics[`, `\includegraphics[width=0.9\textwidth,`, 1)
			}
		}
		
		// 在 figure 环境中调整图片宽度
		if inFigure && strings.Contains(line, `\includegraphics`) {
			// 使用 0.95\columnwidth 确保不超出单栏宽度
			if strings.Contains(line, `width=\linewidth`) {
				lines[i] = strings.Replace(line, `width=\linewidth`, `width=0.95\columnwidth`, 1)
			} else if !strings.Contains(line, `width=`) {
				lines[i] = strings.Replace(line, `\includegraphics{`, `\includegraphics[width=0.95\columnwidth]{`, 1)
				lines[i] = strings.Replace(lines[i], `\includegraphics[`, `\includegraphics[width=0.95\columnwidth,`, 1)
			}
		}
		
		// 确保每个 figure 环境都有 \centering
		if (inFigureStar || inFigure) && strings.Contains(line, `\includegraphics`) {
			// 检查前一行是否有 \centering
			if i > 0 && !strings.Contains(lines[i-1], `\centering`) {
				// 在 includegraphics 前插入 \centering
				indent := ""
				for _, ch := range line {
					if ch == ' ' || ch == '\t' {
						indent += string(ch)
					} else {
						break
					}
				}
				lines[i] = indent + `\centering` + "\n" + line
			}
		}
		
		// 检测离开 figure 环境
		if strings.Contains(line, `\end{figure*}`) {
			inFigureStar = false
		}
		if strings.Contains(line, `\end{figure}`) && !strings.Contains(line, `\end{figure*}`) {
			inFigure = false
		}
	}
	
	text = strings.Join(lines, "\n")
	
	// 4. 在每个 section 后添加 \FloatBarrier，防止图片漂移太远
	sectionPattern := regexp.MustCompile(`(\\section\{[^}]+\})`)
	text = sectionPattern.ReplaceAllString(text, "$1\n\\FloatBarrier")
	
	// 5. 修复可能的表格宽度问题
	// 确保 table* 环境也有正确的结束标签
	tableStarPattern := regexp.MustCompile(`(\\begin\{table\*\}[^\\]*(?:\\[^e][^\\]*)*?)\\end\{table\}`)
	text = tableStarPattern.ReplaceAllString(text, `$1\end{table*}`)
	
	// 写回文件
	outputFile := "testdata/arxiv_test/2601.22156_extracted/translated_example_paper_fixed_layout.tex"
	err = os.WriteFile(outputFile, []byte(text), 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("\n✅ Comprehensive layout fixes applied successfully!\n")
	fmt.Printf("📄 Output written to: %s\n", outputFile)
	fmt.Printf("\n🔧 Changes made:\n")
	fmt.Printf("   1. Added placeins package for better float control\n")
	fmt.Printf("   2. Optimized float spacing parameters\n")
	fmt.Printf("   3. Fixed all figure*/figure environment mismatches\n")
	fmt.Printf("   4. Adjusted image widths:\n")
	fmt.Printf("      - figure*: 0.9\\textwidth (双栏宽度)\n")
	fmt.Printf("      - figure:  0.95\\columnwidth (单栏宽度)\n")
	fmt.Printf("   5. Improved float placement parameters [!htbp]\n")
	fmt.Printf("   6. Ensured all figures are centered\n")
	fmt.Printf("   7. Added \\FloatBarrier after sections\n")
	fmt.Printf("   8. Fixed table* environment mismatches\n")
	
	fmt.Printf("\n📋 Next steps:\n")
	fmt.Printf("   1. Copy the fixed file:\n")
	fmt.Printf("      Copy-Item %s testdata/arxiv_test/2601.22156_extracted/translated_example_paper_complete.tex -Force\n", outputFile)
	fmt.Printf("\n   2. Recompile the PDF (run twice for cross-references):\n")
	fmt.Printf("      cd testdata/arxiv_test/2601.22156_extracted\n")
	fmt.Printf("      lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
	fmt.Printf("      lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
	fmt.Printf("\n   3. Check the PDF for:\n")
	fmt.Printf("      - 图片是否在页面边界内\n")
	fmt.Printf("      - 图片与文字是否有适当间距\n")
	fmt.Printf("      - 图片是否居中显示\n")
}
