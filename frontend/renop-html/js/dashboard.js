/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {fetchProto, getAuthHeaders} from './api.js';
import {formatBytes} from './browser/utils.js';
import {t} from './i18n.js';
import {createChartBar} from './components.js';
import {logout} from './auth.js';
import {InstanceStatus, StatusSnapshotList, UpdateState} from './proto/index.js';

let refreshInterval = null;
let snapshotsInterval = null;

let _uptimeBase = null;
let _uptimeReceivedAt = null;
let _uptimeTicker = null;

/**
 * Starts a 1s interval that advances the dashboard uptime display from the last calibrated base.
 * @returns {void}
 */
function _startUptimeTicker() {
    if (_uptimeTicker) return;
    _uptimeTicker = setInterval(() => {
        const el = document.getElementById('dashboard-uptime');
        if (!el) return;
        if (_uptimeBase === null || _uptimeReceivedAt === null) return;
        const elapsed = (Date.now() - _uptimeReceivedAt) / 1000;
        el.textContent = prettyUptime(_uptimeBase + elapsed);
    }, 1000);
}

/**
 * Stops the local uptime ticker interval if running.
 * @returns {void}
 */
function _stopUptimeTicker() {
    if (_uptimeTicker) {
        clearInterval(_uptimeTicker);
        _uptimeTicker = null;
    }
}

/**
 * Called whenever we receive a fresh uptime value from the API.
 * Calibrates the local base and smoothly continues from there.
 * @param {number} apiSeconds - Uptime in seconds from the instance status API.
 * @returns {void}
 */
function _calibrateUptime(apiSeconds) {
    _uptimeBase = apiSeconds;
    _uptimeReceivedAt = Date.now();
    const el = document.getElementById('dashboard-uptime');
    if (el) el.textContent = prettyUptime(apiSeconds);
}

/**
 * Shows skeleton placeholders on empty dashboard stat elements while data loads.
 * @returns {void}
 */
function renderDashboardSkeleton() {
    ['dashboard-version', 'dashboard-uptime', 'dashboard-memory', 'dashboard-disk', 'dashboard-threads', 'dashboard-cores', 'dashboard-os-arch', 'dashboard-failures'].forEach(id => {
        const el = document.getElementById(id);
        if (el && (!el.textContent || el.textContent.trim() === '' || el.textContent.trim() === '-')) {
            el.classList.add('skeleton-bone');
        }
    });
}

/**
 * Removes skeleton placeholders and plays a brief enter animation on previously skeleton elements.
 * @returns {void}
 */
function clearDashboardSkeleton() {
    ['dashboard-version', 'dashboard-uptime', 'dashboard-memory', 'dashboard-disk', 'dashboard-threads', 'dashboard-cores', 'dashboard-os-arch', 'dashboard-failures'].forEach(id => {
        const el = document.getElementById(id);
        if (el) {
            const wasSkeleton = el.classList.contains('skeleton-bone');
            el.classList.remove('skeleton-bone');
            if (wasSkeleton) {
                el.classList.remove('is-content-entering');
                void el.offsetWidth;
                el.classList.add('is-content-entering');
            }
        }
    });
}

let longPressTimer = null;
let isLongPressTriggered = false;

/**
 * Binds long-press (1.5s) on the update button to open the offline update modal.
 * Suppresses the subsequent click when a long-press fires. Idempotent per button.
 * @param {HTMLElement|null} updateBtn - The dashboard update button element.
 * @returns {void}
 */
