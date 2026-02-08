// Command test_fix tests the reference-based fix logic
package main

import (
	"fmt"
	"os"

	"latex-translator/internal/translator"
)

func main() {
	// Test case 1: Line-aligned errors (same line count)
	fmt.Println("========== TEST 1: Line-aligned errors ==========")
	testLineAligned()
	
	// Test case 2: Structural errors (different line count)
	fmt.Println("\n========== TEST 2: Structural errors ==========")
	testStructuralErrors()
}

func testLineAligned() {
	original := `\begin{document}
\maketitle
\begin{figure*}[htbp]
\includegraphics{test.png}
\caption{Test caption}
\end{figure*}
\textit{Vanilla} & 89.6
% This is a comment
\end{document}`

	// Same line count, but with errors on specific lines
	translated := `\begin{document}
\maketitle
% \begin{figure*}[htbp]
\includegraphics{test.png}
\caption{测试标题}
% \end{figure*}
\textit{Vanilla}} & 89.6
This should be a comment
\end{document}`

	fmt.Println("Original:")
	fmt.Println(original)
	fmt.Println()
	fmt.Println("Translated (with errors):")
	fmt.Println(translated)
	fmt.Println()

	// Test line-by-line fix
	fmt.Println("Line-by-Line Fix Result:")
	lineResult := translator.FixByLineComparison(translated, original)
	fmt.Printf("Fix count: %d\n", lineResult.FixCount)
	for _, detail := range lineResult.FixDetails {
		fmt.Printf("  - %s\n", detail)
	}
	fmt.Println()

	// Test full reference-based fix
	result := translator.ApplyReferenceBasedFixes(translated, original)

	fmt.Println("Final Fixed Result:")
	fmt.Println(result)
	fmt.Println()

	// Check if fixes were applied
	issues := []string{}
	
	if contains(result, "% \\begin{figure*}") {
		issues = append(issues, "Still has commented \\begin{figure*}")
	}
	if contains(result, "% \\end{figure*}") {
		issues = append(issues, "Still has commented \\end{figure*}")
	}
	if count(result, "\\end{document}") > 1 {
		issues = append(issues, fmt.Sprintf("Still has %d \\end{document}", count(result, "\\end{document}")))
	}
	if contains(result, "\\textit{Vanilla}}") {
		issues = append(issues, "Still has extra closing brace in \\textit{}")
	}
	// Check that comment was preserved
	if !contains(result, "% This") && !contains(result, "%This") {
		issues = append(issues, "Missing comment marker on line 8")
	}

	if len(issues) > 0 {
		fmt.Println("ISSUES FOUND:")
		for _, issue := range issues {
			fmt.Println("- " + issue)
		}
		os.Exit(1)
	} else {
		fmt.Println("TEST 1 PASSED")
	}
}

func testStructuralErrors() {
	// This test has different line counts - tests the non-line-by-line fixes
	original := `\begin{document}
\maketitle
\begin{figure*}[htbp]
\includegraphics{test.png}
\end{figure*}
\end{document}`

	// LLM added extra \end{document} after \maketitle
	translated := `\begin{document}
\maketitle
\end{document}
% \begin{figure*}[htbp]
\includegraphics{test.png}
% \end{figure*}
\end{document}`

	fmt.Println("Original:")
	fmt.Println(original)
	fmt.Println()
	fmt.Println("Translated (with structural errors):")
	fmt.Println(translated)
	fmt.Println()

	result := translator.ApplyReferenceBasedFixes(translated, original)

	fmt.Println("Final Fixed Result:")
	fmt.Println(result)
	fmt.Println()

	issues := []string{}
	
	// Should have only 1 \end{document}
	if count(result, "\\end{document}") > 1 {
		issues = append(issues, fmt.Sprintf("Still has %d \\end{document}", count(result, "\\end{document}")))
	}
	// Should have uncommented figure environment
	if contains(result, "% \\begin{figure*}") {
		issues = append(issues, "Still has commented \\begin{figure*}")
	}
	if contains(result, "% \\end{figure*}") {
		issues = append(issues, "Still has commented \\end{figure*}")
	}

	if len(issues) > 0 {
		fmt.Println("ISSUES FOUND:")
		for _, issue := range issues {
			fmt.Println("- " + issue)
		}
		os.Exit(1)
	} else {
		fmt.Println("TEST 2 PASSED")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func count(s, substr string) int {
	n := 0
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			n++
		}
	}
	return n
}
