// Package translator provides functionality for translating LaTeX documents.
package translator

import (
	"testing"
)

// ============================================================
// CompareStructure Tests (Task 11.1)
// Validates: Requirement 7.1, Property 21
// ============================================================

func TestCompareStructure_IdenticalContent(t *testing.T) {
	content := `\documentclass{article}
\usepackage{amsmath}
\begin{document}
\section{Introduction}
This is the introduction.
\subsection{Background}
Some background.
\begin{figure}
\caption{A figure}
\label{fig:test}
\end{figure}
\cite{ref1}
\end{document}`

	result := CompareStructure(content, content)

	if !result.IsMatch {
		t.Error("CompareStructure() should return IsMatch=true for identical content")
	}

	if len(result.Differences) != 0 {
		t.Errorf("CompareStructure() should have 0 differences for identical content, got %d", len(result.Differences))
	}
}

func TestCompareStructure_DocumentClassChange(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
content
\end{document}`

	translated := `\documentclass{report}
\begin{document}
content
\end{document}`

	result := CompareStructure(original, translated)

	// Should detect document class change
	found := false
	for _, diff := range result.Differences {
		if diff.Type == "documentclass" {
			found = true
			break
		}
	}

	if !found {
		t.Error("CompareStructure() should detect document class change")
	}
}

func TestCompareStructure_CompatibleDocumentClass(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
content
\end{document}`

	translated := `\documentclass{ctexart}
\begin{document}
content
\end{document}`

	result := CompareStructure(original, translated)

	// article -> ctexart should be compatible
	for _, diff := range result.Differences {
		if diff.Type == "documentclass" && diff.Severity == "error" {
			t.Error("CompareStructure() should treat article->ctexart as compatible")
		}
	}
}

func TestCompareStructure_MissingSections(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
\section{Introduction}
\section{Methods}
\section{Results}
\end{document}`

	translated := `\documentclass{article}
\begin{document}
\section{Introduction}
\section{Methods}
\end{document}`

	result := CompareStructure(original, translated)

	if result.IsMatch {
		t.Error("CompareStructure() should return IsMatch=false when sections are missing")
	}

	// Should detect section count difference
	found := false
	for _, diff := range result.Differences {
		if diff.Type == "section" && diff.Name == "section" {
			found = true
			if diff.OriginalCount != 3 {
				t.Errorf("Expected original section count 3, got %d", diff.OriginalCount)
			}
			if diff.TranslatedCount != 2 {
				t.Errorf("Expected translated section count 2, got %d", diff.TranslatedCount)
			}
			if diff.Difference != -1 {
				t.Errorf("Expected difference -1, got %d", diff.Difference)
			}
		}
	}

	if !found {
		t.Error("CompareStructure() should detect missing sections")
	}
}

func TestCompareStructure_MissingEnvironments(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
\begin{figure}
\caption{Figure 1}
\end{figure}
\begin{figure}
\caption{Figure 2}
\end{figure}
\end{document}`

	translated := `\documentclass{article}
\begin{document}
\begin{figure}
\caption{Figure 1}
\end{figure}
\end{document}`

	result := CompareStructure(original, translated)

	if result.IsMatch {
		t.Error("CompareStructure() should return IsMatch=false when environments are missing")
	}

	// Should detect figure count difference
	found := false
	for _, diff := range result.Differences {
		if diff.Type == "environment" && diff.Name == "figure" {
			found = true
			if diff.OriginalCount != 2 {
				t.Errorf("Expected original figure count 2, got %d", diff.OriginalCount)
			}
			if diff.TranslatedCount != 1 {
				t.Errorf("Expected translated figure count 1, got %d", diff.TranslatedCount)
			}
		}
	}

	if !found {
		t.Error("CompareStructure() should detect missing figure environment")
	}
}