function setupUpdateBtnLongPress(updateBtn) {
    if (!updateBtn || updateBtn.dataset.longPressBound === 'true') return;
    updateBtn.dataset.longPressBound = 'true';

    const startPress = () => {
        if (updateBtn.disabled) return;
        isLongPressTriggered = false;
        if (currentUpdateData && (currentUpdateData.status === 'ready_to_restart' || currentUpdateData.status === 'downloading')) {
            return;
        }
        updateBtn.classList.add('is-pressing');
        longPressTimer = setTimeout(() => {
            isLongPressTriggered = true;
            updateBtn.classList.remove('is-pressing');
            if (window.showOfflineUpdateModal) {
                window.showOfflineUpdateModal();
            }
            setTimeout(() => {
                isLongPressTriggered = false;
            }, 500);
        }, 1500);
    };

    const cancelPress = () => {
        if (longPressTimer) {
            clearTimeout(longPressTimer);
            longPressTimer = null;
        }
        updateBtn.classList.remove('is-pressing');
    };

    updateBtn.addEventListener('mousedown', startPress);
    updateBtn.addEventListener('touchstart', startPress, {passive: true});

    updateBtn.addEventListener('mouseup', cancelPress);
    updateBtn.addEventListener('mouseleave', cancelPress);
    updateBtn.addEventListener('touchend', cancelPress);
    updateBtn.addEventListener('touchcancel', cancelPress);

    updateBtn.addEventListener('click', (e) => {
        if (isLongPressTriggered) {
            e.preventDefault();
            e.stopPropagation();
            e.stopImmediatePropagation();
            isLongPressTriggered = false;
            return false;
        }
    }, true);
}

/**
 * Fetches instance status and updates dashboard stats (version, uptime, memory, disk, etc.).
 * Also wires the update button and applies updater UI when an update state is present.
 * @returns {Promise<void>}
 */
export async function fetchInstanceStatus() {
    renderDashboardSkeleton();
    try {
        const {response, data} = await fetchProto('/api/status/instance', InstanceStatus);
        if (response.ok && data) {
            clearDashboardSkeleton();

            let displayVersion = data.version || '';
            if (data.development && /^[0-9a-fA-F]{7,}$/.test(displayVersion)) {
                displayVersion = displayVersion.substring(0, 7);
            } else if (/^[0-9a-fA-F]{40}$/.test(displayVersion)) {
                displayVersion = displayVersion.substring(0, 7);
            }
            document.getElementById('dashboard-version').textContent = displayVersion;
            _calibrateUptime(data.uptime / 1000);
            const memEl = document.getElementById('dashboard-memory');
            const usedMem = data.used_memory !== undefined ? data.used_memory : data.usedMemory;
            const totalMem = data.total_memory !== undefined ? data.total_memory : data.totalMemory;
            if (memEl && usedMem !== undefined && totalMem !== undefined && usedMem !== null && totalMem !== null) {
                const usedBytes = Number(usedMem || 0) * 1024 * 1024;
                const totalBytes = Number(totalMem || 0) * 1024 * 1024;
                memEl.dataset.usedMemory = String(usedBytes);
                memEl.dataset.totalMemory = String(totalBytes);
                memEl.textContent = t('dashboard.memoryFormat', {
                    used: formatBytes(usedBytes),
                    total: formatBytes(totalBytes)
                });
            }
            const diskEl = document.getElementById('dashboard-disk');
            const renopUsedDisk = data.renop_used_disk !== undefined ? data.renop_used_disk : data.renopUsedDisk;
            const diskUsed = data.disk_used !== undefined ? data.disk_used : data.diskUsed;
            const diskTotal = data.disk_total !== undefined ? data.disk_total : data.diskTotal;
            if (diskEl && renopUsedDisk !== undefined && diskUsed !== undefined && diskTotal !== undefined) {
                diskEl.dataset.renopUsed = String(renopUsedDisk);
                diskEl.dataset.diskUsed = String(diskUsed);
                diskEl.dataset.diskTotal = String(diskTotal);
                diskEl.textContent = t('dashboard.diskFormat', {
                    renopUsed: formatBytes(renopUsedDisk),
                    diskUsed: formatBytes(diskUsed),
                    diskTotal: formatBytes(diskTotal)
                });
            }
            document.getElementById('dashboard-threads').textContent = `${data.used_threads}`;

            document.getElementById('dashboard-cores').textContent = `${data.logical_cores} / ${data.physical_cores}`;
            document.getElementById('dashboard-os-arch').textContent = `${data.os} / ${data.architecture}`;
            document.getElementById('dashboard-failures').textContent = data.failures_count !== undefined ? data.failures_count : '0';

            const updateBtn = document.getElementById('btn-dashboard-update');
            if (updateBtn) {
                setupUpdateBtnLongPress(updateBtn);
                if (!updateBtn.onclick) {
                    updateBtn.onclick = () => checkAppUpdate(true);
                }
            }

            if (data.update_state) {
                if (data.update_state.status !== 'idle' || currentUpdateData) {
                    currentUpdateData = data.update_state.status === 'idle' ? null : data.update_state;
                    updateDashboardVersionUI(data.update_state);
                }
            }
        } else if (response.status === 401 || response.status === 403) {
            logout('kicked');
        }
    } catch (e) {
        console.error('Failed to fetch instance status', e);
    }
}

