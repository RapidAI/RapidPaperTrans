package main

import (
	"fmt"
	"regexp"
)

func main() {
	s := `& \multicolumn{2}{|c}{\textbf{Overall} \\`
	
	// Print the actual bytes with characters
	fmt.Println("Input analysis:")
	for i, c := range s {
		fmt.Printf("  %d: %c (%d)\n", i, c, c)
	}
	
	// I see! The input has:
	// Position 38: } (125)
	// So the input is: \multicolumn{2}{|c}{\textbf{Overall} \\
	// The } at position 38 is the closing brace of \textbf{}
	// But wait, that means the input HAS the closing brace!
	
	// Let me re-read the problem. The issue is:
	// Original: \multicolumn{2}{|c}{\textbf{Overall}} \\
	// Translated: \multicolumn{2}{|c}{\textbf{Overall} \\
	// 
	// The translated is missing ONE closing brace (for \multicolumn)
	// The \textbf{Overall} has its closing brace, but \multicolumn{ is missing its }
	
	// So the pattern should be:
	// \multicolumn{...}{...}{\textbf{...}} followed by space and \\
	// But we have:
	// \multicolumn{...}{...}{\textbf{...} followed by space and \\
	// (missing one } before the space)
	
	// Let me create the correct pattern
	// We want to match: \multicolumn{N}{spec}{\textbf{text} followed by space and \\
	// And replace with: \multicolumn{N}{spec}{\textbf{text}} followed by space and \\
	
	p1 := regexp.MustCompile(`(\\multicolumn\{[^}]+\}\{[^}]+\}\{\\textbf\{[^}]+\})\s+(\\\\)`)
	fmt.Println("\nPattern 1 Match:", p1.MatchString(s))
	fmt.Println("Pattern 1 Find:", p1.FindString(s))
	fmt.Println("Pattern 1 Replace:", p1.ReplaceAllString(s, "$1} $2"))
}



