package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// ArxivEntry 表示一篇 arXiv 论文
type ArxivEntry struct {
	ArxivID  string `json:"arxiv_id"`
	Title    string `json:"title"`
	Abstract string `json:"abstract"`
}

// arXiv API 响应结构
type Feed struct {
	XMLName xml.Name `xml:"feed"`
	Entries []Entry  `xml:"entry"`
	Total   int      `xml:"totalResults"`
}

type Entry struct {
	ID      string  `xml:"id"`
	Title   string  `xml:"title"`
	Summary string  `xml:"summary"`
	Links   []Link  `xml:"link"`
}

type Link struct {
	Href  string `xml:"href,attr"`
	Type  string `xml:"type,attr"`
	Title string `xml:"title,attr"`
	Rel   string `xml:"rel,attr"`
}

// 经典 AI 搜索关键词列表
var searchQueries = []string{
	// 深度学习基础
	"deep learning neural network",
	"convolutional neural network CNN",
	"recurrent neural network LSTM",
	"transformer attention mechanism",
	"generative adversarial network GAN",
	"variational autoencoder VAE",
	"reinforcement learning deep",
	"graph neural network GNN",
	
	// 大语言模型
	"large language model LLM",
	"GPT language model",
	"BERT pre-training",
	"instruction tuning LLM",
	"prompt engineering",
	"chain of thought reasoning",
	"retrieval augmented generation RAG",
	"in-context learning",
	
	// 计算机视觉
	"image classification deep learning",
	"object detection neural network",
	"semantic segmentation deep",
	"diffusion model image generation",
	"vision transformer ViT",
	"contrastive learning visual",
	"image captioning neural",
	"video understanding deep learning",
	
	// 自然语言处理
	"natural language processing neural",
	"machine translation transformer",
	"text classification BERT",
	"named entity recognition deep",
	"question answering neural",
	"sentiment analysis deep learning",
	"text summarization neural",
	"dialogue system neural",
	
	// 优化与训练
	"batch normalization deep learning",
	"dropout regularization neural",
	"Adam optimizer deep learning",
	"learning rate schedule",
	"stochastic gradient descent",
	"transfer learning neural",
	"few-shot learning",
	"meta learning neural",
	"curriculum learning",
	
	// 前沿方向
	"multimodal learning vision language",
	"self-supervised learning",
	"federated learning privacy",
	"neural architecture search NAS",
	"knowledge distillation neural",
	"model compression quantization",
	"explainable AI neural network",
	"AI alignment safety",
	"continual learning neural",
	"zero-shot learning",
	
	// 特定模型架构
	"ResNet deep residual learning",
	"attention is all you need",
	"CLIP contrastive language image",
	"stable diffusion",
	"LLaMA language model",
	"mixture of experts MoE",
}

var (
	httpClient = &http.Client{
		Timeout: 30 * time.Second,
	}
	
	// 用于限制并发检查源码
	semaphore = make(chan struct{}, 5)
)

func main() {
	fmt.Println("开始从 arXiv 获取有 LaTeX 源码的 AI 相关经典论文...")
	fmt.Println("注意：只收集有 LaTeX 源码的论文\n")
	
	allEntries := make(map[string]ArxivEntry) // 用 map 去重
	var mu sync.Mutex
	targetCount := 1000
	
	// 加载已有的进度（如果存在）
	existingEntries := loadExistingEntries()
	for id, entry := range existingEntries {
		allEntries[id] = entry
	}
	if len(allEntries) > 0 {
		fmt.Printf("从已有文件加载了 %d 篇论文\n\n", len(allEntries))
	}
	
	for _, query := range searchQueries {
		mu.Lock()
		currentCount := len(allEntries)
		mu.Unlock()
		
		if currentCount >= targetCount {
			break
		}
		
		fmt.Printf("\n搜索: %s (当前: %d/%d)\n", query, currentCount, targetCount)
		
		// 每个查询获取多页结果
		for start := 0; start < 300 && currentCount < targetCount; start += 100 {
			entries, err := searchArxiv(query, start, 100)
			if err != nil {
				fmt.Printf("  搜索出错: %v\n", err)
				break
			}
			
			if len(entries) == 0 {
				fmt.Println("  没有更多结果")
				break
			}
			
			fmt.Printf("  获取到 %d 篇候选论文，检查源码...\n", len(entries))
			
			// 并发检查源码可用性
			var wg sync.WaitGroup
			for _, entry := range entries {
				mu.Lock()
				if _, exists := allEntries[entry.ArxivID]; exists {
					mu.Unlock()
					continue
				}
				mu.Unlock()
				
				wg.Add(1)
				go func(e ArxivEntry) {
					defer wg.Done()
					
					semaphore <- struct{}{} // 获取信号量
					defer func() { <-semaphore }() // 释放信号量
					
					if hasLatexSource(e.ArxivID) {
						mu.Lock()
						if _, exists := allEntries[e.ArxivID]; !exists {
							allEntries[e.ArxivID] = e
							fmt.Printf("  ✓ [%d] %s: %s\n", len(allEntries), e.ArxivID, truncate(e.Title, 50))
						}
						mu.Unlock()
					}
				}(entry)
			}
			wg.Wait()
			
			mu.Lock()
			currentCount = len(allEntries)
			mu.Unlock()
			
			// 定期保存进度
			if currentCount%50 == 0 {
				saveProgress(allEntries)
			}
			
			// arXiv API 限制：每 3 秒最多 1 次请求
			time.Sleep(3 * time.Second)
		}
	}
	
	fmt.Printf("\n\n共获取 %d 篇有 LaTeX 源码的论文\n", len(allEntries))
	
	// 最终保存
	saveProgress(allEntries)
	
	fmt.Println("\n完成!")
}

