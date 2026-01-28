# GitHub 分享功能设置指南

本指南将帮助你设置 GitHub 分享功能，让你可以将翻译结果分享到 GitHub，并搜索下载其他人分享的翻译。

## 为什么需要 GitHub 分享？

- **避免重复翻译**：搜索已有的翻译，节省时间和 API 费用
- **分享成果**：将你的翻译分享给其他人使用
- **协作改进**：利用 GitHub 的版本控制和协作功能改进翻译质量
- **备份存储**：将翻译结果安全地存储在 GitHub 上

## 设置步骤

### 第一步：创建 GitHub 仓库

1. 登录你的 GitHub 账号
2. 点击右上角的 "+" 按钮，选择 "New repository"
3. 填写仓库信息：
   - **Repository name**：例如 `translated-papers` 或 `chinese-papers`
   - **Description**：例如 "AI 翻译的学术论文中文版"
   - **Public**：选择公开（这样其他人可以搜索和下载）
   - 不需要勾选 "Initialize this repository with a README"
4. 点击 "Create repository"

### 第二步：生成 GitHub Personal Access Token

1. 点击右上角头像，选择 "Settings"
2. 在左侧菜单最底部，点击 "Developer settings"
3. 点击 "Personal access tokens" > "Tokens (classic)"
4. 点击 "Generate new token" > "Generate new token (classic)"
5. 填写 Token 信息：
   - **Note**：例如 "LaTeX Translator"
   - **Expiration**：建议选择 "No expiration" 或 "1 year"
   - **Select scopes**：勾选 `repo`（完整的仓库访问权限）
6. 点击 "Generate token"
7. **重要**：复制生成的 token（格式类似 `ghp_xxxxxxxxxxxx`）
   - Token 只显示一次，请妥善保存
   - 如果丢失，需要重新生成

### 第三步：在应用中配置

1. 打开 LaTeX Translator 应用
2. 点击右上角的 ⚙️ 设置按钮
3. 滚动到 "GitHub 设置" 部分
4. 填写以下信息：
   - **GitHub Token**：粘贴刚才复制的 token
   - **GitHub Owner**：你的 GitHub 用户名（例如：`your-username`）
   - **GitHub Repo**：仓库名称（例如：`translated-papers`）
5. 点击 "测试连接" 按钮验证配置
   - 如果显示 ✅ 连接成功，说明配置正确
   - 如果显示错误，请检查 token 和仓库信息
6. 点击 "保存" 按钮

## 使用功能

### 搜索已翻译的论文

1. 在主界面顶部的搜索框中输入 arXiv ID
   - 例如：`2405.04304` 或 `1706.03762`
2. 点击 🔍 搜索按钮
3. 如果找到翻译：
   - 会显示可用的文件（中文 PDF、双语 PDF、LaTeX 源码）
   - 点击确认后选择保存目录
   - 文件会自动下载到指定目录

### 分享你的翻译

1. 完成论文翻译后，点击 "分享到 GitHub" 按钮
2. 选择要上传的文件：
   - ✅ 中文 PDF：纯中文版本
   - ✅ 双语对照 PDF：中英文对照版本
   - ✅ LaTeX 源码：翻译后的 LaTeX 源文件
3. 如果文件已存在，会提示是否覆盖
4. 点击 "确认分享"
5. 等待上传完成

## 文件命名规范

上传到 GitHub 的文件会自动按以下格式命名：

- 中文 PDF：`{arxiv_id}_cn.pdf`
  - 例如：`2405.04304_cn.pdf`
- 双语 PDF：`{arxiv_id}_bilingual.pdf`
  - 例如：`2405.04304_bilingual.pdf`
- LaTeX 源码：`{arxiv_id}_latex.zip`
  - 例如：`2405.04304_latex.zip`

**重要**：搜索功能依赖这个命名规范，请不要手动修改文件名。

## 常见问题

### Q1: 测试连接失败，显示 "Token 无效"

**解决方法**：
1. 确认 Token 是否正确复制（包括 `ghp_` 前缀）
2. 检查 Token 是否有 `repo` 权限
3. 确认 Token 没有过期
4. 尝试重新生成 Token

