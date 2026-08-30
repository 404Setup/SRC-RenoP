/*
 * Copyright (c) 2026 404Setup. All rights reserved.
 *
 * This Source Code Form is subject to the terms of the Mozilla Public License, v. 2.0. If a copy of the MPL was not distributed with this file, You can obtain one at https://mozilla.org/MPL/2.0/.
 *
 * If it is not possible or desirable to put the notice in a particular file, then You may include the notice in a location (such as a LICENSE file in a relevant directory) where a recipient would be likely to look for such a notice.
 *
 * This Source Code Form is "Incompatible With Secondary Licenses", as defined by the Mozilla Public License, v. 2.0.
 */

import {apiRequest, fetchProto, getAuthHeaders} from './api.js';
import {formatBytes} from './browser/utils.js';
import {t} from './i18n.js';
import {logout} from './auth.js';
import {showAlert} from './alert.js';
import {InstanceStatus, StatusSnapshotList, UpdateState} from './proto/index.js';
import {formatTimestamp} from './time.js';
import {refreshMessageUnreadCount} from './messages.js';
import {updaterErrorMessage} from './updater-errors.js';
import {responseErrorMessage} from './response-errors.js';
import {exitProtectedRouteOnDenial} from './protected-route.js';

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
 * Shows or hides debug memory dump tools (requires server.debug_mode + restart).
 * @param {boolean} active
 * @returns {void}
 */
function updateDebugMemoryTools(active) {
    const panel = document.getElementById('dashboard-debug-tools');
    if (!panel) return;
    panel.hidden = !active;
    if (!active) return;
    _wireDebugDumpButton('btn-dump-heap', 'heap');
    _wireDebugDumpButton('btn-dump-allocs', 'allocs');
    _wireDebugDumpButton('btn-dump-goroutine', 'goroutine');
}

/**
 * @param {string} buttonId
 * @param {'heap'|'allocs'|'goroutine'} kind
 */
function _wireDebugDumpButton(buttonId, kind) {
    const btn = document.getElementById(buttonId);
    if (!btn || btn.dataset.wired === '1') return;
    btn.dataset.wired = '1';
    btn.addEventListener('click', () => {
        void downloadMemoryProfile(kind, btn);
    });
}

/**
 * Downloads a pprof profile for flame-graph analysis (Speedscope / go tool pprof).
 * @param {'heap'|'allocs'|'goroutine'} kind
 * @param {HTMLButtonElement|null} [btn]
 * @returns {Promise<void>}
 */