/**
 * Formats a duration in seconds as a compact human-readable uptime string (e.g. "1d 2h 3min 4s").
 * @param {number} seconds - Total uptime in seconds.
 * @returns {string} Formatted uptime string.
 */
function prettyUptime(seconds) {
    const d = Math.floor(seconds / 86400);
    const h = Math.floor((seconds % 86400) / 3600);
    const m = Math.floor(((seconds % 86400) % 3600) / 60);
    const s = Math.floor(((seconds % 86400) % 3600) % 60);

    const dDisplay = d > 0 ? d + "d " : "";
    const hDisplay = h > 0 ? h + "h " : "";
    const mDisplay = m > 0 ? m + "min " : "";
    const sDisplay = s >= 0 ? s + "s" : "";

    return dDisplay + hDisplay + mDisplay + sDisplay;
}

/**
 * Fetches memory status snapshots and redraws the snapshots chart.
 * @returns {Promise<void>}
 */
async function fetchSnapshots() {
    try {
        const {response, data} = await fetchProto('/api/status/snapshots', StatusSnapshotList);
        if (response.ok && data) {
            drawSnapshotsChart(data.snapshots || []);
        }
    } catch (e) {
        console.error('Failed to fetch status snapshots', e);
    }
}

/**
 * Renders or updates the memory usage bar chart from status snapshot points.
 * Reuses existing bars when length matches; otherwise rebuilds the chart and drag handlers.
 * @param {Array<object>} data - Snapshot points with used_memory/usedMemory and timestamp.
 * @returns {void}
 */
function drawSnapshotsChart(data) {
    const container = document.getElementById('snapshots-chart-container');
    const chart = document.getElementById('snapshots-chart');
    if (!container || !chart) return;

    if (!Array.isArray(data) || data.length === 0) {
        container.style.display = 'none';
        return;
    }

    container.style.display = '';

    const card = chart.parentElement || chart;

    container.className = 'snapshots-chart-container';
    chart.className = 'snapshots-chart';

    const maxMemory = Math.max(...data.map(d => d.used_memory !== undefined ? d.used_memory : (d.usedMemory || 0)), 10);

    const existingBars = chart.querySelectorAll('.chart-bar');
    const existingLabels = card.querySelectorAll('.chart-y-label');

    if (existingBars.length === data.length) {
        existingLabels.forEach(el => el.remove());
        _appendYLabels(card, maxMemory);

        data.forEach((point, i) => {
            const usedMem = point.used_memory !== undefined ? point.used_memory : (point.usedMemory || 0);
            const heightPct = (usedMem / maxMemory) * 100;
            const bar = existingBars[i];
            bar.style.height = `${heightPct}%`;
            bar.title = `${formatBytes(usedMem * 1024 * 1024)} at ${new Date(point.timestamp).toLocaleTimeString()}`;
        });
    } else {
        existingLabels.forEach(el => el.remove());
        chart.innerHTML = '';
        _appendYLabels(card, maxMemory);

        data.forEach((point) => {
            const usedMem = point.used_memory !== undefined ? point.used_memory : (point.usedMemory || 0);
            const heightPct = (usedMem / maxMemory) * 100;
            const barTitle = `${formatBytes(usedMem * 1024 * 1024)} at ${new Date(point.timestamp).toLocaleTimeString()}`;

            const bar = createChartBar(0, barTitle);
            chart.appendChild(bar);

            bar.getBoundingClientRect();
            bar.setAttribute('height-pct', String(heightPct));
            bar.style.height = `${heightPct}%`;
        });
    }
    if (!chart.dataset.dragInitialized) {
        chart.dataset.dragInitialized = 'true';
        let isDown = false;
        let startX = 0;
        let startScrollLeft = 0;

        chart.addEventListener('mousedown', (e) => {
            isDown = true;
            chart.style.cursor = 'grabbing';
            startX = e.pageX - chart.offsetLeft;
            startScrollLeft = chart.scrollLeft;
        });
        chart.addEventListener('mouseleave', () => {
            isDown = false;
            chart.style.cursor = 'grab';
        });
        chart.addEventListener('mouseup', () => {
            isDown = false;
            chart.style.cursor = 'grab';
        });
        chart.addEventListener('mousemove', (e) => {
            if (!isDown) return;
            e.preventDefault();
            const x = e.pageX - chart.offsetLeft;
            const walk = (x - startX) * 1.5;
            chart.scrollLeft = startScrollLeft - walk;
        });
        chart.addEventListener('wheel', (e) => {
            if (e.deltaY !== 0 && Math.abs(e.deltaX) < Math.abs(e.deltaY)) {
                chart.scrollLeft += e.deltaY;
                e.preventDefault();
            }
        }, {passive: false});
    }

    requestAnimationFrame(() => {
        chart.scrollLeft = chart.scrollWidth;
    });
}

