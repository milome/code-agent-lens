import { api } from '../api.js';
import { state } from '../state.js';
import { escapeHtml, formatDateTime } from '../utils/formatters.js';
import { t } from '../utils/i18n.js';
import { notifications } from '../utils/notifications.js';

const LOG_LEVELS = [
    { value: 0, key: 'debug', className: 'debug' },
    { value: 1, key: 'info', className: 'info' },
    { value: 2, key: 'warn', className: 'warn' },
    { value: 3, key: 'error', className: 'error' }
];

class Logs {
    constructor() {
        this.container = document.getElementById('view-container');
        this.level = 0;
        this.limit = 200;
        this.autoRefresh = true;
        this.refreshTimer = null;

        window.addEventListener('languageChanged', () => {
            if (state.get('currentView') === 'logs') {
                this.render();
            }
        });
    }

    async render() {
        this.stopAutoRefresh();
        this.container.innerHTML = `
            <div class="logs-view">
                <div class="flex-between mb-3">
                    <div>
                        <h1>${t('logs.title')}</h1>
                        <p class="text-muted">${t('logs.subtitle')}</p>
                    </div>
                    <div class="flex gap-2 logs-actions">
                        <button class="btn btn-secondary" id="logs-copy-btn">${t('logs.copy')}</button>
                        <button class="btn btn-secondary" id="logs-refresh-btn">${t('common.refresh')}</button>
                        <button class="btn btn-danger" id="logs-clear-btn">${t('logs.clear')}</button>
                    </div>
                </div>

                <div class="card mb-3">
                    <div class="logs-toolbar">
                        <label class="form-label logs-toolbar-item">
                            ${t('logs.level')}
                            <select class="form-select" id="logs-level-select">
                                ${LOG_LEVELS.map(level => `
                                    <option value="${level.value}" ${this.level === level.value ? 'selected' : ''}>
                                        ${t(`logs.levels.${level.key}`)}
                                    </option>
                                `).join('')}
                            </select>
                        </label>
                        <label class="form-label logs-toolbar-item">
                            ${t('logs.limit')}
                            <select class="form-select" id="logs-limit-select">
                                ${[100, 200, 500, 1000].map(limit => `
                                    <option value="${limit}" ${this.limit === limit ? 'selected' : ''}>${limit}</option>
                                `).join('')}
                            </select>
                        </label>
                        <label class="logs-auto-refresh">
                            <input type="checkbox" id="logs-auto-refresh" ${this.autoRefresh ? 'checked' : ''}>
                            ${t('logs.autoRefresh')}
                        </label>
                    </div>
                </div>

                <div class="card">
                    <div class="logs-meta text-muted" id="logs-meta">${t('common.loading')}</div>
                    <div class="logs-panel" id="logs-panel">
                        <div class="flex-center"><div class="spinner"></div></div>
                    </div>
                </div>
            </div>
        `;

        this.bindEvents();
        await this.loadLogs();
        this.startAutoRefresh();
    }

    bindEvents() {
        document.getElementById('logs-refresh-btn').addEventListener('click', () => this.loadLogs());
        document.getElementById('logs-copy-btn').addEventListener('click', () => this.copyLogs());
        document.getElementById('logs-clear-btn').addEventListener('click', () => this.clearLogs());

        document.getElementById('logs-level-select').addEventListener('change', (event) => {
            this.level = Number(event.target.value);
            this.loadLogs();
        });

        document.getElementById('logs-limit-select').addEventListener('change', (event) => {
            this.limit = Number(event.target.value);
            this.loadLogs();
        });

        document.getElementById('logs-auto-refresh').addEventListener('change', (event) => {
            this.autoRefresh = event.target.checked;
            if (this.autoRefresh) {
                this.startAutoRefresh();
            } else {
                this.stopAutoRefresh();
            }
        });
    }

    async loadLogs() {
        const panel = document.getElementById('logs-panel');
        const meta = document.getElementById('logs-meta');
        if (!panel || !meta) {
            return;
        }

        try {
            const data = await api.getLogs({ level: this.level, limit: this.limit });
            const logs = data.logs || [];
            meta.textContent = this.formatMeta(data);

            if (logs.length === 0) {
                panel.innerHTML = `
                    <div class="empty-state">
                        <div class="empty-state-icon">📋</div>
                        <div class="empty-state-title">${t('logs.empty')}</div>
                        <div class="empty-state-message">${t('logs.emptyHint')}</div>
                    </div>
                `;
                return;
            }

            panel.innerHTML = logs.map(entry => this.renderEntry(entry)).join('');
            panel.scrollTop = panel.scrollHeight;
        } catch (error) {
            panel.innerHTML = `<div class="logs-error">${escapeHtml(error.message)}</div>`;
            notifications.error(`${t('logs.failedToLoad')}: ${error.message}`);
        }
    }

    renderEntry(entry) {
        const level = LOG_LEVELS.find(item => item.value === entry.level) || LOG_LEVELS[0];
        const timestamp = entry.timestamp ? formatDateTime(entry.timestamp) : '';
        const message = escapeHtml(entry.message || '');
        const icon = escapeHtml(entry.icon || '');
        const levelText = escapeHtml(entry.levelStr || t(`logs.levels.${level.key}`));

        return `
            <div class="log-entry log-entry-${level.className}">
                <span class="log-entry-time">${escapeHtml(timestamp)}</span>
                <span class="log-entry-level">${icon} ${levelText}</span>
                <span class="log-entry-message">${message}</span>
            </div>
        `;
    }

    formatMeta(data) {
        const total = data.total ?? 0;
        const shown = (data.logs || []).length;
        const truncated = data.isTruncated ? ` ${t('logs.truncated')}` : '';
        return t('logs.meta')
            .replace('{shown}', shown)
            .replace('{total}', total)
            .replace('{limit}', data.limit ?? this.limit) + truncated;
    }

    async copyLogs() {
        const panel = document.getElementById('logs-panel');
        const text = panel ? panel.innerText.trim() : '';
        if (!text) {
            notifications.warning(t('logs.nothingToCopy'));
            return;
        }

        try {
            await navigator.clipboard.writeText(text);
            notifications.success(t('logs.copied'));
        } catch (error) {
            notifications.error(`${t('logs.copyFailed')}: ${error.message}`);
        }
    }

    async clearLogs() {
        if (!confirm(t('logs.confirmClear'))) {
            return;
        }

        try {
            await api.clearLogs();
            notifications.success(t('logs.cleared'));
            await this.loadLogs();
        } catch (error) {
            notifications.error(`${t('logs.clearFailed')}: ${error.message}`);
        }
    }

    startAutoRefresh() {
        this.stopAutoRefresh();
        if (!this.autoRefresh) {
            return;
        }
        this.refreshTimer = setInterval(() => {
            if (state.get('currentView') !== 'logs') {
                this.stopAutoRefresh();
                return;
            }
            this.loadLogs();
        }, 3000);
    }

    stopAutoRefresh() {
        if (this.refreshTimer) {
            clearInterval(this.refreshTimer);
            this.refreshTimer = null;
        }
    }
}

export const logs = new Logs();
