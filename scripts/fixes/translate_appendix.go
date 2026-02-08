package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func translateWithDeepSeek(text string) (string, error) {
	apiKey := os.Getenv("DEEPSEEK_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("DEEPSEEK_API_KEY environment variable not set")
	}

	systemPrompt := `你是一个专业的学术论文翻译助手。请将以下 LaTeX 格式的英文学术论文附录翻译成中文。

翻译要求：
1. 保持所有 LaTeX 命令、公式、引用标签不变
2. 只翻译文本内容，不翻译数学公式
3. 保持专业术语的准确性
4. 保持原文的段落结构和格式
5. 翻译要流畅自然，符合中文学术写作习惯
6. 保留所有 \section, \subsection, \paragraph 等命令
7. 保留所有 \label, \ref, \citep, \cite 等引用命令
8. 保留所有表格和图表的 LaTeX 结构`

	reqBody := ChatRequest{
		Model: "deepseek-chat",
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: text},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequest("POST", "https://api.deepseek.com/v1/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var chatResp ChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from API")
	}

	return chatResp.Choices[0].Message.Content, nil
}

func main() {
	inputFile := "testdata/arxiv_test/2601.22156_extracted/appendix.tex"
	outputFile := "testdata/arxiv_test/2601.22156_extracted/appendix_translated.tex"

	// Read the file
	content, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	text := string(content)
	
	// Split into chunks if too long (max ~8000 tokens per chunk)
	const maxChunkSize = 15000 // characters
	var chunks []string
	
	if len(text) > maxChunkSize {
		// Split by sections
		sections := strings.Split(text, "\\section{")
		currentChunk := ""
		
		for i, section := range sections {
			if i == 0 {
				currentChunk = section
				continue
			}
			
			sectionText := "\\section{" + section
			if len(currentChunk)+len(sectionText) > maxChunkSize {
				chunks = append(chunks, currentChunk)
				currentChunk = sectionText
			} else {
				currentChunk += sectionText
			}
		}
		if currentChunk != "" {
			chunks = append(chunks, currentChunk)
		}
	} else {
		chunks = []string{text}
	}

	fmt.Printf("Translating %d chunks...\n", len(chunks))

	var translatedChunks []string
	for i, chunk := range chunks {
		fmt.Printf("Translating chunk %d/%d...\n", i+1, len(chunks))
		
		translated, err := translateWithDeepSeek(chunk)
		if err != nil {
			fmt.Printf("Error translating chunk %d: %v\n", i+1, err)
			return
		}
		
		translatedChunks = append(translatedChunks, translated)
		
		// Rate limiting
		if i < len(chunks)-1 {
			time.Sleep(2 * time.Second)
		}
	}

	// Combine chunks
	finalTranslation := strings.Join(translatedChunks, "\n\n")

	// Write output
	err = os.WriteFile(outputFile, []byte(finalTranslation), 0644)
	if err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		return
	}

	fmt.Printf("Translation complete! Output written to: %s\n", outputFile)
	fmt.Printf("Total characters: %d\n", len(finalTranslation))
}
