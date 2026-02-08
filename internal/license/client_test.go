package license

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateSerialNumber(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name     string
		sn       string
		expected bool
	}{
		// Valid serial numbers
		{
			name:     "valid all uppercase letters",
			sn:       "ABCD-EFGH-IJKL-MNOP",
			expected: true,
		},
		{
			name:     "valid all digits",
			sn:       "1234-5678-9012-3456",
			expected: true,
		},
		{
			name:     "valid mixed letters and digits",
			sn:       "ABCD-1234-EFGH-5678",
			expected: true,
		},
		{
			name:     "valid with zeros",
			sn:       "0000-0000-0000-0000",
			expected: true,
		},
		{
			name:     "valid typical serial number",
			sn:       "A1B2-C3D4-E5F6-G7H8",
			expected: true,
		},

		// Invalid serial numbers - wrong format
		{
			name:     "empty string",
			sn:       "",
			expected: false,
		},
		{
			name:     "too short",
			sn:       "ABCD-EFGH-IJKL",
			expected: false,
		},
		{
			name:     "too long",
			sn:       "ABCD-EFGH-IJKL-MNOP-QRST",
			expected: false,
		},
		{
			name:     "missing dashes",
			sn:       "ABCDEFGHIJKLMNOP",
			expected: false,
		},
		{
			name:     "wrong separator",
			sn:       "ABCD_EFGH_IJKL_MNOP",
			expected: false,
		},
		{
			name:     "spaces instead of dashes",
			sn:       "ABCD EFGH IJKL MNOP",
			expected: false,
		},

		// Invalid serial numbers - wrong characters
		{
			name:     "lowercase letters",
			sn:       "abcd-efgh-ijkl-mnop",
			expected: false,
		},
		{
			name:     "mixed case letters",
			sn:       "AbCd-EfGh-IjKl-MnOp",
			expected: false,
		},
		{
			name:     "special characters",
			sn:       "AB@D-EF#H-IJ$L-MN%P",
			expected: false,
		},
		{
			name:     "contains spaces",
			sn:       "AB D-EFGH-IJKL-MNOP",
			expected: false,
		},

		// Invalid serial numbers - wrong segment length
		{
			name:     "first segment too short",
			sn:       "ABC-EFGH-IJKL-MNOP",
			expected: false,
		},
		{
			name:     "first segment too long",
			sn:       "ABCDE-EFGH-IJKL-MNOP",
			expected: false,
		},
		{
			name:     "second segment too short",
			sn:       "ABCD-EFG-IJKL-MNOP",
			expected: false,
		},
		{
			name:     "third segment too long",
			sn:       "ABCD-EFGH-IJKLM-MNOP",
			expected: false,
		},
		{
			name:     "fourth segment too short",
			sn:       "ABCD-EFGH-IJKL-MNO",
			expected: false,
		},

		// Edge cases
		{
			name:     "only dashes",
			sn:       "----",
			expected: false,
		},
		{
			name:     "whitespace only",
			sn:       "    ",
			expected: false,
		},
		{
			name:     "leading whitespace",
			sn:       " ABCD-EFGH-IJKL-MNOP",
			expected: false,
		},
		{
			name:     "trailing whitespace",
			sn:       "ABCD-EFGH-IJKL-MNOP ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.ValidateSerialNumber(tt.sn)
			if result != tt.expected {
				t.Errorf("ValidateSerialNumber(%q) = %v, expected %v", tt.sn, result, tt.expected)
			}
		})
	}
}


