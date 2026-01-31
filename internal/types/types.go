// Package types defines core data types and enums for the LaTeX translator application.
package types

// Config 应用配置
type Config struct {
	OpenAIAPIKey    string `json:"openai_api_key"`
	OpenAIBaseURL   string `json:"openai_base_url"`   // OpenAI 兼容 API 的 Base URL
	OpenAIModel     string `json:"openai_model"`
	ContextWindow   int    `json:"context_window"`    // 上下文窗口大小（tokens），用于 LaTeX 和 PDF 翻译的批次大小控制
	DefaultCompiler string `json:"default_compiler"`  // "pdflatex" 或 "xelatex"
	WorkDirectory   string `json:"work_directory"`
	LastInput       string `json:"last_input"`        // 最后一次输入的 ID/URL/路径
	InputHistory    []InputHistoryItem `json:"input_history"` // 输入历史记录
	Concurrency     int    `json:"concurrency"`       // 翻译并发数，用于 LaTeX 和 PDF 翻译的并发批次处理，默认为 3
	// GitHub 分享配置
	GitHubToken     string `json:"github_token"`      // GitHub Personal Access Token
	GitHubOwner     string `json:"github_owner"`      // GitHub 仓库所有者
	GitHubRepo      string `json:"github_repo"`       // GitHub 仓库名称
	// 浏览库配置
	LibraryPageSize int    `json:"library_page_size"` // 浏览库每页显示数量，默认20
	// 分享提示配置
	SharePromptEnabled bool `json:"share_prompt_enabled"` // 翻译完成后是否提示分享，默认true
}

// InputHistoryItem 输入历史记录项
type InputHistoryItem struct {
	Input     string `json:"input"`      // 输入内容（arXiv ID、URL 或文件路径）
	Timestamp int64  `json:"timestamp"`  // 时间戳（Unix 毫秒）
	Type      string `json:"type"`       // 类型：arxiv, url, zip, pdf
}

// PaperCategory AI论文类别
type PaperCategory struct {
	ID          string `json:"id"`          // 类别ID，用于文件名
	Name        string `json:"name"`        // 中文名称
	Description string `json:"description"` // 说明
}