func loadExistingEntries() map[string]ArxivEntry {
	entries := make(map[string]ArxivEntry)
	
	data, err := os.ReadFile("arxiv_title.json")
	if err != nil {
		return entries
	}
	
	var list []ArxivEntry
	if err := json.Unmarshal(data, &list); err != nil {
		return entries
	}
	
	for _, e := range list {
		entries[e.ArxivID] = e
	}
	return entries
}

func saveProgress(allEntries map[string]ArxivEntry) {
	var ids []string
	var entriesList []ArxivEntry
	for id, entry := range allEntries {
		ids = append(ids, id)
		entriesList = append(entriesList, entry)
	}
	
	// 保存 arxiv_id.txt
	err := os.WriteFile("arxiv_id.txt", []byte(strings.Join(ids, "\n")), 0644)
	if err != nil {
		fmt.Printf("保存 arxiv_id.txt 失败: %v\n", err)
		return
	}
	
	// 保存 arxiv_title.json
	jsonData, err := json.MarshalIndent(entriesList, "", "  ")
	if err != nil {
		fmt.Printf("JSON 序列化失败: %v\n", err)
		return
	}
	
	err = os.WriteFile("arxiv_title.json", jsonData, 0644)
	if err != nil {
		fmt.Printf("保存 arxiv_title.json 失败: %v\n", err)
		return
	}
	
	fmt.Printf("  [进度已保存: %d 篇]\n", len(entriesList))
}

func searchArxiv(query string, start, maxResults int) ([]ArxivEntry, error) {
	// 构建 arXiv API URL
	// 搜索 cs.AI, cs.LG, cs.CL, cs.CV, stat.ML 分类
	searchQuery := fmt.Sprintf("all:%s AND (cat:cs.AI OR cat:cs.LG OR cat:cs.CL OR cat:cs.CV OR cat:stat.ML)", query)
	
	params := url.Values{}
	params.Set("search_query", searchQuery)
	params.Set("start", fmt.Sprintf("%d", start))
	params.Set("max_results", fmt.Sprintf("%d", maxResults))
	params.Set("sortBy", "relevance")
	params.Set("sortOrder", "descending")
	
	apiURL := "http://export.arxiv.org/api/query?" + params.Encode()
	
	resp, err := httpClient.Get(apiURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	var feed Feed
	err = xml.Unmarshal(body, &feed)
	if err != nil {
		return nil, err
	}
	
	var entries []ArxivEntry
	for _, e := range feed.Entries {
		arxivID := extractArxivID(e.ID)
		if arxivID == "" {
			continue
		}
		
		entries = append(entries, ArxivEntry{
			ArxivID:  arxivID,
			Title:    cleanText(e.Title),
			Abstract: cleanText(e.Summary),
		})
	}
	
	return entries, nil
}

// hasLatexSource 检查论文是否有 LaTeX 源码
// 通过 HEAD 请求检查 e-print 端点
func hasLatexSource(arxivID string) bool {
	// arXiv 源码下载地址
	sourceURL := fmt.Sprintf("https://arxiv.org/e-print/%s", arxivID)
	
	req, err := http.NewRequest("HEAD", sourceURL, nil)
	if err != nil {
		return false
	}
	
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	
	// 检查响应状态和内容类型
	if resp.StatusCode != http.StatusOK {
		return false
	}
	
	contentType := resp.Header.Get("Content-Type")
	// LaTeX 源码通常是 gzip 压缩的 tar 包
	// application/x-eprint-tar, application/gzip, application/x-gzip
	if strings.Contains(contentType, "gzip") || 
	   strings.Contains(contentType, "tar") ||
	   strings.Contains(contentType, "x-eprint") {
		return true
	}
	
	// 有些可能是单个 tex 文件
	if strings.Contains(contentType, "x-tex") ||
	   strings.Contains(contentType, "text/plain") {
		return true
	}
	
	// 排除纯 PDF（没有源码）
	if strings.Contains(contentType, "pdf") {
		return false
	}
	
	// 其他情况也可能有源码
	return resp.StatusCode == http.StatusOK
}

func extractArxivID(idURL string) string {
	// 从 URL 提取 arXiv ID
	// 例如: http://arxiv.org/abs/2301.00001v1 -> 2301.00001
	parts := strings.Split(idURL, "/abs/")
	if len(parts) < 2 {
		return ""
	}
	id := parts[1]
	// 移除版本号
	if idx := strings.Index(id, "v"); idx > 0 {
		id = id[:idx]
	}
	return id
}

func cleanText(text string) string {
	// 清理文本中的多余空白
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.TrimSpace(text)
	// 压缩多个空格
	for strings.Contains(text, "  ") {
		text = strings.ReplaceAll(text, "  ", " ")
	}
	return text
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
