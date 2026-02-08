// Package license provides license management and activation functionality.
// It handles communication with the license server and manages commercial license data.
package license

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	// LicenseServerURL is the URL of the license server
	LicenseServerURL = "http://license.vantagedata.chat:6699"
)

// WorkMode represents the application work mode
type WorkMode string

const (
	// WorkModeCommercial represents commercial software mode
	WorkModeCommercial WorkMode = "commercial"
	// WorkModeOpenSource represents open source software mode
	WorkModeOpenSource WorkMode = "opensource"
)

// ActivationData contains the decrypted activation data from the license server
type ActivationData struct {
	LLMType         string `json:"llm_type"`
	LLMBaseURL      string `json:"llm_base_url"`
	LLMAPIKey       string `json:"llm_api_key"`
	LLMModel        string `json:"llm_model"`
	LLMStartDate    string `json:"llm_start_date"`    // LLM 配置生效开始日期
	LLMEndDate      string `json:"llm_end_date"`      // LLM 配置生效结束日期
	SearchType      string `json:"search_type"`       // 搜索引擎类型
	SearchAPIKey    string `json:"search_api_key"`    // 搜索引擎 API 密钥
	SearchStartDate string `json:"search_start_date"` // 搜索配置生效开始日期
	SearchEndDate   string `json:"search_end_date"`   // 搜索配置生效结束日期
	ExpiresAt       string `json:"expires_at"`
	ActivatedAt     string `json:"activated_at"`
	DailyAnalysis   int    `json:"daily_analysis"` // 每日分析次数限制，0 表示无限制
}