// GetPaperCategories 返回所有AI论文类别
func GetPaperCategories() []PaperCategory {
	return []PaperCategory{
		{ID: "llm", Name: "大语言模型", Description: "GPT、LLaMA、Claude、Qwen等"},
		{ID: "cv", Name: "计算机视觉", Description: "图像识别、目标检测、分割等"},
		{ID: "multimodal", Name: "多模态", Description: "视觉-语言模型、CLIP、GPT-4V等"},
		{ID: "diffusion", Name: "扩散模型", Description: "Stable Diffusion、DALL-E等图像生成"},
		{ID: "rl", Name: "强化学习", Description: "RLHF、游戏AI、机器人控制"},
		{ID: "agent", Name: "AI智能体", Description: "AutoGPT、工具使用、规划推理"},
		{ID: "rag", Name: "检索增强", Description: "检索增强生成、知识库问答"},
		{ID: "nlp", Name: "自然语言处理", Description: "文本分类、NER、情感分析等"},
		{ID: "speech", Name: "语音处理", Description: "ASR、TTS、语音合成"},
		{ID: "graph", Name: "图神经网络", Description: "GNN、知识图谱、图学习"},
		{ID: "efficient", Name: "高效AI", Description: "模型压缩、量化、蒸馏、剪枝"},
		{ID: "safety", Name: "AI安全对齐", Description: "对齐、可解释性、红队测试"},
		{ID: "security", Name: "AI网络安全", Description: "漏洞检测、恶意代码、入侵检测"},
		{ID: "attack", Name: "对抗攻击", Description: "对抗样本、模型攻击、越狱"},
		{ID: "medical", Name: "医疗AI", Description: "医学影像、药物发现、临床诊断"},
		{ID: "robotics", Name: "机器人", Description: "具身智能、机器人学习、控制"},
		{ID: "theory", Name: "理论基础", Description: "学习理论、泛化、优化理论"},
		{ID: "accel", Name: "训练加速", Description: "分布式训练、并行计算、硬件"},
		{ID: "data", Name: "数据工程", Description: "数据增强、合成数据、标注"},
		{ID: "code", Name: "代码生成", Description: "代码生成、程序合成、软件工程"},
		{ID: "video", Name: "视频生成", Description: "Sora、视频理解、视频编辑"},
		{ID: "3d", Name: "3D生成", Description: "NeRF、3D重建、点云、高斯"},
		{ID: "world", Name: "世界模型", Description: "环境建模、物理仿真"},
		{ID: "moe", Name: "混合专家", Description: "MoE架构、稀疏模型"},
		{ID: "memory", Name: "记忆机制", Description: "长上下文、记忆网络、状态空间"},
		{ID: "reason", Name: "推理能力", Description: "CoT、逻辑推理、数学推理"},
		{ID: "retrieval", Name: "信息检索", Description: "搜索、向量检索、重排序"},
		{ID: "recommend", Name: "推荐系统", Description: "协同过滤、序列推荐、CTR"},
		{ID: "finance", Name: "金融AI", Description: "量化交易、风控、金融预测"},
		{ID: "auto", Name: "自动驾驶", Description: "感知、规划、端到端驾驶"},
		{ID: "bio", Name: "生物AI", Description: "蛋白质、基因组、AlphaFold"},
		{ID: "science", Name: "科学计算", Description: "AI4Science、物理模拟、材料"},
		{ID: "edge", Name: "边缘AI", Description: "端侧部署、移动端、IoT"},
		{ID: "federated", Name: "联邦学习", Description: "隐私保护、分布式学习"},
		{ID: "continual", Name: "持续学习", Description: "增量学习、灾难性遗忘"},
		{ID: "meta", Name: "元学习", Description: "小样本学习、学会学习"},
		{ID: "ssl", Name: "自监督学习", Description: "对比学习、掩码预训练"},
		{ID: "gan", Name: "生成对抗", Description: "GAN、图像生成、风格迁移"},
		{ID: "vae", Name: "变分自编码", Description: "VAE、隐空间、表示学习"},
		{ID: "attention", Name: "注意力机制", Description: "Transformer、线性注意力"},
		{ID: "arch", Name: "模型架构", Description: "新架构设计、Mamba、RWKV"},
		{ID: "benchmark", Name: "评测基准", Description: "数据集、评估方法、排行榜"},
		{ID: "survey", Name: "综述", Description: "领域综述、技术总结"},
		{ID: "infra", Name: "AI基础设施", Description: "框架、编译器、推理引擎"},
		{ID: "ethics", Name: "AI伦理", Description: "公平性、偏见、社会影响"},
		{ID: "human", Name: "人机交互", Description: "HCI、用户研究、界面设计"},
		{ID: "education", Name: "教育AI", Description: "智能教育、自适应学习"},
		{ID: "game", Name: "游戏AI", Description: "游戏智能、NPC、游戏生成"},
		{ID: "music", Name: "音乐AI", Description: "音乐生成、音频处理"},
		{ID: "art", Name: "艺术AI", Description: "AI艺术、创意生成"},
		{ID: "translation", Name: "机器翻译", Description: "NMT、同声传译、多语言"},
		{ID: "dialog", Name: "对话系统", Description: "聊天机器人、任务对话"},
		{ID: "qa", Name: "问答系统", Description: "阅读理解、知识问答"},
		{ID: "summarize", Name: "文本摘要", Description: "抽取式、生成式摘要"},
		{ID: "ocr", Name: "文字识别", Description: "OCR、文档理解、版面分析"},
		{ID: "face", Name: "人脸技术", Description: "人脸识别、表情、换脸"},
		{ID: "pose", Name: "姿态估计", Description: "人体姿态、手势识别"},
		{ID: "segment", Name: "图像分割", Description: "语义分割、实例分割、SAM"},
		{ID: "detect", Name: "目标检测", Description: "YOLO、检测器、小目标"},
		{ID: "track", Name: "目标跟踪", Description: "MOT、SOT、视频跟踪"},
		{ID: "depth", Name: "深度估计", Description: "单目深度、立体匹配"},
		{ID: "super", Name: "超分辨率", Description: "图像超分、视频超分"},
		{ID: "restore", Name: "图像修复", Description: "去噪、去模糊、修复"},
		{ID: "other", Name: "其他", Description: "其他AI相关论文"},
	}
}

// SourceType 源码类型枚举
type SourceType string

