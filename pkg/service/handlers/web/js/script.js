async function fetchSettings() {
    try {
        const response = await fetch('/setup/settings');
        const settings = await response.json();
        if (settings.server_url) {
            document.getElementById('target-domain').value = settings.server_url;
        }
        if (settings.proxy_url) {
            document.getElementById('soundcork-url').value = settings.proxy_url;
        }
        if (settings.discovery_interval) {
            document.getElementById('discovery-interval').value = settings.discovery_interval;
        }
        if (settings.discovery_enabled !== undefined) {
            document.getElementById('discovery-enabled').checked = settings.discovery_enabled;
        }
        if (settings.enable_soundcork_proxy !== undefined) {
            document.getElementById('enable-soundcork-proxy').checked = settings.enable_soundcork_proxy;
        }
        fetchProxySettings();
    } catch (error) {
        console.error('Failed to fetch settings', error);
    }
}

async function fetchProxySettings() {
    try {
        const response = await fetch('/setup/proxy-settings');
        const settings = await response.json();
        document.getElementById('proxy-redact').checked = settings.redact;
        document.getElementById('proxy-log-body').checked = settings.log_body;
        document.getElementById('proxy-record').checked = settings.record;
        if (settings.enable_soundcork_proxy !== undefined) {
            document.getElementById('enable-soundcork-proxy').checked = settings.enable_soundcork_proxy;
        }
    } catch (error) {
        console.error('Failed to fetch proxy settings', error);
    }
}

async function updateProxySettings() {
    const settings = {
        redact: document.getElementById('proxy-redact').checked,
        log_body: document.getElementById('proxy-log-body').checked,
        record: document.getElementById('proxy-record').checked,
        enable_soundcork_proxy: document.getElementById('enable-soundcork-proxy').checked
    };
    try {
        await fetch('/setup/proxy-settings', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(settings)
        });
    } catch (error) {
        console.error('Failed to update proxy settings', error);
    }
}

async function updateSettings() {
    const settings = {
        server_url: document.getElementById('target-domain').value,
        proxy_url: document.getElementById('soundcork-url').value,
        discovery_interval: document.getElementById('discovery-interval').value,
        discovery_enabled: document.getElementById('discovery-enabled').checked,
        enable_soundcork_proxy: document.getElementById('enable-soundcork-proxy').checked
    };
    const status = document.getElementById('settings-status');
    status.innerText = 'Saving...';
    status.style.color = 'blue';

    try {
        const response = await fetch('/setup/settings', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(settings)
        });
        if (response.ok) {
            status.innerText = '✅ Settings saved. Restart service to apply all changes (like certificate SANs).';
            status.style.color = 'green';
            setTimeout(() => fetchSettings(), 500); // Give backend a moment to settle
        } else {
            const err = await response.text();
            status.innerText = '❌ Failed: ' + err;
            status.style.color = 'red';
        }
    } catch (error) {
        status.innerText = '❌ Error: ' + error.message;
        status.style.color = 'red';
    }
}

async function fetchDevices() {
    try {
        const response = await fetch('/setup/devices');
        const devices = await response.json();
        const container = document.getElementById('device-list');
        const syncSelector = document.getElementById('sync-device-list');
        const migrationSelector = document.getElementById('migration-device-list');

        if (devices.length === 0) {
            container.innerHTML = 'No devices known yet.';
        } else {
            let html = '<table><tr><th>Name & Model</th><th>IP Address</th><th>Device & Account ID</th><th>Firmware & Serial</th><th>Method</th><th>Action</th></tr>';

            // Clear and repopulate selectors
            const currentSyncVal = syncSelector.value;
            const currentMigrationVal = migrationSelector.value;
            const eventSelector = document.getElementById('event-device-selector');
            const currentEventVal = eventSelector ? eventSelector.value : "";

            syncSelector.innerHTML = '<option value="">-- Select a device --</option>';
            migrationSelector.innerHTML = '<option value="">-- Select a device --</option>';
            if (eventSelector) eventSelector.innerHTML = '<option value="">-- Select a device --</option>';

            devices.forEach(d => {
                const methodLabel = d.discovery_method === 'manual' ? '👤 Manual' : '🔍 Auto';
                html += `
                    <tr id="device-row-${d.ip_address.replace(/\./g, '-')}">
                        <td class="col-name-model"><div class="col-name">${d.name}</div><div class="col-model" style="font-size: 0.8em; color: #666;">${d.product_code}</div></td>
                        <td class="col-ip">${d.ip_address}</td>
                        <td class="col-ids"><div class="col-deviceid">${d.device_id}</div><div class="col-accountid" style="font-size: 0.8em; color: #666;">${d.account_id || 'default'}</div></td>
                        <td class="col-fw-serial"><div class="col-firmware">${d.firmware_version || '0.0.0'}</div><div class="col-serial" style="font-size: 0.8em; color: #666;">${d.device_serial_number}</div></td>
                        <td class="col-method">${methodLabel}</td>
                        <td>
                            <button onclick="prepareSync('${d.ip_address}')">Sync Data</button>
                            <button onclick="prepareMigration('${d.ip_address}')">Migrate</button>
                            <button class="btn-danger" onclick="removeDevice('${d.device_id}', '${d.name}')">Remove</button>
                        </td>
                    </tr>
                `;

                const optSync = document.createElement('option');
                optSync.value = d.ip_address;
                optSync.textContent = `${d.name} (${d.ip_address})`;
                syncSelector.appendChild(optSync);

                const optMigrate = document.createElement('option');
                optMigrate.value = d.ip_address;
                optMigrate.textContent = `${d.name} (${d.ip_address})`;
                migrationSelector.appendChild(optMigrate);

                if (eventSelector) {
                    const optEvent = document.createElement('option');
                    optEvent.value = d.device_id || d.ip_address;
                    optEvent.textContent = `${d.name} (${d.ip_address})`;
                    eventSelector.appendChild(optEvent);
                }
            });
            html += '</table>';
            container.innerHTML = html;

            if (currentSyncVal) syncSelector.value = currentSyncVal;
            if (currentMigrationVal) migrationSelector.value = currentMigrationVal;
            if (eventSelector && currentEventVal) eventSelector.value = currentEventVal;

            // Asynchronously fetch live info for each device
            devices.forEach(d => updateDeviceInfo(d.ip_address));
        }
    } catch (error) {
        document.getElementById('device-list').innerHTML = 'Error loading devices: ' + error;
    }
}

