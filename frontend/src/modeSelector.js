/**
 * Mode Selector Module
 * Handles startup mode selection (commercial vs opensource)
 * Validates: Requirements 1.1, 1.2, 2.1, 2.2, 2.3
 */

// Backend bindings - will be initialized
let GetWorkMode, SetWorkMode, ActivateLicense, RequestSerialNumber, 
    GetLicenseInfo, CheckLicenseValidity, RefreshLicense;

// State
let isInitialized = false;

/**
 * Initialize mode selector bindings
 * Must be called before using any other functions
 */
async function initModeBindings() {
    if (isInitialized) return;
    
    try {
        const App = await import('../wailsjs/go/main/App.js');
        GetWorkMode = App.GetWorkMode;
        SetWorkMode = App.SetWorkMode;
        ActivateLicense = App.ActivateLicense;
        RequestSerialNumber = App.RequestSerialNumber;
        GetLicenseInfo = App.GetLicenseInfo;
        CheckLicenseValidity = App.CheckLicenseValidity;
        RefreshLicense = App.RefreshLicense;
        isInitialized = true;
        console.log('Mode selector bindings initialized');
    } catch (error) {
        console.error('Failed to initialize mode bindings:', error);
        throw error;
    }
}

/**
 * Check startup mode and determine what to show
 * Returns true if mode is configured and valid, false if selection is needed
 * Validates: Requirements 1.1, 1.2, 1.3
 */
async function checkStartupMode() {
    if (!isInitialized) {
        await initModeBindings();
    }
    
    try {
        const workMode = await GetWorkMode();
        console.log('Current work mode:', workMode);
        
        if (!workMode) {
            // No mode configured, show selection
            showModeSelectionModal();
            return false;
        }
        
        if (workMode === 'commercial') {
            // Commercial mode - check license validity
            const validity = await CheckLicenseValidity();
            console.log('License validity:', validity);
            
            if (!validity.is_valid) {
                if (validity.is_expired) {
                    showLicenseExpiredDialog(validity.message);
                    return false;
                }
            }
            
            if (validity.is_expiring_soon) {
                showExpiryWarning(validity.message);
            }
        }
        
        return true;
    } catch (error) {
        console.error('Error checking startup mode:', error);
        // On error, show mode selection as fallback
        showModeSelectionModal();
        return false;
    }
}

/**
 * Show the mode selection modal
 * Validates: Requirements 2.1, 2.2
 */
function showModeSelectionModal() {
    const modal = document.getElementById('mode-selection-modal');
    if (!modal) {
        console.error('Mode selection modal not found');
        return;
    }
    
    modal.classList.add('visible');
    
    // Bind click events for mode options
    const commercialOption = document.getElementById('mode-commercial');
    const opensourceOption = document.getElementById('mode-opensource');
    
    if (commercialOption) {
        commercialOption.onclick = () => {
            hideModeSelectionModal();
            showSerialNumberModal();
        };
    }
    
    if (opensourceOption) {
        opensourceOption.onclick = () => {
            hideModeSelectionModal();
            handleOpenSourceModeSelection();
        };
    }
}

/**
 * Hide the mode selection modal
 */
function hideModeSelectionModal() {
    const modal = document.getElementById('mode-selection-modal');
    if (modal) {
        modal.classList.remove('visible');
    }
}

/**
 * Show the serial number input modal
 * Validates: Requirements 2.4, 3.1
 */
function showSerialNumberModal() {
    const modal = document.getElementById('serial-number-modal');
    if (!modal) {
        console.error('Serial number modal not found');
        return;
    }
    
    // Clear previous input and errors
    const snInput = document.getElementById('serial-number-input');
    const snError = document.getElementById('sn-error');
    
    // Don't clear snInput if it already has a value (e.g., from email request)
    if (snInput && !snInput.value) {
        snInput.value = '';
    }
    if (snError) snError.style.display = 'none';
    
    modal.classList.add('visible');
    
    // Bind events
    bindSerialNumberModalEvents();
}

/**
 * Hide the serial number modal
 */
