package errors

import (
	"os"
	"strings"
	"testing"
)

func TestErrorManager(t *testing.T) {
	// 创建临时目录
	tempDir := t.TempDir()

	// 创建错误管理器
	em, err := NewErrorManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create error manager: %v", err)
	}

	// 测试记录错误
	err = em.RecordError("2501.17161", "Test Paper", "https://arxiv.org/abs/2501.17161", StageDownload, "download failed")
	if err != nil {
		t.Fatalf("Failed to record error: %v", err)
	}

	// 测试获取错误
	record, ok := em.GetError("2501.17161")
	if !ok {
		t.Fatal("Error record not found")
	}

	if record.ID != "2501.17161" {
		t.Errorf("Expected ID 2501.17161, got %s", record.ID)
	}

	if record.Stage != StageDownload {
		t.Errorf("Expected stage download, got %s", record.Stage)
	}

	// 测试增加重试次数
	err = em.IncrementRetry("2501.17161")
	if err != nil {
		t.Fatalf("Failed to increment retry: %v", err)
	}

	record, _ = em.GetError("2501.17161")
	if record.RetryCount != 1 {
		t.Errorf("Expected retry count 1, got %d", record.RetryCount)
	}

	// 测试列出所有错误
	records := em.ListErrors()
	if len(records) != 1 {
		t.Errorf("Expected 1 error record, got %d", len(records))
	}

	// 测试移除错误
	err = em.RemoveError("2501.17161")
	if err != nil {
		t.Fatalf("Failed to remove error: %v", err)
	}

	records = em.ListErrors()
	if len(records) != 0 {
		t.Errorf("Expected 0 error records, got %d", len(records))
	}
}

func TestErrorManagerPersistence(t *testing.T) {
	tempDir := t.TempDir()

	// 创建第一个管理器并记录错误
	em1, err := NewErrorManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create error manager: %v", err)
	}

	err = em1.RecordError("test1", "Test 1", "input1", StageTranslation, "error1")
	if err != nil {
		t.Fatalf("Failed to record error: %v", err)
	}

	// 创建第二个管理器，应该能加载之前的错误
	em2, err := NewErrorManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create second error manager: %v", err)
	}

	record, ok := em2.GetError("test1")
	if !ok {
		t.Fatal("Error record not found after reload")
	}

	if record.ErrorMsg != "error1" {
		t.Errorf("Expected error message 'error1', got '%s'", record.ErrorMsg)
	}
}

func TestGetStageDisplayName(t *testing.T) {
	tests := []struct {
		stage    ErrorStage
		expected string
	}{
		{StageDownload, "下载"},
		{StageExtract, "解压"},
		{StageOriginalCompile, "原始文档编译"},
		{StageTranslation, "翻译"},
		{StageTranslatedCompile, "翻译后编译"},
		{StagePDFGeneration, "PDF生成"},
	}

	for _, tt := range tests {
		t.Run(string(tt.stage), func(t *testing.T) {
			result := GetStageDisplayName(tt.stage)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestExportErrorIDs(t *testing.T) {
	tempDir := t.TempDir()

	// 创建错误管理器
	em, err := NewErrorManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create error manager: %v", err)
	}

	// 记录多个错误
	testErrors := []struct {
		id    string
		title string
		input string
	}{
		{"2501.17161", "Paper 1", "https://arxiv.org/abs/2501.17161"},
		{"2504.04365", "Paper 2", "https://arxiv.org/abs/2504.04365"},
		{"2507.12345", "Paper 3", "https://arxiv.org/abs/2507.12345"},
	}

	for _, te := range testErrors {
		err = em.RecordError(te.id, te.title, te.input, StageDownload, "test error")
		if err != nil {
			t.Fatalf("Failed to record error: %v", err)
		}
	}

	// 导出 ID 列表
	outputPath := tempDir + "/error_ids.txt"
	err = em.ExportErrorIDs(outputPath)
	if err != nil {
		t.Fatalf("Failed to export error IDs: %v", err)
	}

	// 读取导出的文件
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read exported file: %v", err)
	}

	// 验证内容
	contentStr := string(content)
	for _, te := range testErrors {
		if !strings.Contains(contentStr, te.id) {
			t.Errorf("Expected exported file to contain ID '%s'", te.id)
		}
	}

	// 验证每个 ID 在单独的行上
	lines := strings.Split(strings.TrimSpace(contentStr), "\n")
	if len(lines) != len(testErrors) {
		t.Errorf("Expected %d lines, got %d", len(testErrors), len(lines))
	}
}

func TestExportErrorIDsEmpty(t *testing.T) {
	tempDir := t.TempDir()

	// 创建错误管理器（没有错误）
	em, err := NewErrorManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create error manager: %v", err)
	}

	// 导出空列表
	outputPath := tempDir + "/empty_ids.txt"
	err = em.ExportErrorIDs(outputPath)
	if err != nil {
		t.Fatalf("Failed to export empty error IDs: %v", err)
	}

	// 读取导出的文件
	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("Failed to read exported file: %v", err)
	}

	// 验证文件为空
	if len(content) != 0 {
		t.Errorf("Expected empty file, got %d bytes", len(content))
	}
}
