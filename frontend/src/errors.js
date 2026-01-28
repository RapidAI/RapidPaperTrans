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
let btnReportErrors;

// Generic Confirm Dialog elements
let genericConfirmModal;
let genericConfirmTitle;
let genericConfirmMessage;
let genericConfirmModalClose;
let btnGenericCancel;
let btnGenericConfirm;
let genericConfirmResolve = null;

// Alert Dialog elements
let alertModal;
let alertTitle;
let alertMessage;
let alertModalClose;
let btnAlertOk;
let alertResolve = null;

// Backend bindings for error management
let ListErrors, RetryFromError, ClearError, ClearAllErrors, ExportErrorsToFile, ExportErrorIDsToFile, ReportErrorsToGitHub;

// Callback to update input source when retrying
let onRetryUpdateInput = null;
// Callback when retry completes (success or failure)
let onRetryComplete = null;

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
    ReportErrorsToGitHub = bindings.ReportErrorsToGitHub;
    
    // 保存更新输入框的回调
    onRetryUpdateInput = bindings.onRetryUpdateInput;
    // 保存重试完成的回调
    onRetryComplete = bindings.onRetryComplete;

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
    btnReportErrors = document.getElementById('btn-report-errors');

    // 初始化通用确认对话框元素
    genericConfirmModal = document.getElementById('generic-confirm-modal');
    genericConfirmTitle = document.getElementById('generic-confirm-title');
    genericConfirmMessage = document.getElementById('generic-confirm-message');
    genericConfirmModalClose = document.getElementById('generic-confirm-modal-close');
    btnGenericCancel = document.getElementById('btn-generic-cancel');
    btnGenericConfirm = document.getElementById('btn-generic-confirm');

    // 初始化提示对话框元素
    alertModal = document.getElementById('alert-modal');
    alertTitle = document.getElementById('alert-title');
    alertMessage = document.getElementById('alert-message');
    alertModalClose = document.getElementById('alert-modal-close');
    btnAlertOk = document.getElementById('btn-alert-ok');

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
    if (btnReportErrors) {
        btnReportErrors.addEventListener('click', handleReportErrors);
    }
    if (errorsModal) {
        errorsModal.addEventListener('mousedown', (e) => {
            if (e.target === errorsModal) {
                closeErrorsModal();
            }
        });
    }

    // 设置通用确认对话框事件监听器
    if (genericConfirmModalClose) {
        genericConfirmModalClose.addEventListener('click', () => closeGenericConfirm(false));
    }
    if (btnGenericCancel) {
        btnGenericCancel.addEventListener('click', () => closeGenericConfirm(false));
    }
    if (btnGenericConfirm) {
        btnGenericConfirm.addEventListener('click', () => closeGenericConfirm(true));
    }
    if (genericConfirmModal) {
        genericConfirmModal.addEventListener('mousedown', (e) => {
            if (e.target === genericConfirmModal) {
                closeGenericConfirm(false);
            }
        });
    }

    // 设置提示对话框事件监听器
    if (alertModalClose) {
        alertModalClose.addEventListener('click', closeAlert);
    }
    if (btnAlertOk) {
        btnAlertOk.addEventListener('click', closeAlert);
    }
    if (alertModal) {
        alertModal.addEventListener('mousedown', (e) => {
            if (e.target === alertModal) {
                closeAlert();
            }
        });
    }
}

/**
 * 显示自定义确认对话框
 * @param {string} message - 确认消息
 * @param {string} title - 对话框标题（可选）
 * @param {string} confirmText - 确认按钮文本（可选）
 * @param {string} cancelText - 取消按钮文本（可选）
 * @returns {Promise<boolean>} - 用户选择结果
 */
function showConfirmDialog(message, title = '确认', confirmText = '确定', cancelText = '取消') {
    return new Promise((resolve) => {
        if (!genericConfirmModal) {
            // 降级到原生 confirm
            resolve(confirm(message));
            return;
        }

        genericConfirmResolve = resolve;
        
        if (genericConfirmTitle) genericConfirmTitle.textContent = title;
        if (genericConfirmMessage) genericConfirmMessage.textContent = message;
        if (btnGenericConfirm) btnGenericConfirm.textContent = confirmText;
        if (btnGenericCancel) btnGenericCancel.textContent = cancelText;
        
        genericConfirmModal.classList.add('visible');
    });
}

/**
 * 关闭通用确认对话框
 * @param {boolean} result - 用户选择结果
 */
function closeGenericConfirm(result) {
    if (genericConfirmModal) {
        genericConfirmModal.classList.remove('visible');
    }
    if (genericConfirmResolve) {
        genericConfirmResolve(result);
        genericConfirmResolve = null;
    }
}

/**
 * 显示提示对话框
 * @param {string} message - 提示消息
 * @param {string} title - 对话框标题（可选）
 * @returns {Promise<void>}
 */
