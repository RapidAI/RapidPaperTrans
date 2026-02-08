package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	// Read the test preamble
	content, err := os.ReadFile("test_preamble.txt")
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	preamble := string(content)
	fmt.Printf("Preamble length: %d\n", len(preamble))

	// Check if \bibliography{main} is in the preamble
	bibIdx := strings.Index(preamble, `\bibliography{main}`)
	fmt.Printf("\\bibliography{main} position: %d\n", bibIdx)

	if bibIdx != -1 {
		// Show context around the bibliography
		start := bibIdx - 50
		if start < 0 {
			start = 0
		}
		end := bibIdx + 50
		if end > len(preamble) {
			end = len(preamble)
		}
		fmt.Printf("Context: %q\n", preamble[start:end])
	}

	// Test the regex pattern
	pattern := regexp.MustCompile(`(?m)^([^%\n]*?)(\\bibliography\{([^}]+)\})`)
	matches := pattern.FindAllStringSubmatch(preamble, -1)
	fmt.Printf("Regex matches: %d\n", len(matches))
	for i, match := range matches {
		fmt.Printf("Match %d:\n", i)
		fmt.Printf("  Full: %q\n", match[0])
		fmt.Printf("  Group 1: %q\n", match[1])
		fmt.Printf("  Group 2: %q\n", match[2])
		if len(match) > 3 {
			fmt.Printf("  Group 3: %q\n", match[3])
		}
	}

	// Also test with a simpler pattern
	simplePattern := regexp.MustCompile(`\\bibliography\{[^}]+\}`)
	simpleMatches := simplePattern.FindAllString(preamble, -1)
	fmt.Printf("\nSimple pattern matches: %d\n", len(simpleMatches))
	for i, match := range simpleMatches {
		fmt.Printf("  Match %d: %q\n", i, match)
	}
}