/**
 * Appends top/bottom Y-axis labels for the snapshots chart based on max memory (MB).
 * @param {HTMLElement} card - Chart card element that hosts the Y labels.
 * @param {number} maxMemory - Maximum used memory in megabytes across snapshots.
 * @returns {void}
 */
function _appendYLabels(card, maxMemory) {
    const maxLabel = document.createElement('div');
    maxLabel.className = 'chart-y-label chart-y-label--top';
    maxLabel.textContent = formatBytes(maxMemory * 1024 * 1024, 0);

    const minLabel = document.createElement('div');
    minLabel.className = 'chart-y-label chart-y-label--bottom';
    minLabel.textContent = '0 B';

    card.appendChild(maxLabel);
    card.appendChild(minLabel);
}

let currentUpdateData = null;

/**
 * Updates the version badge and update button according to the current updater state.
 * @param {object} data - Updater state (has_update, status, latest_version, progress, etc.).
 * @returns {void}
 */
export function updateDashboardVersionUI(data) {
    const versionEl = document.getElementById('dashboard-version');
    const updateBtn = document.getElementById('btn-dashboard-update');
    if (!versionEl || !updateBtn) return;

    setupUpdateBtnLongPress(updateBtn);

    let badge = document.getElementById('dashboard-version-badge');
    if (data.has_update || data.status === 'available') {
        if (!badge) {
            badge = document.createElement('span');
            badge.id = 'dashboard-version-badge';
            badge.className = 'badge badge-warning';
            versionEl.parentNode.insertBefore(badge, versionEl.nextSibling);
        } else {
            badge.className = 'badge badge-warning';
        }
        badge.textContent = t('dashboard.hasNewVersion', {version: data.latest_version});

        updateBtn.textContent = t('dashboard.downloadUpdate');
        updateBtn.className = 'pill-btn pill-btn--primary pill-btn--sm manager-only';
        updateBtn.disabled = false;
        updateBtn.onclick = async () => {
            const updateInfo = currentUpdateData || data;
            if (window.showUpdateModal) {
                const confirmed = await window.showUpdateModal(updateInfo);
                if (confirmed) {
                    await installAppUpdate();
                }
            } else if (await window.showConfirm(t('dashboard.confirmDownload', {version: updateInfo.latest_version}))) {
                await installAppUpdate();
            }
        };
    } else if (data.status === 'ready_to_restart') {
        isLongPressTriggered = false;
        if (!badge) {
            badge = document.createElement('span');
            badge.id = 'dashboard-version-badge';
            badge.className = 'badge badge-success';
            versionEl.parentNode.insertBefore(badge, versionEl.nextSibling);
        } else {
            badge.className = 'badge badge-success';
        }
        badge.textContent = t('dashboard.versionReady');

        updateBtn.textContent = t('dashboard.restartToUpdate');
        updateBtn.className = 'pill-btn pill-btn--success pill-btn--sm manager-only';
        updateBtn.disabled = false;
        updateBtn.onclick = async () => {
            if (await window.showConfirm(t('dashboard.confirmRestart'))) {
                await restartApp();
            }
        };
    } else if (data.status === 'downloading') {
        updateBtn.textContent = t('dashboard.downloadingProgress', {progress: data.progress || 0});
        updateBtn.className = 'pill-btn pill-btn--soft pill-btn--sm manager-only';
        updateBtn.disabled = true;
    } else {
        if (badge) badge.remove();
        updateBtn.textContent = t('dashboard.checkUpdate');
        updateBtn.className = 'pill-btn pill-btn--soft pill-btn--sm manager-only';
        updateBtn.disabled = false;
        updateBtn.onclick = () => checkAppUpdate(true);
    }
}

