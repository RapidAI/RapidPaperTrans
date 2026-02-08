# 需求文档

## 简介

本功能实现应用程序启动时的工作模式选择机制。用户在首次启动或未选择工作模式时，需要选择"商业软件模式"或"开源软件模式"。商业软件模式通过 license.vantagedata.chat 服务器获取授权和 LLM 配置；开源软件模式则让用户手动配置 LLM 信息。

## 术语表

- **Mode_Selector**: 工作模式选择器，负责显示模式选择界面并处理用户选择
- **License_Client**: 授权客户端，负责与授权服务器通信，激活序列号并获取配置
- **Settings_Manager**: 设置管理器，负责存储和读取用户配置信息
- **LLM_Config_Dialog**: LLM 配置对话框，用于开源模式下用户手动配置 LLM 参数
- **Work_Mode**: 工作模式，包括商业软件模式（commercial）和开源软件模式（opensource）
- **Serial_Number**: 序列号，格式为 XXXX-XXXX-XXXX-XXXX 的授权码
- **Activation_Data**: 激活数据，从授权服务器获取的加密配置信息

## 需求

### 需求 1：启动模式检测

**用户故事：** 作为用户，我希望应用程序在启动时自动检测是否已选择工作模式，以便决定是否显示模式选择界面。

#### 验收标准

1. WHEN 应用程序启动 THEN Mode_Selector SHALL 检查本地存储的工作模式配置
2. IF 未找到工作模式配置 THEN Mode_Selector SHALL 显示模式选择界面
3. IF 已存在工作模式配置 THEN Mode_Selector SHALL 跳过模式选择并进入相应的验证流程
4. WHEN 检测到商业模式配置 THEN Mode_Selector SHALL 验证授权是否有效
5. WHEN 检测到开源模式配置 THEN Mode_Selector SHALL 验证 LLM 配置是否有效

### 需求 2：模式选择界面

**用户故事：** 作为用户，我希望看到清晰的模式选择界面，以便我能够选择适合自己的工作模式。

#### 验收标准

1. WHEN 显示模式选择界面 THEN Mode_Selector SHALL 展示两个选项：商业软件模式和开源软件模式
2. WHEN 用户选择商业软件模式 THEN Mode_Selector SHALL 显示序列号输入界面
3. WHEN 用户选择开源软件模式 THEN Mode_Selector SHALL 打开 LLM 配置对话框
4. THE Mode_Selector SHALL 提供每种模式的简要说明
5. THE Mode_Selector SHALL 在界面上清晰标识当前选择状态

### 需求 3：商业软件模式授权

**用户故事：** 作为商业用户，我希望通过序列号激活软件并自动获取 LLM 配置，以便快速开始使用。

#### 验收标准

1. WHEN 用户输入序列号 THEN License_Client SHALL 验证序列号格式（XXXX-XXXX-XXXX-XXXX）
2. IF 序列号格式无效 THEN License_Client SHALL 显示格式错误提示
3. WHEN 序列号格式有效 THEN License_Client SHALL 向 license.vantagedata.chat 发送激活请求
4. IF 激活成功 THEN License_Client SHALL 解密并存储配置数据
5. IF 激活失败 THEN License_Client SHALL 显示具体错误信息（如序列号无效、已过期、已禁用等）
6. WHEN 激活成功 THEN Settings_Manager SHALL 保存工作模式为商业模式
7. WHEN 激活成功 THEN Mode_Selector SHALL 进入软件主界面
8. THE License_Client SHALL 支持通过邮箱申请序列号功能

### 需求 4：开源软件模式配置

**用户故事：** 作为开源用户，我希望能够手动配置 LLM 参数，以便使用自己的 API 密钥。

#### 验收标准

1. WHEN 用户选择开源模式 THEN LLM_Config_Dialog SHALL 显示配置表单
2. THE LLM_Config_Dialog SHALL 包含以下配置项：API 密钥、API 基础 URL、模型名称
3. WHEN 用户填写配置后点击保存 THEN LLM_Config_Dialog SHALL 测试 API 连接
4. IF API 连接测试成功 THEN Settings_Manager SHALL 保存配置和工作模式
5. IF API 连接测试成功 THEN Mode_Selector SHALL 进入软件主界面
6. IF API 连接测试失败 THEN LLM_Config_Dialog SHALL 显示错误信息并保持对话框打开
7. THE LLM_Config_Dialog SHALL 提供"取消"按钮供用户退出

