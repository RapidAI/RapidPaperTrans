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
	
	// 1. 确保所有 figure* 都用 \end{figure*} 结束
	// 查找 \begin{figure*} ... \end{figure} 的模式并修复
	figureStarPattern := regexp.MustCompile(`(\\begin\{figure\*\}[^\\]*(?:\\[^e][^\\]*)*?)\\end\{figure\}`)
	text = figureStarPattern.ReplaceAllString(text, `$1\end{figure*}`)
	
	// 2. 对于使用 \linewidth 的图片，如果在 figure* 环境中，改为 0.95\textwidth
	// 这样可以避免图片超出页面边界
	lines := strings.Split(text, "\n")
	inFigureStar := false
	
	for i, line := range lines {
		if strings.Contains(line, `\begin{figure*}`) {
			inFigureStar = true
		}
		
		if inFigureStar && strings.Contains(line, `\includegraphics[width=\linewidth]`) {
			// 在 figure* 环境中，使用 0.95\textwidth 而不是 \linewidth
			lines[i] = strings.Replace(line, `width=\linewidth`, `width=0.95\textwidth`, 1)
		}
		
		if strings.Contains(line, `\end{figure*}`) || strings.Contains(line, `\end{figure}`) {
			inFigureStar = false
		}
	}
	
	text = strings.Join(lines, "\n")
	
	// 3. 确保所有 figure 环境都有正确的位置参数
	// 将 [!t] 改为 [htbp] 以获得更好的浮动体放置
	text = strings.ReplaceAll(text, `\begin{figure*}[!t]`, `\begin{figure*}[!htbp]`)
	text = strings.ReplaceAll(text, `\begin{figure}[!t]`, `\begin{figure}[htbp]`)
	text = strings.ReplaceAll(text, `\begin{figure}[!h]`, `\begin{figure}[htbp]`)
	
	// 4. 对于单栏图片，确保宽度不超过 \columnwidth
	singleFigPattern := regexp.MustCompile(`(\\begin\{figure\}[^\}]*\}[^\\]*\\includegraphics\[width=)\\linewidth(\])`)
	text = singleFigPattern.ReplaceAllString(text, `${1}0.95\columnwidth${2}`)
	
	// 5. 添加 \centering 到所有没有它的 figure 环境
	// 这有助于图片居中显示
	figurePattern := regexp.MustCompile(`(\\begin\{figure[*]?\}[^\n]*\n)(\s*)(\\includegraphics)`)
	text = figurePattern.ReplaceAllStringFunc(text, func(match string) string {
		if !strings.Contains(match, `\centering`) {
			parts := figurePattern.FindStringSubmatch(match)
			if len(parts) >= 4 {
				return parts[1] + parts[2] + `\centering` + "\n" + parts[2] + parts[3]
			}
		}
		return match
	})
	
	// 写回文件
	outputFile := "testdata/arxiv_test/2601.22156_extracted/translated_example_paper_layout_fixed.tex"
	err = os.WriteFile(outputFile, []byte(text), 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("Layout fixes applied successfully!\n")
	fmt.Printf("Output written to: %s\n", outputFile)
	fmt.Printf("\nChanges made:\n")
	fmt.Printf("1. Fixed figure*/figure environment mismatches\n")
	fmt.Printf("2. Adjusted image widths in figure* environments to 0.95\\textwidth\n")
	fmt.Printf("3. Adjusted image widths in figure environments to 0.95\\columnwidth\n")
	fmt.Printf("4. Improved float placement parameters\n")
	fmt.Printf("5. Ensured all figures are centered\n")
	
	fmt.Printf("\nNext steps:\n")
	fmt.Printf("1. Copy the fixed file:\n")
	fmt.Printf("   Copy-Item %s testdata/arxiv_test/2601.22156_extracted/translated_example_paper_complete.tex -Force\n", outputFile)
	fmt.Printf("2. Recompile the PDF:\n")
	fmt.Printf("   cd testdata/arxiv_test/2601.22156_extracted\n")
	fmt.Printf("   lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
}
