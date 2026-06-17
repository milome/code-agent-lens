import './style.css'
import './effects/festival-effects.css'
import '../wailsjs/runtime/runtime.js'
import { setLanguage } from './i18n/index.js'
import { initUI, changeLanguage } from './modules/ui.js'
import { loadConfig } from './modules/config.js'
import { loadStats, switchStatsPeriod, loadStatsByPeriod, getCurrentPeriod, updateStatsIncremental, updateEndpointStatsCache, updateTotalStatsCache } from './modules/stats.js'
import { renderEndpoints, toggleEndpointPanel, initEndpointSuccessListener, checkAllEndpointsOnStartup, switchEndpointViewMode, initEndpointViewMode, isDropdownOpen, updateEndpointStatsIncremental } from './modules/endpoints.js'
import { loadLogs, toggleLogPanel, changeLogLevel, copyLogs, clearLogs } from './modules/logs.js'
import { initTips } from './modules/tips.js'
import { initSession } from './modules/session.js'
import { showSettingsModal, closeSettingsModal, saveSettings, applyTheme, initTheme, showAutoThemeConfigModal, closeAutoThemeConfigModal, saveAutoThemeConfig } from './modules/settings.js'
import { checkUpdatesOnStartup, checkForUpdates, initUpdateSettings } from './modules/updater.js'
import { initFilterDropdowns, clearAllFilters } from './modules/filters.js'
import { formatTokens } from './utils/format.js'
import {
    showAddEndpointModal,
    showAddEndpointModalWithPreset,
    editEndpoint,
    saveEndpoint,
    openEndpointTokenPoolFromModal,
    deleteEndpoint,
    closeModal,
    handleAuthModeChange,
    handleTransformerChange,
    fetchModels,
    initModelInputEvents,
    toggleModelDropdown,
    showEditPortModal,
    savePort,
    closePortModal,
    showWelcomeModal,
    closeWelcomeModal,
    showWelcomeModalIfFirstTime,
    showChangelogModal,
    closeChangelogModal,
    showChangelogIfNewVersion,
    testEndpointHandler,
    closeTestResultModal,
    openGitHub,
    togglePasswordVisibility,
    acceptConfirm,
    cancelConfirm,
    showCloseActionDialog,
    quitApplication,
    minimizeToTray
} from './modules/modal.js'

function showStartupStatus(message, detail = '') {
    const app = document.getElementById('app');
    if (!app) return;
    app.innerHTML = `
        <div style="min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 32px; color: white;">
            <div style="max-width: 760px; width: 100%; background: rgba(15, 23, 42, 0.72); border: 1px solid rgba(255,255,255,0.18); border-radius: 16px; padding: 24px; box-shadow: 0 24px 80px rgba(0,0,0,0.28);">
                <h1 style="margin: 0 0 12px; font-size: 24px;">CodeAgentLens is starting</h1>
                <div style="font-size: 15px; line-height: 1.7;">${message}</div>
                ${detail ? `<pre style="margin-top: 16px; white-space: pre-wrap; color: #fecaca; background: rgba(127,29,29,0.35); border-radius: 10px; padding: 12px; overflow: auto;">${detail}</pre>` : ''}
            </div>
        </div>
    `;
}

function formatStartupError(error) {
    if (!error) return '';
    if (error.stack) return error.stack;
    if (error.message) return error.message;
    return String(error);
}

async function waitForWailsBridge(timeoutMs = 10000) {
    const start = Date.now();
    while (!window.go?.main?.App) {
        if (Date.now() - start > timeoutMs) {
            throw new Error('Wails bridge was not injected. window.go.main.App is unavailable.');
        }
        await new Promise(resolve => setTimeout(resolve, 100));
    }
}

function withTimeout(promise, timeoutMs, label) {
    let timer;
    const timeout = new Promise((_, reject) => {
        timer = setTimeout(() => {
            reject(new Error(`${label} timed out after ${timeoutMs}ms`));
        }, timeoutMs);
    });

    return Promise.race([
        Promise.resolve(promise).finally(() => clearTimeout(timer)),
        timeout
    ]);
}

async function callBackend(methodName, args = [], timeoutMs = 5000) {
    const app = window.go?.main?.App;
    if (!app || typeof app[methodName] !== 'function') {
        throw new Error(`Backend method is unavailable: ${methodName}`);
    }
    return withTimeout(app[methodName](...args), timeoutMs, methodName);
}