function prepareSync(ip) {
    document.getElementById('sync-device-list').value = ip;
    openTab(null, 'tab-sync');
}

function prepareMigration(ip) {
    document.getElementById('migration-device-list').value = ip;
    openTab(null, 'tab-migration');
    showSummary(ip);
}

function openTab(evt, tabId) {
    const tabcontents = document.getElementsByClassName("tab-content");
    for (let i = 0; i < tabcontents.length; i++) {
        tabcontents[i].className = tabcontents[i].className.replace(" active", "");
    }

    const tablinks = document.getElementsByClassName("tab-btn");
    for (let i = 0; i < tablinks.length; i++) {
        tablinks[i].className = tablinks[i].className.replace(" active", "");
    }

    const content = document.getElementById(tabId);
    if (content) {
        content.className += " active";
    }

    if (tabId === 'tab-interactions') {
        fetchInteractionStats();
        fetchInteractions();
    }

    if (evt) {
        evt.currentTarget.className += " active";
    } else {
        // Find the button that corresponds to the tabId and activate it
        for (let i = 0; i < tablinks.length; i++) {
            const onclick = tablinks[i].getAttribute('onclick');
            if (onclick && onclick.includes(tabId)) {
                tablinks[i].className += " active";
                break;
            }
        }
    }
}

async function startSync() {
    const ip = document.getElementById('sync-device-list').value;
    if (!ip) {
        alert('Please select a device first');
        return;
    }

    const status = document.getElementById('sync-status');
    const results = document.getElementById('sync-results');
    const log = document.getElementById('sync-log');

    status.style.display = 'block';
    status.style.backgroundColor = '#eef';
    status.textContent = 'Syncing data from ' + ip + '...';
    results.style.display = 'none';
    log.innerHTML = '';

    try {
        const response = await fetch('/setup/sync/' + ip, { method: 'POST' });
        if (response.ok) {
            status.style.backgroundColor = '#dfd';
            status.textContent = '✅ Sync completed successfully!';
            results.style.display = 'block';
            log.innerHTML = 'Data fetched and saved to local datastore.\nPresets: OK\nRecents: OK\nSources: OK';
        } else {
            const err = await response.text();
            throw new Error(err);
        }
    } catch (error) {
        status.style.backgroundColor = '#fdd';
        status.textContent = '❌ Sync failed: ' + error.message;
    }
}

async function fetchVersion() {
    try {
        const response = await fetch('/setup/version');
        const data = await response.json();
        const info = document.getElementById('version-info');
        if (info && data.version) {
            info.innerText = `AfterTouch ${data.version} (${data.commit}) - ${data.date}`;
        }
    } catch (error) {
        console.error('Failed to fetch version info', error);
    }
}

async function fetchInteractionStats() {
    console.log('Fetching interaction stats...');
    try {
        const response = await fetch('/setup/interaction-stats');
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const stats = await response.json();
        console.log('Fetched interaction stats:', stats);

        document.getElementById('total-requests').innerText = stats.total_requests || stats.TotalRequests || 0;

        const statsContainer = document.getElementById('interaction-stats-container');
        if (statsContainer) {
            statsContainer.style.display = 'block';
        }

        const serviceList = document.getElementById('stats-by-service');
        serviceList.innerHTML = '';
        const byService = stats.by_service || stats.ByService;
        if (byService) {
            Object.entries(byService).forEach(([service, count]) => {
                const li = document.createElement('li');
                li.innerHTML = `<strong>${service || "unknown"}:</strong> ${count || 0} requests`;
                serviceList.appendChild(li);
            });
        }

        const sessionList = document.getElementById('stats-by-session');
        const sessionFilter = document.getElementById('filter-session');
        const currentFilter = sessionFilter.value;

        sessionList.innerHTML = '';
        sessionFilter.innerHTML = '<option value="">All Sessions</option>';

        const bySession = stats.by_session || stats.BySession;
        if (bySession) {
            // Sort by session ID (timestamp) descending
            const sortedSessions = Object.entries(bySession)
                .sort((a, b) => {
                    const sessionA = a[0] || "";
                    const sessionB = b[0] || "";
                    return sessionB.localeCompare(sessionA);
                });

            sortedSessions.forEach(([session, count]) => {
                // Session format is like 20260215-160705-99213
                // Try to make it more readable: 2026-02-15 16:07:05 (PID 99213)
                let sessionDisplay = session || "unknown";
                if (session && session.includes('-')) {
                    const parts = session.split('-');
                    if (parts.length >= 2) {
                        const date = parts[0]; // 20260215
                        const time = parts[1]; // 160705
                        if (date.length === 8 && time.length === 6) {
                            sessionDisplay = `${date.substring(0, 4)}-${date.substring(4, 6)}-${date.substring(6, 8)} ${time.substring(0, 2)}:${time.substring(2, 4)}:${time.substring(4, 6)}`;
                            if (parts.length >= 3) {
                                sessionDisplay += ` (PID ${parts[2]})`;
                            }
                        }
                    }
                }

                const li = document.createElement('li');
                li.innerHTML = `
                    <span class="session-info"><strong>${sessionDisplay}:</strong> ${count || 0} requests</span>
                    <div style="display: flex; gap: 5px;">
                        <button onclick="filterBySession('${session || ""}')" style="font-size: 0.8em; padding: 2px 5px;">Filter</button>
                        <button onclick="deleteSession('${session || ""}')" class="btn-danger" style="font-size: 0.8em; padding: 2px 5px;">Delete</button>
                    </div>
                `;
                sessionList.appendChild(li);

                const opt = document.createElement('option');
                opt.value = session || "";
                opt.innerText = sessionDisplay;
                sessionFilter.appendChild(opt);
            });

            sessionFilter.value = currentFilter;
        }
    } catch (error) {
        console.error('Failed to fetch interaction stats', error);
    }
}