/**
 * Quietly polls updater status and refreshes the dashboard version UI when not idle.
 * @returns {Promise<void>}
 */
export async function fetchUpdaterStatusQuietly() {
    try {
        const {response, data: status} = await fetchProto('/api/updater/status', UpdateState);
        if (response.ok && status) {
            if (status.status !== 'idle') {
                currentUpdateData = status;
                updateDashboardVersionUI(status);
            }
        }
    } catch {}
}
window.fetchUpdaterStatusQuietly = fetchUpdaterStatusQuietly;

/**
 * Checks for application updates via the updater API.
 * When manual is true, shows loading state, alerts, and optional install confirmation.
 * @param {boolean} [manual=false] - Whether the check was user-initiated.
 * @returns {Promise<void>}
 */
export async function checkAppUpdate(manual = false) {
    const updateBtn = document.getElementById('btn-dashboard-update');
    const origText = updateBtn ? updateBtn.textContent : '';

    if (manual && updateBtn) {
        updateBtn.disabled = true;
        updateBtn.textContent = t('dashboard.checkingUpdate');
    }

    try {
        const headers = getAuthHeaders();
        const resp = await fetch('/api/updater/check', {method: 'POST', headers});
        if (!resp.ok) {
            let errorText = `HTTP ${resp.status}`;
            try {
                const errJson = await resp.json();
                if (errJson.error) errorText = errJson.error;
            } catch {}
            if (manual) {
                window.showAlert(t('dashboard.checkUpdateFailed', {error: errorText}), 'error');
            }
            return;
        }

        const data = await resp.json();
        currentUpdateData = data;

        if (data.has_update) {
            updateDashboardVersionUI(data);
            if (manual) {
                const confirmed = window.showUpdateModal ? await window.showUpdateModal(data) : await window.showConfirm(t('dashboard.confirmDownload', {version: data.latest_version}));
                if (confirmed) {
                    await installAppUpdate();
                }
            }
        } else {
            updateDashboardVersionUI(data);
            if (manual) {
                window.showAlert(t('dashboard.alreadyLatest'), 'info');
            }
        }
    } catch (e) {
        console.error('Update check error', e);
        if (manual) {
            window.showAlert(t('dashboard.checkUpdateFailed', {error: e.message || t('common.unknown')}), 'error');
        }
    } finally {
        if (manual && updateBtn && (!currentUpdateData || !currentUpdateData.has_update)) {
            updateBtn.disabled = false;
            if (updateBtn.textContent === t('dashboard.checkingUpdate') || updateBtn.textContent === '检查中...') {
                updateBtn.textContent = origText || t('dashboard.checkUpdate');
            }
        }
    }
}

/**
 * Starts downloading/installing the available app update and begins status polling.
 * @returns {Promise<void>}
 */
export async function installAppUpdate() {
    try {
        const headers = getAuthHeaders();
        const resp = await fetch('/api/updater/install', {method: 'POST', headers});
        if (resp.ok) {
            window.showAlert(t('dashboard.downloadingBg'), 'info');
            pollUpdaterStatus();
        } else {
            let errorText = `HTTP ${resp.status}`;
            try {
                const errJson = await resp.json();
                if (errJson.error) errorText = errJson.error;
            } catch {}
            window.showAlert(t('dashboard.startDownloadFailed', {error: errorText}), 'error');
        }
    } catch (e) {
        console.error('Install update error', e);
        window.showAlert(t('dashboard.startDownloadFailed', {error: e.message || t('common.unknown')}), 'error');
    }
}

let updaterPollTimer = null;