async function downloadMemoryProfile(kind, btn = null) {
    const path = kind === 'allocs'
        ? '/api/debug/memory/allocs'
        : kind === 'goroutine'
            ? '/api/debug/memory/goroutine'
            : '/api/debug/memory/heap?gc=1';
    const filename = kind === 'allocs'
        ? 'renop-allocs.pprof'
        : kind === 'goroutine'
            ? 'renop-goroutine.pprof'
            : 'renop-heap.pprof';

    const prevText = btn ? btn.textContent : '';
    if (btn) {
        btn.disabled = true;
        btn.textContent = t('dashboard.dumpWorking');
    }
    try {
        const response = await apiRequest(path, {method: 'GET'});
        if (!response.ok) {
            showAlert(await responseErrorMessage(response, 'dashboard.dumpFailed'), 'error');
            return;
        }
        const blob = await response.blob();
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
        showAlert(t('dashboard.dumpReadyBody', {file: filename}), 'success');
    } catch (e) {
        if (e && e.message === 'Unauthorized') return;
        console.error('memory profile dump failed', e);
        showAlert(t('dashboard.dumpFailed'), 'error');
    } finally {
        if (btn) {
            btn.disabled = false;
            btn.textContent = prevText || t(
                kind === 'allocs' ? 'dashboard.dumpAllocs'
                    : kind === 'goroutine' ? 'dashboard.dumpGoroutine'
                        : 'dashboard.dumpHeap',
            );
        }
    }
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
            const rssMem = data.used_memory !== undefined ? data.used_memory : data.usedMemory;
            const vssMem = data.vss_memory !== undefined ? data.vss_memory : data.vssMemory;
            const totalMem = data.total_memory !== undefined ? data.total_memory : data.totalMemory;
            if (memEl && rssMem !== undefined && totalMem !== undefined && rssMem !== null && totalMem !== null) {
                // API reports memory fields in bytes (not MiB).
                const rssBytes = Number(rssMem || 0);
                const vssBytes = Number(vssMem || 0);
                const totalBytes = Number(totalMem || 0);
                memEl.dataset.rssMemory = String(rssBytes);
                memEl.dataset.vssMemory = String(vssBytes);
                memEl.dataset.totalMemory = String(totalBytes);
                // Backward-compat keys for languageChanged handlers / older markup.
                memEl.dataset.usedMemory = String(rssBytes);
                memEl.textContent = t('dashboard.memoryFormat', {
                    rss: formatBytes(rssBytes),
                    vss: formatBytes(vssBytes),
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
            const archDisplay = data.architecture || '';
            document.getElementById('dashboard-os-arch').textContent = `${data.os} / ${archDisplay}`;
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

            updateDebugMemoryTools(!!data.debug_mode);
        } else if (exitProtectedRouteOnDenial(response)) {
            if (response.status === 401) void logout('kicked');
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
 * Reads RSS (used_memory) and VSS from a snapshot point, in bytes.
 * @param {object} point - Status snapshot.
 * @returns {{rss: number, vss: number}}
 */
function _snapshotMemoryBytes(point) {
    const rss = Number(point.used_memory !== undefined ? point.used_memory : (point.usedMemory || 0)) || 0;
    let vss = Number(point.vss_memory !== undefined ? point.vss_memory : (point.vssMemory || 0)) || 0;
    if (vss < rss) vss = rss;
    return {rss, vss};
}

/**
 * Builds a tooltip for a memory snapshot bar (RSS / VSS / total process occupancy).
 * @param {number} rssBytes
 * @param {number} vssBytes
 * @param {number|string|Date} timestamp
 * @returns {string}
 */
function _memoryBarTitle(rssBytes, vssBytes, timestamp) {
    const time = formatTimestamp(timestamp, {timeOnly: true, fallback: t('common.unknown')});
    const totalBytes = Math.max(rssBytes, vssBytes);
    return t('dashboard.chartBarTitle', {
        rss: formatBytes(rssBytes),
        vss: formatBytes(vssBytes),
        total: formatBytes(totalBytes),
        time
    });
}

/**
 * Applies RSS/VSS segment heights on a chart bar group.
 * Full bar height follows process total occupancy (max of RSS/VSS); VSS and RSS
 * are layered inside the same column.
 * @param {HTMLElement} group
 * @param {number} rssBytes
 * @param {number} vssBytes
 * @param {number} maxMemoryBytes
 * @returns {void}
 */
function _applyMemoryBarHeights(group, rssBytes, vssBytes, maxMemoryBytes) {
    const totalBytes = Math.max(rssBytes, vssBytes);
    const totalPct = maxMemoryBytes > 0 ? (totalBytes / maxMemoryBytes) * 100 : 0;
    const vssPctOfBar = totalBytes > 0 ? (vssBytes / totalBytes) * 100 : 0;
    const rssPctOfBar = totalBytes > 0 ? (rssBytes / totalBytes) * 100 : 0;

    group.style.height = `${totalPct}%`;
    group.title = group.dataset.barTitle || '';

    const vssSeg = group.querySelector('.chart-bar-seg--vss');
    const rssSeg = group.querySelector('.chart-bar-seg--rss');
    if (vssSeg) vssSeg.style.height = `${vssPctOfBar}%`;
    if (rssSeg) rssSeg.style.height = `${rssPctOfBar}%`;
}

/**
 * Creates a single chart column with layered RSS / VSS segments.
 * @param {string} title
 * @returns {HTMLElement}
 */
function _createMemoryChartBar(title) {
    const group = document.createElement('div');
    group.className = 'chart-bar-group';
    group.dataset.barTitle = title;
    group.title = title;

    const vssSeg = document.createElement('div');
    vssSeg.className = 'chart-bar-seg chart-bar-seg--vss';
    vssSeg.style.height = '0%';

    const rssSeg = document.createElement('div');
    rssSeg.className = 'chart-bar-seg chart-bar-seg--rss';
    rssSeg.style.height = '0%';

    group.appendChild(vssSeg);
    group.appendChild(rssSeg);
    return group;
}

/**
 * Renders or updates the memory usage bar chart from status snapshot points.
 * Each column layers RSS and VSS (and thus process total occupancy) in one bar.
 * Reuses existing bars when length matches; otherwise rebuilds the chart and drag handlers.
 * @param {Array<object>} data - Snapshot points with used_memory/vss_memory and timestamp.
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

    const maxMemory = Math.max(
        ...data.map((d) => {
            const {rss, vss} = _snapshotMemoryBytes(d);
            return Math.max(rss, vss);
        }),
        10 * 1024 * 1024 // floor axis at 10 MiB so tiny samples still render
    );

    const existingBars = chart.querySelectorAll('.chart-bar-group');
    const existingLabels = card.querySelectorAll('.chart-y-label');

    if (existingBars.length === data.length) {
        existingLabels.forEach(el => el.remove());
        _appendYLabels(card, maxMemory);

        data.forEach((point, i) => {
            const {rss, vss} = _snapshotMemoryBytes(point);
            const bar = existingBars[i];
            bar.dataset.barTitle = _memoryBarTitle(rss, vss, point.timestamp);
            _applyMemoryBarHeights(bar, rss, vss, maxMemory);
        });
    } else {
        existingLabels.forEach(el => el.remove());
        chart.innerHTML = '';
        _appendYLabels(card, maxMemory);

        data.forEach((point) => {
            const {rss, vss} = _snapshotMemoryBytes(point);
            const barTitle = _memoryBarTitle(rss, vss, point.timestamp);
            const bar = _createMemoryChartBar(barTitle);
            chart.appendChild(bar);

            bar.getBoundingClientRect();
            _applyMemoryBarHeights(bar, rss, vss, maxMemory);
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
 * Appends top/bottom Y-axis labels for the snapshots chart based on max memory (bytes).
 * @param {HTMLElement} card - Chart card element that hosts the Y labels.
 * @param {number} maxMemoryBytes - Maximum used memory in bytes across snapshots.
 * @returns {void}
 */
function _appendYLabels(card, maxMemoryBytes) {
    const maxLabel = document.createElement('div');
    maxLabel.className = 'chart-y-label chart-y-label--top';
    maxLabel.textContent = formatBytes(maxMemoryBytes, 0);

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
    } catch {
    }
}

window.fetchUpdaterStatusQuietly = fetchUpdaterStatusQuietly;

/**
 * Checks for application updates via the updater API.
 * Manual checks show button progress while the backend records the result in the message center.
 * @param {boolean} [manual=false] - Whether the check was user-initiated.
 * @returns {Promise<void>}
 */
export async function checkAppUpdate(manual = false) {
    const updateBtn = document.getElementById('btn-dashboard-update');
    const origText = updateBtn ? updateBtn.textContent : '';
    let renderedResult = false;

    if (manual && updateBtn) {
        updateBtn.disabled = true;
        updateBtn.textContent = t('dashboard.checkingUpdate');
    }

    try {
        const headers = getAuthHeaders();
        const resp = await fetch('/api/updater/check', {method: 'POST', headers});
        if (!resp.ok) {
            return;
        }

        const data = await resp.json();
        currentUpdateData = data;

        updateDashboardVersionUI(data);
        renderedResult = true;
    } catch (e) {
        console.error('Update check error', e);
    } finally {
        if (manual) await refreshMessageUnreadCount();
        if (manual && updateBtn && !renderedResult) {
            if (currentUpdateData) updateDashboardVersionUI(currentUpdateData);
            else {
                updateBtn.disabled = false;
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
            showAlert(t('dashboard.downloadingBg'), 'info');
            pollUpdaterStatus();
        } else {
            showAlert(updaterErrorMessage(resp, 'updaterNotice.installFailedTitle'), 'error');
        }
    } catch (e) {
        console.error('Install update error', e);
        showAlert(t('updaterNotice.installFailedTitle'), 'error');
    } finally {
        await refreshMessageUnreadCount();
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
                await refreshMessageUnreadCount();
            } else if (status.status === 'error') {
                clearInterval(updaterPollTimer);
                updaterPollTimer = null;
                await refreshMessageUnreadCount();
            }
        } catch {
        }
    }, 3000);
}

/**
 * Requests an application restart to apply a downloaded update, then reloads the page.
 * @returns {Promise<void>}
 */
export async function restartApp() {
    showAlert(t('dashboard.restarting'), 'info');
    try {
        const headers = getAuthHeaders();
        const resp = await fetch('/api/updater/restart', {method: 'POST', headers});
        if (!resp.ok) {
            showAlert(updaterErrorMessage(resp, 'updaterNotice.restartFailedTitle'), 'error');
            await refreshMessageUnreadCount();
            return;
        }
        setTimeout(() => window.location.reload(), 4000);
    } catch (e) {
        console.error('Restart error', e);
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
    if (memoryEl && memoryEl.dataset.totalMemory !== undefined) {
        const rssBytes = Number(memoryEl.dataset.rssMemory ?? memoryEl.dataset.usedMemory ?? 0);
        const vssBytes = Number(memoryEl.dataset.vssMemory ?? 0);
        memoryEl.textContent = t('dashboard.memoryFormat', {
            rss: formatBytes(rssBytes),
            vss: formatBytes(vssBytes),
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