### Q2: 上传失败，显示 "仓库不存在"

**解决方法**：
1. 确认 Owner 和 Repo 名称拼写正确
2. 确认仓库是公开的
3. 在浏览器中访问 `https://github.com/{owner}/{repo}` 确认仓库存在

### Q3: 搜索不到已上传的文件

**解决方法**：
1. 确认文件已成功上传到仓库根目录
2. 检查文件命名是否符合规范
3. 确认 arXiv ID 输入正确
4. 刷新 GitHub 仓库页面确认文件存在

### Q4: 可以使用别人的仓库吗？

**可以**：
- 搜索功能不需要 Token，可以搜索任何公开仓库
- 在设置中修改 Owner 和 Repo 即可
- 但上传功能需要对应仓库的写权限

### Q5: Token 安全吗？

**安全措施**：
- Token 只存储在本地配置文件中
- 不会被上传或分享到任何地方
- 建议使用最小权限（只需要 `repo`）
- 可以随时在 GitHub 设置中撤销 Token
- 不要将 Token 分享给他人

### Q6: 可以批量上传吗？

**当前版本**：
- 每次翻译完成后需要手动点击分享
- 可以选择上传部分或全部文件

**未来计划**：
- 支持自动上传选项
- 支持批量管理已翻译的论文

## 高级用法

### 组织仓库结构

虽然应用会将文件上传到根目录，但你可以手动在 GitHub 上组织文件：

```
translated-papers/
├── README.md
├── machine-learning/
│   ├── 2405.04304_cn.pdf
│   └── 2405.04304_bilingual.pdf
├── computer-vision/
│   ├── 1706.03762_cn.pdf
│   └── 1706.03762_bilingual.pdf
└── nlp/
    ├── 1810.04805_cn.pdf
    └── 1810.04805_bilingual.pdf
```

**注意**：移动文件后，搜索功能将无法找到这些文件（因为它只搜索根目录）。

### 添加 README

在仓库中添加 README.md 文件，列出已翻译的论文：

```markdown
# 已翻译论文列表

## 机器学习

- [2405.04304](https://arxiv.org/abs/2405.04304) - 论文标题
  - [中文版](2405.04304_cn.pdf)
  - [双语版](2405.04304_bilingual.pdf)

## 计算机视觉

- [1706.03762](https://arxiv.org/abs/1706.03762) - Attention Is All You Need
  - [中文版](1706.03762_cn.pdf)
  - [双语版](1706.03762_bilingual.pdf)
```

### 协作翻译

1. 邀请其他人成为仓库的协作者
2. 他们可以：
   - 上传自己的翻译
   - 改进现有翻译
   - 提交 Pull Request
3. 利用 GitHub Issues 讨论翻译质量

## 推荐的仓库设置

### 仓库描述

```
AI-powered Chinese translations of academic papers from arXiv.
使用 AI 翻译的 arXiv 学术论文中文版。
```

### Topics（标签）

添加以下 topics 让其他人更容易找到：
- `arxiv`
- `translation`
- `chinese`
- `academic-papers`
- `machine-learning`
- `ai-translation`

### README 模板

```markdown
# 学术论文中文翻译

本仓库包含使用 [LaTeX Translator](https://github.com/RapidAI/RapidTrans) 翻译的学术论文中文版。

## 使用方法

1. 浏览文件列表找到你需要的论文
2. 或使用 LaTeX Translator 应用搜索 arXiv ID

## 贡献

欢迎贡献翻译！请确保：
- 使用 LaTeX Translator 进行翻译
- 遵循文件命名规范
- 翻译质量良好

## 许可证

翻译内容遵循原论文的许可证。
```

## 下一步

设置完成后，你可以：

1. ✅ 翻译一篇论文测试功能
2. ✅ 将翻译分享到 GitHub
3. ✅ 尝试搜索和下载
4. ✅ 邀请朋友使用你的仓库

## 获取帮助

如果遇到问题：
1. 查看 [GITHUB_SHARING.md](../GITHUB_SHARING.md) 了解详细功能说明
2. 查看 [README.md](../README.md) 了解应用的基本使用
3. 在 GitHub 上提交 Issue

---

祝你使用愉快！🎉