function hideSerialNumberModal() {
    const modal = document.getElementById('serial-number-modal');
    if (modal) {
        modal.classList.remove('visible');
    }
}

/**
 * Show the email request modal
 */
function showEmailRequestModal() {
    const modal = document.getElementById('email-request-modal');
    if (!modal) {
        console.error('Email request modal not found');
        return;
    }
    
    // Clear previous input and errors
    const emailInput = document.getElementById('email-input');
    const emailError = document.getElementById('email-error');
    
    if (emailInput) emailInput.value = '';
    if (emailError) emailError.style.display = 'none';
    
    modal.classList.add('visible');
    
    // Bind events
    bindEmailRequestModalEvents();
}

/**
 * Hide the email request modal
 */
function hideEmailRequestModal() {
    const modal = document.getElementById('email-request-modal');
    if (modal) {
        modal.classList.remove('visible');
    }
}

/**
 * Bind events for email request modal
 */
function bindEmailRequestModalEvents() {
    // Close button
    const closeBtn = document.getElementById('email-modal-close');
    if (closeBtn) {
        closeBtn.onclick = () => {
            hideEmailRequestModal();
            showSerialNumberModal();
        };
    }
    
    // Back button
    const backBtn = document.getElementById('btn-email-back');
    if (backBtn) {
        backBtn.onclick = () => {
            hideEmailRequestModal();
            showSerialNumberModal();
        };
    }
    
    // Submit email button
    const submitEmailBtn = document.getElementById('btn-submit-email');
    if (submitEmailBtn) {
        submitEmailBtn.onclick = submitEmailRequest;
    }
}

/**
 * Bind events for serial number modal
 */
function bindSerialNumberModalEvents() {
    // Close button
    const closeBtn = document.getElementById('sn-modal-close');
    if (closeBtn) {
        closeBtn.onclick = () => {
            hideSerialNumberModal();
            showModeSelectionModal();
        };
    }
    
    // Back button
    const backBtn = document.getElementById('btn-sn-back');
    if (backBtn) {
        backBtn.onclick = () => {
            hideSerialNumberModal();
            showModeSelectionModal();
        };
    }
    
    // Activate button
    const activateBtn = document.getElementById('btn-activate');
    if (activateBtn) {
        activateBtn.onclick = activateSerialNumber;
    }
    
    // Request SN button - opens email request modal
    const requestSnBtn = document.getElementById('btn-request-sn');
    if (requestSnBtn) {
        requestSnBtn.onclick = () => {
            hideSerialNumberModal();
            showEmailRequestModal();
        };
    }
    
    // Auto-format serial number input
    const snInput = document.getElementById('serial-number-input');
    if (snInput) {
        snInput.oninput = formatSerialNumberInput;
    }
}

/**
 * Format serial number input with dashes
 */
function formatSerialNumberInput(event) {
    let value = event.target.value.toUpperCase().replace(/[^A-Z0-9]/g, '');
    let formatted = '';
    
    for (let i = 0; i < value.length && i < 16; i++) {
        if (i > 0 && i % 4 === 0) {
            formatted += '-';
        }
        formatted += value[i];
    }
    
    event.target.value = formatted;
}

/**
 * Validate serial number format
 * Validates: Requirements 3.1, 3.2
 */
function validateSNFormat(sn) {
    const pattern = /^[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}-[A-Z0-9]{4}$/;
    return pattern.test(sn);
}

/**
 * Show error in serial number modal
 */
function showSNError(message) {
    const snError = document.getElementById('sn-error');
    if (snError) {
        snError.textContent = message;
        snError.style.display = 'block';
    }
}

/**
 * Hide error in serial number modal
 */
function hideSNError() {
    const snError = document.getElementById('sn-error');
    if (snError) {
        snError.style.display = 'none';
    }
}

/**
 * Activate serial number
 * Validates: Requirements 3.3, 3.4, 3.5, 3.6, 3.7
 */