// encryptTestData is a helper function to encrypt test data using AES-256-GCM
// This mirrors the encryption process that the license server would use
func encryptTestData(data *ActivationData, serialNumber string) (string, error) {
	// Marshal the data to JSON
	plaintext, err := json.Marshal(data)
	if err != nil {
		return "", err
	}

	// Derive key using SHA-256(serialNumber)
	key := sha256.Sum256([]byte(serialNumber))

	// Create AES cipher block
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	// Encrypt the plaintext
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	// Combine nonce and ciphertext, then base64 encode
	combined := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

func TestDecryptData(t *testing.T) {
	client := NewClient()

	t.Run("successful decryption with valid data", func(t *testing.T) {
		// Create test activation data
		originalData := &ActivationData{
			LLMType:       "openai",
			LLMBaseURL:    "https://api.openai.com/v1",
			LLMAPIKey:     "sk-test-key-12345",
			LLMModel:      "gpt-4",
			ExpiresAt:     "2025-12-31T23:59:59+08:00",
			ActivatedAt:   "2024-06-01T10:30:00+08:00",
			DailyAnalysis: 20,
		}
		serialNumber := "ABCD-1234-EFGH-5678"

		// Encrypt the data
		encryptedData, err := encryptTestData(originalData, serialNumber)
		if err != nil {
			t.Fatalf("Failed to encrypt test data: %v", err)
		}

		// Decrypt the data
		decryptedData, err := client.DecryptData(encryptedData, serialNumber)
		if err != nil {
			t.Fatalf("DecryptData failed: %v", err)
		}

		// Verify the decrypted data matches the original
		if decryptedData.LLMType != originalData.LLMType {
			t.Errorf("LLMType mismatch: got %q, want %q", decryptedData.LLMType, originalData.LLMType)
		}
		if decryptedData.LLMBaseURL != originalData.LLMBaseURL {
			t.Errorf("LLMBaseURL mismatch: got %q, want %q", decryptedData.LLMBaseURL, originalData.LLMBaseURL)
		}
		if decryptedData.LLMAPIKey != originalData.LLMAPIKey {
			t.Errorf("LLMAPIKey mismatch: got %q, want %q", decryptedData.LLMAPIKey, originalData.LLMAPIKey)
		}
		if decryptedData.LLMModel != originalData.LLMModel {
			t.Errorf("LLMModel mismatch: got %q, want %q", decryptedData.LLMModel, originalData.LLMModel)
		}
		if decryptedData.ExpiresAt != originalData.ExpiresAt {
			t.Errorf("ExpiresAt mismatch: got %q, want %q", decryptedData.ExpiresAt, originalData.ExpiresAt)
		}
		if decryptedData.ActivatedAt != originalData.ActivatedAt {
			t.Errorf("ActivatedAt mismatch: got %q, want %q", decryptedData.ActivatedAt, originalData.ActivatedAt)
		}
		if decryptedData.DailyAnalysis != originalData.DailyAnalysis {
			t.Errorf("DailyAnalysis mismatch: got %d, want %d", decryptedData.DailyAnalysis, originalData.DailyAnalysis)
		}
	})

	t.Run("decryption fails with wrong serial number", func(t *testing.T) {
		originalData := &ActivationData{
			LLMType:    "openai",
			LLMBaseURL: "https://api.openai.com/v1",
			LLMAPIKey:  "sk-test-key",
			LLMModel:   "gpt-4",
		}
		correctSN := "ABCD-1234-EFGH-5678"
		wrongSN := "WXYZ-9876-STUV-4321"

		// Encrypt with correct serial number
		encryptedData, err := encryptTestData(originalData, correctSN)
		if err != nil {
			t.Fatalf("Failed to encrypt test data: %v", err)
		}

		// Try to decrypt with wrong serial number
		_, err = client.DecryptData(encryptedData, wrongSN)
		if err == nil {
			t.Error("Expected decryption to fail with wrong serial number, but it succeeded")
		}
	})

	t.Run("decryption fails with invalid base64", func(t *testing.T) {
		_, err := client.DecryptData("not-valid-base64!!!", "ABCD-1234-EFGH-5678")
		if err == nil {
			t.Error("Expected decryption to fail with invalid base64, but it succeeded")
		}
	})

	t.Run("decryption fails with data too short", func(t *testing.T) {
		// Create data that's too short (less than nonce + tag = 28 bytes)
		shortData := base64.StdEncoding.EncodeToString([]byte("short"))
		_, err := client.DecryptData(shortData, "ABCD-1234-EFGH-5678")
		if err == nil {
			t.Error("Expected decryption to fail with data too short, but it succeeded")
		}
	})

	t.Run("decryption fails with corrupted ciphertext", func(t *testing.T) {
		originalData := &ActivationData{
			LLMType:    "openai",
			LLMBaseURL: "https://api.openai.com/v1",
		}
		serialNumber := "ABCD-1234-EFGH-5678"

		// Encrypt the data
		encryptedData, err := encryptTestData(originalData, serialNumber)
		if err != nil {
			t.Fatalf("Failed to encrypt test data: %v", err)
		}

		// Decode, corrupt, and re-encode
		decoded, _ := base64.StdEncoding.DecodeString(encryptedData)
		if len(decoded) > 20 {
			decoded[20] ^= 0xFF // Flip bits in the ciphertext
		}
		corruptedData := base64.StdEncoding.EncodeToString(decoded)

		// Try to decrypt corrupted data
		_, err = client.DecryptData(corruptedData, serialNumber)
		if err == nil {
			t.Error("Expected decryption to fail with corrupted ciphertext, but it succeeded")
		}
	})

	t.Run("decryption with empty serial number", func(t *testing.T) {
		originalData := &ActivationData{
			LLMType: "openai",
		}
		emptySN := ""

		// Encrypt with empty serial number
		encryptedData, err := encryptTestData(originalData, emptySN)
		if err != nil {
			t.Fatalf("Failed to encrypt test data: %v", err)
		}

		// Decrypt with empty serial number should work (it's a valid key derivation)
		decryptedData, err := client.DecryptData(encryptedData, emptySN)
		if err != nil {
			t.Fatalf("DecryptData failed: %v", err)
		}

		if decryptedData.LLMType != originalData.LLMType {
			t.Errorf("LLMType mismatch: got %q, want %q", decryptedData.LLMType, originalData.LLMType)
		}
	})

	t.Run("decryption with special characters in data", func(t *testing.T) {
		originalData := &ActivationData{
			LLMType:    "openai",
			LLMBaseURL: "https://api.example.com/v1?param=value&other=测试",
			LLMAPIKey:  "sk-key-with-special-chars-!@#$%^&*()",
			LLMModel:   "gpt-4-turbo-中文",
		}
		serialNumber := "TEST-1234-ABCD-5678"

		// Encrypt the data
		encryptedData, err := encryptTestData(originalData, serialNumber)
		if err != nil {
			t.Fatalf("Failed to encrypt test data: %v", err)
		}

		// Decrypt the data
		decryptedData, err := client.DecryptData(encryptedData, serialNumber)
		if err != nil {
			t.Fatalf("DecryptData failed: %v", err)
		}

		// Verify special characters are preserved
		if decryptedData.LLMBaseURL != originalData.LLMBaseURL {
			t.Errorf("LLMBaseURL mismatch: got %q, want %q", decryptedData.LLMBaseURL, originalData.LLMBaseURL)
		}
		if decryptedData.LLMAPIKey != originalData.LLMAPIKey {
			t.Errorf("LLMAPIKey mismatch: got %q, want %q", decryptedData.LLMAPIKey, originalData.LLMAPIKey)
		}
		if decryptedData.LLMModel != originalData.LLMModel {
			t.Errorf("LLMModel mismatch: got %q, want %q", decryptedData.LLMModel, originalData.LLMModel)
		}
	})
}


func TestActivate(t *testing.T) {
	t.Run("invalid serial number format returns error", func(t *testing.T) {
		client := NewClient()

		// Test with invalid format
		_, _, err := client.Activate("invalid-sn")
		if err == nil {
			t.Error("Expected error for invalid serial number format, but got nil")
		}
		if err != nil && !strings.Contains(err.Error(), "格式无效") {
			t.Errorf("Expected format error message, got: %v", err)
		}
	})

	t.Run("empty serial number returns error", func(t *testing.T) {
		client := NewClient()

		_, _, err := client.Activate("")
		if err == nil {
			t.Error("Expected error for empty serial number, but got nil")
		}
	})

	t.Run("serial number is normalized to uppercase", func(t *testing.T) {
		// This test verifies that lowercase input is normalized
		// We can't test the actual HTTP call without a mock server,
		// but we can verify the validation passes for lowercase input
		client := NewClient()

		// lowercase should be normalized and pass validation
		// The actual HTTP call will fail, but we're testing normalization
		_, _, err := client.Activate("abcd-1234-efgh-5678")
		// Error should be about network, not format
		if err != nil && strings.Contains(err.Error(), "格式无效") {
			t.Error("Expected lowercase serial number to be normalized, but got format error")
		}
	})

	t.Run("serial number with whitespace is trimmed", func(t *testing.T) {
		client := NewClient()

		// whitespace should be trimmed
		_, _, err := client.Activate("  ABCD-1234-EFGH-5678  ")
		// Error should be about network, not format
		if err != nil && strings.Contains(err.Error(), "格式无效") {
			t.Error("Expected whitespace to be trimmed, but got format error")
		}
	})
}

func TestActivateWithMockServer(t *testing.T) {
	t.Run("successful activation", func(t *testing.T) {
		// Create test activation data
		testData := &ActivationData{
			LLMType:       "openai",
			LLMBaseURL:    "https://api.openai.com/v1",
			LLMAPIKey:     "sk-test-key",
			LLMModel:      "gpt-4",
			ExpiresAt:     "2025-12-31T23:59:59+08:00",
			ActivatedAt:   "2024-06-01T10:30:00+08:00",
			DailyAnalysis: 20,
		}
		serialNumber := "ABCD-1234-EFGH-5678"

		// Encrypt the test data
		encryptedData, err := encryptTestData(testData, serialNumber)
		if err != nil {
			t.Fatalf("Failed to encrypt test data: %v", err)
		}

		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request method and path
			if r.Method != http.MethodPost {
				t.Errorf("Expected POST method, got %s", r.Method)
			}
			if r.URL.Path != "/activate" {
				t.Errorf("Expected /activate path, got %s", r.URL.Path)
			}

			// Verify content type
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected application/json content type, got %s", r.Header.Get("Content-Type"))
			}

			// Parse request body
			var reqBody struct {
				SN string `json:"sn"`
			}
			if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
				t.Errorf("Failed to decode request body: %v", err)
			}
			if reqBody.SN != serialNumber {
				t.Errorf("Expected serial number %s, got %s", serialNumber, reqBody.SN)
			}

			// Return success response
			resp := ActivationResponse{
				Success:       true,
				Code:          "SUCCESS",
				Message:       "激活成功",
				EncryptedData: encryptedData,
				ExpiresAt:     "2025-12-31T23:59:59+08:00",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		// Create client with mock server URL
		client := &Client{serverURL: server.URL}

		// Call Activate
		resp, data, err := client.Activate(serialNumber)
		if err != nil {
			t.Fatalf("Activate failed: %v", err)
		}

		// Verify response
		if !resp.Success {
			t.Error("Expected success to be true")
		}
		if resp.Code != "SUCCESS" {
			t.Errorf("Expected code SUCCESS, got %s", resp.Code)
		}

		// Verify decrypted data
		if data == nil {
			t.Fatal("Expected activation data, got nil")
		}
		if data.LLMType != testData.LLMType {
			t.Errorf("LLMType mismatch: got %s, want %s", data.LLMType, testData.LLMType)
		}
		if data.LLMAPIKey != testData.LLMAPIKey {
			t.Errorf("LLMAPIKey mismatch: got %s, want %s", data.LLMAPIKey, testData.LLMAPIKey)
		}
		if data.DailyAnalysis != testData.DailyAnalysis {
			t.Errorf("DailyAnalysis mismatch: got %d, want %d", data.DailyAnalysis, testData.DailyAnalysis)
		}
	})

	t.Run("activation failure - invalid serial number", func(t *testing.T) {
		serialNumber := "ABCD-1234-EFGH-5678"

		// Create mock server that returns error
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := ActivationResponse{
				Success: false,
				Code:    "INVALID_SN",
				Message: "序列号无效",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		resp, data, err := client.Activate(serialNumber)
		if err == nil {
			t.Error("Expected error for invalid serial number, but got nil")
		}
		if !strings.Contains(err.Error(), "序列号无效") {
			t.Errorf("Expected error message about invalid serial number, got: %v", err)
		}
		if resp == nil {
			t.Error("Expected response even on failure")
		}
		if data != nil {
			t.Error("Expected nil data on failure")
		}
	})

	t.Run("activation failure - expired serial number", func(t *testing.T) {
		serialNumber := "ABCD-1234-EFGH-5678"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := ActivationResponse{
				Success: false,
				Code:    "SN_EXPIRED",
				Message: "序列号已过期",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, _, err := client.Activate(serialNumber)
		if err == nil {
			t.Error("Expected error for expired serial number")
		}
		if !strings.Contains(err.Error(), "过期") {
			t.Errorf("Expected error message about expiration, got: %v", err)
		}
	})

	t.Run("activation failure - disabled serial number", func(t *testing.T) {
		serialNumber := "ABCD-1234-EFGH-5678"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := ActivationResponse{
				Success: false,
				Code:    "SN_DISABLED",
				Message: "序列号已被禁用",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, _, err := client.Activate(serialNumber)
		if err == nil {
			t.Error("Expected error for disabled serial number")
		}
		if !strings.Contains(err.Error(), "禁用") {
			t.Errorf("Expected error message about disabled, got: %v", err)
		}
	})

	t.Run("activation failure - missing encrypted data", func(t *testing.T) {
		serialNumber := "ABCD-1234-EFGH-5678"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := ActivationResponse{
				Success:       true,
				Code:          "SUCCESS",
				Message:       "激活成功",
				EncryptedData: "", // Missing encrypted data
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, _, err := client.Activate(serialNumber)
		if err == nil {
			t.Error("Expected error for missing encrypted data")
		}
		if !strings.Contains(err.Error(), "未返回配置数据") {
			t.Errorf("Expected error message about missing data, got: %v", err)
		}
	})

	t.Run("activation failure - invalid JSON response", func(t *testing.T) {
		serialNumber := "ABCD-1234-EFGH-5678"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, _, err := client.Activate(serialNumber)
		if err == nil {
			t.Error("Expected error for invalid JSON response")
		}
		if !strings.Contains(err.Error(), "解析响应失败") {
			t.Errorf("Expected error message about parsing, got: %v", err)
		}
	})
}

func TestMapActivationErrorCode(t *testing.T) {
	tests := []struct {
		code           string
		defaultMessage string
		expectedSubstr string
	}{
		{"INVALID_SN", "", "序列号无效"},
		{"SN_EXPIRED", "", "已过期"},
		{"SN_DISABLED", "", "已被禁用"},
		{"SN_NOT_FOUND", "", "不存在"},
		{"ENCRYPT_FAILED", "", "加密失败"},
		{"SERVER_ERROR", "", "服务器内部错误"},
		{"UNKNOWN_CODE", "自定义错误消息", "自定义错误消息"},
		{"UNKNOWN_CODE", "", "激活失败"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			result := mapActivationErrorCode(tt.code, tt.defaultMessage)
			if !strings.Contains(result, tt.expectedSubstr) {
				t.Errorf("mapActivationErrorCode(%q, %q) = %q, expected to contain %q",
					tt.code, tt.defaultMessage, result, tt.expectedSubstr)
			}
		})
	}
}


func TestValidateEmail(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name     string
		email    string
		expected bool
	}{
		// Valid email addresses
		{
			name:     "valid simple email",
			email:    "test@example.com",
			expected: true,
		},
		{
			name:     "valid email with subdomain",
			email:    "user@mail.example.com",
			expected: true,
		},
		{
			name:     "valid email with plus sign",
			email:    "user+tag@example.com",
			expected: true,
		},
		{
			name:     "valid email with dots in local part",
			email:    "first.last@example.com",
			expected: true,
		},
		{
			name:     "valid email with numbers",
			email:    "user123@example123.com",
			expected: true,
		},
		{
			name:     "valid email with underscore",
			email:    "user_name@example.com",
			expected: true,
		},
		{
			name:     "valid email with hyphen in domain",
			email:    "user@my-domain.com",
			expected: true,
		},
		{
			name:     "valid email with long TLD",
			email:    "user@example.company",
			expected: true,
		},

		// Invalid email addresses
		{
			name:     "empty string",
			email:    "",
			expected: false,
		},
		{
			name:     "missing @ symbol",
			email:    "userexample.com",
			expected: false,
		},
		{
			name:     "missing domain",
			email:    "user@",
			expected: false,
		},
		{
			name:     "missing local part",
			email:    "@example.com",
			expected: false,
		},
		{
			name:     "missing TLD",
			email:    "user@example",
			expected: false,
		},
		{
			name:     "single character TLD",
			email:    "user@example.c",
			expected: false,
		},
		{
			name:     "spaces in email",
			email:    "user @example.com",
			expected: false,
		},
		{
			name:     "multiple @ symbols",
			email:    "user@@example.com",
			expected: false,
		},
		{
			name:     "special characters",
			email:    "user!#$@example.com",
			expected: false,
		},
		{
			name:     "only whitespace",
			email:    "   ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.ValidateEmail(tt.email)
			if result != tt.expected {
				t.Errorf("ValidateEmail(%q) = %v, expected %v", tt.email, result, tt.expected)
			}
		})
	}
}

func TestRequestSN(t *testing.T) {
	t.Run("empty email returns error", func(t *testing.T) {
		client := NewClient()

		_, err := client.RequestSN("")
		if err == nil {
			t.Error("Expected error for empty email, but got nil")
		}
		if !strings.Contains(err.Error(), "不能为空") {
			t.Errorf("Expected error message about empty email, got: %v", err)
		}
	})

	t.Run("invalid email format returns error", func(t *testing.T) {
		client := NewClient()

		_, err := client.RequestSN("invalid-email")
		if err == nil {
			t.Error("Expected error for invalid email format, but got nil")
		}
		if !strings.Contains(err.Error(), "格式无效") {
			t.Errorf("Expected error message about invalid format, got: %v", err)
		}
	})

	t.Run("whitespace only email returns error", func(t *testing.T) {
		client := NewClient()

		_, err := client.RequestSN("   ")
		if err == nil {
			t.Error("Expected error for whitespace only email, but got nil")
		}
		if !strings.Contains(err.Error(), "不能为空") {
			t.Errorf("Expected error message about empty email, got: %v", err)
		}
	})

	t.Run("email with whitespace is trimmed", func(t *testing.T) {
		client := NewClient()

		// Email with whitespace should be trimmed and pass validation
		// The actual HTTP call will fail, but we're testing trimming
		_, err := client.RequestSN("  test@example.com  ")
		// Error should be about network, not format
		if err != nil && strings.Contains(err.Error(), "格式无效") {
			t.Error("Expected whitespace to be trimmed, but got format error")
		}
	})
}

func TestRequestSNWithMockServer(t *testing.T) {
	t.Run("successful request", func(t *testing.T) {
		testEmail := "test@example.com"

		// Create mock server
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Verify request method and path
			if r.Method != http.MethodPost {
				t.Errorf("Expected POST method, got %s", r.Method)
			}
			if r.URL.Path != "/request-sn" {
				t.Errorf("Expected /request-sn path, got %s", r.URL.Path)
			}

			// Verify content type
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("Expected application/json content type, got %s", r.Header.Get("Content-Type"))
			}

			// Parse request body
			var reqBody struct {
				Email string `json:"email"`
			}
			if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
				t.Errorf("Failed to decode request body: %v", err)
			}
			if reqBody.Email != testEmail {
				t.Errorf("Expected email %s, got %s", testEmail, reqBody.Email)
			}

			// Return success response
			resp := RequestSNResponse{
				Success: true,
				Code:    "SUCCESS",
				Message: "序列号已发送到您的邮箱，请查收",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		// Create client with mock server URL
		client := &Client{serverURL: server.URL}

		// Call RequestSN
		result, err := client.RequestSN(testEmail)
		if err != nil {
			t.Fatalf("RequestSN failed: %v", err)
		}

		// Verify response message
		if result.Message != "序列号已发送到您的邮箱，请查收" {
			t.Errorf("Expected success message, got: %s", result.Message)
		}
	})

	t.Run("successful request with default message", func(t *testing.T) {
		testEmail := "test@example.com"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RequestSNResponse{
				Success: true,
				Code:    "SUCCESS",
				Message: "", // Empty message should use default
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		result, err := client.RequestSN(testEmail)
		if err != nil {
			t.Fatalf("RequestSN failed: %v", err)
		}

		// Should use default message
		if result.Message != "序列号申请已提交，请查收邮件" {
			t.Errorf("Expected default success message, got: %s", result.Message)
		}
	})

	t.Run("request failure - invalid email", func(t *testing.T) {
		testEmail := "test@example.com"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RequestSNResponse{
				Success: false,
				Code:    "INVALID_EMAIL",
				Message: "",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, err := client.RequestSN(testEmail)
		if err == nil {
			t.Error("Expected error for invalid email, but got nil")
		}
		if !strings.Contains(err.Error(), "无效的邮箱地址") {
			t.Errorf("Expected error message about invalid email, got: %v", err)
		}
	})

	t.Run("request failure - email exists", func(t *testing.T) {
		testEmail := "test@example.com"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RequestSNResponse{
				Success: false,
				Code:    "EMAIL_EXISTS",
				Message: "",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, err := client.RequestSN(testEmail)
		if err == nil {
			t.Error("Expected error for existing email, but got nil")
		}
		if !strings.Contains(err.Error(), "已申请过序列号") {
			t.Errorf("Expected error message about existing email, got: %v", err)
		}
	})

	t.Run("request failure - rate limited", func(t *testing.T) {
		testEmail := "test@example.com"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RequestSNResponse{
				Success: false,
				Code:    "RATE_LIMITED",
				Message: "",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, err := client.RequestSN(testEmail)
		if err == nil {
			t.Error("Expected error for rate limited, but got nil")
		}
		if !strings.Contains(err.Error(), "过于频繁") {
			t.Errorf("Expected error message about rate limiting, got: %v", err)
		}
	})

	t.Run("request failure - server error", func(t *testing.T) {
		testEmail := "test@example.com"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RequestSNResponse{
				Success: false,
				Code:    "SERVER_ERROR",
				Message: "",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, err := client.RequestSN(testEmail)
		if err == nil {
			t.Error("Expected error for server error, but got nil")
		}
		if !strings.Contains(err.Error(), "服务器内部错误") {
			t.Errorf("Expected error message about server error, got: %v", err)
		}
	})

	t.Run("request failure - unknown error with custom message", func(t *testing.T) {
		testEmail := "test@example.com"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			resp := RequestSNResponse{
				Success: false,
				Code:    "UNKNOWN_ERROR",
				Message: "自定义错误消息",
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, err := client.RequestSN(testEmail)
		if err == nil {
			t.Error("Expected error for unknown error, but got nil")
		}
		if !strings.Contains(err.Error(), "自定义错误消息") {
			t.Errorf("Expected custom error message, got: %v", err)
		}
	})

	t.Run("request failure - invalid JSON response", func(t *testing.T) {
		testEmail := "test@example.com"

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("invalid json"))
		}))
		defer server.Close()

		client := &Client{serverURL: server.URL}

		_, err := client.RequestSN(testEmail)
		if err == nil {
			t.Error("Expected error for invalid JSON response")
		}
		if !strings.Contains(err.Error(), "解析响应失败") {
			t.Errorf("Expected error message about parsing, got: %v", err)
		}
	})
}

func TestMapRequestSNErrorCode(t *testing.T) {
	tests := []struct {
		code           string
		defaultMessage string
		expectedSubstr string
	}{
		{"INVALID_EMAIL", "", "无效的邮箱地址"},
		{"EMAIL_EXISTS", "", "已申请过序列号"},
		{"RATE_LIMITED", "", "过于频繁"},
		{"SERVER_ERROR", "", "服务器内部错误"},
		{"UNKNOWN_CODE", "自定义错误消息", "自定义错误消息"},
		{"UNKNOWN_CODE", "", "申请失败"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			result := mapRequestSNErrorCode(tt.code, tt.defaultMessage)
			if !strings.Contains(result, tt.expectedSubstr) {
				t.Errorf("mapRequestSNErrorCode(%q, %q) = %q, expected to contain %q",
					tt.code, tt.defaultMessage, result, tt.expectedSubstr)
			}
		})
	}
}


// TestIsExpired tests the IsExpired function
func TestIsExpired(t *testing.T) {
	client := NewClient()

	t.Run("nil data returns true", func(t *testing.T) {
		result := client.IsExpired(nil)
		if !result {
			t.Error("Expected IsExpired(nil) to return true")
		}
	})

	t.Run("empty ExpiresAt returns true", func(t *testing.T) {
		data := &ActivationData{
			ExpiresAt: "",
		}
		result := client.IsExpired(data)
		if !result {
			t.Error("Expected IsExpired with empty ExpiresAt to return true")
		}
	})

	t.Run("invalid ExpiresAt format returns true", func(t *testing.T) {
		data := &ActivationData{
			ExpiresAt: "invalid-date-format",
		}
		result := client.IsExpired(data)
		if !result {
			t.Error("Expected IsExpired with invalid date format to return true")
		}
	})

	t.Run("past date returns true (expired)", func(t *testing.T) {
		data := &ActivationData{
			ExpiresAt: "2020-01-01T00:00:00+08:00",
		}
		result := client.IsExpired(data)
		if !result {
			t.Error("Expected IsExpired with past date to return true")
		}
	})

	t.Run("future date returns false (not expired)", func(t *testing.T) {
		// Use a date far in the future
		data := &ActivationData{
			ExpiresAt: "2099-12-31T23:59:59+08:00",
		}
		result := client.IsExpired(data)
		if result {
			t.Error("Expected IsExpired with future date to return false")
		}
	})

	t.Run("date one year from now returns false", func(t *testing.T) {
		futureDate := time.Now().AddDate(1, 0, 0).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.IsExpired(data)
		if result {
			t.Error("Expected IsExpired with date one year from now to return false")
		}
	})

	t.Run("date one hour ago returns true", func(t *testing.T) {
		pastDate := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: pastDate,
		}
		result := client.IsExpired(data)
		if !result {
			t.Error("Expected IsExpired with date one hour ago to return true")
		}
	})

	t.Run("date one hour from now returns false", func(t *testing.T) {
		futureDate := time.Now().Add(1 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.IsExpired(data)
		if result {
			t.Error("Expected IsExpired with date one hour from now to return false")
		}
	})

	t.Run("different timezone formats work correctly", func(t *testing.T) {
		// UTC format
		futureDate := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.IsExpired(data)
		if result {
			t.Error("Expected IsExpired with UTC format to return false for future date")
		}
	})
}

// TestDaysUntilExpiry tests the DaysUntilExpiry function
func TestDaysUntilExpiry(t *testing.T) {
	client := NewClient()

	t.Run("nil data returns -1", func(t *testing.T) {
		result := client.DaysUntilExpiry(nil)
		if result != -1 {
			t.Errorf("Expected DaysUntilExpiry(nil) to return -1, got %d", result)
		}
	})

	t.Run("empty ExpiresAt returns -1", func(t *testing.T) {
		data := &ActivationData{
			ExpiresAt: "",
		}
		result := client.DaysUntilExpiry(data)
		if result != -1 {
			t.Errorf("Expected DaysUntilExpiry with empty ExpiresAt to return -1, got %d", result)
		}
	})

	t.Run("invalid ExpiresAt format returns -1", func(t *testing.T) {
		data := &ActivationData{
			ExpiresAt: "not-a-valid-date",
		}
		result := client.DaysUntilExpiry(data)
		if result != -1 {
			t.Errorf("Expected DaysUntilExpiry with invalid date to return -1, got %d", result)
		}
	})

	t.Run("date 7 days from now returns approximately 7", func(t *testing.T) {
		futureDate := time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.DaysUntilExpiry(data)
		// Allow for some variance due to test execution time
		if result < 6 || result > 7 {
			t.Errorf("Expected DaysUntilExpiry to return approximately 7, got %d", result)
		}
	})

	t.Run("date 30 days from now returns approximately 30", func(t *testing.T) {
		futureDate := time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.DaysUntilExpiry(data)
		// Allow for some variance due to test execution time
		if result < 29 || result > 30 {
			t.Errorf("Expected DaysUntilExpiry to return approximately 30, got %d", result)
		}
	})

	t.Run("date 1 day ago returns negative value", func(t *testing.T) {
		pastDate := time.Now().Add(-24 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: pastDate,
		}
		result := client.DaysUntilExpiry(data)
		if result >= 0 {
			t.Errorf("Expected DaysUntilExpiry with past date to return negative, got %d", result)
		}
	})

	t.Run("date 7 days ago returns approximately -7", func(t *testing.T) {
		pastDate := time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: pastDate,
		}
		result := client.DaysUntilExpiry(data)
		// Allow for some variance due to test execution time
		if result > -6 || result < -8 {
			t.Errorf("Expected DaysUntilExpiry to return approximately -7, got %d", result)
		}
	})

	t.Run("date 12 hours from now returns 0", func(t *testing.T) {
		futureDate := time.Now().Add(12 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.DaysUntilExpiry(data)
		if result != 0 {
			t.Errorf("Expected DaysUntilExpiry with 12 hours remaining to return 0, got %d", result)
		}
	})

	t.Run("date 36 hours from now returns 1", func(t *testing.T) {
		futureDate := time.Now().Add(36 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.DaysUntilExpiry(data)
		if result != 1 {
			t.Errorf("Expected DaysUntilExpiry with 36 hours remaining to return 1, got %d", result)
		}
	})

	t.Run("expiring soon detection (within 7 days)", func(t *testing.T) {
		// Test that we can detect expiring soon condition
		futureDate := time.Now().Add(5 * 24 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.DaysUntilExpiry(data)
		isExpiringSoon := result >= 0 && result <= 7
		if !isExpiringSoon {
			t.Errorf("Expected DaysUntilExpiry to indicate expiring soon (0-7 days), got %d", result)
		}
	})

	t.Run("not expiring soon (more than 7 days)", func(t *testing.T) {
		futureDate := time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.DaysUntilExpiry(data)
		isExpiringSoon := result >= 0 && result <= 7
		if isExpiringSoon {
			t.Errorf("Expected DaysUntilExpiry to indicate NOT expiring soon (>7 days), got %d", result)
		}
	})

	t.Run("different timezone formats work correctly", func(t *testing.T) {
		// Test with UTC timezone
		futureDate := time.Now().Add(10 * 24 * time.Hour).UTC().Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}
		result := client.DaysUntilExpiry(data)
		if result < 9 || result > 10 {
			t.Errorf("Expected DaysUntilExpiry with UTC format to return approximately 10, got %d", result)
		}
	})

	t.Run("typical license expiry date format", func(t *testing.T) {
		// Test with the format used in the design document
		data := &ActivationData{
			ExpiresAt: "2099-12-31T23:59:59+08:00",
		}
		result := client.DaysUntilExpiry(data)
		// Should be a large positive number
		if result <= 0 {
			t.Errorf("Expected DaysUntilExpiry with far future date to return positive, got %d", result)
		}
	})
}

// TestIsExpiredAndDaysUntilExpiryConsistency tests that IsExpired and DaysUntilExpiry are consistent
func TestIsExpiredAndDaysUntilExpiryConsistency(t *testing.T) {
	client := NewClient()

	t.Run("expired data: IsExpired=true and DaysUntilExpiry<0", func(t *testing.T) {
		pastDate := time.Now().Add(-7 * 24 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: pastDate,
		}

		isExpired := client.IsExpired(data)
		daysUntil := client.DaysUntilExpiry(data)

		if !isExpired {
			t.Error("Expected IsExpired to return true for past date")
		}
		if daysUntil >= 0 {
			t.Errorf("Expected DaysUntilExpiry to return negative for past date, got %d", daysUntil)
		}
	})

	t.Run("valid data: IsExpired=false and DaysUntilExpiry>0", func(t *testing.T) {
		futureDate := time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339)
		data := &ActivationData{
			ExpiresAt: futureDate,
		}

		isExpired := client.IsExpired(data)
		daysUntil := client.DaysUntilExpiry(data)

		if isExpired {
			t.Error("Expected IsExpired to return false for future date")
		}
		if daysUntil <= 0 {
			t.Errorf("Expected DaysUntilExpiry to return positive for future date, got %d", daysUntil)
		}
	})

	t.Run("nil data: both indicate expired", func(t *testing.T) {
		isExpired := client.IsExpired(nil)
		daysUntil := client.DaysUntilExpiry(nil)

		if !isExpired {
			t.Error("Expected IsExpired(nil) to return true")
		}
		if daysUntil != -1 {
			t.Errorf("Expected DaysUntilExpiry(nil) to return -1, got %d", daysUntil)
		}
	})

	t.Run("empty ExpiresAt: both indicate expired", func(t *testing.T) {
		data := &ActivationData{
			ExpiresAt: "",
		}

		isExpired := client.IsExpired(data)
		daysUntil := client.DaysUntilExpiry(data)

		if !isExpired {
			t.Error("Expected IsExpired with empty ExpiresAt to return true")
		}
		if daysUntil != -1 {
			t.Errorf("Expected DaysUntilExpiry with empty ExpiresAt to return -1, got %d", daysUntil)
		}
	})
}


// TestGetEffectiveBaseURL tests the GetEffectiveBaseURL function
func TestGetEffectiveBaseURL(t *testing.T) {
	client := NewClient()

	tests := []struct {
		name     string
		data     *ActivationData
		expected string
	}{
		{
			name:     "nil data returns OpenAI default",
			data:     nil,
			expected: "https://api.openai.com/v1",
		},
		{
			name: "explicit base URL is used",
			data: &ActivationData{
				LLMType:    "deepseek",
				LLMBaseURL: "https://custom.api.com/v1",
			},
			expected: "https://custom.api.com/v1",
		},
		{
			name: "deepseek type with empty URL",
			data: &ActivationData{
				LLMType:    "deepseek",
				LLMBaseURL: "",
			},
			expected: "https://api.deepseek.com/v1",
		},
		{
			name: "DeepSeek type (uppercase) with empty URL",
			data: &ActivationData{
				LLMType:    "DeepSeek",
				LLMBaseURL: "",
			},
			expected: "https://api.deepseek.com/v1",
		},
		{
			name: "openai type with empty URL",
			data: &ActivationData{
				LLMType:    "openai",
				LLMBaseURL: "",
			},
			expected: "https://api.openai.com/v1",
		},
		{
			name: "anthropic type with empty URL",
			data: &ActivationData{
				LLMType:    "anthropic",
				LLMBaseURL: "",
			},
			expected: "https://api.anthropic.com/v1",
		},
		{
			name: "claude type with empty URL",
			data: &ActivationData{
				LLMType:    "claude",
				LLMBaseURL: "",
			},
			expected: "https://api.anthropic.com/v1",
		},
		{
			name: "moonshot type with empty URL",
			data: &ActivationData{
				LLMType:    "moonshot",
				LLMBaseURL: "",
			},
			expected: "https://api.moonshot.cn/v1",
		},
		{
			name: "kimi type with empty URL",
			data: &ActivationData{
				LLMType:    "kimi",
				LLMBaseURL: "",
			},
			expected: "https://api.moonshot.cn/v1",
		},
		{
			name: "qwen type with empty URL",
			data: &ActivationData{
				LLMType:    "qwen",
				LLMBaseURL: "",
			},
			expected: "https://dashscope.aliyuncs.com/compatible-mode/v1",
		},
		{
			name: "zhipu type with empty URL",
			data: &ActivationData{
				LLMType:    "zhipu",
				LLMBaseURL: "",
			},
			expected: "https://open.bigmodel.cn/api/paas/v4",
		},
		{
			name: "unknown type with empty URL defaults to OpenAI",
			data: &ActivationData{
				LLMType:    "unknown",
				LLMBaseURL: "",
			},
			expected: "https://api.openai.com/v1",
		},
		{
			name: "empty type with empty URL defaults to OpenAI",
			data: &ActivationData{
				LLMType:    "",
				LLMBaseURL: "",
			},
			expected: "https://api.openai.com/v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := client.GetEffectiveBaseURL(tt.data)
			if result != tt.expected {
				t.Errorf("GetEffectiveBaseURL() = %q, want %q", result, tt.expected)
			}
		})
	}
}
