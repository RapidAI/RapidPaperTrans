# 实现计划：启动模式选择

## 概述

本实现计划将启动模式选择功能分解为可执行的编码任务。实现顺序为：后端核心逻辑 → 前端界面 → 集成和测试。

## 任务

- [ ] 1. 实现授权客户端核心功能
  - [x] 1.1 创建 `internal/license/client.go` 文件，定义 LicenseClient 结构和基本类型
    - 定义 WorkMode、ActivationData、ActivationResponse、LicenseInfo 类型
    - 实现 NewClient 构造函数
    - _Requirements: 3.1, 7.1_
  
  - [x] 1.2 实现序列号格式验证函数 ValidateSerialNumber
    - 验证格式 XXXX-XXXX-XXXX-XXXX（X 为字母或数字）
    - _Requirements: 3.1, 3.2_
  
  - [ ]* 1.3 编写序列号格式验证的属性测试
    - **Property 2: 序列号格式验证**
    - **Validates: Requirements 3.1, 3.2**
  
  - [x] 1.4 实现 AES-256-GCM 解密函数 DecryptData
    - 使用 SHA-256(序列号) 派生密钥
    - Base64 解码 → 提取 nonce → 解密 → JSON 解析
    - _Requirements: 7.1, 7.2, 7.3, 7.4, 7.5_
  
  - [ ]* 1.5 编写解密往返的属性测试
    - **Property 3: 激活数据解密往返**
    - **Validates: Requirements 7.1, 7.2, 7.3**
  
  - [x] 1.6 实现激活请求函数 Activate
    - 向 license.vantagedata.chat 发送 POST 请求
    - 处理响应并解密数据
    - _Requirements: 3.3, 3.4, 3.5_
  
  - [x] 1.7 实现邮箱申请序列号函数 RequestSN
    - 向 /request-sn 端点发送请求
    - _Requirements: 3.8_
  
  - [x] 1.8 实现授权过期检测函数 IsExpired 和 DaysUntilExpiry
    - 解析过期时间并与当前时间比较
    - _Requirements: 8.1, 8.2, 8.3_
  
  - [ ]* 1.9 编写授权过期检测的属性测试
    - **Property 5: 授权过期检测**
    - **Validates: Requirements 8.1, 8.2**

- [ ] 2. 扩展设置管理器
  - [x] 2.1 扩展 `internal/settings/settings.go`，添加工作模式和授权信息字段
    - 扩展 LocalSettings 结构
    - 添加 GetWorkMode、SetWorkMode、GetLicenseInfo、SetLicenseInfo 方法
    - _Requirements: 6.1, 6.2, 6.3_
  
  - [x] 2.2 实现配置加载时的工作模式检测
    - 在 Load 方法中处理工作模式
    - _Requirements: 1.1, 1.2, 1.3_
  
  - [ ]* 2.3 编写配置持久化往返的属性测试
    - **Property 4: 配置持久化往返**
    - **Validates: Requirements 6.1, 6.2, 6.3, 6.4**
  
  - [x] 2.4 实现配置损坏检测和恢复
    - 检测 JSON 解析错误或无效数据
    - 损坏时重置配置
    - _Requirements: 6.5_
  
  - [ ]* 2.5 编写配置损坏恢复的属性测试
    - **Property 10: 配置损坏恢复**
    - **Validates: Requirements 6.5**

- [x] 3. Checkpoint - 确保后端核心功能测试通过
  - 运行所有单元测试和属性测试
  - 确保所有测试通过，如有问题请询问用户

- [x] 4. 扩展 App 绑定方法
  - [x] 4.1 在 `app.go` 中添加授权相关的 Wails 绑定方法
    - GetWorkMode、SetWorkMode
    - ActivateLicense、RequestSerialNumber
    - GetLicenseInfo、CheckLicenseValidity
    - _Requirements: 1.4, 1.5, 3.3, 3.4, 8.1, 8.2, 8.3_
  
  - [x] 4.2 修改 startup 方法，添加启动模式检测逻辑
    - 检查工作模式配置
    - 商业模式验证授权有效性
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5_
  
  - [x] 4.3 实现根据工作模式加载 LLM 配置
    - 商业模式从授权数据加载
    - 开源模式从用户配置加载
    - _Requirements: 3.4, 4.4_

