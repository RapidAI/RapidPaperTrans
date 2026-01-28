@echo off
REM LaTeX 翻译器 - Windows 构建脚本
REM 用于编译生成可执行文件和安装程序

echo ========================================
echo   LaTeX 翻译器 - 构建脚本
echo ========================================
echo.

REM 解析命令行参数
set BUILD_INSTALLER=1
set SKIP_BUILD=0

:parse_args
if "%~1"=="" goto end_parse
if /i "%~1"=="--no-installer" set BUILD_INSTALLER=0
if /i "%~1"=="-n" set BUILD_INSTALLER=0
if /i "%~1"=="--skip-build" set SKIP_BUILD=1
shift
goto parse_args
:end_parse

REM 检查是否在正确的目录
if not exist "wails.json" (
    echo 错误: 请在 latex-translator 目录下运行此脚本
    pause
    exit /b 1
)

REM 如果跳过构建，直接跳到打包步骤
if %SKIP_BUILD%==1 goto create_installer

REM 检查 wails 是否安装
where wails >nul 2>nul
if %errorlevel% neq 0 (
    echo 错误: 未找到 wails 命令
    echo 请先安装 Wails: go install github.com/wailsapp/wails/v2/cmd/wails@latest
    pause
    exit /b 1
)

REM 检查 Go 是否安装
where go >nul 2>nul
if %errorlevel% neq 0 (
    echo 错误: 未找到 go 命令
    echo 请先安装 Go: https://golang.org/dl/
    pause
    exit /b 1
)

REM 检查 Node.js 是否安装
where node >nul 2>nul
if %errorlevel% neq 0 (
    echo 错误: 未找到 node 命令
    echo 请先安装 Node.js: https://nodejs.org/
    pause
    exit /b 1
)

echo [1/3] 安装前端依赖...
cd frontend
call npm install
if %errorlevel% neq 0 (
    echo 错误: 前端依赖安装失败
    cd ..
    pause
    exit /b 1
)
cd ..

echo.
echo [2/3] 构建前端...
cd frontend
call npm run build
if %errorlevel% neq 0 (
    echo 错误: 前端构建失败
    cd ..
    pause
    exit /b 1
)
cd ..

echo.
echo [3/3] 构建 Windows 可执行文件...
wails build -clean -platform windows/amd64
if %errorlevel% neq 0 (
    echo 错误: Wails 构建失败
    pause
    exit /b 1
)

:create_installer
REM 检查是否需要创建安装程序
if %BUILD_INSTALLER%==0 goto show_result

echo.
echo [4/4] 创建 NSIS 安装程序...

REM 检查 NSIS 是否安装
where makensis >nul 2>nul
if %errorlevel% neq 0 (
    echo 警告: 未找到 makensis 命令
    echo 请安装 NSIS: https://nsis.sourceforge.io/Download
    echo 或将 NSIS 添加到系统 PATH 环境变量
    echo.
    echo 跳过安装程序创建...
    goto show_result
)

REM 检查可执行文件是否存在
if not exist "build\bin\latex-translator.exe" (
    echo 错误: 未找到 build\bin\latex-translator.exe
    echo 请先运行构建步骤
    pause
    exit /b 1
)

REM 创建 dist 目录
if not exist "dist" mkdir dist

REM 运行 NSIS 编译
echo 正在编译安装程序...
pushd build\windows\installer
makensis installer.nsi
if %errorlevel% neq 0 (
    echo 错误: NSIS 编译失败
    popd
    pause
    exit /b 1
)
popd

echo.
echo 安装程序创建成功!
echo 安装程序位置: dist\latex-translator-setup.exe
echo.

:show_result

echo.
echo ========================================
echo   构建完成!
echo ========================================
echo.
echo 可执行文件位置: build\bin\latex-translator.exe

if %BUILD_INSTALLER%==1 (
    if exist "dist\latex-translator-setup.exe" (
        echo 安装程序位置: dist\latex-translator-setup.exe
    )
)
echo.

REM 显示文件信息
if exist "build\bin\latex-translator.exe" (
    echo 文件大小:
    for %%A in ("build\bin\latex-translator.exe") do echo   可执行文件: %%~zA bytes
)

if %BUILD_INSTALLER%==1 (
    if exist "dist\latex-translator-setup.exe" (
        for %%A in ("dist\latex-translator-setup.exe") do echo   安装程序: %%~zA bytes
    )
)

echo.
echo ----------------------------------------
echo 使用说明:
echo   build.cmd              - 构建并创建安装程序
echo   build.cmd --no-installer - 仅构建可执行文件
echo   build.cmd -n           - 同上（简写）
echo   build.cmd --skip-build - 仅创建安装程序（跳过构建）
echo ----------------------------------------
echo.