async function activateSerialNumber() {
    const snInput = document.getElementById('serial-number-input');
    if (!snInput) return;
    
    const sn = snInput.value.trim().toUpperCase();
    
    // Validate format
    if (!validateSNFormat(sn)) {
        showSNError('序列号格式无效，请检查后重试');
        return;
    }
    
    hideSNError();
    
    // Show loading state
    const activateBtn = document.getElementById('btn-activate');
    const originalText = activateBtn ? activateBtn.textContent : '激活';
    if (activateBtn) {
        activateBtn.textContent = '激活中...';
        activateBtn.disabled = true;
    }
    
    try {
        const result = await ActivateLicense(sn);
        console.log('Activation result:', result);
        
        if (result.success) {
            hideSerialNumberModal();
            showToast('激活成功！', 'success');
            // Proceed to main interface
            if (typeof initMainInterface === 'function') {
                initMainInterface();
            }
        } else {
            showSNError(result.message || '激活失败，请稍后重试');
        }
    } catch (error) {
        console.error('Activation error:', error);
        showSNError('激活失败：' + (error.message || '未知错误'));
    } finally {
        if (activateBtn) {
            activateBtn.textContent = originalText;
            activateBtn.disabled = false;
        }
    }
}

/**
 * Show error in email request modal
 */
function showEmailError(message) {
    const emailError = document.getElementById('email-error');
    if (emailError) {
        emailError.textContent = message;
        emailError.style.display = 'block';
    }
}

/**
 * Hide error in email request modal
 */
function hideEmailError() {
    const emailError = document.getElementById('email-error');
    if (emailError) {
        emailError.style.display = 'none';
    }
}

/**
 * Submit email request for serial number
 * Validates: Requirements 3.8
 */
async function submitEmailRequest() {
    const emailInput = document.getElementById('email-input');
    if (!emailInput) return;
    
    const email = emailInput.value.trim();
    
    if (!email) {
        showEmailError('请输入邮箱地址');
        return;
    }
    
    // Basic email validation
    const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
    if (!emailPattern.test(email)) {
        showEmailError('邮箱地址格式无效');
        return;
    }
    
    hideEmailError();
    
    // Show loading state
    const submitBtn = document.getElementById('btn-submit-email');
    const originalText = submitBtn ? submitBtn.textContent : '提交申请';
    if (submitBtn) {
        submitBtn.textContent = '提交中...';
        submitBtn.disabled = true;
    }
    
    try {
        const result = await RequestSerialNumber(email);
        
        // Hide email modal
        hideEmailRequestModal();
        
        // If serial number is returned, auto-fill it into the input and show activation modal
        if (result.serial_number) {
            const snInput = document.getElementById('serial-number-input');
            if (snInput) {
                snInput.value = result.serial_number;
            }
            showSerialNumberModal();
            showToast('序列号已获取，请点击激活', 'success');
        } else {
            // No serial number returned, show message and return to activation modal
            showSerialNumberModal();
            showToast(result.message || '申请已提交，请查收邮件获取序列号', 'success');
        }
        
        // Clear email input
        emailInput.value = '';
    } catch (error) {
        console.error('Email request error:', error);
        showEmailError(error.message || '申请失败，请稍后重试');
    } finally {
        if (submitBtn) {
            submitBtn.textContent = originalText;
            submitBtn.disabled = false;
        }
    }
}

/**
 * Handle open source mode selection
 * Validates: Requirements 4.1, 4.2
 */
async function handleOpenSourceModeSelection() {
    try {
        // Set work mode to opensource
        await SetWorkMode('opensource');
        console.log('Work mode set to opensource');
        
        // Open settings dialog for LLM configuration
        // The settings dialog should be modified to handle first-time setup
        if (typeof openSettingsForFirstTimeSetup === 'function') {
            openSettingsForFirstTimeSetup();
        } else if (typeof openSettingsModal === 'function') {
            openSettingsModal();
        } else {
            // Fallback: dispatch event for main.js to handle
            window.dispatchEvent(new CustomEvent('open-settings-first-time'));
        }
    } catch (error) {
        console.error('Error setting opensource mode:', error);
        showToast('设置失败：' + error.message, 'error');
        // Show mode selection again
        showModeSelectionModal();
    }
}