// ActivationResponse represents the response from the license server activation endpoint
type ActivationResponse struct {
	Success       bool   `json:"success"`
	Code          string `json:"code"`
	Message       string `json:"message"`
	EncryptedData string `json:"encrypted_data,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
}

// LicenseInfo contains the complete license information stored locally
type LicenseInfo struct {
	WorkMode       WorkMode        `json:"work_mode"`
	SerialNumber   string          `json:"serial_number,omitempty"`
	ActivationData *ActivationData `json:"activation_data,omitempty"`
	ActivatedAt    time.Time       `json:"activated_at,omitempty"`
}

// Client is the license client that handles license activation and validation
type Client struct {
	serverURL string
}

// NewClient creates a new license client with the default server URL
func NewClient() *Client {
	return &Client{
		serverURL: LicenseServerURL,
	}
}

// serialNumberPattern is the compiled regex pattern for validating serial numbers
// Format: XXXX-XXXX-XXXX-XXXX where X is uppercase letter (A-Z) or digit (0-9)
var serialNumberPattern = regexp.MustCompile(`^[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$`)

// ValidateSerialNumber validates the format of a serial number.
// The serial number must match the format XXXX-XXXX-XXXX-XXXX where X is an uppercase letter (A-Z) or digit (0-9).
// Returns true if the format is valid, false otherwise.
func (c *Client) ValidateSerialNumber(sn string) bool {
	return serialNumberPattern.MatchString(sn)
}

// GCM nonce size constant
const gcmNonceSize = 12

// DecryptData decrypts the activation data received from the license server.
// It uses AES-256-GCM algorithm with SHA-256(serialNumber) as the decryption key.
//
// The encrypted data format is: Base64(nonce || ciphertext || tag)
// - nonce: 12 bytes (GCM standard)
// - ciphertext: variable length
// - tag: 16 bytes (GCM authentication tag, appended to ciphertext by Go's GCM)
//
// Parameters:
//   - encryptedData: Base64 encoded encrypted data from the license server
//   - serialNumber: The serial number used to derive the decryption key
//
// Returns:
//   - *ActivationData: The decrypted and parsed activation data
//   - error: Decryption or parsing error
//
// Validates: Requirements 7.1, 7.2, 7.3, 7.4, 7.5
func (c *Client) DecryptData(encryptedData, serialNumber string) (*ActivationData, error) {
	// Step 1: Base64 decode the encrypted data
	cipherData, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("解密错误：Base64 解码失败: %w", err)
	}

	// Step 2: Validate minimum data length (nonce + at least some ciphertext)
	// GCM requires at least nonce (12 bytes) + tag (16 bytes) = 28 bytes minimum
	minLength := gcmNonceSize + 16 // nonce + GCM tag
	if len(cipherData) < minLength {
		return nil, errors.New("解密错误：加密数据长度无效")
	}

	// Step 3: Extract nonce (first 12 bytes)
	nonce := cipherData[:gcmNonceSize]
	ciphertext := cipherData[gcmNonceSize:]

	// Step 4: Derive key using SHA-256(serialNumber)
	// SHA-256 produces a 32-byte hash, which is exactly what AES-256 needs
	key := sha256.Sum256([]byte(serialNumber))

	// Step 5: Create AES cipher block
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("解密错误：创建 AES 密码块失败: %w", err)
	}

	// Step 6: Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("解密错误：创建 GCM 模式失败: %w", err)
	}

	// Step 7: Decrypt the ciphertext
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("解密错误：解密失败，序列号可能不正确: %w", err)
	}

	// Step 8: Parse JSON into ActivationData struct
	var activationData ActivationData
	if err := json.Unmarshal(plaintext, &activationData); err != nil {
		return nil, fmt.Errorf("解密错误：JSON 解析失败: %w", err)
	}

	return &activationData, nil
}

// activationRequest represents the request body for the activation endpoint
type activationRequest struct {
	SN string `json:"sn"`
}

// httpTimeout is the timeout for HTTP requests to the license server
const httpTimeout = 30 * time.Second

// Activate activates a serial number by sending a request to the license server.
// It validates the serial number format, sends the activation request, and decrypts
// the response data if successful.
//
// Parameters:
//   - serialNumber: The serial number to activate (format: XXXX-XXXX-XXXX-XXXX)
//
// Returns:
//   - *ActivationResponse: The raw response from the license server
//   - *ActivationData: The decrypted activation data (nil if activation failed)
//   - error: Any error that occurred during the activation process
//
// Validates: Requirements 3.3, 3.4, 3.5
func (c *Client) Activate(serialNumber string) (*ActivationResponse, *ActivationData, error) {
	// Step 1: Normalize serial number to uppercase
	serialNumber = strings.ToUpper(strings.TrimSpace(serialNumber))

	// Step 2: Validate serial number format
	if !c.ValidateSerialNumber(serialNumber) {
		return nil, nil, errors.New("序列号格式无效，请使用格式：XXXX-XXXX-XXXX-XXXX")
	}

	// Step 3: Prepare the request body
	reqBody := activationRequest{
		SN: serialNumber,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, fmt.Errorf("激活错误：请求数据序列化失败: %w", err)
	}

	// Step 4: Create HTTP request
	url := c.serverURL + "/activate"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, nil, fmt.Errorf("激活错误：创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Step 5: Send the request with timeout
	client := &http.Client{
		Timeout: httpTimeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("激活错误：无法连接服务器，请检查网络连接: %w", err)
	}
	defer resp.Body.Close()

	// Step 6: Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("激活错误：读取响应失败: %w", err)
	}

	// Step 7: Parse the response
	var activationResp ActivationResponse
	if err := json.Unmarshal(body, &activationResp); err != nil {
		return nil, nil, fmt.Errorf("激活错误：解析响应失败: %w", err)
	}

	// Step 8: Check if activation was successful
	if !activationResp.Success {
		// Return the response with the error message from the server
		// Map error codes to Chinese error messages
		errMsg := mapActivationErrorCode(activationResp.Code, activationResp.Message)
		return &activationResp, nil, errors.New(errMsg)
	}

	// Step 9: Decrypt the activation data
	if activationResp.EncryptedData == "" {
		return &activationResp, nil, errors.New("激活错误：服务器未返回配置数据")
	}

	activationData, err := c.DecryptData(activationResp.EncryptedData, serialNumber)
	if err != nil {
		return &activationResp, nil, fmt.Errorf("激活错误：解密配置数据失败: %w", err)
	}

	return &activationResp, activationData, nil
}

// mapActivationErrorCode maps server error codes to user-friendly Chinese error messages
func mapActivationErrorCode(code, defaultMessage string) string {
	switch code {
	case "INVALID_SN":
		return "序列号无效，请检查序列号是否正确"
	case "SN_EXPIRED":
		return "序列号已过期，请续费或联系管理员"
	case "SN_DISABLED":
		return "序列号已被禁用，请联系管理员"
	case "SN_NOT_FOUND":
		return "序列号不存在，请检查序列号是否正确"
	case "ENCRYPT_FAILED":
		return "服务器加密失败，请稍后重试"
	case "SERVER_ERROR":
		return "服务器内部错误，请稍后重试"
	default:
		if defaultMessage != "" {
			return defaultMessage
		}
		return "激活失败，请稍后重试"
	}
}


// emailPattern is a basic regex pattern for validating email addresses
var emailPattern = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail validates the format of an email address.
// Returns true if the format is valid, false otherwise.
func (c *Client) ValidateEmail(email string) bool {
	return emailPattern.MatchString(email)
}

// requestSNRequest represents the request body for the request-sn endpoint
type requestSNRequest struct {
	Email     string `json:"email"`
	ProductID int    `json:"product_id"`
}

// RequestSNResponse represents the response from the request-sn endpoint
type RequestSNResponse struct {
	Success bool   `json:"success"`
	Code    string `json:"code"`
	Message string `json:"message"`
	SN      string `json:"sn,omitempty"` // 分配的序列号（仅成功时返回）
}

// RequestSNResult represents the result of a serial number request
type RequestSNResult struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	SerialNumber string `json:"serial_number,omitempty"` // 分配的序列号（仅成功时返回）
}

// RequestSN requests a serial number via email.
// It sends a POST request to the /request-sn endpoint with the provided email address.
//
// Parameters:
//   - email: The email address to send the serial number request to
//
// Returns:
//   - *RequestSNResult: Result containing success status, message, and serial number if available
//   - error: Any error that occurred during the request process
//
// Validates: Requirements 3.8
func (c *Client) RequestSN(email string) (*RequestSNResult, error) {
	// Step 1: Trim whitespace from email
	email = strings.TrimSpace(email)

	// Step 2: Validate email format
	if email == "" {
		return nil, errors.New("邮箱地址不能为空")
	}
	if !c.ValidateEmail(email) {
		return nil, errors.New("邮箱地址格式无效，请检查后重试")
	}

	// Step 3: Prepare the request body
	// Product ID 1 is for RapidPaperTrans (论译)
	reqBody := requestSNRequest{
		Email:     email,
		ProductID: 1,
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("申请错误：请求数据序列化失败: %w", err)
	}

	// Step 4: Create HTTP request
	url := c.serverURL + "/request-sn"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("申请错误：创建请求失败: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Step 5: Send the request with timeout
	httpClient := &http.Client{
		Timeout: httpTimeout,
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("申请错误：无法连接服务器，请检查网络连接: %w", err)
	}
	defer resp.Body.Close()

	// Step 6: Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("申请错误：读取响应失败: %w", err)
	}

	// Step 7: Parse the response
	var snResp RequestSNResponse
	if err := json.Unmarshal(body, &snResp); err != nil {
		return nil, fmt.Errorf("申请错误：解析响应失败: %w", err)
	}

	// Step 8: Check if request was successful
	if !snResp.Success {
		// Map error codes to Chinese error messages
		errMsg := mapRequestSNErrorCode(snResp.Code, snResp.Message)
		return nil, errors.New(errMsg)
	}

	// Build result
	result := &RequestSNResult{
		Success:      true,
		Message:      snResp.Message,
		SerialNumber: snResp.SN,
	}
	
	if result.Message == "" {
		if result.SerialNumber != "" {
			result.Message = "序列号申请成功"
		} else {
			result.Message = "序列号申请已提交，请查收邮件"
		}
	}
	
	return result, nil
}

// mapRequestSNErrorCode maps server error codes to user-friendly Chinese error messages
func mapRequestSNErrorCode(code, defaultMessage string) string {
	switch code {
	case "INVALID_EMAIL":
		return "无效的邮箱地址，请检查邮箱格式"
	case "EMAIL_EXISTS":
		return "该邮箱已申请过序列号，请查收邮件或联系管理员"
	case "RATE_LIMITED":
		return "申请过于频繁，请稍后重试"
	case "SERVER_ERROR":
		return "服务器内部错误，请稍后重试"
	default:
		if defaultMessage != "" {
			return defaultMessage
		}
		return "申请失败，请稍后重试"
	}
}

// IsExpired checks if the activation data has expired.
// It parses the ExpiresAt field (RFC3339 format) and compares it with the current time.
//
// Parameters:
//   - data: The activation data containing the expiration time
//
// Returns:
//   - true if the authorization has expired or if the expiration time cannot be parsed
//   - false if the authorization is still valid
//
// Validates: Requirements 8.1, 8.2
func (c *Client) IsExpired(data *ActivationData) bool {
	if data == nil {
		return true
	}

	if data.ExpiresAt == "" {
		// No expiration time means treat as expired for safety
		return true
	}

	expiresAt, err := time.Parse(time.RFC3339, data.ExpiresAt)
	if err != nil {
		// If we can't parse the expiration time, treat as expired for safety
		return true
	}

	return time.Now().After(expiresAt)
}

// GetEffectiveBaseURL returns the effective base URL for the LLM API.
// If LLMBaseURL is empty, it derives the URL from LLMType.
// This handles the case where the license server returns an empty base URL
// but provides the LLM type (e.g., "deepseek").
//
// Supported LLM types and their default base URLs:
//   - "deepseek": https://api.deepseek.com/v1
//   - "openai": https://api.openai.com/v1
//   - "azure": (requires explicit URL)
//   - default: https://api.openai.com/v1
//
// Parameters:
//   - data: The activation data containing LLMType and LLMBaseURL
//
// Returns:
//   - The effective base URL to use for API calls
func (c *Client) GetEffectiveBaseURL(data *ActivationData) string {
	if data == nil {
		return "https://api.openai.com/v1"
	}

	// If base URL is explicitly provided, use it
	if data.LLMBaseURL != "" {
		return data.LLMBaseURL
	}

	// Derive base URL from LLM type
	switch strings.ToLower(data.LLMType) {
	case "deepseek":
		return "https://api.deepseek.com/v1"
	case "openai":
		return "https://api.openai.com/v1"
	case "anthropic", "claude":
		return "https://api.anthropic.com/v1"
	case "moonshot", "kimi":
		return "https://api.moonshot.cn/v1"
	case "qwen", "tongyi", "dashscope":
		return "https://dashscope.aliyuncs.com/compatible-mode/v1"
	case "zhipu", "glm":
		return "https://open.bigmodel.cn/api/paas/v4"
	default:
		// Default to OpenAI-compatible endpoint
		return "https://api.openai.com/v1"
	}
}

// DaysUntilExpiry calculates the number of days until the authorization expires.
// It parses the ExpiresAt field (RFC3339 format) and calculates the difference from the current time.
//
// Parameters:
//   - data: The activation data containing the expiration time
//
// Returns:
//   - Positive number: days remaining until expiry
//   - Zero: expires today
//   - Negative number: days since expiration (already expired)
//   - If data is nil or ExpiresAt cannot be parsed, returns -1 (treated as expired)
//
// Note: The calculation is based on 24-hour periods, not calendar days.
// For example, if expiry is in 36 hours, this returns 1 (one full day remaining).
//
// Validates: Requirements 8.1, 8.2, 8.3
func (c *Client) DaysUntilExpiry(data *ActivationData) int {
	if data == nil {
		return -1
	}

	if data.ExpiresAt == "" {
		// No expiration time means treat as expired
		return -1
	}

	expiresAt, err := time.Parse(time.RFC3339, data.ExpiresAt)
	if err != nil {
		// If we can't parse the expiration time, treat as expired
		return -1
	}

	// Calculate the duration until expiry
	duration := time.Until(expiresAt)

	// Convert to days (truncate towards zero)
	// For positive durations: floor division (e.g., 36 hours = 1 day)
	// For negative durations: ceiling division towards zero (e.g., -36 hours = -1 day)
	days := int(duration.Hours() / 24)

	return days
}