async function waitForBackendStartup(timeoutMs = 15000) {
    const app = window.go?.main?.App;
    if (!app || typeof app.GetStartupStatus !== 'function') {
        return;
    }

    const start = Date.now();
    let lastStatus = null;
    let lastError = null;
    while (Date.now() - start <= timeoutMs) {
        try {
            const statusStr = await callBackend('GetStartupStatus', [], 1500);
            const status = JSON.parse(statusStr || '{}');
            lastStatus = status;
            lastError = null;

            if (status.error) {
                throw new Error(`${status.stage || 'Startup failed'}: ${status.error}`);
            }
            if (status.ready) {
                return;
            }
            if (status.stage) {
                showStartupStatus(status.stage);
            }
        } catch (error) {
            lastError = error;
            showStartupStatus('Waiting for backend startup...', formatStartupError(error));
        }
        await new Promise(resolve => setTimeout(resolve, 100));
    }

    const stage = lastStatus?.stage ? ` Last stage: ${lastStatus.stage}.` : '';
    const detail = lastError ? ` ${formatStartupError(lastError)}` : '';
    throw new Error(`Backend startup did not become ready within ${timeoutMs}ms.${stage}${detail}`);
}

window.addEventListener('error', (event) => {
    showStartupStatus('Frontend startup failed.', formatStartupError(event.error || event.message));
});

window.addEventListener('unhandledrejection', (event) => {
    showStartupStatus('Frontend startup failed.', formatStartupError(event.reason));
});

// Handle real-time stats update events from backend (4-period data)
function handleStatsUpdate(data) {
    if (!data || !data.endpointName) {
        return;
    }

    // Update all 4-period caches first (before DOM updates)
    if (data.endpoint) {
        updateEndpointStatsCache(data.endpointName, data.endpoint);
    }
    if (data.totals) {
        updateTotalStatsCache(data.totals);
    }

    const currentPeriod = getCurrentPeriod(); // Get current selected period

    // 1. Update header stats (top summary) using backend-provided aggregated data
    const totalStats = data.totals && data.totals[currentPeriod];
    if (totalStats) {
        const totalRequestsEl = document.getElementById('periodTotalRequests');
        const successRequestsEl = document.getElementById('periodSuccess');
        const failedRequestsEl = document.getElementById('periodFailed');
        const totalTokensEl = document.getElementById('periodTotalTokens');
        const totalInputTokensEl = document.getElementById('periodInputTokens');
        const totalOutputTokensEl = document.getElementById('periodOutputTokens');

        if (totalRequestsEl) totalRequestsEl.textContent = totalStats.requests;
        if (successRequestsEl) successRequestsEl.textContent = totalStats.requests - totalStats.errors;
        if (failedRequestsEl) failedRequestsEl.textContent = totalStats.errors;
        if (totalTokensEl) totalTokensEl.textContent = formatTokens(totalStats.inputTokens + totalStats.outputTokens);
        if (totalInputTokensEl) totalInputTokensEl.textContent = formatTokens(totalStats.inputTokens);
        if (totalOutputTokensEl) totalOutputTokensEl.textContent = formatTokens(totalStats.outputTokens);
    }

    // 2. Update endpoint card using single endpoint period data
    const endpointStats = data.endpoint && data.endpoint[currentPeriod];
    if (endpointStats) {
        updateEndpointStatsIncremental(data.endpointName, endpointStats);
    }
}