func TestCompareStructure_MissingCommands(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
\cite{ref1}
\cite{ref2}
\cite{ref3}
\ref{fig:1}
\label{sec:intro}
\end{document}`

	translated := `\documentclass{article}
\begin{document}
\cite{ref1}
\ref{fig:1}
\end{document}`

	result := CompareStructure(original, translated)

	if result.IsMatch {
		t.Error("CompareStructure() should return IsMatch=false when commands are missing")
	}

	// Should detect cite count difference
	foundCite := false
	foundLabel := false
	for _, diff := range result.Differences {
		if diff.Type == "command" && diff.Name == "cite" {
			foundCite = true
			if diff.OriginalCount != 3 {
				t.Errorf("Expected original cite count 3, got %d", diff.OriginalCount)
			}
			if diff.TranslatedCount != 1 {
				t.Errorf("Expected translated cite count 1, got %d", diff.TranslatedCount)
			}
		}
		if diff.Type == "command" && diff.Name == "label" {
			foundLabel = true
		}
	}

	if !foundCite {
		t.Error("CompareStructure() should detect missing cite commands")
	}
	if !foundLabel {
		t.Error("CompareStructure() should detect missing label command")
	}
}

func TestCompareStructure_MissingPackages(t *testing.T) {
	original := `\documentclass{article}
\usepackage{amsmath}
\usepackage{graphicx}
\usepackage{hyperref}
\begin{document}
content
\end{document}`

	translated := `\documentclass{article}
\usepackage{amsmath}
\begin{document}
content
\end{document}`

	result := CompareStructure(original, translated)

	// Should detect missing packages
	missingCount := 0
	for _, diff := range result.Differences {
		if diff.Type == "package" && diff.Difference < 0 {
			missingCount++
		}
	}

	if missingCount != 2 {
		t.Errorf("Expected 2 missing packages, got %d", missingCount)
	}
}

func TestCompareStructure_AddedPackages(t *testing.T) {
	original := `\documentclass{article}
\usepackage{amsmath}
\begin{document}
content
\end{document}`

	translated := `\documentclass{article}
\usepackage{amsmath}
\usepackage{ctex}
\begin{document}
content
\end{document}`

	result := CompareStructure(original, translated)

	// Should detect added package (ctex is expected, so should be info level)
	found := false
	for _, diff := range result.Differences {
		if diff.Type == "package" && diff.Name == "ctex" {
			found = true
			if diff.Severity != "info" {
				t.Errorf("Expected ctex package addition to be 'info' severity, got %s", diff.Severity)
			}
		}
	}

	if !found {
		t.Error("CompareStructure() should detect added ctex package")
	}
}

func TestCompareStructure_AllSectionTypes(t *testing.T) {
	original := `\documentclass{book}
\begin{document}
\part{Part One}
\chapter{Chapter One}
\section{Section One}
\subsection{Subsection One}
\subsubsection{Subsubsection One}
\paragraph{Paragraph One}
\subparagraph{Subparagraph One}
\end{document}`

	result := CompareStructure(original, original)

	// Check that all section types are counted
	expectedSections := map[string]int{
		"part":          1,
		"chapter":       1,
		"section":       1,
		"subsection":    1,
		"subsubsection": 1,
		"paragraph":     1,
		"subparagraph":  1,
	}

	for secType, expectedCount := range expectedSections {
		actualCount := result.OriginalStructure.Sections[secType]
		if actualCount != expectedCount {
			t.Errorf("Expected %s count %d, got %d", secType, expectedCount, actualCount)
		}
	}
}

func TestCompareStructure_AllEnvironmentTypes(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
\begin{figure}
\end{figure}
\begin{table}
\begin{tabular}{cc}
\end{tabular}
\end{table}
\begin{equation}
\end{equation}
\begin{align}
\end{align}
\begin{itemize}
\end{itemize}
\begin{enumerate}
\end{enumerate}
\end{document}`

	result := CompareStructure(original, original)

	// Check that all environment types are counted
	expectedEnvs := map[string]int{
		"document":  1,
		"figure":    1,
		"table":     1,
		"tabular":   1,
		"equation":  1,
		"align":     1,
		"itemize":   1,
		"enumerate": 1,
	}

	for envType, expectedCount := range expectedEnvs {
		actualCount := result.OriginalStructure.Environments[envType]
		if actualCount != expectedCount {
			t.Errorf("Expected %s count %d, got %d", envType, expectedCount, actualCount)
		}
	}
}

func TestCompareStructure_AllCommandTypes(t *testing.T) {
	original := `\documentclass{article}
\title{Test}
\author{Author}
\date{2024}
\begin{document}
\maketitle
\tableofcontents
\cite{ref1}
\citep{ref2}
\citet{ref3}
\ref{fig:1}
\eqref{eq:1}
\pageref{page:1}
\label{sec:1}
\caption{Caption}
\footnote{Note}
\includegraphics{image}
\input{file}
\include{chapter}
\bibliography{refs}
\bibliographystyle{plain}
\end{document}`

	result := CompareStructure(original, original)

	// Check that key commands are counted
	expectedCmds := map[string]int{
		"title":            1,
		"author":           1,
		"date":             1,
		"maketitle":        1,
		"tableofcontents":  1,
		"cite":             1,
		"citep":            1,
		"citet":            1,
		"ref":              1,
		"eqref":            1,
		"pageref":          1,
		"label":            1,
		"caption":          1,
		"footnote":         1,
		"includegraphics":  1,
		"input":            1,
		"include":          1,
		"bibliography":     1,
		"bibliographystyle": 1,
	}

	for cmdType, expectedCount := range expectedCmds {
		actualCount := result.OriginalStructure.Commands[cmdType]
		if actualCount != expectedCount {
			t.Errorf("Expected %s count %d, got %d", cmdType, expectedCount, actualCount)
		}
	}
}