/**
 * Polls updater status every 3s until ready_to_restart or error; prompts restart when ready.
 * No-ops if a poll interval is already running.
 * @returns {void}
 */
export function pollUpdaterStatus() {
    if (updaterPollTimer) return;
    updaterPollTimer = setInterval(async () => {
        try {
            const {response, data: status} = await fetchProto('/api/updater/status', UpdateState);
            if (!response.ok || !status) return;
            currentUpdateData = status;
            updateDashboardVersionUI(status);

            if (status.status === 'ready_to_restart') {
                clearInterval(updaterPollTimer);
                updaterPollTimer = null;
                if (await window.showConfirm(t('dashboard.confirmRestart'))) {
                    await restartApp();
                }
            } else if (status.status === 'error') {
                clearInterval(updaterPollTimer);
                updaterPollTimer = null;
                window.showAlert(t('dashboard.updateError', {error: status.error_message || t('common.unknown')}), 'error');
            }
        } catch {}
    }, 3000);
}

/**
 * Requests an application restart to apply a downloaded update, then reloads the page.
 * @returns {Promise<void>}
 */
export async function restartApp() {
    try {
        const headers = getAuthHeaders();
        const resp = await fetch('/api/updater/restart', {method: 'POST', headers});
        if (!resp.ok) {
            let errorText = `HTTP ${resp.status}`;
            try {
                const errJson = await resp.json();
                if (errJson.error) errorText = errJson.error;
            } catch {
                try {
                    const text = await resp.text();
                    if (text) errorText = text;
                } catch {}
            }
            window.showAlert(t('dashboard.restartFailed', {error: errorText}), 'error');
            return;
        }
        window.showAlert(t('dashboard.restarting'), 'success');
        setTimeout(() => window.location.reload(), 4000);
    } catch (e) {
        console.error('Restart error', e);
        window.showAlert(t('dashboard.restartFailed', {error: e.message || t('common.unknown')}), 'error');
    }
}

/**
 * Starts uptime ticker, initial status/snapshot fetches, and periodic refresh intervals.
 * @returns {void}
 */
export function startDashboardRefresh() {
    _startUptimeTicker();
    fetchInstanceStatus();
    fetchSnapshots();
    if (!refreshInterval) {
        refreshInterval = setInterval(fetchInstanceStatus, 10000);
    }
    if (!snapshotsInterval) {
        snapshotsInterval = setInterval(fetchSnapshots, 60000);
    }
}

/**
 * Stops the uptime ticker and all dashboard refresh intervals.
 * @returns {void}
 */
export function stopDashboardRefresh() {
    _stopUptimeTicker();
    if (refreshInterval) {
        clearInterval(refreshInterval);
        refreshInterval = null;
    }
    if (snapshotsInterval) {
        clearInterval(snapshotsInterval);
        snapshotsInterval = null;
    }
}

window.addEventListener('languageChanged', () => {
    const memoryEl = document.getElementById('dashboard-memory');
    if (memoryEl && memoryEl.dataset.usedMemory !== undefined && memoryEl.dataset.totalMemory !== undefined) {
        memoryEl.textContent = t('dashboard.memoryFormat', {
            used: formatBytes(Number(memoryEl.dataset.usedMemory)),
            total: formatBytes(Number(memoryEl.dataset.totalMemory))
        });
    }
    const diskEl = document.getElementById('dashboard-disk');
    if (diskEl && diskEl.dataset.renopUsed !== undefined && diskEl.dataset.diskUsed !== undefined && diskEl.dataset.diskTotal !== undefined) {
        diskEl.textContent = t('dashboard.diskFormat', {
            renopUsed: formatBytes(Number(diskEl.dataset.renopUsed)),
            diskUsed: formatBytes(Number(diskEl.dataset.diskUsed)),
            diskTotal: formatBytes(Number(diskEl.dataset.diskTotal))
        });
    }
    if (currentUpdateData) {
        updateDashboardVersionUI(currentUpdateData);
    } else {
        const updateBtn = document.getElementById('btn-dashboard-update');
        if (updateBtn && !updateBtn.disabled && (!currentUpdateData || !currentUpdateData.has_update)) {
            updateBtn.textContent = t('dashboard.checkUpdate');
        }
    }
});