const (
	SourceTypeURL      SourceType = "url"
	SourceTypeArxivID  SourceType = "arxiv_id"
	SourceTypeLocalZip SourceType = "local_zip"
	SourceTypeLocalPDF SourceType = "local_pdf"
)

// SourceInfo 源码信息
type SourceInfo struct {
	SourceType  SourceType `json:"source_type"` // URL, ArxivID, LocalZip
	OriginalRef string     `json:"original_ref"`
	ExtractDir  string     `json:"extract_dir"`
	MainTexFile string     `json:"main_tex_file"`
	AllTexFiles []string   `json:"all_tex_files"`
}

// ProcessPhase 处理阶段枚举
type ProcessPhase string

const (
	PhaseIdle        ProcessPhase = "idle"
	PhaseDownloading ProcessPhase = "downloading"
	PhaseExtracting  ProcessPhase = "extracting"
	PhaseCompiling   ProcessPhase = "compiling"
	PhaseTranslating ProcessPhase = "translating"
	PhaseValidating  ProcessPhase = "validating"
	PhaseComplete    ProcessPhase = "complete"
	PhaseError       ProcessPhase = "error"
)

// Status 处理状态
type Status struct {
	Phase    ProcessPhase `json:"phase"`
	Progress int          `json:"progress"` // 0-100
	Message  string       `json:"message"`
	Error    string       `json:"error,omitempty"`
}

// ProcessResult 处理结果
type ProcessResult struct {
	OriginalPDFPath   string      `json:"original_pdf_path"`
	TranslatedPDFPath string      `json:"translated_pdf_path"`
	BilingualPDFPath  string      `json:"bilingual_pdf_path"` // 双语并排 PDF 路径
	SourceInfo        *SourceInfo `json:"source_info"`
	SourceID          string      `json:"source_id"` // arXiv ID 或 zip 文件名（不含扩展名）
}

// TranslationResult 翻译结果
type TranslationResult struct {
	OriginalContent   string `json:"original_content"`
	TranslatedContent string `json:"translated_content"`
	TokensUsed        int    `json:"tokens_used"`
}

// ValidationResult 语法验证结果
type ValidationResult struct {
	IsValid bool          `json:"is_valid"`
	Errors  []SyntaxError `json:"errors"`
}

// SyntaxError 语法错误
type SyntaxError struct {
	Line    int    `json:"line"`
	Column  int    `json:"column"`
	Message string `json:"message"`
	Type    string `json:"type"`
}

// CompileResult 编译结果
type CompileResult struct {
	Success  bool   `json:"success"`
	PDFPath  string `json:"pdf_path"`
	Log      string `json:"log"`
	ErrorMsg string `json:"error_msg,omitempty"`
}

// ErrorCode 错误代码枚举
type ErrorCode string

const (
	ErrNetwork      ErrorCode = "NETWORK_ERROR"
	ErrDownload     ErrorCode = "DOWNLOAD_ERROR"
	ErrExtract      ErrorCode = "EXTRACT_ERROR"
	ErrFileNotFound ErrorCode = "FILE_NOT_FOUND"
	ErrInvalidInput ErrorCode = "INVALID_INPUT"
	ErrAPICall      ErrorCode = "API_CALL_ERROR"
	ErrAPIRateLimit ErrorCode = "API_RATE_LIMIT"
	ErrCompile      ErrorCode = "COMPILE_ERROR"
	ErrConfig       ErrorCode = "CONFIG_ERROR"
	ErrInternal     ErrorCode = "INTERNAL_ERROR"
	ErrTranslation  ErrorCode = "TRANSLATION_ERROR"
)

// AppError 应用错误
type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
	Details string    `json:"details,omitempty"`
	Cause   error     `json:"-"`
}

// Error implements the error interface for AppError
func (e *AppError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// Unwrap returns the underlying cause of the error
func (e *AppError) Unwrap() error {
	return e.Cause
}

// NewAppError creates a new AppError with the given code, message, and optional cause
func NewAppError(code ErrorCode, message string, cause error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// NewAppErrorWithDetails creates a new AppError with details
func NewAppErrorWithDetails(code ErrorCode, message, details string, cause error) *AppError {
	return &AppError{
		Code:    code,
		Message: message,
		Details: details,
		Cause:   cause,
	}
}