func TestCompareStructure_EmptyContent(t *testing.T) {
	result := CompareStructure("", "")

	if !result.IsMatch {
		t.Error("CompareStructure() should return IsMatch=true for empty content")
	}

	if len(result.Differences) != 0 {
		t.Errorf("CompareStructure() should have 0 differences for empty content, got %d", len(result.Differences))
	}
}

func TestCompareStructure_LineCount(t *testing.T) {
	original := `line1
line2
line3
line4
line5`

	translated := `line1
line2
line3`

	result := CompareStructure(original, translated)

	if result.OriginalStructure.TotalLines != 5 {
		t.Errorf("Expected original total lines 5, got %d", result.OriginalStructure.TotalLines)
	}

	if result.TranslatedStructure.TotalLines != 3 {
		t.Errorf("Expected translated total lines 3, got %d", result.TranslatedStructure.TotalLines)
	}
}

func TestCompareStructure_NonEmptyLineCount(t *testing.T) {
	original := `line1

line2

line3`

	result := CompareStructure(original, original)

	if result.OriginalStructure.TotalLines != 5 {
		t.Errorf("Expected total lines 5, got %d", result.OriginalStructure.TotalLines)
	}

	if result.OriginalStructure.NonEmptyLines != 3 {
		t.Errorf("Expected non-empty lines 3, got %d", result.OriginalStructure.NonEmptyLines)
	}
}

func TestCompareStructure_SeverityLevels(t *testing.T) {
	// Test that critical elements get error severity
	original := `\documentclass{article}
\begin{document}
\begin{figure}
\caption{Test}
\label{fig:test}
\end{figure}
\cite{ref1}
\end{document}`

	translated := `\documentclass{article}
\begin{document}
\end{document}`

	result := CompareStructure(original, translated)

	// Missing figure should be error
	// Missing cite should be error
	// Missing caption should be error
	// Missing label should be error
	errorCount := 0
	for _, diff := range result.Differences {
		if diff.Severity == "error" {
			errorCount++
		}
	}

	if errorCount == 0 {
		t.Error("CompareStructure() should report errors for missing critical elements")
	}
}

func TestCompareStructure_MultiplePackagesInOneUsepackage(t *testing.T) {
	original := `\documentclass{article}
\usepackage{amsmath,amssymb,amsthm}
\begin{document}
\end{document}`

	result := CompareStructure(original, original)

	// Should count all three packages
	packages := result.OriginalStructure.Packages
	if len(packages) != 3 {
		t.Errorf("Expected 3 packages, got %d: %v", len(packages), packages)
	}

	// Check each package is present
	packageSet := make(map[string]bool)
	for _, pkg := range packages {
		packageSet[pkg] = true
	}

	expectedPackages := []string{"amsmath", "amssymb", "amsthm"}
	for _, pkg := range expectedPackages {
		if !packageSet[pkg] {
			t.Errorf("Expected package %s not found", pkg)
		}
	}
}

func TestCompareStructure_StarredSections(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
\section{Normal Section}
\section*{Starred Section}
\subsection{Normal Subsection}
\subsection*{Starred Subsection}
\end{document}`

	result := CompareStructure(original, original)

	// Both starred and non-starred should be counted
	if result.OriginalStructure.Sections["section"] != 2 {
		t.Errorf("Expected section count 2, got %d", result.OriginalStructure.Sections["section"])
	}

	if result.OriginalStructure.Sections["subsection"] != 2 {
		t.Errorf("Expected subsection count 2, got %d", result.OriginalStructure.Sections["subsection"])
	}
}

func TestCompareStructure_SectionsWithOptionalArgs(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
\section[Short Title]{Long Section Title}
\subsection[Short]{Long Subsection Title}
\end{document}`

	result := CompareStructure(original, original)

	if result.OriginalStructure.Sections["section"] != 1 {
		t.Errorf("Expected section count 1, got %d", result.OriginalStructure.Sections["section"])
	}

	if result.OriginalStructure.Sections["subsection"] != 1 {
		t.Errorf("Expected subsection count 1, got %d", result.OriginalStructure.Sections["subsection"])
	}
}

