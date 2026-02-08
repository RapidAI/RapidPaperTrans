package main

import (
	"os"

	"latex-translator/internal/pdf"
)

func main() {
	os.WriteFile("testdata/output/pymupdf_translate_test.py", []byte(pdf.PyMuPDFTranslateScript), 0644)
}