async function filterBySession(sessionId) {
    document.getElementById('filter-session').value = sessionId;
    fetchInteractions();
    const browseContainer = document.getElementById('browse-recordings');
    if (browseContainer) {
        browseContainer.scrollIntoView({ behavior: 'smooth' });
    }
}

async function deleteSession(sessionId) {
    if (!sessionId) return;
    if (!confirm(`Are you sure you want to delete session ${sessionId}?`)) {
        return;
    }

    try {
        const response = await fetch(`/setup/interactions/sessions/${sessionId}`, {
            method: 'DELETE'
        });
        if (response.ok) {
            // If the deleted session was selected in the filter, clear the filter
            const sessionFilter = document.getElementById('filter-session');
            if (sessionFilter.value === sessionId) {
                sessionFilter.value = "";
                fetchInteractions();
            }
            fetchInteractionStats();
        } else {
            const err = await response.text();
            alert('Failed to delete session: ' + err);
        }
    } catch (error) {
        alert('Error deleting session: ' + error.message);
    }
}

async function cleanupSessions() {
    if (!confirm('Are you sure you want to cleanup old sessions? Only the 10 most recent ones will be kept.')) {
        return;
    }

    try {
        const response = await fetch('/setup/interactions/sessions?keep=10', {
            method: 'DELETE'
        });
        if (response.ok) {
            // Refresh everything
            document.getElementById('filter-session').value = "";
            fetchInteractionStats();
            fetchInteractions();
        } else {
            const err = await response.text();
            alert('Failed to cleanup sessions: ' + err);
        }
    } catch (error) {
        alert('Error cleaning up sessions: ' + error.message);
    }
}

async function fetchInteractions() {
    console.log('Fetching interactions...');
    const session = document.getElementById('filter-session').value;
    const category = document.getElementById('filter-category').value;
    const since = document.getElementById('filter-since').value;

    let url = '/setup/interactions';
    const params = [];
    if (session) params.push(`session=${encodeURIComponent(session)}`);
    if (category) params.push(`category=${encodeURIComponent(category)}`);
    if (since) params.push(`since=${encodeURIComponent(since)}`);
    if (params.length > 0) url += '?' + params.join('&');

    try {
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const interactions = await response.json();
        console.log('Fetched interactions:', interactions);
        const list = document.getElementById('interactions-list');
        if (!list) {
            console.error('Could not find interactions-list element');
            return;
        }

        // Show the parent summary box if it was hidden
        const browseContainer = list.closest('.summary-box');
        if (browseContainer) {
            browseContainer.style.display = 'block';
        }

        list.innerHTML = '';

        if (!interactions || interactions.length === 0) {
            list.innerHTML = '<tr><td colspan="7" style="padding: 20px; text-align: center; color: #666;">No interactions found for current filters.</td></tr>';
            return;
        }

        // Default sort: Session desc, then Counter asc
        // If a specific session is selected, sort primarily by counter asc
        interactions.sort((a, b) => {
            const sessionA = a.session || a.Session || "";
            const sessionB = b.session || b.Session || "";
            if (sessionA !== sessionB) {
                return sessionB.localeCompare(sessionA);
            }
            const counterA = a.counter || a.Counter || 0;
            const counterB = b.counter || b.Counter || 0;
            return counterA - counterB;
        });

        interactions.forEach(i => {
            const tr = document.createElement('tr');
            tr.style.borderBottom = '1px solid #eee';

            const counter = i.counter || i.Counter || 0;
            const timestamp = i.timestamp || i.Timestamp || "";
            const method = i.method || i.Method || "";
            const path = i.path || i.Path || "";
            const status = i.status || i.Status || "";
            const category = i.category || i.Category || "";
            const session = i.session || i.Session || "";
            const file = i.file || i.File || "";

            let statusClass = '';
            if (status >= 200 && status < 300) statusClass = 'status-success';
            else if (status >= 400) statusClass = 'status-error';

            tr.innerHTML = `
                <td style="padding: 8px; color: #888;">${counter}</td>
                <td style="padding: 8px; font-size: 0.8em; white-space: nowrap;">${timestamp}</td>
                <td style="padding: 8px; font-family: monospace;">${method}</td>
                <td style="padding: 8px; font-size: 0.9em;">${path}</td>
                <td style="padding: 8px;"><span class="badge ${statusClass}">${status || '???'}</span></td>
                <td style="padding: 8px;"><span class="badge category-${category}">${category}</span></td>
                <td style="padding: 8px;"><button onclick="viewInteraction('${file}')">View</button></td>
            `;
            list.appendChild(tr);
        });
    } catch (error) {
        console.error('Failed to fetch interactions', error);
    }
}

