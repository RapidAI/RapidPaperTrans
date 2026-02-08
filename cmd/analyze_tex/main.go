// Command analyze_tex compares two LaTeX files to find structural differences
package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: analyze_tex <original.tex> <translated.tex>")
		os.Exit(1)
	}

	origPath := os.Args[1]
	transPath := os.Args[2]

	origContent, err := os.ReadFile(origPath)
	if err != nil {
		fmt.Printf("Error reading original: %v\n", err)
		os.Exit(1)
	}

	transContent, err := os.ReadFile(transPath)
	if err != nil {
		fmt.Printf("Error reading translated: %v\n", err)
		os.Exit(1)
	}

	orig := string(origContent)
	trans := string(transContent)

	fmt.Println("=== File Statistics ===")
	fmt.Printf("Original: %d lines, %d bytes\n", len(strings.Split(orig, "\n")), len(orig))
	fmt.Printf("Translated: %d lines, %d bytes\n", len(strings.Split(trans, "\n")), len(trans))

	fmt.Println("\n=== Structure Comparison ===")
	
	patterns := map[string]string{
		"section":       `\\section\{`,
		"subsection":    `\\subsection\{`,
		"subsubsection": `\\subsubsection\{`,
		"figure":        `\\begin\{figure`,
		"figure*":       `\\begin\{figure\*`,
		"table":         `\\begin\{table[^*]`,
		"table*":        `\\begin\{table\*`,
		"equation":      `\\begin\{equation`,
		"align":         `\\begin\{align`,
		"itemize":       `\\begin\{itemize`,
		"enumerate":     `\\begin\{enumerate`,
		"sidewaystable": `\\begin\{sidewaystable`,
	}

	for name, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		origCount := len(re.FindAllString(orig, -1))
		transCount := len(re.FindAllString(trans, -1))
		diff := ""
		if origCount != transCount {
			diff = fmt.Sprintf(" [DIFF: %+d]", transCount-origCount)
		}
		fmt.Printf("%-15s: orig=%2d, trans=%2d%s\n", name, origCount, transCount, diff)
	}

	// Check for commented environments
	fmt.Println("\n=== Commented Environments ===")
	commentedPatterns := map[string]string{
		"% \\begin{table":  `%\s*\\begin\{table`,
		"% \\begin{figure": `%\s*\\begin\{figure`,
		"% \\end{table":    `%\s*\\end\{table`,
		"% \\end{figure":   `%\s*\\end\{figure`,
	}

	for name, pattern := range commentedPatterns {
		re := regexp.MustCompile(pattern)
		origCount := len(re.FindAllString(orig, -1))
		transCount := len(re.FindAllString(trans, -1))
		if origCount > 0 || transCount > 0 {
			fmt.Printf("%-20s: orig=%2d, trans=%2d\n", name, origCount, transCount)
		}
	}

	// Extract and compare section titles
	fmt.Println("\n=== Section Titles ===")
	sectionRe := regexp.MustCompile(`\\section\{([^}]+)\}`)
	
	origSections := sectionRe.FindAllStringSubmatch(orig, -1)
	transSections := sectionRe.FindAllStringSubmatch(trans, -1)
	
	fmt.Printf("Original sections (%d):\n", len(origSections))
	for i, s := range origSections {
		fmt.Printf("  %d. %s\n", i+1, s[1])
	}
	
	fmt.Printf("\nTranslated sections (%d):\n", len(transSections))
	for i, s := range transSections {
		fmt.Printf("  %d. %s\n", i+1, s[1])
	}

	// Check for missing content by looking at unique patterns
	fmt.Println("\n=== Potential Missing Content ===")
	
	// Check for labels
	labelRe := regexp.MustCompile(`\\label\{([^}]+)\}`)
	origLabels := labelRe.FindAllStringSubmatch(orig, -1)
	transLabels := labelRe.FindAllStringSubmatch(trans, -1)
	
	origLabelSet := make(map[string]bool)
	for _, l := range origLabels {
		origLabelSet[l[1]] = true
	}
	
	transLabelSet := make(map[string]bool)
	for _, l := range transLabels {
		transLabelSet[l[1]] = true
	}
	
	missingLabels := []string{}
	for label := range origLabelSet {
		if !transLabelSet[label] {
			missingLabels = append(missingLabels, label)
		}
	}
	
	if len(missingLabels) > 0 {
		fmt.Printf("Missing labels (%d):\n", len(missingLabels))
		for _, l := range missingLabels {
			fmt.Printf("  - %s\n", l)
		}
	} else {
		fmt.Println("All labels present!")
	}
}
