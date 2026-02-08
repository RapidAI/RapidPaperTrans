package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	inputFile := "testdata/arxiv_test/2601.22156_extracted/translated_example_paper_complete.tex"
	
	// 先恢复原始文件
	originalFile := "testdata/arxiv_test/2601.22156_extracted/translated_example_paper_layout_fixed.tex"
	content, err := os.ReadFile(originalFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}
	
	text := string(content)
	
	fmt.Println("Applying simple but effective figure layout fixes...")
	
	// 1. 修复所有 figure* 环境的结束标签
	figureStarPattern := regexp.MustCompile(`(\\begin\{figure\*\}[^\\]*(?:\\[^e][^\\]*)*?)\\end\{figure\}`)
	text = figureStarPattern.ReplaceAllString(text, `$1\end{figure*}`)
	
	// 2. 修复所有 table* 环境的结束标签
	tableStarPattern := regexp.MustCompile(`(\\begin\{table\*\}[^\\]*(?:\\[^e][^\\]*)*?)\\end\{table\}`)
	text = tableStarPattern.ReplaceAllString(text, `$1\end{table*}`)
	
	// 3. 处理图片宽度 - 使用更保守的宽度
	lines := strings.Split(text, "\n")
	inFigureStar := false
	inFigure := false
	
	for i, line := range lines {
		// 检测进入 figure* 环境
		if strings.Contains(line, `\begin{figure*}`) {
			inFigureStar = true
			inFigure = false
		}
		
		// 检测进入 figure 环境
		if strings.Contains(line, `\begin{figure}`) && !strings.Contains(line, `\begin{figure*}`) {
			inFigure = true
			inFigureStar = false
		}
		
		// 在 figure* 环境中调整图片宽度 - 使用 0.85\textwidth 更保守
		if inFigureStar && strings.Contains(line, `\includegraphics`) {
			if strings.Contains(line, `width=0.9\textwidth`) {
				lines[i] = strings.Replace(line, `width=0.9\textwidth`, `width=0.85\textwidth`, 1)
			} else if strings.Contains(line, `width=\linewidth`) {
				lines[i] = strings.Replace(line, `width=\linewidth`, `width=0.85\textwidth`, 1)
			}
		}
		
		// 在 figure 环境中调整图片宽度
		if inFigure && strings.Contains(line, `\includegraphics`) {
			if strings.Contains(line, `width=\linewidth`) {
				lines[i] = strings.Replace(line, `width=\linewidth`, `width=0.9\columnwidth`, 1)
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
	
	// 写回文件
	err = os.WriteFile(inputFile, []byte(text), 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("\n✅ Simple layout fixes applied successfully!\n")
	fmt.Printf("📄 File updated: %s\n", inputFile)
	fmt.Printf("\n🔧 Changes made:\n")
	fmt.Printf("   1. Fixed all figure*/figure environment mismatches\n")
	fmt.Printf("   2. Fixed all table*/table environment mismatches\n")
	fmt.Printf("   3. Adjusted image widths:\n")
	fmt.Printf("      - figure*: 0.85\\textwidth (更保守，避免超出边界)\n")
	fmt.Printf("      - figure:  0.9\\columnwidth\n")
	
	fmt.Printf("\n📋 Next step - Recompile:\n")
	fmt.Printf("   cd testdata/arxiv_test/2601.22156_extracted\n")
	fmt.Printf("   lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
}