async function viewInteraction(file) {
    try {
        const response = await fetch(`/setup/interaction-content?file=${encodeURIComponent(file)}`);
        const content = await response.text();

        document.getElementById('viewer-filename').innerText = file;
        document.getElementById('interaction-content').innerText = content;
        document.getElementById('interaction-viewer').style.display = 'block';
        document.getElementById('interaction-viewer').scrollIntoView({ behavior: 'smooth' });
    } catch (error) {
        alert('Failed to load interaction content: ' + error);
    }
}

async function showDeviceEvents() {
    const overlay = document.getElementById('device-events-overlay');
    overlay.style.display = 'block';
    overlay.scrollIntoView({ behavior: 'smooth' });

    // Ensure device selector is populated (handled by fetchDevices)
    // but if it's still empty, we can try to trigger a fetch
    const selector = document.getElementById('event-device-selector');
    if (selector.options.length <= 1) {
        fetchDevices();
    }
}

async function fetchDeviceEvents(deviceId) {
    if (!deviceId) return;

    const list = document.getElementById('events-list');
    list.innerHTML = '<tr><td colspan="3" style="padding: 20px; text-align: center; color: #666;">Loading events...</td></tr>';

    try {
        const response = await fetch(`/setup/devices/${deviceId}/events`);
        const data = await response.json();
        const events = data.events;

        list.innerHTML = '';
        if (!events || events.length === 0) {
            list.innerHTML = '<tr><td colspan="3" style="padding: 20px; text-align: center; color: #666;">No events found for this device.</td></tr>';
            return;
        }

        // Sort events by time descending
        events.sort((a, b) => (b.time || "").localeCompare(a.time || ""));

        events.forEach(e => {
            const tr = document.createElement('tr');
            tr.style.borderBottom = '1px solid #eee';

            const time = e.time || "";
            const type = e.type || "";
            const data = JSON.stringify(e.data || {});

            tr.innerHTML = `
                <td style="padding: 8px; font-size: 0.8em; white-space: nowrap;">${time}</td>
                <td style="padding: 8px;"><span class="badge category-self">${type}</span></td>
                <td style="padding: 8px; font-size: 0.85em; font-family: monospace; max-width: 400px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap;" title='${data}'>${data}</td>
            `;
            list.appendChild(tr);
        });
    } catch (error) {
        list.innerHTML = `<tr><td colspan="3" style="padding: 20px; text-align: center; color: #f44336;">Error loading events: ${error.message}</td></tr>`;
    }
}

document.addEventListener('DOMContentLoaded', () => {
    fetchSettings();
    fetchDevices();
    triggerDiscovery();
    fetchVersion();

    const syncBtn = document.getElementById('sync-now-btn');
    if (syncBtn) syncBtn.onclick = startSync;
});


async function addManualDevice() {
    const ip = document.getElementById('add-manual-ip').value.trim();
    if (!ip) {
        alert('Please enter an IP address');
        return;
    }

    try {
        const response = await fetch('/setup/devices', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ ip: ip })
        });

        if (response.ok) {
            document.getElementById('add-manual-ip').value = '';
            fetchDevices();
        } else {
            const err = await response.text();
            alert('Failed to add device: ' + err);
        }
    } catch (error) {
        alert('Error adding device: ' + error.message);
    }
}

async function removeDevice(deviceId, name) {
    if (!confirm(`Are you sure you want to remove device "${name}"?`)) {
        return;
    }

    try {
        const response = await fetch(`/setup/devices/${deviceId}`, {
            method: 'DELETE'
        });

        if (response.ok) {
            fetchDevices();
        } else {
            const err = await response.text();
            alert('Failed to remove device: ' + err);
        }
    } catch (error) {
        alert('Error removing device: ' + error.message);
    }
}

async function triggerDiscovery() {
    const indicator = document.getElementById('discovery-indicator');
    if (indicator) indicator.style.display = 'inline';
    try {
        await fetch('/setup/discover', { method: 'POST' });
        pollDiscoveryStatus();
    } catch (error) {
        console.error('Failed to trigger discovery', error);
        if (indicator) indicator.style.display = 'none';
    }
}

async function pollDiscoveryStatus() {
    const indicator = document.getElementById('discovery-indicator');
    try {
        const response = await fetch('/setup/discovery-status');
        const data = await response.json();
        if (data.discovering) {
            setTimeout(pollDiscoveryStatus, 2000);
        } else {
            if (indicator) indicator.style.display = 'none';
            fetchDevices();
        }
    } catch (error) {
        console.error('Failed to check discovery status', error);
        if (indicator) indicator.style.display = 'none';
    }
}

