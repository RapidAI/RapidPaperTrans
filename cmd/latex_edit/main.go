// latex-edit is a command-line tool for editing and fixing LaTeX files
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"latex-translator/internal/editor"
	"latex-translator/internal/logger"
)

func main() {
	// Initialize logger
	logger.Init(&logger.Config{
		Level: logger.LevelInfo,
	})

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "lines":
		handleLinesCommand()
	case "encoding":
		handleEncodingCommand()
	case "validate":
		handleValidateCommand()
	case "fix":
		handleFixCommand()
	case "backup":
		handleBackupCommand()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	usage := `latex-edit - LaTeX File Editing Tool

Usage:
  latex-edit <command> [arguments]

Commands:
  lines       Line-level editing operations
  encoding    Encoding detection and conversion
  validate    Validate LaTeX files
  fix         Automatically fix common issues
  backup      Backup management

Line Commands:
  latex-edit lines read <file> <start> <end>
  latex-edit lines replace <file> <line> <content>
  latex-edit lines insert <file> <line> <content>
  latex-edit lines delete <file> <line>
  latex-edit lines count <file>
  latex-edit lines search <file> <text>

Encoding Commands:
  latex-edit encoding detect <file>
  latex-edit encoding convert <file> --from=<encoding> --to=<encoding>
  latex-edit encoding remove-bom <file>
  latex-edit encoding ensure-utf8 <file>
  latex-edit encoding info <file>

Validation Commands:
  latex-edit validate <file>
  latex-edit validate --quick <file>

Fix Commands:
  latex-edit fix <file>
  latex-edit fix --encoding-only <file>
  latex-edit fix --auto <file>

Backup Commands:
  latex-edit backup create <file>
  latex-edit backup restore <backup-file> <original-file>
  latex-edit backup list <file>
  latex-edit backup cleanup <file> --keep=<count>

Examples:
  latex-edit encoding detect main.tex
  latex-edit validate main.tex
  latex-edit fix --auto main.tex
  latex-edit lines read main.tex 10 20
`
	fmt.Println(usage)
}

func handleLinesCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: latex-edit lines <subcommand> [arguments]")
		os.Exit(1)
	}

	subcommand := os.Args[2]
	backupMgr := editor.NewBackupManager("")
	lineEditor := editor.NewLineEditor(backupMgr)

	switch subcommand {
	case "read":
		if len(os.Args) < 6 {
			fmt.Println("Usage: latex-edit lines read <file> <start> <end>")
			os.Exit(1)
		}
		file := os.Args[3]
		start, _ := strconv.Atoi(os.Args[4])
		end, _ := strconv.Atoi(os.Args[5])

		lines, err := lineEditor.ReadLines(file, start, end)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		for i, line := range lines {
			fmt.Printf("%d: %s\n", start+i, line)
		}

	case "replace":
		if len(os.Args) < 6 {
			fmt.Println("Usage: latex-edit lines replace <file> <line> <content>")
			os.Exit(1)
		}
		file := os.Args[3]
		lineNum, _ := strconv.Atoi(os.Args[4])
		content := strings.Join(os.Args[5:], " ")

		if err := lineEditor.ReplaceLine(file, lineNum, content); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Line %d replaced successfully\n", lineNum)

	case "insert":
		if len(os.Args) < 6 {
			fmt.Println("Usage: latex-edit lines insert <file> <line> <content>")
			os.Exit(1)
		}
		file := os.Args[3]
		lineNum, _ := strconv.Atoi(os.Args[4])
		content := strings.Join(os.Args[5:], " ")

		if err := lineEditor.InsertLine(file, lineNum, content); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Line inserted at %d successfully\n", lineNum)

	case "delete":
		if len(os.Args) < 5 {
			fmt.Println("Usage: latex-edit lines delete <file> <line>")
			os.Exit(1)
		}
		file := os.Args[3]
		lineNum, _ := strconv.Atoi(os.Args[4])

		if err := lineEditor.DeleteLine(file, lineNum); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Line %d deleted successfully\n", lineNum)

	case "count":
		if len(os.Args) < 4 {
			fmt.Println("Usage: latex-edit lines count <file>")
			os.Exit(1)
		}
		file := os.Args[3]

		count, err := lineEditor.CountLines(file)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("File has %d lines\n", count)

	case "search":
		if len(os.Args) < 5 {
			fmt.Println("Usage: latex-edit lines search <file> <text>")
			os.Exit(1)
		}
		file := os.Args[3]
		searchText := strings.Join(os.Args[4:], " ")

		lines, err := lineEditor.SearchLines(file, searchText)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if len(lines) == 0 {
			fmt.Println("No matches found")
		} else {
			fmt.Printf("Found %d matches:\n", len(lines))
			for _, lineNum := range lines {
				fmt.Printf("  Line %d\n", lineNum)
			}
		}

	default:
		fmt.Printf("Unknown subcommand: %s\n", subcommand)
		os.Exit(1)
	}
}

func handleEncodingCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: latex-edit encoding <subcommand> [arguments]")
		os.Exit(1)
	}

	subcommand := os.Args[2]
	backupMgr := editor.NewBackupManager("")
	encodingHandler := editor.NewEncodingHandler(backupMgr)

	switch subcommand {
	case "detect":
		if len(os.Args) < 4 {
			fmt.Println("Usage: latex-edit encoding detect <file>")
			os.Exit(1)
		}
		file := os.Args[3]

		encoding, err := encodingHandler.DetectEncoding(file)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("Encoding: %s\n", encoding)

	case "convert":
		if len(os.Args) < 4 {
			fmt.Println("Usage: latex-edit encoding convert <file> --from=<encoding> --to=<encoding>")
			os.Exit(1)
		}
		file := os.Args[3]
		from := ""
		to := ""

		for _, arg := range os.Args[4:] {
			if strings.HasPrefix(arg, "--from=") {
				from = strings.TrimPrefix(arg, "--from=")
			} else if strings.HasPrefix(arg, "--to=") {
				to = strings.TrimPrefix(arg, "--to=")
			}
		}

		if from == "" || to == "" {
			fmt.Println("Error: --from and --to are required")
			os.Exit(1)
		}

		if err := encodingHandler.ConvertEncoding(file, from, to); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Converted from %s to %s\n", from, to)

	case "remove-bom":
		if len(os.Args) < 4 {
			fmt.Println("Usage: latex-edit encoding remove-bom <file>")
			os.Exit(1)
		}
		file := os.Args[3]

		if err := encodingHandler.RemoveBOM(file); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ BOM removed")

	case "ensure-utf8":
		if len(os.Args) < 4 {
			fmt.Println("Usage: latex-edit encoding ensure-utf8 <file>")
			os.Exit(1)
		}
		file := os.Args[3]

		if err := encodingHandler.EnsureUTF8(file); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ File is now UTF-8")

	case "info":
		if len(os.Args) < 4 {
			fmt.Println("Usage: latex-edit encoding info <file>")
			os.Exit(1)
		}
		file := os.Args[3]

		info, err := encodingHandler.GetEncodingInfo(file)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Encoding Information:\n")
		fmt.Printf("  Encoding: %s\n", info.Encoding)
		fmt.Printf("  Has BOM: %v\n", info.HasBOM)
		fmt.Printf("  Valid: %v\n", info.IsValid)
		fmt.Printf("  File Size: %d bytes\n", info.FileSize)
		if info.SampleText != "" {
			fmt.Printf("  Sample: %s...\n", info.SampleText)
		}

	default:
		fmt.Printf("Unknown subcommand: %s\n", subcommand)
		os.Exit(1)
	}
}

func handleValidateCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: latex-edit validate [--quick] <file>")
		os.Exit(1)
	}

	quick := false
	file := ""

	for _, arg := range os.Args[2:] {
		if arg == "--quick" {
			quick = true
		} else {
			file = arg
		}
	}

	if file == "" {
		fmt.Println("Error: file path required")
		os.Exit(1)
	}

	validator := editor.NewLaTeXValidator()

	if quick {
		valid, err := validator.QuickCheck(file)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		if valid {
			fmt.Println("✓ Quick check passed")
		} else {
			fmt.Println("✗ Quick check failed")
			os.Exit(1)
		}
	} else {
		report, err := validator.ValidateAndReport(file)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(report)
	}
}

func handleFixCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: latex-edit fix [--auto|--encoding-only] <file>")
		os.Exit(1)
	}

	encodingOnly := false
	file := ""

	for _, arg := range os.Args[2:] {
		if arg == "--encoding-only" {
			encodingOnly = true
		} else if arg == "--auto" {
			// auto is the default behavior
		} else {
			file = arg
		}
	}

	if file == "" {
		fmt.Println("Error: file path required")
		os.Exit(1)
	}

	workflow := editor.NewFixWorkflow("")

	var result *editor.FixResult
	var err error

	if encodingOnly {
		result, err = workflow.FixEncodingOnly(file)
	} else {
		result, err = workflow.AutoFix(file)
	}

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(editor.FormatFixResult(result))

	if !result.Success {
		os.Exit(1)
	}
}

func handleBackupCommand() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: latex-edit backup <subcommand> [arguments]")
		os.Exit(1)
	}

	subcommand := os.Args[2]
	backupMgr := editor.NewBackupManager("")

	switch subcommand {
	case "create":
		if len(os.Args) < 4 {
			fmt.Println("Usage: latex-edit backup create <file>")
			os.Exit(1)
		}
		file := os.Args[3]

		backupPath, err := backupMgr.CreateBackup(file)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Backup created: %s\n", backupPath)

	case "restore":
		if len(os.Args) < 5 {
			fmt.Println("Usage: latex-edit backup restore <backup-file> <original-file>")
			os.Exit(1)
		}
		backupFile := os.Args[3]
		originalFile := os.Args[4]

		if err := backupMgr.Restore(backupFile, originalFile); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✓ File restored from backup")

	case "list":
		if len(os.Args) < 4 {
			fmt.Println("Usage: latex-edit backup list <file>")
			os.Exit(1)
		}
		file := os.Args[3]

		backups, err := backupMgr.ListBackups(file)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}

		if len(backups) == 0 {
			fmt.Println("No backups found")
		} else {
			fmt.Printf("Found %d backup(s):\n", len(backups))
			for i, backup := range backups {
				fmt.Printf("  %d. %s\n", i+1, filepath.Base(backup))
			}
		}

	case "cleanup":
		if len(os.Args) < 4 {
			fmt.Println("Usage: latex-edit backup cleanup <file> --keep=<count>")
			os.Exit(1)
		}
		file := os.Args[3]
		keepCount := 5 // default

		for _, arg := range os.Args[4:] {
			if strings.HasPrefix(arg, "--keep=") {
				keepCount, _ = strconv.Atoi(strings.TrimPrefix(arg, "--keep="))
			}
		}

		if err := backupMgr.CleanupBackups(file, keepCount); err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✓ Cleanup completed (kept %d backups)\n", keepCount)

	default:
		fmt.Printf("Unknown subcommand: %s\n", subcommand)
		os.Exit(1)
	}
}
