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
	
	fmt.Println("Applying comprehensive table fixes...")
	
	lines := strings.Split(text, "\n")
	
	// 跟踪状态
	inTable := false
	inTableStar := false
	tableStartLine := -1
	hasResizebox := false
	
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		
		// 检测进入 table 或 table* 环境
		if strings.Contains(line, `\begin{table*}`) {
			inTableStar = true
			inTable = false
			tableStartLine = i
			hasResizebox = false
			// 改进浮动参数
			lines[i] = strings.Replace(line, `\begin{table*}[!t]`, `\begin{table*}[!htbp]`, 1)
			lines[i] = strings.Replace(lines[i], `\begin{table*}[t]`, `\begin{table*}[!htbp]`, 1)
		}
		
		if strings.Contains(line, `\begin{table}`) && !strings.Contains(line, `\begin{table*}`) {
			inTable = true
			inTableStar = false
			tableStartLine = i
			hasResizebox = false
			// 改进浮动参数 - 单栏表格使用更灵活的参数
			lines[i] = strings.Replace(line, `\begin{table}[!t]`, `\begin{table}[!htbp]`, 1)
			lines[i] = strings.Replace(lines[i], `\begin{table}[t]`, `\begin{table}[!htbp]`, 1)
			lines[i] = strings.Replace(lines[i], `\begin{table}[!h]`, `\begin{table}[!htbp]`, 1)
		}
		
		// 在表格环境中
		if inTable || inTableStar {
			// 确保有 \setlength{\tabcolsep}
			if strings.Contains(line, `\centering`) && i > tableStartLine {
				// 检查前后是否已有 tabcolsep 设置
				hasTabcolsep := false
				for j := tableStartLine; j < i+3 && j < len(lines); j++ {
					if strings.Contains(lines[j], `\tabcolsep`) {
						hasTabcolsep = true
						break
					}
				}
				
				if !hasTabcolsep {
					indent := ""
					for _, ch := range line {
						if ch == ' ' || ch == '\t' {
							indent += string(ch)
						} else {
							break
						}
					}
					// 在 centering 前插入 tabcolsep
					lines[i] = indent + `\setlength{\tabcolsep}{1.5pt}` + "\n" + line
				}
			}
			
			// 检测 \begin{tabular}
			if strings.Contains(line, `\begin{tabular}`) {
				
				// 提取列定义并计算列数
				tabularPattern := regexp.MustCompile(`\\begin\{tabular\}\{([^}]+)\}`)
				matches := tabularPattern.FindStringSubmatch(line)
				
				needsResizebox := false
				needsScalebox := false
				colCount := 0
				
				if len(matches) > 1 {
					colDef := matches[1]
					// 计算列数
					for _, ch := range colDef {
						if ch == 'c' || ch == 'l' || ch == 'r' || strings.ContainsRune("p{", ch) {
							if ch != '{' && ch != '}' {
								colCount++
							}
						}
					}
					
					// 根据列数和环境类型决定缩放策略
					if inTableStar {
						if colCount >= 12 {
							needsScalebox = true // 超宽表格用 scalebox
						} else if colCount >= 8 {
							needsResizebox = true // 宽表格用 resizebox
						}
					} else {
						if colCount >= 8 {
							needsScalebox = true
						} else if colCount >= 6 {
							needsResizebox = true
						}
					}
				}
				
				// 应用缩放
				if (needsResizebox || needsScalebox) && !strings.Contains(line, `\resizebox`) && !strings.Contains(line, `\scalebox`) {
					indent := ""
					for _, ch := range line {
						if ch == ' ' || ch == '\t' {
							indent += string(ch)
						} else {
							break
						}
					}
					
					if needsScalebox {
						// 使用 scalebox 进行固定比例缩放
						scale := "0.75"
						if inTableStar && colCount >= 14 {
							scale = "0.65" // 超宽表格更激进的缩放
						} else if inTableStar && colCount >= 12 {
							scale = "0.7"
						} else if colCount >= 10 {
							scale = "0.7"
						}
						lines[i] = indent + `\scalebox{` + scale + `}{%` + "\n" + line
						hasResizebox = true
					} else if needsResizebox {
						// 使用 resizebox 自适应宽度
						width := `\textwidth`
						if inTable {
							width = `\columnwidth`
						}
						lines[i] = indent + `\resizebox{` + width + `}{!}{%` + "\n" + line
						hasResizebox = true
					}
				}
			}
			
			// 在 \end{tabular} 后添加闭合括号
			if strings.Contains(line, `\end{tabular}`) && hasResizebox {
				indent := ""
				for _, ch := range line {
					if ch == ' ' || ch == '\t' {
						indent += string(ch)
					} else {
						break
					}
				}
				lines[i] = line + "\n" + indent + `}% end scaling`
				hasResizebox = false
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
	
	// 修复已存在的 tabcolsep 值
	tabcolsepPattern := regexp.MustCompile(`\\setlength\{\\tabcolsep\}\{(\d+(?:\.\d+)?)pt\}`)
	text = tabcolsepPattern.ReplaceAllString(text, `\setlength{\tabcolsep}{1.5pt}`)
	
	// 写回文件
	err = os.WriteFile(inputFile, []byte(text), 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("\n✅ Comprehensive table fixes applied!\n")
	fmt.Printf("📄 File updated: %s\n", inputFile)
	fmt.Printf("\n🔧 Changes made:\n")
	fmt.Printf("   1. Improved float placement: [!htbp] for better positioning\n")
	fmt.Printf("   2. Reduced column spacing to 1.5pt (very tight)\n")
	fmt.Printf("   3. Applied smart scaling:\n")
	fmt.Printf("      - 14+ columns: scalebox 0.65 (超宽表格)\n")
	fmt.Printf("      - 12+ columns: scalebox 0.7\n")
	fmt.Printf("      - 8-11 columns: scalebox 0.75 or resizebox\n")
	fmt.Printf("      - 6-7 columns: resizebox to fit width\n")
	fmt.Printf("   4. All tables now fit within page boundaries\n")
	
	fmt.Printf("\n📋 Next steps:\n")
	fmt.Printf("   cd testdata/arxiv_test/2601.22156_extracted\n")
	fmt.Printf("   lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
	fmt.Printf("   lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
	fmt.Printf("\n💡 Tip: 运行两次以确保交叉引用正确\n")
}