async function updateDeviceInfo(ip) {
    try {
        const response = await fetch('/setup/info/' + ip);
        if (!response.ok) return;
        const info = await response.json();

        const rowId = 'device-row-' + ip.replace(/\./g, '-');
        const row = document.getElementById(rowId);
        if (row) {
            const nameEl = row.querySelector('.col-name');
            if (nameEl && info.name) nameEl.innerText = info.name;

            const modelEl = row.querySelector('.col-model');
            if (modelEl && info.type) modelEl.innerText = info.type;

            const serialEl = row.querySelector('.col-serial');
            if (serialEl && info.serialNumber) serialEl.innerText = info.serialNumber;

            const firmwareEl = row.querySelector('.col-firmware');
            if (firmwareEl && info.softwareVersion) firmwareEl.innerText = info.softwareVersion;

            const deviceIdEl = row.querySelector('.col-deviceid');
            if (deviceIdEl && info.deviceID) deviceIdEl.innerText = info.deviceID;

            const accountIdEl = row.querySelector('.col-accountid');
            if (accountIdEl && info.margeAccountUUID) accountIdEl.innerText = info.margeAccountUUID;
        }
    } catch (error) {
        console.warn('Failed to fetch live info for ' + ip, error);
    }
}

async function showSummary(ip) {
    if (!ip) {
        document.getElementById('migration-summary').style.display = 'none';
        return;
    }
    const targetUrl = document.getElementById('target-domain').value;
    const proxyUrl = document.getElementById('proxy-domain').value;

    const opts = {
        marge: document.getElementById('opt-marge').value,
        stats: document.getElementById('opt-stats').value,
        sw_update: document.getElementById('opt-sw_update').value,
        bmx: document.getElementById('opt-bmx').value
    };

    const statusDiv = document.getElementById('status');
    statusDiv.style.display = 'block';
    statusDiv.style.backgroundColor = '#ffffcc';
    statusDiv.innerHTML = 'Fetching summary for ' + ip + '...';

    let query = '?target_url=' + encodeURIComponent(targetUrl) + '&proxy_url=' + encodeURIComponent(proxyUrl);
    for (let k in opts) {
        query += '&' + k + '=' + encodeURIComponent(opts[k]);
    }

    const outputBox = document.getElementById('command-output-box');
    if (outputBox) outputBox.style.display = 'none';

    try {
        const response = await fetch('/setup/summary/' + ip + query);
        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(errorText);
        }
        const summary = await response.json();

        statusDiv.style.display = 'none';
        document.getElementById('summary-ip').innerText = ip;

        // Update table row if it exists
        const rowId = 'device-row-' + ip.replace(/\./g, '-');
        const row = document.getElementById(rowId);
        if (row) {
            const nameEl = row.querySelector('.col-name');
            if (nameEl && summary.device_name) nameEl.innerText = summary.device_name;

            const modelEl = row.querySelector('.col-model');
            if (modelEl && summary.device_model) modelEl.innerText = summary.device_model;

            const serialEl = row.querySelector('.col-serial');
            if (serialEl && summary.device_serial) serialEl.innerText = summary.device_serial;

            const firmwareEl = row.querySelector('.col-firmware');
            if (firmwareEl && summary.firmware_version) firmwareEl.innerText = summary.firmware_version;

            const deviceIdEl = row.querySelector('.col-deviceid');
            if (deviceIdEl && summary.device_id) deviceIdEl.innerText = summary.device_id;

            const accountIdEl = row.querySelector('.col-accountid');
            if (accountIdEl && summary.account_id) accountIdEl.innerText = summary.account_id;
        }

        document.getElementById('ssh-status').innerText = summary.ssh_success ? '✅ Success' : '❌ Failed';
        document.getElementById('ssh-status').style.color = summary.ssh_success ? 'green' : 'red';

        const migrationStatus = document.getElementById('migration-status');
        migrationStatus.innerText = summary.is_migrated ? '✅ Migrated to AfterTouch' : '❌ Not Migrated';
        migrationStatus.style.color = summary.is_migrated ? 'green' : 'red';
        migrationStatus.style.fontWeight = 'bold';

        document.getElementById('original-config-status').style.display = summary.original_config ? 'block' : 'none';
        document.getElementById('no-original-config-status').style.display = summary.original_config ? 'none' : 'block';
        document.getElementById('original-config-content').innerText = summary.original_config || '';
        document.getElementById('original-config-pane').style.display = 'none';

        if (summary.parsed_current_config) {
            document.getElementById('service-options').style.display = 'block';
            document.getElementById('orig-marge').innerText = summary.parsed_current_config.margeServerUrl;
            document.getElementById('orig-stats').innerText = summary.parsed_current_config.statsServerUrl;
            document.getElementById('orig-sw_update').innerText = summary.parsed_current_config.swUpdateUrl;
            document.getElementById('orig-bmx').innerText = summary.parsed_current_config.bmxRegistryUrl;
        } else {
            document.getElementById('service-options').style.display = 'none';
        }

        const remoteStatus = document.getElementById('remote-services-status');
        const remoteFound = document.getElementById('remote-services-found');
        if (summary.ssh_success) {
            if (summary.remote_services_enabled) {
                remoteStatus.innerText = summary.remote_services_persistent ? '✅ Yes' : '⚠️ Yes (non-persistent)';
                remoteStatus.style.color = summary.remote_services_persistent ? 'green' : 'orange';
            } else {
                remoteStatus.innerText = '❌ No';
                remoteStatus.style.color = 'red';
            }
            remoteFound.innerText = summary.remote_services_found && summary.remote_services_found.length > 0
                ? '(' + summary.remote_services_found.join(', ') + ')'
                : '';

            const caTrustStatus = document.getElementById('ca-trust-status');
            caTrustStatus.innerText = summary.ca_cert_trusted ? '✅ Yes' : '❌ No';
            caTrustStatus.style.color = summary.ca_cert_trusted ? 'green' : 'red';
            document.getElementById('trust-ca-btn').style.display = summary.ca_cert_trusted ? 'none' : 'inline-block';
            document.getElementById('trust-ca-btn').onclick = () => trustCA(ip);
        } else {
            remoteStatus.innerText = '❓ Unknown';
            remoteStatus.style.color = 'gray';
            remoteFound.innerText = '';

            const caTrustStatus = document.getElementById('ca-trust-status');
            caTrustStatus.innerText = '❓ Unknown';
            caTrustStatus.style.color = 'gray';
        }

        const currentConfigElem = document.getElementById('current-config');
        currentConfigElem.innerText = summary.current_config;
        currentConfigElem.style.color = summary.ssh_success ? 'black' : 'red';

        document.getElementById('planned-config').innerText = summary.planned_config;
        document.getElementById('planned-hosts').innerText = summary.planned_hosts || '';

        const testUrlElem = document.getElementById('test-url');
        testUrlElem.innerText = summary.server_https_url || 'N/A';
        const testResultDiv = document.getElementById('test-result');
        testResultDiv.style.display = 'none';
        testResultDiv.innerText = '';

        document.getElementById('test-connection-explicit-btn').onclick = () => testConnection(ip, true);
        document.getElementById('test-connection-trusted-btn').onclick = () => testConnection(ip, false);
        document.getElementById('test-hosts-btn').onclick = () => testHostsRedirection(ip);

        toggleMigrationMethod();

        const migrateBtn = document.getElementById('confirm-migrate-btn');
        migrateBtn.onclick = () => migrate(ip);
        migrateBtn.disabled = !summary.ssh_success;

        const revertBtn = document.getElementById('revert-migrate-btn');
        revertBtn.onclick = () => revert(ip);
        revertBtn.disabled = !summary.ssh_success;
        revertBtn.style.display = summary.original_config ? 'inline-block' : 'none';

        const rebootBtn = document.getElementById('reboot-speaker-btn');
        rebootBtn.onclick = () => reboot(ip);
        rebootBtn.disabled = !summary.ssh_success;
        rebootBtn.style.border = 'none'; // Reset border if it was set during migration

        const remoteBtn = document.getElementById('ensure-remote-btn');
        remoteBtn.onclick = () => ensureRemoteServices(ip);
        remoteBtn.disabled = !summary.ssh_success;

        const removeRemoteBtn = document.getElementById('remove-remote-btn');
        removeRemoteBtn.onclick = () => removeRemoteServices(ip);
        removeRemoteBtn.disabled = !summary.ssh_success || !summary.remote_services_enabled;

        const backupBtn = document.getElementById('backup-config-btn');
        backupBtn.onclick = () => backupConfig(ip);
        backupBtn.disabled = !summary.ssh_success || !!summary.original_config;

        document.getElementById('migration-summary').style.display = 'block';
        document.getElementById('migration-summary').scrollIntoView();
    } catch (error) {
        statusDiv.style.backgroundColor = '#ffcccc';
        statusDiv.innerHTML = 'Error fetching summary for ' + ip + ': ' + error;
    }
}

