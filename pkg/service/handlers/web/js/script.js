async function fetchSettings() {
    try {
        const response = await fetch('/setup/settings');
        const settings = await response.json();
        if (settings.server_url) {
            document.getElementById('target-domain').value = settings.server_url;
        }
        if (settings.proxy_url) {
            document.getElementById('proxy-domain').value = settings.proxy_url;
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
    } catch (error) {
        console.error('Failed to fetch proxy settings', error);
    }
}

async function updateProxySettings() {
    const settings = {
        redact: document.getElementById('proxy-redact').checked,
        log_body: document.getElementById('proxy-log-body').checked,
        record: document.getElementById('proxy-record').checked
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
            let html = '<table><tr><th>Name</th><th>IP Address</th><th>Model</th><th>Serial Number</th><th>Firmware</th><th>Method</th><th>Action</th></tr>';

            // Clear and repopulate selectors
            const currentSyncVal = syncSelector.value;
            const currentMigrationVal = migrationSelector.value;
            syncSelector.innerHTML = '<option value="">-- Select a device --</option>';
            migrationSelector.innerHTML = '<option value="">-- Select a device --</option>';

            devices.forEach(d => {
                const methodLabel = d.discovery_method === 'manual' ? '👤 Manual' : '🔍 Auto';
                html += `
                    <tr id="device-row-${d.ip_address.replace(/\./g, '-')}">
                        <td class="col-name">${d.name}</td>
                        <td class="col-ip">${d.ip_address}</td>
                        <td class="col-model">${d.product_code}</td>
                        <td class="col-serial">${d.device_serial_number}</td>
                        <td class="col-firmware">${d.firmware_version || '0.0.0'}</td>
                        <td class="col-method">${methodLabel}</td>
                        <td>
                            <button onclick="prepareSync('${d.ip_address}')">Sync Data</button>
                            <button onclick="prepareMigration('${d.ip_address}')">Migrate</button>
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
            });
            html += '</table>';
            container.innerHTML = html;

            if (currentSyncVal) syncSelector.value = currentSyncVal;
            if (currentMigrationVal) migrationSelector.value = currentMigrationVal;

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

document.addEventListener('DOMContentLoaded', () => {
    fetchSettings();
    fetchDevices();
    triggerDiscovery();

    document.getElementById('sync-now-btn').onclick = startSync;
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
        }

        document.getElementById('ssh-status').innerText = summary.ssh_success ? '✅ Success' : '❌ Failed';
        document.getElementById('ssh-status').style.color = summary.ssh_success ? 'green' : 'red';

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
            statusDiv.innerHTML = 'Successfully started migration for ' + ip + '.';
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
