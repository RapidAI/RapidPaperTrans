@echo off
REM LaTeX 翻译器 - Windows 构建脚本（无控制台窗口版本）

echo ========================================
echo   LaTeX 翻译器 - 构建脚本 (无控制台)
echo ========================================
echo.

REM 检查是否在正确的目录
if not exist "wails.json" (
    echo 错误: 请在 latex-translator 目录下运行此脚本
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
echo [3/3] 构建 Windows 可执行文件（无控制台窗口）...
wails build -platform windows/amd64 -ldflags "-H=windowsgui"
if %errorlevel% neq 0 (
    echo 错误: Wails 构建失败
    pause
    exit /b 1
)

echo.
echo ========================================
echo   构建完成!
echo ========================================
echo.
echo 可执行文件位置: build\bin\RapidPaperTrans.exe
echo.

REM 显示文件信息
if exist "build\bin\RapidPaperTrans.exe" (
    echo 文件大小:
    for %%A in ("build\bin\RapidPaperTrans.exe") do echo   %%~zA bytes
)

echo.
pause