// Load data on startup
window.addEventListener('DOMContentLoaded', async () => {
    showStartupStatus('Waiting for Wails bridge...');

    try {
        await waitForWailsBridge();
        await waitForBackendStartup();

        showStartupStatus('Loading language settings...');
        let lang = 'zh-CN';
        try {
            lang = await callBackend('GetLanguage', [], 5000);
        } catch (error) {
            console.error('Failed to get language, using zh-CN fallback:', error);
        }
        setLanguage(lang);

        showStartupStatus('Loading theme settings...');
        await withTimeout(initTheme(), 7000, 'initTheme');

        showStartupStatus('Rendering UI...');
        initUI();

        // Initialize endpoint view mode
        initEndpointViewMode();

        // Initialize filter dropdowns
        initFilterDropdowns();

        // Initialize session module
        initSession();

        // Initialize model input events
        initModelInputEvents();

        // Load and display version
        try {
            const version = await callBackend('GetVersion', [], 5000);
            document.getElementById('appVersion').textContent = version;
        } catch (error) {
            console.error('Failed to get version:', error);
        }

        // Load initial data
        // IMPORTANT: Load stats first to populate cache, then render endpoints
        await withTimeout(loadStatsByPeriod('daily'), 7000, 'loadStatsByPeriod'); // Load today's stats by default
        await withTimeout(loadConfigAndRender(), 7000, 'loadConfigAndRender');    // Render endpoints after stats are loaded

        // Restore log level from config
        try {
            const logLevel = await callBackend('GetLogLevel', [], 5000);
            document.getElementById('logLevel').value = logLevel;
        } catch (error) {
            console.error('Failed to get log level:', error);
        }

        loadLogs();

        // Initialize tips
        initTips();

        // Initialize endpoint success listener
        initEndpointSuccessListener();

        // Check all endpoints on startup (zero-cost methods only)
        checkAllEndpointsOnStartup();

        // Listen for real-time stats updates from backend
        if (window.runtime && window.runtime.EventsOn) {
            window.runtime.EventsOn('stats:updated', (data) => {
                handleStatsUpdate(data);
            });
        }

    // Fallback: If event-based updates fail, uncomment the following to restore polling
    // setInterval(async () => {
    //     await loadStats(); // Refresh cumulative stats for endpoint cards
    //     const currentPeriod = getCurrentPeriod(); // Get current selected period
    //     await loadStatsByPeriod(currentPeriod); // Refresh period stats (daily/weekly/monthly)
    //     // 如果下拉菜单打开，跳过渲染避免菜单消失
    //     if (isDropdownOpen()) {
    //         return;
    //     }
    //     const config = await window.go.main.App.GetConfig();
    //     if (config) {
    //         renderEndpoints(JSON.parse(config).endpoints);
    //     }
    // }, 30000); // 降低频率到 30 秒

        // Refresh logs every 2 seconds
        setInterval(loadLogs, 2000);

        // Show welcome modal on first launch
        showWelcomeModalIfFirstTime();
        // showChangelogIfNewVersion(); // 暂时禁用自动弹窗

        // Check for updates on startup
        checkUpdatesOnStartup();

        // Initialize update settings
        initUpdateSettings();

        // Listen for close dialog event from backend
        if (window.runtime) {
            window.runtime.EventsOn('show-close-dialog', () => {
                showCloseActionDialog();
            });
        }

        // Handle Cmd/Ctrl+W to hide window
        window.addEventListener('keydown', (e) => {
            if ((e.metaKey || e.ctrlKey) && e.key === 'w') {
                e.preventDefault();
                window.runtime.WindowHide();
            }
        });
    } catch (error) {
        console.error('Frontend startup failed:', error);
        showStartupStatus('Frontend startup failed.', formatStartupError(error));
    }
});

// Helper function to load config and render endpoints
async function loadConfigAndRender() {
    const config = await loadConfig();
    if (config) {
        await renderEndpoints(config.endpoints);
    }
}

// Expose functions to window for onclick handlers
window.loadConfig = loadConfigAndRender;
window.showAddEndpointModal = showAddEndpointModal;
window.showAddEndpointModalWithPreset = showAddEndpointModalWithPreset;
window.editEndpoint = editEndpoint;
window.saveEndpoint = saveEndpoint;
window.openEndpointTokenPoolFromModal = openEndpointTokenPoolFromModal;
window.deleteEndpoint = deleteEndpoint;
window.closeModal = closeModal;
window.handleAuthModeChange = handleAuthModeChange;
window.handleTransformerChange = handleTransformerChange;
window.fetchModels = fetchModels;
window.toggleModelDropdown = toggleModelDropdown;
window.showEditPortModal = showEditPortModal;
window.savePort = savePort;
window.closePortModal = closePortModal;
window.showWelcomeModal = showWelcomeModal;
window.closeWelcomeModal = closeWelcomeModal;
window.showChangelogModal = showChangelogModal;
window.closeChangelogModal = closeChangelogModal;
window.testEndpoint = testEndpointHandler;
window.closeTestResultModal = closeTestResultModal;
window.openGitHub = openGitHub;
window.toggleLogPanel = toggleLogPanel;
window.changeLogLevel = changeLogLevel;
window.copyLogs = copyLogs;
window.clearLogs = clearLogs;
window.changeLanguage = changeLanguage;
window.togglePasswordVisibility = togglePasswordVisibility;
window.acceptConfirm = acceptConfirm;
window.checkForUpdates = checkForUpdates;
window.cancelConfirm = cancelConfirm;
window.showCloseActionDialog = showCloseActionDialog;
window.quitApplication = quitApplication;
window.minimizeToTray = minimizeToTray;
window.switchStatsPeriod = switchStatsPeriod;
window.toggleEndpointPanel = toggleEndpointPanel;
window.switchEndpointViewMode = switchEndpointViewMode;
window.clearAllFilters = clearAllFilters;
window.showSettingsModal = showSettingsModal;
window.closeSettingsModal = closeSettingsModal;
window.saveSettings = saveSettings;
window.showAutoThemeConfigModal = showAutoThemeConfigModal;
window.closeAutoThemeConfigModal = closeAutoThemeConfigModal;
window.saveAutoThemeConfig = saveAutoThemeConfig;

// History modal functions
window.closeHistoryModal = async () => {
    const { closeHistoryModal } = await import('./modules/history.js');
    closeHistoryModal();
};

window.deleteHistoryArchive = async () => {
    const { deleteHistoryArchive } = await import('./modules/history.js');
    deleteHistoryArchive();
};
