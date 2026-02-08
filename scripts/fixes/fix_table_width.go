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
	
	fmt.Println("Fixing table width issues...")
	
	// 1. 为所有 table* 环境添加 \resizebox 来自动缩放表格
	// 这会确保表格不会超出页面宽度
	lines := strings.Split(text, "\n")
	inTableStar := false
	inTable := false
	tableStartLine := -1
	needsResizebox := false
	
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		
		// 检测进入 table* 环境
		if strings.Contains(line, `\begin{table*}`) {
			inTableStar = true
			inTable = false
			tableStartLine = i
			needsResizebox = false
		}
		
		// 检测进入 table 环境
		if strings.Contains(line, `\begin{table}`) && !strings.Contains(line, `\begin{table*}`) {
			inTable = true
			inTableStar = false
			tableStartLine = i
			needsResizebox = false
		}
		
		// 检测 tabular 环境，检查列数
		if (inTableStar || inTable) && strings.Contains(line, `\begin{tabular}`) {
			// 提取列定义
			tabularPattern := regexp.MustCompile(`\\begin\{tabular\}\{([^}]+)\}`)
			matches := tabularPattern.FindStringSubmatch(line)
			if len(matches) > 1 {
				colDef := matches[1]
				// 计算列数（简单计数 c, l, r, p 等）
				colCount := 0
				for _, ch := range colDef {
					if ch == 'c' || ch == 'l' || ch == 'r' || ch == 'p' {
						colCount++
					}
				}
				
				// 如果列数超过 8，或者在 table* 环境中列数超过 6，需要缩放
				if (inTableStar && colCount > 6) || (inTable && colCount > 8) {
					needsResizebox = true
				}
			}
		}
		
		// 在 \begin{tabular} 前插入 \resizebox
		if needsResizebox && strings.Contains(line, `\begin{tabular}`) && !strings.Contains(line, `\resizebox`) {
			indent := ""
			for _, ch := range line {
				if ch == ' ' || ch == '\t' {
					indent += string(ch)
				} else {
					break
				}
			}
			
			// 根据环境类型选择宽度
			width := `\textwidth`
			if inTable {
				width = `\columnwidth`
			}
			
			// 插入 \resizebox
			lines[i] = indent + `\resizebox{` + width + `}{!}{%` + "\n" + line
			needsResizebox = false // 标记已处理
		}
		
		// 在 \end{tabular} 后添加闭合括号
		if (inTableStar || inTable) && strings.Contains(line, `\end{tabular}`) {
			// 检查是否已经有 resizebox
			foundResizebox := false
			for j := i - 1; j >= tableStartLine && j >= 0; j-- {
				if strings.Contains(lines[j], `\resizebox`) {
					foundResizebox = true
					break
				}
				if strings.Contains(lines[j], `\begin{tabular}`) {
					break
				}
			}
			
			if foundResizebox {
				lines[i] = line + "\n" + strings.Repeat(" ", len(line)-len(strings.TrimLeft(line, " \t"))) + `}% end resizebox`
			}
		}
		
		// 检测离开 table 环境
		if strings.Contains(line, `\end{table*}`) {
			inTableStar = false
			tableStartLine = -1
		}
		if strings.Contains(line, `\end{table}`) && !strings.Contains(line, `\end{table*}`) {
			inTable = false
			tableStartLine = -1
		}
	}
	
	text = strings.Join(lines, "\n")
	
	// 2. 减小表格列间距
	// 在每个 table 环境开始后添加 \setlength{\tabcolsep}{2pt}
	tablePattern := regexp.MustCompile(`(\\begin\{table\*?\}[^\n]*\n)(\s*)(\\centering)`)
	text = tablePattern.ReplaceAllString(text, `$1$2\setlength{\tabcolsep}{2pt}$2$3`)
	
	// 3. 对于已经有 \setlength{\tabcolsep} 的，确保值不超过 3pt
	tabcolsepPattern := regexp.MustCompile(`\\setlength\{\\tabcolsep\}\{(\d+)pt\}`)
	text = tabcolsepPattern.ReplaceAllStringFunc(text, func(match string) string {
		matches := tabcolsepPattern.FindStringSubmatch(match)
		if len(matches) > 1 {
			// 如果值大于 2，改为 2
			return `\setlength{\tabcolsep}{2pt}`
		}
		return match
	})
	
	// 4. 为宽表格添加 \small 或 \footnotesize
	// 查找没有字体大小设置的表格
	lines = strings.Split(text, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		
		if strings.Contains(line, `\begin{table*}`) || strings.Contains(line, `\begin{table}`) {
			// 检查接下来几行是否有字体大小设置
			hasFontSize := false
			for j := i + 1; j < i + 5 && j < len(lines); j++ {
				if strings.Contains(lines[j], `\footnotesize`) || 
				   strings.Contains(lines[j], `\small`) || 
				   strings.Contains(lines[j], `\tiny`) {
					hasFontSize = true
					break
				}
				if strings.Contains(lines[j], `\begin{tabular}`) {
					break
				}
			}
			
			// 如果没有字体大小设置，在 \centering 后添加 \small
			if !hasFontSize {
				for j := i + 1; j < i + 5 && j < len(lines); j++ {
					if strings.Contains(lines[j], `\centering`) {
						indent := ""
						for _, ch := range lines[j] {
							if ch == ' ' || ch == '\t' {
								indent += string(ch)
							} else {
								break
							}
						}
						lines[j] = lines[j] + "\n" + indent + `\small`
						break
					}
				}
			}
		}
	}
	
	text = strings.Join(lines, "\n")
	
	// 写回文件
	err = os.WriteFile(inputFile, []byte(text), 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("\n✅ Table width fixes applied successfully!\n")
	fmt.Printf("📄 File updated: %s\n", inputFile)
	fmt.Printf("\n🔧 Changes made:\n")
	fmt.Printf("   1. Added \\resizebox to wide tables (auto-scale to page width)\n")
	fmt.Printf("   2. Reduced column spacing (\\tabcolsep) to 2pt\n")
	fmt.Printf("   3. Added \\small font size to tables without size specification\n")
	fmt.Printf("   4. Ensured tables fit within page boundaries\n")
	
	fmt.Printf("\n📋 Next step - Recompile:\n")
	fmt.Printf("   cd testdata/arxiv_test/2601.22156_extracted\n")
	fmt.Printf("   lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
	fmt.Printf("   lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
}
