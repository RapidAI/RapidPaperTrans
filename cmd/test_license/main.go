// Package main provides a test program for license activation and decryption
package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"latex-translator/internal/config"
	"latex-translator/internal/license"
)

func main() {
	fmt.Println("=== 授权系统测试程序 ===")
	fmt.Println()

	// 显示设备ID
	deviceID := config.GetDeviceID()
	fmt.Println("设备信息:")
	fmt.Printf("  设备ID: %s\n", deviceID)
	fmt.Println("  (此ID用于绑定授权到当前设备)")
	fmt.Println()

	// 创建授权客户端
	client := license.NewClient()

	// 测试邮箱
	email := "znsoft@163.com"

	// 步骤1: 申请序列号
	fmt.Println("步骤1: 申请序列号")
	fmt.Printf("邮箱: %s\n", email)
	fmt.Println("正在发送申请...")

	snResult, err := client.RequestSN(email)
	if err != nil {
		fmt.Printf("申请序列号结果: %v\n", err)
		fmt.Println("(如果邮箱已存在，这是正常的)")
	} else {
		fmt.Printf("申请结果: %s\n", snResult.Message)
		if snResult.SerialNumber != "" {
			fmt.Printf("获得序列号: %s\n", snResult.SerialNumber)
		}
	}
	fmt.Println()

	// 步骤2: 从命令行参数获取序列号，或使用申请返回的序列号
	var serialNumber string
	if len(os.Args) > 1 {
		serialNumber = os.Args[1]
		fmt.Printf("使用命令行参数序列号: %s\n", serialNumber)
	} else if snResult != nil && snResult.SerialNumber != "" {
		serialNumber = snResult.SerialNumber
		fmt.Printf("使用申请返回的序列号: %s\n", serialNumber)
	} else {
		fmt.Println("步骤2: 激活序列号")
		fmt.Println("用法: test_license.exe <序列号>")
		fmt.Println("例如: test_license.exe ABCD-1234-EFGH-5678")
		fmt.Println()
		fmt.Println("请通过命令行参数提供序列号")
		return
	}

	// 验证序列号格式
	fmt.Println()
	fmt.Println("验证序列号格式...")
	if !client.ValidateSerialNumber(serialNumber) {
		fmt.Println("序列号格式无效")
		return
	}
	fmt.Println("序列号格式有效 ✓")

	// 激活
	fmt.Println()
	fmt.Println("正在激活...")
	response, activationData, err := client.Activate(serialNumber)
	if err != nil {
		fmt.Printf("激活失败: %v\n", err)
		if response != nil {
			fmt.Printf("错误代码: %s\n", response.Code)
			fmt.Printf("错误信息: %s\n", response.Message)
		}
		return
	}

	fmt.Println("激活成功 ✓")
	fmt.Println()

	// 步骤3: 显示解密后的授权信息
	fmt.Println("步骤3: 解密后的授权信息")
	fmt.Println("----------------------------------------")
	fmt.Printf("响应状态: %v\n", response.Success)
	fmt.Printf("过期时间: %s\n", response.ExpiresAt)

	if activationData != nil {
		data := activationData
		fmt.Println()
		fmt.Println("LLM 配置:")
		fmt.Printf("  类型: %s\n", data.LLMType)
		fmt.Printf("  Base URL: %s\n", data.LLMBaseURL)
		if len(data.LLMAPIKey) > 12 {
			fmt.Printf("  API Key: %s***%s\n", data.LLMAPIKey[:8], data.LLMAPIKey[len(data.LLMAPIKey)-4:])
		} else {
			fmt.Printf("  API Key: %s\n", data.LLMAPIKey)
		}
		fmt.Printf("  模型: %s\n", data.LLMModel)
		fmt.Printf("  开始日期: %s\n", data.LLMStartDate)
		fmt.Printf("  结束日期: %s\n", data.LLMEndDate)

		fmt.Println()
		fmt.Println("搜索配置:")
		fmt.Printf("  类型: %s\n", data.SearchType)
		if len(data.SearchAPIKey) > 8 {
			fmt.Printf("  API Key: %s***\n", data.SearchAPIKey[:8])
		} else if data.SearchAPIKey != "" {
			fmt.Printf("  API Key: %s\n", data.SearchAPIKey)
		} else {
			fmt.Printf("  API Key: (未配置)\n")
		}
		fmt.Printf("  开始日期: %s\n", data.SearchStartDate)
		fmt.Printf("  结束日期: %s\n", data.SearchEndDate)

		fmt.Println()
		fmt.Println("授权信息:")
		fmt.Printf("  过期时间: %s\n", data.ExpiresAt)
		fmt.Printf("  激活时间: %s\n", data.ActivatedAt)
		if data.DailyAnalysis == 0 {
			fmt.Printf("  每日分析次数: 无限制\n")
		} else {
			fmt.Printf("  每日分析次数: %d 次/天\n", data.DailyAnalysis)
		}

		// 检查过期状态
		fmt.Println()
		fmt.Println("授权状态:")
		if client.IsExpired(data) {
			fmt.Println("  状态: 已过期 ✗")
		} else {
			fmt.Println("  状态: 有效 ✓")
		}
		days := client.DaysUntilExpiry(data)
		if days >= 0 {
			fmt.Printf("  剩余天数: %d 天\n", days)
			if days <= 7 {
				fmt.Println("  警告: 授权即将过期!")
			}
		} else {
			fmt.Printf("  已过期: %d 天\n", -days)
		}
	}
	fmt.Println("----------------------------------------")

	// 步骤4: 测试加密存储到系统配置目录
	fmt.Println()
	fmt.Println("步骤4: 测试加密存储到系统配置目录")

	// 使用真实的系统配置目录
	mgr, err := config.NewConfigManager("")
	if err != nil {
		log.Fatalf("创建配置管理器失败: %v", err)
	}

	fmt.Printf("配置文件路径: %s\n", mgr.GetConfigPath())

	// 设置工作模式
	if err := mgr.SetWorkMode(license.WorkModeCommercial); err != nil {
		log.Fatalf("设置工作模式失败: %v", err)
	}

	// 构建 LicenseInfo
	licenseInfo := &license.LicenseInfo{
		WorkMode:       license.WorkModeCommercial,
		SerialNumber:   serialNumber,
		ActivationData: activationData,
		ActivatedAt:    time.Now(),
	}

	// 保存授权信息（会自动加密）
	fmt.Println("正在加密并保存授权信息...")
	if err := mgr.SetLicenseInfo(licenseInfo); err != nil {
		log.Fatalf("保存授权信息失败: %v", err)
	}
	fmt.Println("保存成功 ✓")

	// 读取原始文件内容
	rawData, _ := os.ReadFile(mgr.GetConfigPath())
	fmt.Println()
	fmt.Println("加密后的配置文件内容:")
	fmt.Println(string(rawData))

	// 验证 API Key 不在明文中
	if activationData != nil && contains(string(rawData), activationData.LLMAPIKey) {
		fmt.Println("警告: API Key 以明文存储!")
	} else {
		fmt.Println("验证: API Key 已加密存储 ✓")
	}

	// 步骤5: 测试解密读取
	fmt.Println()
	fmt.Println("步骤5: 测试解密读取")

	// 创建新的管理器来模拟重新加载
	mgr2, err := config.NewConfigManager("")
	if err != nil {
		log.Fatalf("创建配置管理器失败: %v", err)
	}
	loadedInfo := mgr2.GetLicenseInfo()

	if loadedInfo == nil {
		fmt.Println("解密失败: 无法读取授权信息")
		return
	}

	fmt.Println("解密成功 ✓")
	fmt.Printf("序列号: %s\n", loadedInfo.SerialNumber)
	if loadedInfo.ActivationData != nil && len(loadedInfo.ActivationData.LLMAPIKey) > 12 {
		fmt.Printf("API Key: %s***%s\n",
			loadedInfo.ActivationData.LLMAPIKey[:8],
			loadedInfo.ActivationData.LLMAPIKey[len(loadedInfo.ActivationData.LLMAPIKey)-4:])
	}

	// 验证数据一致性
	fmt.Println()
	fmt.Println("步骤6: 验证数据一致性")
	if loadedInfo.SerialNumber == licenseInfo.SerialNumber {
		fmt.Println("序列号一致 ✓")
	} else {
		fmt.Println("序列号不一致 ✗")
	}
	if loadedInfo.ActivationData != nil && licenseInfo.ActivationData != nil {
		if loadedInfo.ActivationData.LLMAPIKey == licenseInfo.ActivationData.LLMAPIKey {
			fmt.Println("API Key 一致 ✓")
		} else {
			fmt.Println("API Key 不一致 ✗")
		}
		if loadedInfo.ActivationData.DailyAnalysis == licenseInfo.ActivationData.DailyAnalysis {
			fmt.Println("每日分析次数一致 ✓")
		} else {
			fmt.Println("每日分析次数不一致 ✗")
		}
	}

	// 步骤7: 测试 HasValidLicense
	fmt.Println()
	fmt.Println("步骤7: 测试授权有效性检查")
	if mgr2.HasValidLicense() {
		fmt.Println("HasValidLicense: true ✓")
	} else {
		fmt.Println("HasValidLicense: false (可能已过期)")
	}

	status := mgr2.GetWorkModeStatus()
	fmt.Printf("工作模式: %s\n", status.WorkMode)
	fmt.Printf("需要选择模式: %v\n", status.NeedsSelection)
	fmt.Printf("有效授权: %v\n", status.HasValidLicense)
	fmt.Printf("授权过期: %v\n", status.LicenseExpired)
	fmt.Printf("即将过期: %v\n", status.LicenseExpiringSoon)
	fmt.Printf("剩余天数: %d\n", status.DaysUntilExpiry)

	fmt.Println()
	fmt.Println("=== 测试完成 ===")
	fmt.Println()
	fmt.Println("注意: 授权信息已绑定到当前设备")
	fmt.Printf("设备ID: %s\n", deviceID)
	fmt.Println("如果将配置文件复制到其他设备，将无法解密授权信息")
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