- [x] 5. 实现前端模式选择界面
  - [x] 5.1 在 `frontend/index.html` 中添加模式选择模态框 HTML
    - 模式选择界面（商业/开源两个选项）
    - 序列号输入模态框
    - 邮箱申请区域
    - _Requirements: 2.1, 2.2, 2.4_
  
  - [x] 5.2 添加模式选择相关的 CSS 样式
    - 模式选项卡片样式
    - 序列号输入框样式
    - 错误提示样式
    - _Requirements: 2.5_
  
  - [x] 5.3 创建 `frontend/src/modeSelector.js` 模块
    - 初始化后端绑定
    - 实现 checkStartupMode 函数
    - 实现 showModeSelectionModal 函数
    - _Requirements: 1.1, 1.2, 2.1, 2.2, 2.3_
  
  - [x] 5.4 实现序列号激活流程
    - 格式验证
    - 调用后端激活
    - 处理成功/失败
    - _Requirements: 3.1, 3.2, 3.3, 3.4, 3.5, 3.6, 3.7_
  
  - [x] 5.5 实现邮箱申请序列号流程
    - 显示邮箱输入区域
    - 调用后端申请
    - 显示结果
    - _Requirements: 3.8_

- [x] 6. 实现开源模式配置流程
  - [x] 6.1 修改设置对话框，支持开源模式首次配置
    - 打开设置对话框时检测是否为首次配置
    - 首次配置时修改取消按钮行为
    - _Requirements: 4.1, 4.2, 4.7_
  
  - [x] 6.2 实现退出确认对话框
    - 点击取消时显示确认
    - 确认退出时关闭应用
    - 取消退出时返回配置
    - _Requirements: 5.1, 5.2, 5.3, 5.4, 5.5_
  
  - [x] 6.3 实现 API 连接测试后的模式保存
    - 测试成功后保存开源模式配置
    - 进入主界面
    - _Requirements: 4.3, 4.4, 4.5, 4.6_

- [ ] 7. 实现模式相关界面差异
  - [x] 7.1 修改设置界面，根据模式显示/隐藏 LLM 配置
    - 商业模式隐藏 LLM 配置区域
    - 开源模式显示 LLM 配置区域
    - _Requirements: 9.1, 9.2, 9.3_
  
  - [x] 7.2 修改关于界面，添加商业授权信息区域
    - 添加授权信息 HTML 结构
    - 显示有效期、每日限制
    - 即将过期时显示警告
    - _Requirements: 10.1, 10.2, 10.3, 10.4, 10.5, 10.6_
  
  - [x] 7.3 实现关于界面授权信息的动态更新
    - 调用 GetLicenseInfo 获取数据
    - 根据工作模式显示/隐藏区域
    - _Requirements: 10.1, 10.6_

- [x] 8. 集成和连接
  - [x] 8.1 在 `frontend/src/main.js` 中集成模式选择模块
    - 导入 modeSelector 模块
    - 在初始化时调用 checkStartupMode
    - _Requirements: 1.1, 1.2_
  
  - [x] 8.2 修改现有启动检查流程
    - 在 LaTeX 和 LLM 检查之前先检查工作模式
    - 根据模式决定是否检查 LLM 配置
    - _Requirements: 1.1, 1.4, 1.5_
  
  - [x] 8.3 确保模式切换后界面正确更新
    - 设置界面根据模式调整
    - 关于界面根据模式调整
    - _Requirements: 9.1, 9.2, 10.1, 10.6_

- [x] 9. Checkpoint - 确保所有功能集成测试通过
  - 测试完整的启动流程
  - 测试商业模式激活流程
  - 测试开源模式配置流程
  - 确保所有测试通过，如有问题请询问用户

- [ ] 10. 最终验证
  - [ ]* 10.1 编写集成测试
    - 测试无配置时显示模式选择
    - 测试商业模式激活成功进入主界面
    - 测试开源模式配置成功进入主界面
    - 测试过期授权的处理
    - _Requirements: 1.1-1.5, 3.1-3.7, 4.1-4.6_
  
  - [ ]* 10.2 编写 UI 交互测试
    - 测试模式选择界面交互
    - 测试序列号输入和验证
    - 测试退出确认对话框
    - _Requirements: 2.1-2.5, 5.1-5.5_

## 注意事项

- 标记为 `*` 的任务为可选任务，可以跳过以加快 MVP 开发
- 每个任务都引用了具体的需求以便追溯
- Checkpoint 任务用于确保增量验证
- 属性测试验证通用正确性属性
- 单元测试验证具体示例和边界情况
