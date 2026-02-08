package main

import (
	"fmt"
	"os"
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
	
	fmt.Println("Fixing Table 2 overlap issue...")
	
	// 策略：将 Table 2 改为 table* 环境（双栏宽度）
	// 这是最简单有效的方法，因为这个表格有 14 列，太宽了
	
	lines := strings.Split(text, "\n")
	inTable2 := false
	table2StartLine := -1
	table2EndLine := -1
	
	// 找到 Table 2 的位置
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		
		// 检测 Table 2 的开始 - 通过 caption 识别
		if strings.Contains(line, `\caption{`) && 
		   strings.Contains(line, `\ARCH + \DISTILL`) &&
		   strings.Contains(line, `长上下文召回性能对比`) {
			// 向上查找 \begin{table}
			for j := i; j >= 0 && j > i-10; j-- {
				if strings.Contains(lines[j], `\begin{table}`) && !strings.Contains(lines[j], `\begin{table*}`) {
					table2StartLine = j
					inTable2 = true
					fmt.Printf("Found Table 2 at line %d\n", j+1)
					break
				}
			}
		}
		
		// 找到 Table 2 的结束
		if inTable2 && strings.Contains(line, `\end{table}`) && !strings.Contains(line, `\end{table*}`) {
			table2EndLine = i
			fmt.Printf("Table 2 ends at line %d\n", i+1)
			break
		}
	}
	
	if table2StartLine == -1 || table2EndLine == -1 {
		fmt.Println("❌ Could not find Table 2!")
		return
	}
	
	// 修改 Table 2：将 table 改为 table*
	// 1. 修改开始标签
	lines[table2StartLine] = strings.Replace(lines[table2StartLine], `\begin{table}[!htbp]`, `\begin{table*}[!t]`, 1)
	lines[table2StartLine] = strings.Replace(lines[table2StartLine], `\begin{table}[!t]`, `\begin{table*}[!t]`, 1)
	lines[table2StartLine] = strings.Replace(lines[table2StartLine], `\begin{table}`, `\begin{table*}[!t]`, 1)
	
	// 2. 修改结束标签
	lines[table2EndLine] = strings.Replace(lines[table2EndLine], `\end{table}`, `\end{table*}`, 1)
	
	// 3. 确保有适当的缩放 - 查找 tabular 行
	for i := table2StartLine; i <= table2EndLine; i++ {
		if strings.Contains(lines[i], `\begin{tabular}`) {
			// 检查是否已经有缩放
			if !strings.Contains(lines[i-1], `\scalebox`) && !strings.Contains(lines[i-1], `\resizebox`) {
				indent := ""
				for _, ch := range lines[i] {
					if ch == ' ' || ch == '\t' {
						indent += string(ch)
					} else {
						break
					}
				}
				// 添加 scalebox 0.7 用于 14 列表格
				lines[i] = indent + `\scalebox{0.7}{%` + "\n" + lines[i]
				
				// 找到对应的 \end{tabular} 并添加闭合括号
				for j := i + 1; j <= table2EndLine; j++ {
					if strings.Contains(lines[j], `\end{tabular}`) {
						indent2 := ""
						for _, ch := range lines[j] {
							if ch == ' ' || ch == '\t' {
								indent2 += string(ch)
							} else {
								break
							}
						}
						lines[j] = lines[j] + "\n" + indent2 + `}% end scalebox`
						break
					}
				}
			}
			break
		}
	}
	
	text = strings.Join(lines, "\n")
	
	// 写回文件
	err = os.WriteFile(inputFile, []byte(text), 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}
	
	fmt.Printf("\n✅ Table 2 overlap issue fixed!\n")
	fmt.Printf("📄 File updated: %s\n", inputFile)
	fmt.Printf("\n🔧 Changes made:\n")
	fmt.Printf("   1. Changed Table 2 from single-column to double-column (table*)\n")
	fmt.Printf("   2. Applied scalebox 0.7 for better fit\n")
	fmt.Printf("   3. Table 2 will now span both columns and appear at top of page\n")
	fmt.Printf("   4. This prevents overlap with surrounding text\n")
	
	fmt.Printf("\n📋 Next steps:\n")
	fmt.Printf("   cd testdata/arxiv_test/2601.22156_extracted\n")
	fmt.Printf("   lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
	fmt.Printf("   lualatex -interaction=nonstopmode translated_example_paper_complete.tex\n")
	fmt.Printf("\n💡 Tip: table* 环境会让表格跨越两栏，避免与文字重叠\n")
}