function showAlertDialog(message, title = '提示') {
    return new Promise((resolve) => {
        if (!alertModal) {
            // 降级到原生 alert
            alert(message);
            resolve();
            return;
        }

        alertResolve = resolve;
        
        if (alertTitle) alertTitle.textContent = title;
        if (alertMessage) alertMessage.textContent = message;
        
        alertModal.classList.add('visible');
    });
}

/**
 * 关闭提示对话框
 */
function closeAlert() {
    if (alertModal) {
        alertModal.classList.remove('visible');
    }
    if (alertResolve) {
        alertResolve();
        alertResolve = null;
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
    if (error.reported) {
        item.className += ' error-reported';
    }
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

    // 已上报标记
    if (error.reported) {
        const reportedBadge = document.createElement('span');
        reportedBadge.className = 'error-reported-badge';
        reportedBadge.textContent = '✓ 已上报';
        reportedBadge.title = '已上报到 GitHub Issue';
        header.appendChild(reportedBadge);
    }

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
    retryBtn.onclick = () => handleRetry(error.id, error.input);

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
async function handleRetry(errorId, errorInput) {
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

        // 更新输入框并启动状态轮询
        if (onRetryUpdateInput && errorInput) {
            onRetryUpdateInput(errorInput);
        }

        const result = await RetryFromError(errorId);
        
        // 重试完成，通知主界面
        if (onRetryComplete) {
            onRetryComplete(result, null);
        }
        
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
        
        // 重试失败，通知主界面
        if (onRetryComplete) {
            onRetryComplete(null, error);
        }
        
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

    const confirmed = await showConfirmDialog(
        '确定要清除这条错误记录吗？',
        '⚠️ 清除确认',
        '清除',
        '取消'
    );
    
    if (!confirmed) {
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

    const confirmed = await showConfirmDialog(
        '确定要清除所有错误记录吗？\n\n此操作不可恢复。',
        '⚠️ 清除所有错误',
        '全部清除',
        '取消'
    );
    
    if (!confirmed) {
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

/**
 * 处理上报错误到 GitHub Issue
 */
async function handleReportErrors() {
    if (!ReportErrorsToGitHub) {
        console.error('ReportErrorsToGitHub binding not available');
        return;
    }

    // 先检查是否有未上报的错误
    try {
        const errors = await ListErrors();
        if (!errors || errors.length === 0) {
            await showAlertDialog(
                '当前没有任何错误记录。',
                'ℹ️ 提示'
            );
            return;
        }

        const unreportedErrors = errors.filter(e => !e.reported);
        if (unreportedErrors.length === 0) {
            await showAlertDialog(
                '所有错误都已上报，没有需要上报的新错误。',
                'ℹ️ 提示'
            );
            return;
        }

        // 显示确认对话框
        const confirmed = await showConfirmDialog(
            `确定要将 ${unreportedErrors.length} 个未上报的错误上报到 GitHub Issue 吗？\n\n这将在配置的 GitHub 仓库中创建一个新的 Issue，包含所有未上报错误的 arXiv ID 和详细信息。`,
            '🐛 上报错误到 GitHub',
            '上报',
            '取消'
        );

        if (!confirmed) {
            return;
        }
    } catch (error) {
        console.error('Failed to check errors:', error);
        showToast('检查错误列表失败: ' + (error.message || error), 'error');
        return;
    }

    // 禁用按钮防止重复点击
    if (btnReportErrors) {
        btnReportErrors.disabled = true;
        btnReportErrors.innerHTML = '⏳ 上报中...';
    }

    try {
        showToast('正在上报错误到 GitHub...', 'info');
        
        const result = await ReportErrorsToGitHub();
        
        if (result && result.success) {
            showToast('错误已上报到 GitHub Issue', 'success');
            
            // 刷新错误列表以显示已上报状态
            await loadErrors();
            
            // 询问是否打开 Issue 页面
            if (result.issue_url) {
                const openIssue = await showConfirmDialog(
                    `错误已成功上报！\n\nIssue 链接:\n${result.issue_url}\n\n是否打开 Issue 页面？`,
                    '✅ 上报成功',
                    '打开',
                    '关闭'
                );
                if (openIssue) {
                    window.open(result.issue_url, '_blank');
                }
            }
        }
    } catch (error) {
        console.error('Failed to report errors:', error);
        if (error.message && error.message.includes('no unreported')) {
            await showAlertDialog(
                '没有未上报的错误记录。',
                'ℹ️ 提示'
            );
        } else if (error.message && error.message.includes('token')) {
            await showAlertDialog(
                'GitHub Token 未配置或无效，请在设置中配置。',
                '⚠️ 配置错误'
            );
        } else {
            showToast('上报失败: ' + (error.message || error), 'error');
        }
    } finally {
        // 恢复按钮状态
        if (btnReportErrors) {
            btnReportErrors.disabled = false;
            btnReportErrors.innerHTML = '🐛 上报到 GitHub';
        }
    }
}