func TestFormatStructureComparison_Match(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
\section{Test}
\end{document}`

	result := CompareStructure(original, original)
	formatted := FormatStructureComparison(result)

	if formatted == "" {
		t.Error("FormatStructureComparison() should return non-empty string")
	}

	if !containsSubstring(formatted, "文档结构") {
		t.Error("Formatted output should contain '文档结构'")
	}
}

func TestFormatStructureComparison_Differences(t *testing.T) {
	original := `\documentclass{article}
\begin{document}
\section{Test1}
\section{Test2}
\end{document}`

	translated := `\documentclass{article}
\begin{document}
\section{Test1}
\end{document}`

	result := CompareStructure(original, translated)
	formatted := FormatStructureComparison(result)

	if !containsSubstring(formatted, "差异") {
		t.Error("Formatted output should contain '差异' for mismatched content")
	}
}

// Helper function
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ============================================================
// DocumentStructure Tests
// ============================================================

func TestDocumentStructure_Fields(t *testing.T) {
	structure := &DocumentStructure{
		DocumentClass: "article",
		Packages:      []string{"amsmath", "graphicx"},
		Sections:      map[string]int{"section": 3, "subsection": 5},
		Environments:  map[string]int{"figure": 2, "table": 1},
		Commands:      map[string]int{"cite": 10, "ref": 5},
		TotalLines:    100,
		NonEmptyLines: 80,
	}

	if structure.DocumentClass != "article" {
		t.Errorf("DocumentClass = %s, want article", structure.DocumentClass)
	}
	if len(structure.Packages) != 2 {
		t.Errorf("Packages count = %d, want 2", len(structure.Packages))
	}
	if structure.Sections["section"] != 3 {
		t.Errorf("Sections[section] = %d, want 3", structure.Sections["section"])
	}
	if structure.Environments["figure"] != 2 {
		t.Errorf("Environments[figure] = %d, want 2", structure.Environments["figure"])
	}
	if structure.Commands["cite"] != 10 {
		t.Errorf("Commands[cite] = %d, want 10", structure.Commands["cite"])
	}
	if structure.TotalLines != 100 {
		t.Errorf("TotalLines = %d, want 100", structure.TotalLines)
	}
	if structure.NonEmptyLines != 80 {
		t.Errorf("NonEmptyLines = %d, want 80", structure.NonEmptyLines)
	}
}

// ============================================================
// StructureDifference Tests
// ============================================================

func TestStructureDifference_Fields(t *testing.T) {
	diff := StructureDifference{
		Type:            "section",
		Name:            "subsection",
		OriginalCount:   5,
		TranslatedCount: 3,
		Difference:      -2,
		Severity:        "error",
	}

	if diff.Type != "section" {
		t.Errorf("Type = %s, want section", diff.Type)
	}
	if diff.Name != "subsection" {
		t.Errorf("Name = %s, want subsection", diff.Name)
	}
	if diff.OriginalCount != 5 {
		t.Errorf("OriginalCount = %d, want 5", diff.OriginalCount)
	}
	if diff.TranslatedCount != 3 {
		t.Errorf("TranslatedCount = %d, want 3", diff.TranslatedCount)
	}
	if diff.Difference != -2 {
		t.Errorf("Difference = %d, want -2", diff.Difference)
	}
	if diff.Severity != "error" {
		t.Errorf("Severity = %s, want error", diff.Severity)
	}
}

// ============================================================
// StructureComparison Tests
// ============================================================

func TestStructureComparison_Fields(t *testing.T) {
	comparison := &StructureComparison{
		IsMatch:             true,
		OriginalStructure:   &DocumentStructure{DocumentClass: "article"},
		TranslatedStructure: &DocumentStructure{DocumentClass: "article"},
		Differences:         []StructureDifference{},
		Summary:             "文档结构完全匹配",
	}

	if !comparison.IsMatch {
		t.Error("IsMatch should be true")
	}
	if comparison.OriginalStructure.DocumentClass != "article" {
		t.Errorf("OriginalStructure.DocumentClass = %s, want article", comparison.OriginalStructure.DocumentClass)
	}
	if comparison.TranslatedStructure.DocumentClass != "article" {
		t.Errorf("TranslatedStructure.DocumentClass = %s, want article", comparison.TranslatedStructure.DocumentClass)
	}
	if len(comparison.Differences) != 0 {
		t.Errorf("Differences count = %d, want 0", len(comparison.Differences))
	}
	if comparison.Summary != "文档结构完全匹配" {
		t.Errorf("Summary = %s, want 文档结构完全匹配", comparison.Summary)
	}
}

// ============================================================
// Helper Function Tests
// ============================================================

func TestExtractPackages(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "single package",
			input:    `\usepackage{amsmath}`,
			expected: []string{"amsmath"},
		},
		{
			name:     "multiple packages in one usepackage",
			input:    `\usepackage{amsmath,amssymb,amsthm}`,
			expected: []string{"amsmath", "amssymb", "amsthm"},
		},
		{
			name:     "package with options",
			input:    `\usepackage[utf8]{inputenc}`,
			expected: []string{"inputenc"},
		},
		{
			name:     "multiple usepackage commands",
			input:    `\usepackage{amsmath}\n\usepackage{graphicx}`,
			expected: []string{"amsmath", "graphicx"},
		},
		{
			name:     "no packages",
			input:    `\documentclass{article}`,
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractPackages(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("extractPackages() returned %d packages, want %d", len(result), len(tt.expected))
				return
			}
			for i, pkg := range tt.expected {
				if result[i] != pkg {
					t.Errorf("extractPackages()[%d] = %s, want %s", i, result[i], pkg)
				}
			}
		})
	}
}

func TestIsCompatibleDocumentClass(t *testing.T) {
	tests := []struct {
		orig       string
		trans      string
		compatible bool
	}{
		{"article", "article", true},
		{"article", "ctexart", true},
		{"ctexart", "article", true},
		{"report", "ctexrep", true},
		{"book", "ctexbook", true},
		{"article", "report", false},
		{"article", "book", false},
		{"report", "book", false},
	}

	for _, tt := range tests {
		t.Run(tt.orig+"->"+tt.trans, func(t *testing.T) {
			result := isCompatibleDocumentClass(tt.orig, tt.trans)
			if result != tt.compatible {
				t.Errorf("isCompatibleDocumentClass(%s, %s) = %v, want %v", tt.orig, tt.trans, result, tt.compatible)
			}
		})
	}
}

func TestIsExpectedAddedPackage(t *testing.T) {
	tests := []struct {
		pkg      string
		expected bool
	}{
		{"ctex", true},
		{"xeCJK", true},
		{"fontspec", true},
		{"CJKutf8", true},
		{"amsmath", false},
		{"graphicx", false},
		{"hyperref", false},
	}

	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			result := isExpectedAddedPackage(tt.pkg)
			if result != tt.expected {
				t.Errorf("isExpectedAddedPackage(%s) = %v, want %v", tt.pkg, result, tt.expected)
			}
		})
	}
}

func TestIsCriticalEnvironment(t *testing.T) {
	tests := []struct {
		env      string
		critical bool
	}{
		{"document", true},
		{"figure", true},
		{"table", true},
		{"equation", true},
		{"align", true},
		{"theorem", true},
		{"lemma", true},
		{"proof", true},
		{"abstract", true},
		{"itemize", true},
		{"enumerate", true},
		{"description", true},
		{"center", false},
		{"minipage", false},
		{"verbatim", false},
	}

	for _, tt := range tests {
		t.Run(tt.env, func(t *testing.T) {
			result := isCriticalEnvironment(tt.env)
			if result != tt.critical {
				t.Errorf("isCriticalEnvironment(%s) = %v, want %v", tt.env, result, tt.critical)
			}
		})
	}
}

func TestIsCriticalCommand(t *testing.T) {
	tests := []struct {
		cmd      string
		critical bool
	}{
		{"cite", true},
		{"citep", true},
		{"citet", true},
		{"ref", true},
		{"eqref", true},
		{"label", true},
		{"caption", true},
		{"includegraphics", true},
		{"title", false},
		{"author", false},
		{"maketitle", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := isCriticalCommand(tt.cmd)
			if result != tt.critical {
				t.Errorf("isCriticalCommand(%s) = %v, want %v", tt.cmd, result, tt.critical)
			}
		})
	}
}

func TestUniqueStrings(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no duplicates",
			input:    []string{"a", "b", "c"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "with duplicates",
			input:    []string{"a", "b", "a", "c", "b"},
			expected: []string{"a", "b", "c"},
		},
		{
			name:     "all same",
			input:    []string{"a", "a", "a"},
			expected: []string{"a"},
		},
		{
			name:     "empty",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := uniqueStrings(tt.input)
			if len(result) != len(tt.expected) {
				t.Errorf("uniqueStrings() returned %d items, want %d", len(result), len(tt.expected))
				return
			}
			for i, s := range tt.expected {
				if result[i] != s {
					t.Errorf("uniqueStrings()[%d] = %s, want %s", i, result[i], s)
				}
			}
		})
	}
}