### 需求 5：退出确认

**用户故事：** 作为用户，我希望在取消配置时收到确认提示，以防止误操作导致退出。

#### 验收标准

1. WHEN 用户在 LLM 配置对话框点击"取消"按钮 THEN Mode_Selector SHALL 显示退出确认对话框
2. THE 退出确认对话框 SHALL 显示提示信息"确定要退出吗？"
3. IF 用户确认退出 THEN Mode_Selector SHALL 关闭应用程序
4. IF 用户取消退出 THEN Mode_Selector SHALL 返回 LLM 配置对话框
5. WHEN 用户在模式选择界面关闭窗口 THEN Mode_Selector SHALL 显示退出确认对话框

### 需求 6：配置持久化

**用户故事：** 作为用户，我希望我的模式选择和配置能够被保存，以便下次启动时无需重新配置。

#### 验收标准

1. WHEN 配置成功保存 THEN Settings_Manager SHALL 将工作模式存储到本地配置文件
2. WHEN 商业模式激活成功 THEN Settings_Manager SHALL 加密存储序列号和配置数据
3. WHEN 开源模式配置成功 THEN Settings_Manager SHALL 存储 LLM 配置参数
4. THE Settings_Manager SHALL 在应用启动时自动加载已保存的配置
5. IF 配置文件损坏或无法读取 THEN Settings_Manager SHALL 重新显示模式选择界面

### 需求 7：授权数据解密

**用户故事：** 作为系统，我需要能够解密从授权服务器获取的配置数据，以便应用 LLM 配置。

#### 验收标准

1. WHEN 收到加密的激活数据 THEN License_Client SHALL 使用 AES-256-GCM 算法解密
2. THE License_Client SHALL 使用 SHA-256(序列号) 作为解密密钥
3. WHEN 解密成功 THEN License_Client SHALL 解析 JSON 格式的配置数据
4. IF 解密失败 THEN License_Client SHALL 显示解密错误信息
5. THE License_Client SHALL 从解密数据中提取 LLM 配置（类型、URL、密钥、模型）

### 需求 8：授权有效性验证

**用户故事：** 作为系统，我需要验证商业授权的有效性，以确保用户使用有效的授权。

#### 验收标准

1. WHEN 应用启动且为商业模式 THEN License_Client SHALL 检查授权过期时间
2. IF 授权已过期 THEN License_Client SHALL 提示用户重新激活或切换到开源模式
3. WHEN 授权即将过期（7天内）THEN License_Client SHALL 显示续费提醒
4. THE License_Client SHALL 支持离线使用已激活的授权

### 需求 9：模式相关界面差异

**用户故事：** 作为用户，我希望界面能够根据工作模式显示相应的内容，以便获得清晰的使用体验。

#### 验收标准

1. WHILE 处于商业软件模式 THEN Settings_Manager SHALL 隐藏设置界面中的 LLM 配置选项
2. WHILE 处于开源软件模式 THEN Settings_Manager SHALL 显示完整的 LLM 配置选项
3. THE 设置界面 SHALL 根据当前工作模式动态调整显示内容

### 需求 10：商业授权信息展示

**用户故事：** 作为商业用户，我希望在"关于"界面查看我的授权信息，以便了解授权状态。

#### 验收标准

1. WHILE 处于商业软件模式 THEN 关于界面 SHALL 显示商业授权信息区域
2. THE 商业授权信息 SHALL 包含授权有效期（到期日期）
3. THE 商业授权信息 SHALL 包含每日可用次数限制
4. THE 商业授权信息 SHALL 包含已使用次数（如适用）
5. IF 授权即将过期 THEN 关于界面 SHALL 以醒目方式提示续费
6. WHILE 处于开源软件模式 THEN 关于界面 SHALL 不显示商业授权信息区域