function refreshSummary() {
    const ip = document.getElementById('summary-ip').innerText;
    if (ip) {
        showSummary(ip);
    }
}

function showCommandOutput(result) {
    const outputBox = document.getElementById('command-output-box');
    const outputText = document.getElementById('command-output');
    if (outputBox && outputText && result.output) {
        outputBox.style.display = 'block';
        outputText.innerText = result.output;
    } else if (outputBox) {
        outputBox.style.display = 'none';
    }
}

async function revert(ip) {
    if (!ip) {
        alert('Please enter a valid IP address.');
        return;
    }
    if (!confirm('Are you sure you want to revert ' + ip + ' to Bose cloud defaults?')) {
        return;
    }

    const summaryDiv = document.getElementById('migration-summary');
    summaryDiv.style.display = 'none';

    const statusDiv = document.getElementById('status');
    statusDiv.style.display = 'block';
    statusDiv.style.backgroundColor = '#ffffcc';
    statusDiv.innerHTML = 'Reverting ' + ip + ' to defaults...';

    try {
        const response = await fetch('/setup/revert/' + ip, { method: 'POST' });
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = '#ccffcc';
            statusDiv.innerHTML = 'Successfully started revert for ' + ip + '.';
        } else {
            statusDiv.style.backgroundColor = '#ffcccc';
            statusDiv.innerHTML = 'Revert failed for ' + ip + ': ' + (result.message || 'Unknown error');
        }
    } catch (error) {
        statusDiv.style.backgroundColor = '#ffcccc';
        statusDiv.innerHTML = 'Error reverting ' + ip + ': ' + error;
    }
}

