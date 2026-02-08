package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"latex-translator/internal/config"
	"latex-translator/internal/downloader"
	"latex-translator/internal/license"
	"latex-translator/internal/translator"
)

func main() {
	arxivID := "2409.01704"
	if len(os.Args) > 1 {
		arxivID = os.Args[1]
	}

	fmt.Println("=== 授权调试测试 ===")
	fmt.Println()

	// 1. 加载配置
	fmt.Println("1. 加载配置...")
	configMgr, err := config.NewConfigManager("")
	if err != nil {
		log.Fatalf("创建配置管理器失败: %v", err)
	}
	if err := configMgr.Load(); err != nil {
		log.Printf("加载配置失败: %v", err)
	}

	// 2. 检查工作模式
	fmt.Println()
	fmt.Println("2. 检查工作模式...")
	workMode := configMgr.GetWorkMode()
	fmt.Printf("   工作模式: %s\n", workMode)

	// 3. 检查授权信息
	fmt.Println()
	fmt.Println("3. 检查授权信息...")
	licenseInfo := configMgr.GetLicenseInfo()
	if licenseInfo == nil {
		fmt.Println("   授权信息: 无")
	} else {
		fmt.Printf("   序列号: %s\n", licenseInfo.SerialNumber)
		fmt.Printf("   工作模式: %s\n", licenseInfo.WorkMode)
		fmt.Printf("   激活时间: %s\n", licenseInfo.ActivatedAt)
		if licenseInfo.ActivationData != nil {
			ad := licenseInfo.ActivationData
			fmt.Printf("   LLM类型: %s\n", ad.LLMType)
			fmt.Printf("   LLM模型: %s\n", ad.LLMModel)
			fmt.Printf("   LLM BaseURL: %s\n", ad.LLMBaseURL)
			fmt.Printf("   LLM API Key长度: %d\n", len(ad.LLMAPIKey))
			if len(ad.LLMAPIKey) > 10 {
				fmt.Printf("   LLM API Key前10字符: %s...\n", ad.LLMAPIKey[:10])
			}
			fmt.Printf("   过期时间: %s\n", ad.ExpiresAt)
			
			// 检查是否过期
			client := license.NewClient()
			if client.IsExpired(ad) {
				fmt.Println("   状态: 已过期!")
			} else {
				days := client.DaysUntilExpiry(ad)
				fmt.Printf("   状态: 有效 (剩余 %d 天)\n", days)
			}
		} else {
			fmt.Println("   激活数据: 无")
		}
	}

	// 4. 检查配置中的 API 设置
	fmt.Println()
	fmt.Println("4. 检查配置中的 API 设置...")
	apiKey := configMgr.GetAPIKey()
	baseURL := configMgr.GetBaseURL()
	model := configMgr.GetModel()
	fmt.Printf("   API Key长度: %d\n", len(apiKey))
	if len(apiKey) > 10 {
		fmt.Printf("   API Key前10字符: %s...\n", apiKey[:10])
	}
	fmt.Printf("   Base URL: %s\n", baseURL)
	fmt.Printf("   Model: %s\n", model)

	// 5. 如果是商业模式，应用授权配置
	if workMode == license.WorkModeCommercial && licenseInfo != nil && licenseInfo.ActivationData != nil {
		fmt.Println()
		fmt.Println("5. 应用授权配置到配置管理器...")
		ad := licenseInfo.ActivationData
		
		// 使用 GetEffectiveBaseURL 获取有效的 Base URL
		client := license.NewClient()
		effectiveBaseURL := client.GetEffectiveBaseURL(ad)
		fmt.Printf("   原始 LLM BaseURL: %s\n", ad.LLMBaseURL)
		fmt.Printf("   有效 Base URL: %s\n", effectiveBaseURL)
		
		err := configMgr.UpdateConfig(
			ad.LLMAPIKey,
			effectiveBaseURL, // 使用有效的 Base URL
			ad.LLMModel,
			configMgr.GetContextWindow(),
			configMgr.GetDefaultCompiler(),
			configMgr.GetWorkDirectory(),
			configMgr.GetConcurrency(),
			configMgr.GetLibraryPageSize(),
			true,
		)
		if err != nil {
			log.Printf("   更新配置失败: %v", err)
		} else {
			fmt.Println("   配置已更新")
		}

		// 重新获取配置
		apiKey = configMgr.GetAPIKey()
		baseURL = configMgr.GetBaseURL()
		model = configMgr.GetModel()
		fmt.Printf("   更新后 API Key长度: %d\n", len(apiKey))
		fmt.Printf("   更新后 Base URL: %s\n", baseURL)
		fmt.Printf("   更新后 Model: %s\n", model)
	}

	// 6. 创建翻译器并测试连接
	fmt.Println()
	fmt.Println("6. 创建翻译器并测试连接...")
	trans := translator.NewTranslationEngineWithConfig(apiKey, model, baseURL, 30*time.Second, 1)
	fmt.Printf("   翻译器 API Key长度: %d\n", len(trans.GetAPIKey()))
	fmt.Printf("   翻译器 Model: %s\n", trans.GetModel())

	err = trans.TestConnection()
	if err != nil {
		fmt.Printf("   连接测试失败: %v\n", err)
	} else {
		fmt.Println("   连接测试成功!")
	}

	// 7. 下载并翻译测试
	fmt.Println()
	fmt.Printf("7. 下载 arXiv %s...\n", arxivID)
	
	workDir := "testdata/license_debug_test"
	os.MkdirAll(workDir, 0755)
	
	dl := downloader.NewSourceDownloader(workDir)
	sourceInfo, err := dl.DownloadByID(arxivID)
	if err != nil {
		log.Fatalf("下载失败: %v", err)
	}
	fmt.Printf("   下载完成: %s\n", sourceInfo.ExtractDir)

	// 解压
	sourceInfo, err = dl.ExtractZip(sourceInfo.ExtractDir)
	if err != nil {
		log.Fatalf("解压失败: %v", err)
	}
	fmt.Printf("   解压完成: %s\n", sourceInfo.ExtractDir)

	// 查找主文件
	mainTex, err := dl.FindMainTexFile(sourceInfo.ExtractDir)
	if err != nil {
		log.Fatalf("查找主文件失败: %v", err)
	}
	fmt.Printf("   主文件: %s\n", mainTex)

	// 读取一小段内容进行翻译测试
	fmt.Println()
	fmt.Println("8. 翻译测试...")
	testContent := `\section{Introduction}
This is a test paragraph for translation.
We want to verify that the API key works correctly.`

	result, err := trans.TranslateTeX(testContent)
	if err != nil {
		fmt.Printf("   翻译失败: %v\n", err)
	} else {
		fmt.Println("   翻译成功!")
		fmt.Printf("   结果:\n%s\n", result)
	}

	fmt.Println()
	fmt.Println("=== 测试完成 ===")
}
