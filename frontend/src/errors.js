/**
 * 错误管理模块
 * 处理翻译过程中的错误记录、显示和重试
 */

// Error Management Modal elements
let errorsModal;
let btnErrors;
let errorsModalClose;
let btnErrorsClose;
let errorsList;
let errorsEmpty;
let btnClearAllErrors;
let btnExportErrors;
let btnExportErrorIDs;

// Backend bindings for error management
let ListErrors, RetryFromError, ClearError, ClearAllErrors, ExportErrorsToFile, ExportErrorIDsToFile;

/**
 * 初始化错误管理模块
 */
export function initErrorManagement(bindings) {
    // 保存后端绑定
    ListErrors = bindings.ListErrors;
    RetryFromError = bindings.RetryFromError;
    ClearError = bindings.ClearError;
    ClearAllErrors = bindings.ClearAllErrors;
    ExportErrorsToFile = bindings.ExportErrorsToFile;
    ExportErrorIDsToFile = bindings.ExportErrorIDsToFile;

    // 初始化 DOM 元素
    errorsModal = document.getElementById('errors-modal');
    btnErrors = document.getElementById('btn-errors');
    errorsModalClose = document.getElementById('errors-modal-close');
    btnErrorsClose = document.getElementById('btn-errors-close');
    errorsList = document.getElementById('errors-list');
    errorsEmpty = document.getElementById('errors-empty');
    btnClearAllErrors = document.getElementById('btn-clear-all-errors');
    btnExportErrors = document.getElementById('btn-export-errors');
    btnExportErrorIDs = document.getElementById('btn-export-error-ids');

    // 设置事件监听器
    if (btnErrors) {
        btnErrors.addEventListener('click', openErrorsModal);
    }
    if (errorsModalClose) {
        errorsModalClose.addEventListener('click', closeErrorsModal);
    }
    if (btnErrorsClose) {
        btnErrorsClose.addEventListener('click', closeErrorsModal);
    }
    if (btnClearAllErrors) {
        btnClearAllErrors.addEventListener('click', handleClearAllErrors);
    }
    if (btnExportErrors) {
        btnExportErrors.addEventListener('click', handleExportErrors);
    }
    if (btnExportErrorIDs) {
        btnExportErrorIDs.addEventListener('click', handleExportErrorIDs);
    }
    if (errorsModal) {
        errorsModal.addEventListener('mousedown', (e) => {
            if (e.target === errorsModal) {
                closeErrorsModal();
            }
        });
    }
}

/**
 * 打开错误管理模态框
 */
async function openErrorsModal() {
    if (!errorsModal) return;

    errorsModal.style.display = 'flex';
    await loadErrors();
}

/**
 * 关闭错误管理模态框
 */
function closeErrorsModal() {
    if (!errorsModal) return;
    errorsModal.style.display = 'none';
}

/**
 * 加载错误列表
 */
async function loadErrors() {
    if (!ListErrors) {
        console.error('ListErrors binding not available');
        return;
    }

    try {
        const errors = await ListErrors();
        displayErrors(errors);
    } catch (error) {
        console.error('Failed to load errors:', error);
        showToast('加载错误列表失败: ' + (error.message || error), 'error');
    }
}

/**
 * 显示错误列表
 */
function displayErrors(errors) {
    if (!errorsList || !errorsEmpty) return;

    // 清空现有列表
    errorsList.innerHTML = '';

    if (!errors || errors.length === 0) {
        errorsEmpty.style.display = 'block';
        errorsList.appendChild(errorsEmpty);
        return;
    }

    errorsEmpty.style.display = 'none';

    // 按时间倒序排序
    errors.sort((a, b) => new Date(b.timestamp) - new Date(a.timestamp));

    // 创建错误项
    errors.forEach(error => {
        const errorItem = createErrorItem(error);
        errorsList.appendChild(errorItem);
    });
}

/**
 * 创建错误项元素
 */
function createErrorItem(error) {
    const item = document.createElement('div');
    item.className = 'error-item';
    item.dataset.errorId = error.id;

    const icon = document.createElement('div');
    icon.className = 'error-icon';
    icon.textContent = '⚠️';

    const content = document.createElement('div');
    content.className = 'error-content';

    // 错误头部
    const header = document.createElement('div');
    header.className = 'error-header';

    const title = document.createElement('div');
    title.className = 'error-title';
    title.textContent = error.title || error.id;
    title.title = error.title || error.id;

    const stage = document.createElement('span');
    stage.className = 'error-stage';
    stage.textContent = getStageDisplayName(error.stage);

    header.appendChild(title);
    header.appendChild(stage);

    // 错误消息
    const message = document.createElement('div');
    message.className = 'error-message';
    message.textContent = error.error_msg || '未知错误';

    // 元数据
    const meta = document.createElement('div');
    meta.className = 'error-meta';

    const arxivId = document.createElement('span');
    arxivId.className = 'error-arxiv-id';
    arxivId.textContent = error.id;

    const time = document.createElement('span');
    time.className = 'error-time';
    time.textContent = formatTime(error.timestamp);

    const retryCount = document.createElement('span');
    retryCount.className = 'error-retry-count';
    retryCount.textContent = `重试: ${error.retry_count || 0}次`;

    meta.appendChild(arxivId);
    meta.appendChild(time);
    meta.appendChild(retryCount);

    // 操作按钮
    const actions = document.createElement('div');
    actions.className = 'error-actions';

    const retryBtn = document.createElement('button');
    retryBtn.className = 'error-btn error-btn-retry';
    retryBtn.innerHTML = '🔄 重试';
    retryBtn.onclick = () => handleRetry(error.id);

    const clearBtn = document.createElement('button');
    clearBtn.className = 'error-btn error-btn-clear';
    clearBtn.innerHTML = '🗑️ 清除';
    clearBtn.onclick = () => handleClearError(error.id);

    actions.appendChild(retryBtn);
    actions.appendChild(clearBtn);

    // 组装内容
    content.appendChild(header);
    content.appendChild(message);
    content.appendChild(meta);
    content.appendChild(actions);

    item.appendChild(icon);
    item.appendChild(content);

    return item;
}