async function reboot(ip) {
    if (!ip) {
        alert('Please enter a valid IP address.');
        return;
    }
    if (!confirm('Are you sure you want to reboot the speaker at ' + ip + '?')) {
        return;
    }

    const statusDiv = document.getElementById('status');
    statusDiv.style.display = 'block';
    statusDiv.style.backgroundColor = '#ffffcc';
    statusDiv.innerHTML = 'Rebooting ' + ip + '...';

    try {
        const response = await fetch('/setup/reboot/' + ip, { method: 'POST' });
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = '#ccffcc';
            statusDiv.innerHTML = 'Successfully started reboot for ' + ip + '.';
        } else {
            statusDiv.style.backgroundColor = '#ffcccc';
            statusDiv.innerHTML = 'Reboot failed for ' + ip + ': ' + (result.message || 'Unknown error');
        }
    } catch (error) {
        statusDiv.style.backgroundColor = '#ffcccc';
        statusDiv.innerHTML = 'Error rebooting ' + ip + ': ' + error;
    }
}

async function migrate(ip) {
    if (!ip) {
        alert('Please enter a valid IP address.');
        return;
    }
    const targetUrl = document.getElementById('target-domain').value;
    const proxyUrl = document.getElementById('proxy-domain').value;
    const method = document.getElementById('migration-method').value;

    const opts = {
        marge: document.getElementById('opt-marge').value,
        stats: document.getElementById('opt-stats').value,
        sw_update: document.getElementById('opt-sw_update').value,
        bmx: document.getElementById('opt-bmx').value
    };

    const summaryDiv = document.getElementById('migration-summary');
    summaryDiv.style.display = 'none';

    const statusDiv = document.getElementById('status');
    statusDiv.style.display = 'block';
    statusDiv.style.backgroundColor = '#ffffcc';
    statusDiv.innerHTML = 'Migrating ' + ip + ' using ' + method + '...';

    let query = '?method=' + encodeURIComponent(method) + '&target_url=' + encodeURIComponent(targetUrl) + '&proxy_url=' + encodeURIComponent(proxyUrl);
    for (let k in opts) {
        query += '&' + k + '=' + encodeURIComponent(opts[k]);
    }

    try {
        const response = await fetch('/setup/migrate/' + ip + query, { method: 'POST' });
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = '#ccffcc';
            statusDiv.innerHTML = 'Successfully started migration for ' + ip + '. <strong>Please reboot the device to activate the changes.</strong>';

            // Make reboot button available and prominent
            const rebootBtn = document.getElementById('reboot-speaker-btn');
            rebootBtn.style.display = 'inline-block';
            rebootBtn.disabled = false;
            rebootBtn.style.border = '2px solid #000';

            // Re-show summary but with prominence on reboot
            summaryDiv.style.display = 'block';
        } else {
            statusDiv.style.backgroundColor = '#ffcccc';
            statusDiv.innerHTML = 'Migration failed for ' + ip + ': ' + (result.message || 'Unknown error');
        }
    } catch (error) {
        statusDiv.style.backgroundColor = '#ffcccc';
        statusDiv.innerHTML = 'Error migrating ' + ip + ': ' + error;
    }
}

async function trustCA(ip) {
    if (!ip) {
        alert('Please enter a valid IP address.');
        return;
    }
    const statusDiv = document.getElementById('status');
    statusDiv.style.display = 'block';
    statusDiv.style.backgroundColor = '#ffffcc';
    statusDiv.innerHTML = 'Injecting Root CA into shared trust store on ' + ip + '...';

    try {
        const response = await fetch('/setup/trust-ca/' + ip, { method: 'POST' });
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = '#ccffcc';
            statusDiv.innerHTML = 'Successfully injected Root CA on ' + ip + '.';
            showSummary(ip); // Refresh to update status
        } else {
            statusDiv.style.backgroundColor = '#ffcccc';
            statusDiv.innerHTML = 'Failed to trust CA on ' + ip + ': ' + (result.message || 'Unknown error');
        }
    } catch (error) {
        statusDiv.style.backgroundColor = '#ffcccc';
        statusDiv.innerHTML = 'Error trusting CA on ' + ip + ': ' + error;
    }
}

async function ensureRemoteServices(ip) {
    if (!ip) {
        alert('Please enter a valid IP address.');
        return;
    }
    const summaryDiv = document.getElementById('migration-summary');
    summaryDiv.style.display = 'none';

    const statusDiv = document.getElementById('status');
    statusDiv.style.display = 'block';
    statusDiv.style.backgroundColor = '#ffffcc';
    statusDiv.innerHTML = 'Ensuring remote services for ' + ip + '...';

    try {
        const response = await fetch('/setup/ensure-remote-services/' + ip, { method: 'POST' });
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = '#ccffcc';
            statusDiv.innerHTML = 'Successfully ensured remote services for ' + ip + '.';
        } else {
            statusDiv.style.backgroundColor = '#ffcccc';
            statusDiv.innerHTML = 'Failed to ensure remote services for ' + ip + ': ' + (result.message || 'Unknown error');
        }
    } catch (error) {
        statusDiv.style.backgroundColor = '#ffcccc';
        statusDiv.innerHTML = 'Error ensuring remote services for ' + ip + ': ' + error;
    }
}

async function removeRemoteServices(ip) {
    if (!ip) {
        alert('Please enter a valid IP address.');
        return;
    }
    if (!confirm('Are you sure you want to remove remote services from ' + ip + '?')) {
        return;
    }
    const summaryDiv = document.getElementById('migration-summary');
    summaryDiv.style.display = 'none';

    const statusDiv = document.getElementById('status');
    statusDiv.style.display = 'block';
    statusDiv.style.backgroundColor = '#ffffcc';
    statusDiv.innerHTML = 'Removing remote services for ' + ip + '...';

    try {
        const response = await fetch('/setup/remove-remote-services/' + ip, { method: 'POST' });
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = '#ccffcc';
            statusDiv.innerHTML = 'Successfully removed remote services from ' + ip + '.';
        } else {
            statusDiv.style.backgroundColor = '#ffcccc';
            statusDiv.innerHTML = 'Failed to remove remote services for ' + ip + ': ' + (result.message || 'Unknown error');
        }
    } catch (error) {
        statusDiv.style.backgroundColor = '#ffcccc';
        statusDiv.innerHTML = 'Error removing remote services for ' + ip + ': ' + error;
    }
}