/**
 * Show license expired dialog
 */
function showLicenseExpiredDialog(message) {
    // Use existing confirm dialog or create a simple alert
    if (typeof showConfirmDialog === 'function') {
        showConfirmDialog(
            message || '您的授权已过期，请重新激活或切换到开源模式',
            '授权已过期',
            '重新激活',
            '切换到开源模式'
        ).then(confirmed => {
            if (confirmed) {
                showSerialNumberModal();
            } else {
                handleOpenSourceModeSelection();
            }
        });
    } else {
        // Fallback: show mode selection
        showModeSelectionModal();
    }
}

/**
 * Show expiry warning toast
 */
function showExpiryWarning(message) {
    showToast(message || '您的授权即将过期，请及时续费', 'warning');
}

/**
 * Show toast notification
 * Uses existing toast function if available, otherwise creates a simple one
 */
function showToast(message, type = 'info') {
    // Try to use existing toast function
    if (typeof window.showToast === 'function') {
        window.showToast(message, type);
        return;
    }
    
    // Fallback: simple alert for now
    console.log(`[${type}] ${message}`);
    if (type === 'error') {
        alert(message);
    }
}

/**
 * Update about page license info
 * Validates: Requirements 10.1, 10.2, 10.3, 10.4, 10.5
 */
async function updateAboutLicenseInfo() {
    if (!isInitialized) {
        await initModeBindings();
    }
    
    try {
        const licenseInfo = await GetLicenseInfo();
        const licenseSection = document.getElementById('about-license-section');
        
        if (!licenseSection) return;
        
        if (licenseInfo.work_mode === 'commercial') {
            licenseSection.style.display = 'block';
            
            const serialNumberEl = document.getElementById('license-serial-number');
            const expiresEl = document.getElementById('license-expires');
            const dailyAnalysisEl = document.getElementById('license-daily-limit');
            const warningEl = document.getElementById('license-warning');
            
            if (serialNumberEl) {
                serialNumberEl.textContent = licenseInfo.serial_number || '-';
            }
            
            if (expiresEl) {
                expiresEl.textContent = licenseInfo.expires_at || '未知';
            }
            
            if (dailyAnalysisEl) {
                dailyAnalysisEl.textContent = licenseInfo.daily_analysis > 0 
                    ? licenseInfo.daily_analysis + ' 次/天' 
                    : '无限制';
            }
            
            if (warningEl) {
                if (licenseInfo.is_expiring_soon) {
                    warningEl.style.display = 'block';
                    warningEl.textContent = `授权将在 ${licenseInfo.days_remaining} 天后过期，请及时续费`;
                } else {
                    warningEl.style.display = 'none';
                }
            }
        } else {
            licenseSection.style.display = 'none';
        }
    } catch (error) {
        console.error('Error updating license info:', error);
    }
}

/**
 * Adjust settings UI based on work mode
 * Validates: Requirements 9.1, 9.2, 9.3
 */
async function adjustSettingsForMode() {
    if (!isInitialized) {
        await initModeBindings();
    }
    
    try {
        const workMode = await GetWorkMode();
        const llmSection = document.getElementById('llm-settings-section');
        
        if (llmSection) {
            if (workMode === 'commercial') {
                // Commercial mode: hide LLM config
                llmSection.style.display = 'none';
            } else {
                // Opensource mode: show LLM config
                llmSection.style.display = 'block';
            }
        }
    } catch (error) {
        console.error('Error adjusting settings for mode:', error);
    }
}

// Export functions for use in main.js
export {
    initModeBindings,
    checkStartupMode,
    showModeSelectionModal,
    hideModeSelectionModal,
    showSerialNumberModal,
    hideSerialNumberModal,
    updateAboutLicenseInfo,
    adjustSettingsForMode,
    GetWorkMode,
    SetWorkMode,
    GetLicenseInfo,
    RefreshLicense
};