/**
 * 处理重试
 */
async function handleRetry(errorId) {
    if (!RetryFromError) {
        console.error('RetryFromError binding not available');
        return;
    }

    // 禁用重试按钮
    const errorItem = document.querySelector(`[data-error-id="${errorId}"]`);
    if (errorItem) {
        const retryBtn = errorItem.querySelector('.error-btn-retry');
        if (retryBtn) {
            retryBtn.disabled = true;
            retryBtn.textContent = '⏳ 重试中...';
        }
    }

    try {
        showToast('开始重试翻译...', 'info');
        closeErrorsModal();

        const result = await RetryFromError(errorId);
        
        if (result) {
            showToast('重试成功！', 'success');
            // 重新加载错误列表（如果模态框还开着）
            if (errorsModal && errorsModal.style.display === 'flex') {
                await loadErrors();
            }
        }
    } catch (error) {
        console.error('Retry failed:', error);
        showToast('重试失败: ' + (error.message || error), 'error');
        
        // 重新启用按钮
        if (errorItem) {
            const retryBtn = errorItem.querySelector('.error-btn-retry');
            if (retryBtn) {
                retryBtn.disabled = false;
                retryBtn.innerHTML = '🔄 重试';
            }
        }
    }
}

/**
 * 处理清除单个错误
 */
async function handleClearError(errorId) {
    if (!ClearError) {
        console.error('ClearError binding not available');
        return;
    }

    if (!confirm('确定要清除这条错误记录吗？')) {
        return;
    }

    try {
        await ClearError(errorId);
        showToast('错误记录已清除', 'success');
        await loadErrors();
    } catch (error) {
        console.error('Failed to clear error:', error);
        showToast('清除失败: ' + (error.message || error), 'error');
    }
}

/**
 * 处理清除所有错误
 */
async function handleClearAllErrors() {
    if (!ClearAllErrors) {
        console.error('ClearAllErrors binding not available');
        return;
    }

    if (!confirm('确定要清除所有错误记录吗？此操作不可恢复。')) {
        return;
    }

    try {
        await ClearAllErrors();
        showToast('所有错误记录已清除', 'success');
        await loadErrors();
    } catch (error) {
        console.error('Failed to clear all errors:', error);
        showToast('清除失败: ' + (error.message || error), 'error');
    }
}

/**
 * 获取阶段显示名称
 */
function getStageDisplayName(stage) {
    const stageNames = {
        'download': '下载',
        'extract': '解压',
        'original_compile': '原始编译',
        'translation': '翻译',
        'translated_compile': '翻译后编译',
        'pdf_generation': 'PDF生成'
    };
    return stageNames[stage] || stage;
}

/**
 * 格式化时间
 */
function formatTime(timestamp) {
    if (!timestamp) return '';
    
    const date = new Date(timestamp);
    const now = new Date();
    const diff = now - date;
    
    // 小于1分钟
    if (diff < 60000) {
        return '刚刚';
    }
    // 小于1小时
    if (diff < 3600000) {
        return `${Math.floor(diff / 60000)}分钟前`;
    }
    // 小于1天
    if (diff < 86400000) {
        return `${Math.floor(diff / 3600000)}小时前`;
    }
    // 小于7天
    if (diff < 604800000) {
        return `${Math.floor(diff / 86400000)}天前`;
    }
    
    // 超过7天，显示具体日期
    return date.toLocaleDateString('zh-CN', {
        year: 'numeric',
        month: '2-digit',
        day: '2-digit',
        hour: '2-digit',
        minute: '2-digit'
    });
}

/**
 * 显示 Toast 通知
 * 使用全局的 showToast 函数
 */
function showToast(message, type = 'info', duration = 3000) {
    if (window.showToast) {
        window.showToast(message, type, duration);
    } else {
        console.log(`[${type}] ${message}`);
    }
}

/**
 * 处理导出错误列表
 */
async function handleExportErrors() {
    if (!ExportErrorsToFile) {
        console.error('ExportErrorsToFile binding not available');
        return;
    }

    try {
        showToast('正在导出错误详细报告...', 'info');
        
        const filePath = await ExportErrorsToFile();
        
        if (filePath) {
            showToast('错误详细报告已导出', 'success');
        }
    } catch (error) {
        console.error('Failed to export errors:', error);
        if (error.message && error.message.includes('no errors')) {
            showToast('没有错误记录可导出', 'warning');
        } else if (error.message && error.message.includes('cancelled')) {
            showToast('导出已取消', 'info');
        } else {
            showToast('导出失败: ' + (error.message || error), 'error');
        }
    }
}

/**
 * 处理导出错误 arXiv ID 列表
 */
async function handleExportErrorIDs() {
    if (!ExportErrorIDsToFile) {
        console.error('ExportErrorIDsToFile binding not available');
        return;
    }

    try {
        showToast('正在导出 arXiv ID 列表...', 'info');
        
        const filePath = await ExportErrorIDsToFile();
        
        if (filePath) {
            showToast('arXiv ID 列表已导出', 'success');
        }
    } catch (error) {
        console.error('Failed to export error IDs:', error);
        if (error.message && error.message.includes('no errors')) {
            showToast('没有错误记录可导出', 'warning');
        } else if (error.message && error.message.includes('cancelled')) {
            showToast('导出已取消', 'info');
        } else {
            showToast('导出失败: ' + (error.message || error), 'error');
        }
    }
}