async function backupConfig(ip) {
    if (!ip) {
        alert('Please enter a valid IP address.');
        return;
    }
    const statusDiv = document.getElementById('status');
    statusDiv.style.display = 'block';
    statusDiv.style.backgroundColor = '#ffffcc';
    statusDiv.innerHTML = 'Creating backup for ' + ip + '...';

    try {
        const response = await fetch('/setup/backup/' + ip, { method: 'POST' });
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = '#ccffcc';
            statusDiv.innerHTML = 'Successfully created backup for ' + ip + '.';
            showSummary(ip); // Refresh
        } else {
            statusDiv.style.backgroundColor = '#ffcccc';
            statusDiv.innerHTML = 'Backup failed for ' + ip + ': ' + (result.message || 'Unknown error');
        }
    } catch (error) {
        statusDiv.style.backgroundColor = '#ffcccc';
        statusDiv.innerHTML = 'Error creating backup for ' + ip + ': ' + error;
    }
}

async function testConnection(ip, useExplicitCA) {
    const testUrl = document.getElementById('test-url').innerText;
    const testResultDiv = document.getElementById('test-result');

    testResultDiv.style.display = 'block';
    testResultDiv.style.backgroundColor = '#f0f0f0';
    testResultDiv.style.color = 'black';
    testResultDiv.innerText = 'Running connection test from ' + ip + '...\n(This may take a few seconds)';

    try {
        const query = `?target_url=${encodeURIComponent(testUrl)}&use_explicit_ca=${useExplicitCA}`;
        const response = await fetch(`/setup/test-connection/${ip}${query}`, { method: 'POST' });
        const result = await response.json();

        if (result.ok) {
            testResultDiv.style.backgroundColor = '#ccffcc';
            testResultDiv.innerText = '✅ ' + result.message + '\n\nOutput:\n' + result.output;
        } else {
            testResultDiv.style.backgroundColor = '#ffcccc';
            testResultDiv.innerText = '❌ Connection failed: ' + result.message + '\n\nOutput:\n' + result.output;
        }
    } catch (error) {
        testResultDiv.style.backgroundColor = '#ffcccc';
        testResultDiv.innerText = '❌ Error triggering test: ' + error;
    }
}

async function testHostsRedirection(ip) {
    const targetUrl = document.getElementById('target-domain').value;
    const testResultDiv = document.getElementById('hosts-test-result');

    testResultDiv.style.display = 'block';
    testResultDiv.style.backgroundColor = '#f0f0f0';
    testResultDiv.style.color = 'black';
    testResultDiv.innerText = 'Running hosts redirection test from ' + ip + '...\n(This may take a few seconds)';

    try {
        const query = `?target_url=${encodeURIComponent(targetUrl)}`;
        const response = await fetch(`/setup/test-hosts/${ip}${query}`, { method: 'POST' });
        const result = await response.json();

        if (result.ok) {
            testResultDiv.style.backgroundColor = '#ccffcc';
            testResultDiv.innerText = '✅ ' + result.message + '\n\nOutput:\n' + result.output;
        } else {
            testResultDiv.style.backgroundColor = '#ffcccc';
            testResultDiv.innerText = '❌ Test failed: ' + result.message + '\n\nOutput:\n' + result.output;
        }
    } catch (error) {
        testResultDiv.style.backgroundColor = '#ffcccc';
        testResultDiv.innerText = '❌ Error triggering test: ' + error;
    }
}

function toggleOriginalConfig() {
    const pane = document.getElementById('original-config-pane');
    pane.style.display = pane.style.display === 'none' ? 'block' : 'none';
}

function toggleMigrationMethod() {
    const method = document.getElementById('migration-method').value;
    const xmlDiffPane = document.getElementById('xml-diff-pane');
    const plannedXmlPane = document.getElementById('planned-xml-pane');
    const plannedHostsPane = document.getElementById('planned-hosts-pane');
    const serviceOptions = document.getElementById('service-options');
    const hostsTestPane = document.getElementById('hosts-redirection-test');

    if (method === 'hosts') {
        xmlDiffPane.style.display = 'none';
        plannedXmlPane.style.display = 'none';
        plannedHostsPane.style.display = 'block';
        serviceOptions.style.display = 'none';
        hostsTestPane.style.display = 'block';
    } else {
        xmlDiffPane.style.display = 'block';
        plannedXmlPane.style.display = 'block';
        plannedHostsPane.style.display = 'none';
        hostsTestPane.style.display = 'none';
        // Only show service options if we have a parsed config
        const currentConfig = document.getElementById('current-config').innerText;
        if (currentConfig && !currentConfig.startsWith('Error') && currentConfig !== 'loading...') {
            serviceOptions.style.display = 'block';
        }
    }
}

document.addEventListener('DOMContentLoaded', () => {
    fetchDevices();
    fetchSettings();
    triggerDiscovery();
});
