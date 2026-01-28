// Package errors provides error tracking and management for translation processes
package errors

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrorStage 错误阶段枚举
type ErrorStage string

const (
	StageDownload           ErrorStage = "download"            // 下载阶段
	StageExtract            ErrorStage = "extract"             // 解压阶段
	StageOriginalCompile    ErrorStage = "original_compile"    // 原始文档编译阶段
	StageTranslation        ErrorStage = "translation"         // 翻译阶段
	StageTranslatedCompile  ErrorStage = "translated_compile"  // 翻译后编译阶段
	StagePDFGeneration      ErrorStage = "pdf_generation"      // PDF生成阶段
	StagePageCountMismatch  ErrorStage = "page_count_mismatch" // 页数差异过大（可疑错误）
)

// ErrorRecord 错误记录
type ErrorRecord struct {
	ID          string     `json:"id"`           // 唯一标识符（通常是 arXiv ID 或文件名）
	Title       string     `json:"title"`        // 论文标题
	Input       string     `json:"input"`        // 原始输入（URL/ID/路径）
	Stage       ErrorStage `json:"stage"`        // 出错阶段
	ErrorMsg    string     `json:"error_msg"`    // 错误信息
	Timestamp   time.Time  `json:"timestamp"`    // 错误发生时间
	CanRetry    bool       `json:"can_retry"`    // 是否可以重试
	RetryCount  int        `json:"retry_count"`  // 重试次数
	LastRetry   time.Time  `json:"last_retry"`   // 最后重试时间
	Reported    bool       `json:"reported"`     // 是否已上报到 GitHub
	ReportedAt  time.Time  `json:"reported_at"`  // 上报时间
}

// ErrorManager 错误管理器
type ErrorManager struct {
	baseDir string
	mu      sync.RWMutex
	errors  map[string]*ErrorRecord // key: ID
}

// NewErrorManager 创建新的错误管理器
func NewErrorManager(baseDir string) (*ErrorManager, error) {
	if baseDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		baseDir = filepath.Join(homeDir, ".latex-translator", "errors")
	}

	// 确保目录存在
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create errors directory: %w", err)
	}

	em := &ErrorManager{
		baseDir: baseDir,
		errors:  make(map[string]*ErrorRecord),
	}

	// 加载现有错误记录
	if err := em.load(); err != nil {
		return nil, err
	}

	return em, nil
}

// RecordError 记录错误
func (em *ErrorManager) RecordError(id, title, input string, stage ErrorStage, errorMsg string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	record := &ErrorRecord{
		ID:         id,
		Title:      title,
		Input:      input,
		Stage:      stage,
		ErrorMsg:   errorMsg,
		Timestamp:  time.Now(),
		CanRetry:   true,
		RetryCount: 0,
	}

	// 如果已存在，更新重试次数
	if existing, ok := em.errors[id]; ok {
		record.RetryCount = existing.RetryCount
		record.LastRetry = existing.LastRetry
	}

	em.errors[id] = record

	return em.save()
}

// IncrementRetry 增加重试次数
func (em *ErrorManager) IncrementRetry(id string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	if record, ok := em.errors[id]; ok {
		record.RetryCount++
		record.LastRetry = time.Now()
		return em.save()
	}

	return fmt.Errorf("error record not found: %s", id)
}

// RemoveError 移除错误记录（翻译成功后）
func (em *ErrorManager) RemoveError(id string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	delete(em.errors, id)
	return em.save()
}

// ListErrors 列出所有错误记录
func (em *ErrorManager) ListErrors() []*ErrorRecord {
	em.mu.RLock()
	defer em.mu.RUnlock()

	records := make([]*ErrorRecord, 0, len(em.errors))
	for _, record := range em.errors {
		// 创建副本以避免并发修改
		recordCopy := *record
		records = append(records, &recordCopy)
	}

	return records
}

// GetError 获取特定错误记录
func (em *ErrorManager) GetError(id string) (*ErrorRecord, bool) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	record, ok := em.errors[id]
	if !ok {
		return nil, false
	}

	// 返回副本
	recordCopy := *record
	return &recordCopy, true
}

// ClearAll 清除所有错误记录
func (em *ErrorManager) ClearAll() error {
	em.mu.Lock()
	defer em.mu.Unlock()

	em.errors = make(map[string]*ErrorRecord)
	return em.save()
}

// load 从文件加载错误记录
func (em *ErrorManager) load() error {
	filePath := filepath.Join(em.baseDir, "errors.json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在是正常的
			return nil
		}
		return fmt.Errorf("failed to read errors file: %w", err)
	}

	var records []*ErrorRecord
	if err := json.Unmarshal(data, &records); err != nil {
		return fmt.Errorf("failed to unmarshal errors: %w", err)
	}

	for _, record := range records {
		em.errors[record.ID] = record
	}

	return nil
}

// save 保存错误记录到文件
func (em *ErrorManager) save() error {
	records := make([]*ErrorRecord, 0, len(em.errors))
	for _, record := range em.errors {
		records = append(records, record)
	}

	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal errors: %w", err)
	}

	filePath := filepath.Join(em.baseDir, "errors.json")
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write errors file: %w", err)
	}

	return nil
}

// ExportErrorIDs 导出所有错误的 arXiv ID 到文本文件，每行一个 ID
func (em *ErrorManager) ExportErrorIDs(outputPath string) error {
	em.mu.RLock()
	defer em.mu.RUnlock()

	// 收集所有 ID
	ids := make([]string, 0, len(em.errors))
	for id := range em.errors {
		ids = append(ids, id)
	}

	// 如果没有错误，创建空文件
	if len(ids) == 0 {
		return os.WriteFile(outputPath, []byte(""), 0644)
	}

	// 写入文件，每行一个 ID
	content := ""
	for _, id := range ids {
		content += id + "\n"
	}

	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write error IDs file: %w", err)
	}

	return nil
}

// GetStageDisplayName 获取阶段的显示名称
func GetStageDisplayName(stage ErrorStage) string {
	switch stage {
	case StageDownload:
		return "下载"
	case StageExtract:
		return "解压"
	case StageOriginalCompile:
		return "原始文档编译"
	case StageTranslation:
		return "翻译"
	case StageTranslatedCompile:
		return "翻译后编译"
	case StagePDFGeneration:
		return "PDF生成"
	case StagePageCountMismatch:
		return "页数差异过大"
	default:
		return string(stage)
	}
}

// ListUnreportedErrors 列出所有未上报的错误记录
func (em *ErrorManager) ListUnreportedErrors() []*ErrorRecord {
	em.mu.RLock()
	defer em.mu.RUnlock()

	records := make([]*ErrorRecord, 0)
	for _, record := range em.errors {
		if !record.Reported {
			// 创建副本以避免并发修改
			recordCopy := *record
			records = append(records, &recordCopy)
		}
	}

	return records
}

// MarkAsReported 标记错误记录为已上报
func (em *ErrorManager) MarkAsReported(ids []string) error {
	em.mu.Lock()
	defer em.mu.Unlock()

	now := time.Now()
	for _, id := range ids {
		if record, ok := em.errors[id]; ok {
			record.Reported = true
			record.ReportedAt = now
		}
	}

	return em.save()
}

// HasUnreportedErrors 检查是否有未上报的错误
func (em *ErrorManager) HasUnreportedErrors() bool {
	em.mu.RLock()
	defer em.mu.RUnlock()

	for _, record := range em.errors {
		if !record.Reported {
			return true
		}
	}
	return false
}
