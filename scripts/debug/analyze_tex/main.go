package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

func main() {
	orig, _ := os.ReadFile("D:\\RapidPaperTrans\\testdata\\latex\\arxiv.tex")
	trans, _ := os.ReadFile("D:\\RapidPaperTrans\\testdata\\latex\\translated_arxiv.tex")

	fmt.Printf("原始文件大小: %d bytes\n", len(orig))
	fmt.Printf("翻译文件大小: %d bytes\n", len(trans))

	sectionRe := regexp.MustCompile(`\\section\{`)
	subsectionRe := regexp.MustCompile(`\\subsection\{`)
	figureRe := regexp.MustCompile(`\\begin\{figure`)
	tableRe := regexp.MustCompile(`\\begin\{table`)

	fmt.Printf("\n原始文件:\n")
	fmt.Printf("  sections: %d\n", len(sectionRe.FindAllString(string(orig), -1)))
	fmt.Printf("  subsections: %d\n", len(subsectionRe.FindAllString(string(orig), -1)))
	fmt.Printf("  figures: %d\n", len(figureRe.FindAllString(string(orig), -1)))
	fmt.Printf("  tables: %d\n", len(tableRe.FindAllString(string(orig), -1)))

	fmt.Printf("\n翻译文件:\n")
	fmt.Printf("  sections: %d\n", len(sectionRe.FindAllString(string(trans), -1)))
	fmt.Printf("  subsections: %d\n", len(subsectionRe.FindAllString(string(trans), -1)))
	fmt.Printf("  figures: %d\n", len(figureRe.FindAllString(string(trans), -1)))
	fmt.Printf("  tables: %d\n", len(tableRe.FindAllString(string(trans), -1)))

	// Check for common issues
	origLines := strings.Split(string(orig), "\n")
	transLines := strings.Split(string(trans), "\n")
	fmt.Printf("\n原始文件行数: %d\n", len(origLines))
	fmt.Printf("翻译文件行数: %d\n", len(transLines))
}
