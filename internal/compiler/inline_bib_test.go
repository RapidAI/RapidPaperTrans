package compiler

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInlineBibliographyWithDollarSign(t *testing.T) {
	// Test that $ signs in .bbl content are handled correctly
	texContent := `\documentclass{article}
\begin{document}
Hello world
\bibliography{references}
\bibliographystyle{plain}
\end{document}`

	bblContent := `\begin{thebibliography}{1}
\bibitem{test}
Author, A.
\newblock Title with $x^2$ math.
\newblock Journal, 2024.
\end{thebibliography}`

	result, wasInlined := inlineBibliography(texContent, bblContent)
	
	if !wasInlined {
		t.Error("Expected bibliography to be inlined")
	}
	
	// Check that the result contains the bbl content
	if !strings.Contains(result, `\begin{thebibliography}`) {
		t.Error("Result should contain \\begin{thebibliography}")
	}
	
	// Check that the $ sign is preserved
	if !strings.Contains(result, `$x^2$`) {
		t.Errorf("Result should contain $x^2$, got: %s", result)
	}
	
	// Check that \bibliography is removed
	if strings.Contains(result, `\bibliography{references}`) {
		t.Error("Result should not contain \\bibliography{references}")
	}
	
	// Check that the document structure is preserved
	if !strings.Contains(result, `\end{document}`) {
		t.Error("Result should contain \\end{document}")
	}
}

func TestFixMissingBibFileIntegration(t *testing.T) {
	// Skip if test files don't exist
	testDir := filepath.Join("..", "..", "test_2509_new")
	texPath := filepath.Join(testDir, "grok_dyn2.tex")
	bblPath := filepath.Join(testDir, "grok_dyn2.bbl")
	
	if _, err := os.Stat(texPath); os.IsNotExist(err) {
		t.Skip("Test files not found")
	}
	
	// Read the tex file
	texContent, err := os.ReadFile(texPath)
	if err != nil {
		t.Fatalf("Failed to read tex file: %v", err)
	}
	
	// Read the bbl file
	bblContent, err := os.ReadFile(bblPath)
	if err != nil {
		t.Fatalf("Failed to read bbl file: %v", err)
	}
	
	// Check that tex file contains \bibliography{references}
	if !strings.Contains(string(texContent), `\bibliography{references}`) {
		t.Skip("Tex file doesn't contain \\bibliography{references}")
	}
	
	// Check that bbl file contains thebibliography
	if !strings.Contains(string(bblContent), `\begin{thebibliography}`) {
		t.Fatal("BBL file doesn't contain \\begin{thebibliography}")
	}
	
	// Test the fixMissingBibFile function
	result, wasFixed := fixMissingBibFile(string(texContent), testDir, "grok_dyn2")
	
	if !wasFixed {
		t.Error("Expected fixMissingBibFile to fix the file")
	}
	
	// Check that the result contains the bbl content
	if !strings.Contains(result, `\begin{thebibliography}`) {
		t.Error("Result should contain \\begin{thebibliography}")
	}
	
	// Check that \bibliography is removed
	if strings.Contains(result, `\bibliography{references}`) {
		t.Error("Result should not contain \\bibliography{references}")
	}
	
	// Check that the document structure is preserved
	if !strings.Contains(result, `\end{document}`) {
		t.Error("Result should contain \\end{document}")
	}
	
	// Check that the result length is reasonable (should be longer than original)
	if len(result) < len(texContent) {
		t.Errorf("Result length (%d) should be >= original length (%d)", len(result), len(texContent))
	}
	
	t.Logf("Original tex length: %d, Result length: %d", len(texContent), len(result))
}
