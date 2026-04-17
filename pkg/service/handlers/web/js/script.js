async function fetchSpotifyStatus() {
    try {
        const settingsResponse = await fetch("/setup/settings");
        const settings = await settingsResponse.json();
        const header = document.getElementById("spotify-status-header");

        if (!settings.spotify_configured) {
            if (header) header.style.display = "none";
            return;
        }

        if (header) header.style.display = "flex";

        const response = await fetch("/mgmt/spotify/accounts");
        if (!response.ok) return;
        const data = await response.json();
        const nameEl = document.getElementById("spotify-account-name");
        const linkBtn = document.getElementById("link-spotify-btn");

        if (data.accounts && data.accounts.length > 0) {
            header.style.background = "#e6ffed";
            header.style.border = "1px solid #28a745";
            nameEl.innerText = data.accounts[0].display_name || data.accounts[0].user_id || "Linked";
            if (linkBtn) linkBtn.style.display = "none";

            // Show Prime Spotify buttons on all devices
            document.querySelectorAll(".btn-spotify").forEach((btn) => {
                btn.style.display = "inline-block";
            });
        } else {
            header.style.background = "#f0f0f0";
            header.style.border = "1px solid #ccc";
            nameEl.innerText = "Not Linked";
            if (linkBtn) linkBtn.style.display = "inline-block";

            document.querySelectorAll(".btn-spotify").forEach((btn) => {
                btn.style.display = "none";
            });
        }
    } catch (error) {
        console.error("Failed to fetch Spotify status", error);
    }
}

function toggleInfo(id) {
    const el = document.getElementById(id);
    if (el) {
        el.style.display = el.style.display === "block" ? "none" : "block";
    }
}

async function linkSpotify() {
    try {
        const response = await fetch("/mgmt/spotify/init", {method: "POST"});
        if (!response.ok) {
            const err = await response.text();
            alert("Failed to initialize Spotify link: " + err);
            return;
        }
        const data = await response.json();
        if (data.redirectUrl) {
            // Open in a new tab
            const win = window.open(data.redirectUrl, "_blank");
            if (win) {
                win.focus();
                // Start polling for status change
                const pollInterval = setInterval(async () => {
                    const statusResponse = await fetch("/mgmt/spotify/accounts");
                    if (statusResponse.ok) {
                        const statusData = await statusResponse.json();
                        if (statusData.accounts && statusData.accounts.length > 0) {
                            clearInterval(pollInterval);
                            fetchSpotifyStatus();
                        }
                    }
                }, 2000);
                // Stop polling after 2 minutes
                setTimeout(() => clearInterval(pollInterval), 120000);
            } else {
                alert("Please allow popups to link your Spotify account.");
            }
        }
    } catch (error) {
        alert("Error linking Spotify: " + error.message);
    }
}

async function primeSpotify(deviceId) {
    const btn = document.getElementById("prime-spotify-" + deviceId);
    const originalText = btn.innerText;
    btn.innerText = "Priming...";
    btn.disabled = true;

    try {
        const response = await fetch(`/mgmt/spotify/prime?deviceId=${encodeURIComponent(deviceId)}`, {
            method: "POST",
        },);
        if (response.ok) {
            btn.innerText = "✅ Primed";
            btn.style.background = "#28a745";
            setTimeout(() => {
                btn.innerText = originalText;
                btn.style.background = "";
                btn.disabled = false;
            }, 3000);
        } else {
            const err = await response.text();
            alert("Failed to prime Spotify: " + err);
            btn.innerText = "❌ Failed";
            setTimeout(() => {
                btn.innerText = originalText;
                btn.disabled = false;
            }, 3000);
        }
    } catch (error) {
        alert("Error priming Spotify: " + error.message);
        btn.innerText = originalText;
        btn.disabled = false;
    }
}

async function fetchSettings() {
    try {
        const response = await fetch("/setup/settings");
        const settings = await response.json();
        if (settings.server_url) {
            document.getElementById("target-domain").value = settings.server_url;
        }
        if (settings.discovery_interval) {
            document.getElementById("discovery-interval").value = settings.discovery_interval;
        }
        if (settings.discovery_enabled !== undefined) {
            document.getElementById("discovery-enabled").checked = settings.discovery_enabled;
        }
        if (settings.dns_enabled !== undefined) {
            document.getElementById("dns-enabled").checked = settings.dns_enabled;
        }
        if (settings.dns_upstream) {
            document.getElementById("dns-upstream").value = settings.dns_upstream;
        }
        if (settings.dns_bind_addr) {
            document.getElementById("dns-bind").value = settings.dns_bind_addr;
        }

        const dnsCurrentUpstream = document.getElementById("dns-current-upstream");
        if (dnsCurrentUpstream && settings.dns_upstream) {
            dnsCurrentUpstream.innerText = "Current upstreams: " + settings.dns_upstream;
        } else if (dnsCurrentUpstream) {
            dnsCurrentUpstream.innerText = "";
        }

        if (settings.mirror_enabled !== undefined) {
            document.getElementById("mirror-enabled").checked = settings.mirror_enabled;
        }
        if (settings.preferred_source !== undefined) {
            document.getElementById("preferred-source-upstream").checked = settings.preferred_source === "upstream";
        }
        if (settings.mirror_endpoints) {
            document.getElementById("mirror-endpoints").value = settings.mirror_endpoints.join("\n");
        }
        if (settings.skip_mirror_endpoints) {
            document.getElementById("skip-mirror-endpoints").value = settings.skip_mirror_endpoints.join("\n");
        }
        if (settings.internal_paths) {
            document.getElementById("internal-paths").value = settings.internal_paths.join("\n");
        }

        const spotifyStatus = document.getElementById("spotify-config-status");
        if (spotifyStatus) {
            if (settings.spotify_configured) {
                spotifyStatus.innerHTML = '<span style="color: green;">✅ Configured</span> (Client ID present)';
            } else {
                spotifyStatus.innerHTML = '<span style="color: #666;">❌ Not Configured</span><br>' + '<span style="font-size: 0.85em; color: #888;">To enable Spotify, provide <code>SPOTIFY_CLIENT_ID</code> and <code>SPOTIFY_CLIENT_SECRET</code> to the server.</span>';
            }
        }

        fetchProxySettings();
        fetchSpotifyStatus();
    } catch (error) {
        console.error("Failed to fetch settings", error);
    }
}

async function fetchProxySettings() {
    try {
        const response = await fetch("/setup/proxy-settings");
        const settings = await response.json();
        document.getElementById("proxy-redact").checked = settings.redact;
        document.getElementById("proxy-log-body").checked = settings.log_body;
        document.getElementById("proxy-record").checked = settings.record;
    } catch (error) {
        console.error("Failed to fetch proxy settings", error);
    }
}

async function updateProxySettings() {
    const settings = {
        redact: document.getElementById("proxy-redact").checked,
        log_body: document.getElementById("proxy-log-body").checked,
        record: document.getElementById("proxy-record").checked,
    };
    try {
        await fetch("/setup/proxy-settings", {
            method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(settings),
        });
    } catch (error) {
        console.error("Failed to update proxy settings", error);
    }
}

async function updateSettings() {
    const settings = {
        server_url: document.getElementById("target-domain").value,
        discovery_interval: document.getElementById("discovery-interval").value,
        discovery_enabled: document.getElementById("discovery-enabled").checked,
        dns_enabled: document.getElementById("dns-enabled").checked,
        dns_upstream: document.getElementById("dns-upstream").value,
        dns_bind_addr: document.getElementById("dns-bind").value,
        mirror_enabled: document.getElementById("mirror-enabled").checked,
        preferred_source: document.getElementById("preferred-source-upstream").checked ? "upstream" : "local",
        mirror_endpoints: document
            .getElementById("mirror-endpoints")
            .value.split("\n")
            .map((s) => s.trim())
            .filter((s) => s !== ""),
        skip_mirror_endpoints: document
            .getElementById("skip-mirror-endpoints")
            .value.split("\n")
            .map((s) => s.trim())
            .filter((s) => s !== ""),
        internal_paths: document
            .getElementById("internal-paths")
            .value.split("\n")
            .map((s) => s.trim())
            .filter((s) => s !== ""),
    };
    const status = document.getElementById("settings-status");
    status.innerText = "Saving...";
    status.style.color = "blue";

    try {
        const response = await fetch("/setup/settings", {
            method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify(settings),
        });
        if (response.ok) {
            status.innerText = "✅ Settings saved. Restart service to apply all changes (like certificate SANs).";
            status.style.color = "green";
            setTimeout(() => fetchSettings(), 500); // Give backend a moment to settle
        } else {
            const err = await response.text();
            status.innerText = "❌ Failed: " + err;
            status.style.color = "red";
        }
    } catch (error) {
        status.innerText = "❌ Error: " + error.message;
        status.style.color = "red";
    }
}

async function fetchDevices() {
    try {
        const response = await fetch("/setup/devices");
        const devices = await response.json();
        const container = document.getElementById("device-list");
        const syncSelector = document.getElementById("sync-device-list");
        const migrationSelector = document.getElementById("migration-device-list");

        if (devices.length === 0) {
            container.innerHTML = "No devices known yet.";
        } else {
            let html = "<table><tr><th>Name & Model</th><th>IP Address</th><th>Device & Account ID</th><th>Firmware & Serial</th><th>Method</th><th>Action</th></tr>";

            // Clear and repopulate selectors
            const currentSyncVal = syncSelector.value;
            const currentMigrationVal = migrationSelector.value;
            const eventSelector = document.getElementById("event-device-selector");
            const currentEventVal = eventSelector ? eventSelector.value : "";

            syncSelector.innerHTML = '<option value="">-- Select a device --</option>';
            migrationSelector.innerHTML = '<option value="">-- Select a device --</option>';
            if (eventSelector) eventSelector.innerHTML = '<option value="">-- Select a device --</option>';

            devices.forEach((d) => {
                const methodLabel = d.discovery_method === "manual" ? "👤 Manual" : "🔍 Auto";
                html += `
                    <tr id="device-row-${d.device_id}">
                        <td class="col-name-model"><div class="col-name">${d.name}</div><div class="col-model" style="font-size: 0.8em; color: #666;">${d.product_code}</div></td>
                        <td class="col-ip">${d.ip_address}</td>
                        <td class="col-ids"><div class="col-deviceid">${d.device_id}</div><div class="col-accountid" style="font-size: 0.8em; color: #666;">${d.account_id || "default"}</div></td>
                        <td class="col-fw-serial"><div class="col-firmware">${d.firmware_version || "0.0.0"}</div><div class="col-serial" style="font-size: 0.8em; color: #666;">${d.device_serial_number}</div></td>
                        <td class="col-method">${methodLabel}</td>
                        <td>
                            <button onclick="prepareSync('${d.device_id}')">Sync Data</button>
                            <button onclick="prepareMigration('${d.device_id}')">Migrate</button>
                            <button id="prime-spotify-${d.device_id}" class="btn-spotify" style="display: none;" onclick="primeSpotify('${d.device_id}')">Prime Spotify</button>
                            <button class="btn-danger" onclick="removeDevice('${d.device_id}', '${d.name}')">Remove</button>
                        </td>
                    </tr>
                `;

                const optSync = document.createElement("option");
                optSync.value = d.device_id;
                optSync.textContent = `${d.name} (${d.ip_address})`;
                syncSelector.appendChild(optSync);

                const optMigrate = document.createElement("option");
                optMigrate.value = d.device_id;
                optMigrate.textContent = `${d.name} (${d.ip_address})`;
                migrationSelector.appendChild(optMigrate);

                if (eventSelector) {
                    const optEvent = document.createElement("option");
                    optEvent.value = d.device_id;
                    optEvent.textContent = `${d.name} (${d.ip_address})`;
                    eventSelector.appendChild(optEvent);
                }
            });
            html += "</table>";
            container.innerHTML = html;

            if (currentSyncVal) syncSelector.value = currentSyncVal;
            if (currentMigrationVal) migrationSelector.value = currentMigrationVal;
            if (eventSelector && currentEventVal) eventSelector.value = currentEventVal;

            // Asynchronously fetch live info for each device
            devices.forEach((d) => updateDeviceInfo(d.device_id, d.ip_address));
            fetchSpotifyStatus();
        }
    } catch (error) {
        document.getElementById("device-list").innerHTML = "Error loading devices: " + error;
    }
}

function prepareSync(deviceId) {
    document.getElementById("sync-device-list").value = deviceId;
    openTab(null, "tab-sync");
}

function prepareMigration(deviceId) {
    document.getElementById("migration-device-list").value = deviceId;
    openTab(null, "tab-migration");
    showSummary(deviceId);
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

    if (tabId === "tab-interactions") {
        fetchInteractionStats();
        fetchInteractions();
        fetchDNSDiscoveries();
    }

    if (tabId === "tab-parity") {
        fetchParityMismatches();
    }

    if (tabId === "tab-account") {
        fetchAccountList();
    }

    if (evt) {
        evt.currentTarget.className += " active";
    } else {
        // Find the button that corresponds to the tabId and activate it
        for (let i = 0; i < tablinks.length; i++) {
            const onclick = tablinks[i].getAttribute("onclick");
            if (onclick && onclick.includes(tabId)) {
                tablinks[i].className += " active";
                break;
            }
        }
    }
}

function getDeviceDisplayName(deviceId) {
    if (!deviceId) return "Unknown Device";

    // 1. Try migration selector
    const migrationSelector = document.getElementById("migration-device-list");
    if (migrationSelector) {
        for (let opt of migrationSelector.options) {
            if (opt.value === deviceId && opt.textContent !== "-- Select a device --") {
                return opt.textContent;
            }
        }
    }

    // 2. Try sync selector
    const syncSelector = document.getElementById("sync-device-list");
    if (syncSelector) {
        for (let opt of syncSelector.options) {
            if (opt.value === deviceId && opt.textContent !== "-- Select a device --") {
                return opt.textContent;
            }
        }
    }

    // 3. Try table lookup
    const rows = document.querySelectorAll("#device-list tr");
    for (const row of rows) {
        const idCell = row.querySelector(".col-deviceid");
        if (idCell && idCell.innerText === deviceId) {
            const name = row.querySelector(".col-name").innerText;
            const ip = row.querySelector(".col-ip").innerText;
            return `${name} (${ip})`;
        }
    }

    return deviceId;
}

async function startSync() {
    const deviceId = document.getElementById("sync-device-list").value;
    if (!deviceId) {
        alert("Please select a device first");
        return;
    }

    const status = document.getElementById("sync-status");
    const results = document.getElementById("sync-results");
    const log = document.getElementById("sync-log");

    status.style.display = "block";
    status.style.backgroundColor = "#eef";
    const display = getDeviceDisplayName(deviceId);
    status.textContent = "Syncing data from " + display + "...";
    results.style.display = "none";
    log.innerHTML = "";

    try {
        const response = await fetch("/setup/sync/" + encodeURIComponent(deviceId), {method: "POST"},);
        if (response.ok) {
            status.style.backgroundColor = "#dfd";
            status.textContent = "✅ Sync completed successfully for " + display + "!";
            results.style.display = "block";
            log.innerHTML = "Data fetched and saved to local datastore for " + display + ".\nPresets: OK\nRecents: OK\nSources: OK";
        } else {
            const err = await response.text();
            throw new Error(err);
        }
    } catch (error) {
        status.style.backgroundColor = "#fdd";
        status.textContent = "❌ Sync failed for " + display + ": " + error.message;
    }
}

async function fetchVersion() {
    try {
        const response = await fetch("/setup/version");
        const data = await response.json();
        const info = document.getElementById("version-info");
        if (info && data.version) {
            info.innerText = `AfterTouch ${data.version} (${data.commit}) - ${data.date}`;
        }
    } catch (error) {
        console.error("Failed to fetch version info", error);
    }
}

async function fetchAccountList() {
    try {
        const response = await fetch("/mgmt/accounts");
        if (!response.ok) return;
        const data = await response.json();
        const selector = document.getElementById("account-selector");
        if (selector) {
            selector.innerHTML = data.accounts.map(acc => `<option value="${acc}">${acc}</option>`).join("");
            if (data.accounts.length > 0) {
                fetchAccountDetails(selector.value);
            }
        }
    } catch (error) {
        console.error("Failed to fetch account list", error);
    }
}

async function fetchAccountDetails(accountId) {
    if (!accountId) return;
    const metadataEl = document.getElementById("account-metadata");
    const devicesEl = document.getElementById("account-devices-list");
    const regStatus = document.getElementById("spotify-reg-status");

    if (regStatus) regStatus.innerText = "";
    if (metadataEl) metadataEl.innerHTML = "Loading...";
    if (devicesEl) devicesEl.innerHTML = "Loading devices...";

    try {
        const response = await fetch(`/mgmt/accounts/${encodeURIComponent(accountId)}`);
        if (!response.ok) {
            if (metadataEl) metadataEl.innerHTML = `<span style="color:red">Failed to load account details: ${response.statusText}</span>`;
            return;
        }
        const data = await response.json();

        // Render Metadata
        if (metadataEl) {
            const warningNotice = data.account.is_placeholder ?
                `<div style="background: #fff3cd; color: #856404; padding: 10px; border: 1px solid #ffeeba; border-radius: 4px; margin-bottom: 10px; font-size: 0.85em;">
                    <strong>Notice:</strong> Account data (account.json) was not found in the expected location for this account ID.
                </div>` : "";

            metadataEl.innerHTML = `
                ${warningNotice}
                <table style="width: 100%; font-size: 0.9em;">
                    <tr><td style="padding: 4px"><strong>Account ID:</strong></td><td style="padding: 4px">${data.account.account_id}</td></tr>
                    <tr><td style="padding: 4px"><strong>Language:</strong></td><td style="padding: 4px">
                        <select id="account-language-select" style="font-size: 0.9em; padding: 2px;">
                            <option value="en" ${data.account.preferred_language === "en" || !data.account.preferred_language ? "selected" : ""}>en</option>
                            <option value="de" ${data.account.preferred_language === "de" ? "selected" : ""}>de</option>
                        </select>
                        <span id="language-update-status" style="margin-left: 8px; font-size: 0.8em; display: none;">Saving...</span>
                    </td></tr>
                    <tr><td style="padding: 4px"><strong>Provider Settings:</strong></td><td style="padding: 4px">
                        ${data.account.provider_settings && data.account.provider_settings.length > 0 ?
                            (() => {
                                const grouped = data.account.provider_settings.reduce((acc, s) => {
                                    const pName = s.provider_name || s.provider_id;
                                    if (!acc[pName]) acc[pName] = [];
                                    acc[pName].push(s);
                                    return acc;
                                }, {});
                                return Object.entries(grouped).map(([pName, settings]) => `
                                    <div style="margin-bottom: 8px;">
                                        <strong>${pName}</strong>
                                        <ul style="margin: 2px 0 0 0; padding-left: 20px; list-style-type: disc;">
                                            ${settings.map(s => {
                                                if ((s.provider_name === "SPOTIFY" || s.provider_id === "15") && s.key_name === "STREAMING_QUALITY") {
                                                    return `
                                                        <li style="margin-bottom: 4px;">
                                                            Music Streaming Quality:
                                                            <select class="provider-setting-select"
                                                                    data-account-id="${data.account.account_id}"
                                                                    data-provider-id="${s.provider_id}"
                                                                    data-key="${s.key_name}"
                                                                    style="font-size: 0.9em; padding: 2px; margin-left: 4px;">
                                                                <option value="1" ${s.value === "1" ? "selected" : ""}>Fastest Streaming - up to 128 kbit/s</option>
                                                                <option value="2" ${s.value === "2" ? "selected" : ""}>Balanced Quality and Speed - up to 192 kbit/s</option>
                                                                <option value="3" ${s.value === "3" ? "selected" : ""}>Best Quality - up to 320 kbit/s</option>
                                                            </select>
                                                            <span class="setting-update-status" style="margin-left: 8px; font-size: 0.8em; display: none;">Saving...</span>
                                                        </li>
                                                    `;
                                                }
                                                return `<li>${s.key_name}: ${s.value}</li>`;
                                            }).join("")}
                                        </ul>
                                    </div>
                                `).join("");
                            })() : "None"}
                    </td></tr>
                </table>
            `;

            const languageSelect = document.getElementById("account-language-select");
            if (languageSelect) {
                languageSelect.addEventListener("change", async (e) => {
                    const statusEl = document.getElementById("language-update-status");
                    const newLang = e.target.value;
                    if (statusEl) {
                        statusEl.innerText = "Saving...";
                        statusEl.style.display = "inline";
                        statusEl.style.color = "#666";
                    }
                    try {
                        const response = await fetch(`/mgmt/accounts/${data.account.account_id}/language`, {
                            method: "POST",
                            headers: {
                                "Content-Type": "application/json",
                            },
                            body: JSON.stringify({ language: newLang }),
                        });
                        if (response.ok) {
                            if (statusEl) {
                                statusEl.innerText = "Saved!";
                                statusEl.style.color = "#28a745";
                                setTimeout(() => {
                                    statusEl.style.display = "none";
                                }, 2000);
                            }
                        } else {
                            throw new Error(await response.text());
                        }
                    } catch (error) {
                        console.error("Failed to update language", error);
                        if (statusEl) {
                            statusEl.innerText = "Error!";
                            statusEl.style.color = "#dc3545";
                        }
                    }
                });
            }

            const providerSettingSelects = document.querySelectorAll(".provider-setting-select");
            providerSettingSelects.forEach(select => {
                select.addEventListener("change", async (e) => {
                    const statusEl = e.target.parentElement.querySelector(".setting-update-status");
                    const accID = e.target.dataset.accountId;
                    const provID = e.target.dataset.providerId;
                    const key = e.target.dataset.key;
                    const newValue = e.target.value;

                    if (statusEl) {
                        statusEl.innerText = "Saving...";
                        statusEl.style.display = "inline";
                        statusEl.style.color = "#666";
                    }

                    try {
                        const response = await fetch(`/mgmt/accounts/${accID}/provider-settings`, {
                            method: "POST",
                            headers: {
                                "Content-Type": "application/json",
                            },
                            body: JSON.stringify({
                                provider_id: provID,
                                key: key,
                                value: newValue
                            }),
                        });
                        if (response.ok) {
                            if (statusEl) {
                                statusEl.innerText = "Saved!";
                                statusEl.style.color = "#28a745";
                                setTimeout(() => {
                                    statusEl.style.display = "none";
                                }, 2000);
                            }
                        } else {
                            throw new Error(await response.text());
                        }
                    } catch (error) {
                        console.error("Failed to update provider setting", error);
                        if (statusEl) {
                            statusEl.innerText = "Error!";
                            statusEl.style.color = "#dc3545";
                        }
                    }
                });
            });
        }

        // Render Devices
        if (devicesEl) {
            if (!data.devices || data.devices.length === 0) {
                devicesEl.innerHTML = "No devices found for this account.";
                return;
            }

            devicesEl.innerHTML = data.devices.map(device => `
                <div class="summary-box" style="margin-bottom: 15px; border-left: 5px solid #007bff; padding: 15px;">
                    <div style="display: flex; justify-content: space-between; cursor: pointer; align-items: center;" onclick="toggleInfo('device-details-${device.device_id}')">
                        <h4 style="margin: 0">${device.name || "Unnamed Device"} (${device.product_code})</h4>
                        <div style="font-size: 0.8em; color: #666">
                            ${device.ip_address} | ${device.device_id} <span style="font-size: 1.2em; vertical-align: middle;">&#9662;</span>
                        </div>
                    </div>

                    <div id="device-details-${device.device_id}" style="display: none; margin-top: 15px; padding-top: 10px; border-top: 1px solid #eee">
                        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 20px">
                            <div>
                                <h5 style="margin: 10px 0 5px 0">Device Metadata</h5>
                                <div style="font-size: 0.85em; background: #f8f9fa; padding: 8px; border-radius: 4px; border: 1px solid #e9ecef">
                                    <strong>Serial:</strong> ${device.device_serial_number || device.serial_number || "N/A"}<br>
                                    <strong>MAC:</strong> ${device.mac_address || "N/A"}<br>
                                    <strong>Version:</strong> ${device.firmware_version || "N/A"}<br>
                                    <strong>Discovery:</strong> ${device.discovery_method || "N/A"}
                                </div>

                                <h5 style="margin: 15px 0 5px 0">Hardware Components</h5>
                                <ul style="font-size: 0.8em; padding-left: 20px; margin: 0">
                                    ${device.components ? device.components.map(c => `<li><strong>${c.category || c.type || 'Component'}</strong>: ${c.firmware_version || 'N/A'} <br><small style="color:#777">S/N: ${c.serial_number || 'N/A'}</small></li>`).join("") : "<li>No components found</li>"}
                                </ul>
                            </div>

                            <div>
                                <h5 style="margin: 10px 0 5px 0">Presets (1-6)</h5>
                                <div style="display: grid; grid-template-columns: repeat(2, 1fr); gap: 5px">
                                    ${Array.from({length: 6}, (_, i) => {
                                        const p = device.presets ? device.presets.find(pr => pr.button_number == i + 1) : null;
                                        let itemName = "Empty";
                                        let sourceLabel = "";

                                        if (p) {
                                            itemName = p.name || (p.source ? (p.source.source_label || p.source.name || p.source.type) : "Unknown");
                                            if (p.source) {
                                                const s = p.source;
                                                const name = s.source_label || s.source_name || s.name || s.type;
                                                const account = (s.account && s.account !== s.username) ? ` [${s.account}]` : "";
                                                const finalName = name || s.type || "Unknown Source";
                                                if (finalName) {
                                                    sourceLabel = `<br><small style="color: #666; font-size: 0.85em;">via ${finalName}${account}</small>`;
                                                }
                                            }
                                        }

                                        return `
                                            <div style="border: 1px solid #ddd; padding: 5px; font-size: 0.8em; background: ${p ? "#e6ffed" : "#f8f9fa"}; border-radius: 3px;">
                                                <strong>#${i + 1}</strong>: ${itemName}${sourceLabel}
                                            </div>
                                        `;
                                    }).join("")}
                                </div>

                                <h5 style="margin: 15px 0 5px 0">Recent Items</h5>
                                <div style="max-height: 150px; overflow-y: auto; font-size: 0.8em; border: 1px solid #eee; padding: 5px; border-radius: 4px">
                                    <ul style="padding-left: 15px; margin: 0">
                                        ${device.recents ? device.recents.slice(0, 10).map(r => {
                                            const name = r.name || (r.source ? (r.source.source_label || r.source.name || r.source.type) : "Unknown");
                                            let sourceLabel = "";
                                            if (r.source) {
                                                const s = r.source;
                                                const sName = s.source_label || s.source_name || s.name || s.type;
                                                const account = (s.account && s.account !== s.username) ? ` [${s.account}]` : "";
                                                const finalSName = sName || s.type || "Unknown Source";
                                                if (finalSName) {
                                                    sourceLabel = `<br><small style="color: #666; font-size: 0.9em;">via ${finalSName}${account}</small>`;
                                                }
                                            }
                                            return `<li>${name}${sourceLabel} <br><small style="color:#888">${r.created_on ? new Date(r.created_on * 1000).toLocaleString() : 'N/A'}</small></li>`;
                                        }).join("") : "<li>No recents</li>"}
                                    </ul>
                                </div>
                            </div>
                        </div>

                        <div style="margin-top: 15px; border-top: 1px dashed #ddd; padding-top: 10px">
                            <h5 style="margin: 0 0 5px 0">Configured Sources</h5>
                            <div style="display: flex; flex-wrap: wrap; gap: 5px">
                                ${device.sources ? device.sources.filter(s => (s.source_label || s.source_name || s.name || s.type)).map(s => {
                                    const sourceName = s.source_label || s.source_name || s.name || s.type;
                                    const usernameSuffix = (s.username && s.username !== "Local") ? ` (${s.username})` : "";
                                    const accountSuffix = (s.account && s.account !== s.username) ? ` [${s.account}]` : "";
                                    return `
                                        <span style="background: #eefbff; color: #0056b3; border: 1px solid #b8daff; padding: 2px 8px; border-radius: 12px; font-size: 0.75em" title="Source Type: ${s.type}">
                                            ${sourceName}${usernameSuffix}${accountSuffix}
                                        </span>
                                    `;
                                }).join("") : "<small style='color:#999'>None</small>"}
                            </div>
                        </div>
                    </div>
                </div>
            `).join("");
        }

    } catch (error) {
        if (metadataEl) metadataEl.innerHTML = `<span style="color:red">Error: ${error.message}</span>`;
        console.error("Failed to fetch account details", error);
    }
}

async function connectSpotifyToAccount() {
    const selector = document.getElementById("account-selector");
    const accountId = selector ? selector.value : "default";
    const statusEl = document.getElementById("spotify-reg-status");

    if (statusEl) statusEl.innerHTML = "Initializing Spotify authorization...";

    try {
        const response = await fetch(`/mgmt/spotify/init?account=${encodeURIComponent(accountId)}`, {
            method: "POST"
        });
        if (!response.ok) {
            const err = await response.text();
            throw new Error(err || response.statusText);
        }

        const data = await response.json();
        const redirectUrl = data.redirectUrl;

        if (statusEl) {
            statusEl.innerHTML = `Spotify authorization window opened. <br/>If it didn't open, <a href="${redirectUrl}" target="_blank">click here to authorize</a>.`;
        }

        // Open Spotify auth in a new window
        window.open(redirectUrl, "SpotifyAuth", "width=600,height=800");

        // Simple poll to see when we might be done (refresh every 5s for 5 mins)
        let pollCount = 0;
        const interval = setInterval(async () => {
            pollCount++;
            if (pollCount > 60) {
                clearInterval(interval);
                return;
            }
            // Refresh account details to see if source appeared
            await fetchAccountDetails(accountId);
        }, 5000);

    } catch (error) {
        if (statusEl) statusEl.innerHTML = `<span style="color:red">Error: ${error.message}</span>`;
        console.error("Spotify link failed", error);
    }
}

async function fetchInteractionStats() {
    console.log("Fetching interaction stats...");
    try {
        const response = await fetch("/setup/interaction-stats");
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const stats = await response.json();
        console.log("Fetched interaction stats:", stats);

        document.getElementById("total-requests").innerText = stats.total_requests || stats.TotalRequests || 0;

        const statsContainer = document.getElementById("interaction-stats-container",);
        if (statsContainer) {
            statsContainer.style.display = "block";
        }

        const serviceList = document.getElementById("stats-by-service");
        serviceList.innerHTML = "";
        const byService = stats.by_service || stats.ByService;
        if (byService) {
            Object.entries(byService).forEach(([service, count]) => {
                const li = document.createElement("li");
                li.innerHTML = `<strong>${service || "unknown"}:</strong> ${count || 0} requests`;
                serviceList.appendChild(li);
            });
        }

        const sessionList = document.getElementById("stats-by-session");
        const sessionFilter = document.getElementById("filter-session");
        const currentFilter = sessionFilter.value;

        sessionList.innerHTML = "";
        sessionFilter.innerHTML = '<option value="">All Sessions</option>';

        const bySession = stats.by_session || stats.BySession;
        if (bySession) {
            // Sort by session ID (timestamp) descending
            const sortedSessions = Object.entries(bySession).sort((a, b) => {
                const sessionA = a[0] || "";
                const sessionB = b[0] || "";
                return sessionB.localeCompare(sessionA);
            });

            sortedSessions.forEach(([session, count]) => {
                // Session format is like 20260215-160705-99213
                // Try to make it more readable: 2026-02-15 16:07:05 (PID 99213)
                let sessionDisplay = session || "unknown";
                if (session && session.includes("-")) {
                    const parts = session.split("-");
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

                const li = document.createElement("li");
                li.innerHTML = `
                    <span class="session-info"><strong>${sessionDisplay}:</strong> ${count || 0} requests</span>
                    <div style="display: flex; gap: 5px;">
                        <button onclick="downloadSession('${session || ""}')" class="btn-info" style="font-size: 0.8em; padding: 2px 5px;">Download</button>
                        <button onclick="filterBySession('${session || ""}')" style="font-size: 0.8em; padding: 2px 5px;">Filter</button>
                        <button onclick="deleteSession('${session || ""}')" class="btn-danger" style="font-size: 0.8em; padding: 2px 5px;">Delete</button>
                    </div>
                `;
                sessionList.appendChild(li);

                const opt = document.createElement("option");
                opt.value = session || "";
                opt.innerText = sessionDisplay;
                sessionFilter.appendChild(opt);
            });

            sessionFilter.value = currentFilter;
        }
    } catch (error) {
        console.error("Failed to fetch interaction stats", error);
    }
}

function downloadSession(sessionId) {
    if (!sessionId) return;
    window.location.href = `/setup/interactions/sessions/${sessionId}/download`;
}

async function filterBySession(sessionId) {
    document.getElementById("filter-session").value = sessionId;
    fetchInteractions();
    const browseContainer = document.getElementById("browse-recordings");
    if (browseContainer) {
        browseContainer.scrollIntoView({behavior: "smooth"});
    }
}

async function deleteSession(sessionId) {
    if (!sessionId) return;
    if (!confirm(`Are you sure you want to delete session ${sessionId}?`)) {
        return;
    }

    try {
        const response = await fetch(`/setup/interactions/sessions/${sessionId}`, {
            method: "DELETE",
        });
        if (response.ok) {
            // If the deleted session was selected in the filter, clear the filter
            const sessionFilter = document.getElementById("filter-session");
            if (sessionFilter.value === sessionId) {
                sessionFilter.value = "";
                fetchInteractions();
            }
            fetchInteractionStats();
        } else {
            const err = await response.text();
            alert("Failed to delete session: " + err);
        }
    } catch (error) {
        alert("Error deleting session: " + error.message);
    }
}

async function cleanupSessions() {
    if (!confirm("Are you sure you want to cleanup old sessions? Only the 10 most recent ones will be kept.",)) {
        return;
    }

    try {
        const response = await fetch("/setup/interactions/sessions?keep=10", {
            method: "DELETE",
        });
        if (response.ok) {
            // Refresh everything
            document.getElementById("filter-session").value = "";
            fetchInteractionStats();
            fetchInteractions();
        } else {
            const err = await response.text();
            alert("Failed to cleanup sessions: " + err);
        }
    } catch (error) {
        alert("Error cleaning up sessions: " + error.message);
    }
}

async function fetchInteractions() {
    console.log("Fetching interactions...");
    const session = document.getElementById("filter-session").value;
    const category = document.getElementById("filter-category").value;
    const since = document.getElementById("filter-since").value;

    let url = "/setup/interactions";
    const params = [];
    if (session) params.push(`session=${encodeURIComponent(session)}`);
    if (category) params.push(`category=${encodeURIComponent(category)}`);
    if (since) params.push(`since=${encodeURIComponent(since)}`);
    if (params.length > 0) url += "?" + params.join("&");

    try {
        const response = await fetch(url);
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const interactions = await response.json();
        console.log("Fetched interactions:", interactions);
        const list = document.getElementById("interactions-list");
        if (!list) {
            console.error("Could not find interactions-list element");
            return;
        }

        // Show the parent summary box if it was hidden
        const browseContainer = list.closest(".summary-box");
        if (browseContainer) {
            browseContainer.style.display = "block";
        }

        list.innerHTML = "";

        if (!interactions || interactions.length === 0) {
            list.innerHTML = '<tr><td colspan="8" style="padding: 20px; text-align: center; color: #666;">No interactions found for current filters.</td></tr>';
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

        interactions.forEach((i) => {
            const tr = document.createElement("tr");
            tr.style.borderBottom = "1px solid #eee";

            const counter = i.counter || i.Counter || 0;
            const timestamp = i.timestamp || i.Timestamp || "";
            const method = i.method || i.Method || "";
            const path = i.path || i.Path || "";
            const status = i.status || i.Status || "";
            const category = i.category || i.Category || "";
            const session = i.session || i.Session || "";
            const file = i.file || i.File || "";
            const scmudcData = i.scmudc_data || i.SCMUDCData || null;

            let statusClass = "";
            if (status >= 200 && status < 300) statusClass = "status-success"; else if (status >= 400) statusClass = "status-error";

            // Create SCMUDC event details
            let eventDetails = "";
            if (scmudcData) {
                const originIcon = getOriginIcon(scmudcData.origin);
                const actionIcon = getActionIcon(scmudcData.action);
                const truncatedCommand = scmudcData.command && scmudcData.command.length > 30 ? scmudcData.command.substring(0, 27) + "..." : scmudcData.command || "";

                eventDetails = `<span style="font-size: 0.9em;">
                    ${originIcon} ${actionIcon} ${truncatedCommand}
                    ${scmudcData.decoded_data ? '<span style="cursor: pointer;" onclick="showSCMUDCDetails(\'' + file + "')\">(...)</span>" : ""}
                </span>`;
            }

            tr.innerHTML = `
                <td style="padding: 8px; color: #888;">${counter}</td>
                <td style="padding: 8px; font-size: 0.8em; white-space: nowrap;">${timestamp}</td>
                <td style="padding: 8px; font-family: monospace;">${method}</td>
                <td style="padding: 8px; font-size: 0.9em;">${path}</td>
                <td style="padding: 8px;"><span class="badge ${statusClass}">${status || "???"}</span></td>
                <td style="padding: 8px;"><span class="badge category-${category}">${category}</span></td>
                <td style="padding: 8px;">${eventDetails}</td>
                <td style="padding: 8px;"><button onclick="viewInteraction('${file}')">View</button></td>
            `;
            list.appendChild(tr);
        });
    } catch (error) {
        console.error("Failed to fetch interactions", error);
    }
}

// Helper functions for SCMUDC event display
function getOriginIcon(origin) {
    const icons = {
        gabbo: "📱", // SoundTouch App
        console: "🎛️", // Device Hardware
        device: "🔄", // Internal System
    };
    return icons[origin] || "❓";
}

function getActionIcon(action) {
    const icons = {
        "play-pressed": "▶️",
        "pause-pressed": "⏸️",
        "power-pressed": "⚡",
        "preset-pressed": "⭐",
        "play-item": "🎵",
        "skip-forward-pressed": "⏭️",
        "stop-pressed": "⏹️",
    };
    return icons[action] || "🔘";
}

function showSCMUDCDetails(file) {
    // Find the interaction data for this file
    fetch("/setup/interactions")
        .then((response) => response.json())
        .then((interactions) => {
            const interaction = interactions.find((i) => (i.file || i.File) === file);
            if (interaction && interaction.scmudc_data) {
                displaySCMUDCPopover(interaction.scmudc_data);
            }
        })
        .catch((error) => {
            console.error("Failed to fetch SCMUDC details:", error);
        });
}

function displaySCMUDCPopover(scmudcData) {
    const modal = document.createElement("div");
    modal.style.cssText = `
        position: fixed; top: 0; left: 0; right: 0; bottom: 0;
        background: rgba(0,0,0,0.5); z-index: 1000;
        display: flex; align-items: center; justify-content: center;
    `;

    const content = document.createElement("div");
    content.style.cssText = `
        background: white; padding: 20px; border-radius: 8px;
        max-width: 600px; max-height: 80%; overflow-y: auto;
        box-shadow: 0 4px 6px rgba(0,0,0,0.1);
    `;

    let html = `
        <h3>SCMUDC Event Details</h3>
        <p><strong>Origin:</strong> ${getOriginDescription(scmudcData.origin)} (${scmudcData.origin})</p>
        <p><strong>Action:</strong> ${scmudcData.action}</p>
        <p><strong>Summary:</strong> ${scmudcData.summary}</p>
    `;

    if (scmudcData.decoded_data) {
        html += `
            <h4>Decoded Content:</h4>
            <p><strong>Source:</strong> ${scmudcData.decoded_data.content_type}</p>
            <p><strong>Item:</strong> ${scmudcData.decoded_data.item_name}</p>
        `;

        if (scmudcData.decoded_data.source_account) {
            html += `<p><strong>Account:</strong> ${scmudcData.decoded_data.source_account}</p>`;
        }

        if (scmudcData.decoded_data.artwork_url) {
            html += `<p><strong>Artwork:</strong> <a href="${scmudcData.decoded_data.artwork_url}" target="_blank">View</a></p>`;
        }

        if (scmudcData.decoded_data.is_presetable) {
            html += `<p><strong>Presetable:</strong> Yes</p>`;
        }

        if (scmudcData.decoded_data.xml_content) {
            html += `
                <h4>Full XML Content:</h4>
                <pre style="background: #f4f4f4; padding: 10px; border-radius: 4px; overflow-x: auto; font-size: 0.8em;">${escapeHtml(scmudcData.decoded_data.xml_content)}</pre>
            `;
        }
    }

    html += '<br><button onclick="this.closest(\'div\').remove()" style="padding: 8px 16px;">Close</button>';
    content.innerHTML = html;
    modal.appendChild(content);
    document.body.appendChild(modal);

    // Close on background click
    modal.addEventListener("click", (e) => {
        if (e.target === modal) modal.remove();
    });
}

function getOriginDescription(origin) {
    const descriptions = {
        gabbo: "SoundTouch App", console: "Device Hardware", device: "Internal System",
    };
    return descriptions[origin] || origin;
}

function escapeHtml(text) {
    const div = document.createElement("div");
    div.textContent = text;
    return div.innerHTML;
}

async function viewInteraction(file) {
    try {
        const response = await fetch(`/setup/interaction-content?file=${encodeURIComponent(file)}`,);
        const content = await response.text();

        document.getElementById("viewer-filename").innerText = file;
        document.getElementById("interaction-content").innerText = content;
        document.getElementById("interaction-viewer").style.display = "block";
        document
            .getElementById("interaction-viewer")
            .scrollIntoView({behavior: "smooth"});
    } catch (error) {
        alert("Failed to load interaction content: " + error);
    }
}

async function fetchDNSDiscoveries() {
    console.log("Fetching DNS discoveries...");
    try {
        const response = await fetch("/setup/dns-discoveries");
        if (!response.ok) {
            throw new Error(`HTTP error! status: ${response.status}`);
        }
        const discoveries = await response.json();
        console.log("Fetched DNS discoveries:", discoveries);
        const list = document.getElementById("dns-discoveries-list");
        if (!list) {
            console.error("Could not find dns-discoveries-list element");
            return;
        }

        list.innerHTML = "";

        if (!discoveries || discoveries.length === 0) {
            list.innerHTML = '<tr><td colspan="6" style="padding: 20px; text-align: center; color: #666;">No DNS queries discovered yet.</td></tr>';
            return;
        }

        discoveries.forEach((d) => {
            const tr = document.createElement("tr");
            tr.style.borderBottom = "1px solid #eee";

            const hostname = d.hostname || "";
            const lastSeen = d.last_seen || "";
            const count = d.query_count || 0;
            const isBose = d.is_bose_service ? "✅" : "❌";
            const category = d.is_intercepted ? "self" : "upstream";
            const remoteAddr = d.remote_addr || "unknown";

            tr.innerHTML = `
                <td style="padding: 8px; font-weight: bold;">${hostname}</td>
                <td style="padding: 8px; font-size: 0.85em;">${lastSeen}</td>
                <td style="padding: 8px; text-align: center;">${count}</td>
                <td style="padding: 8px; text-align: center;">${isBose}</td>
                <td style="padding: 8px;"><span class="badge category-${category}">${category}</span></td>
                <td style="padding: 8px; font-size: 0.8em; color: #666;">${remoteAddr}</td>
            `;
            list.appendChild(tr);
        });
    } catch (error) {
        console.error("Failed to fetch DNS discoveries", error);
    }
}

async function clearDNSDiscoveries() {
    if (!confirm("Are you sure you want to clear all DNS discovery logs?")) {
        return;
    }

    try {
        const response = await fetch("/setup/dns-discoveries", {
            method: "DELETE",
        });
        if (response.ok) {
            fetchDNSDiscoveries();
        } else {
            const err = await response.text();
            alert("Failed to clear DNS discoveries: " + err);
        }
    } catch (error) {
        alert("Error clearing DNS discoveries: " + error.message);
    }
}

function downloadDNSDiscoveries() {
    window.location.href = "/setup/dns-discoveries/download";
}

async function showDeviceEvents() {
    const overlay = document.getElementById("device-events-overlay");
    overlay.style.display = "block";
    overlay.scrollIntoView({behavior: "smooth"});

    // Ensure device selector is populated (handled by fetchDevices)
    // but if it's still empty, we can try to trigger a fetch
    const selector = document.getElementById("event-device-selector");
    if (selector.options.length <= 1) {
        fetchDevices();
    }
}

async function fetchDeviceEvents(deviceId) {
    if (!deviceId) return;

    const list = document.getElementById("events-list");
    list.innerHTML = '<tr><td colspan="3" style="padding: 20px; text-align: center; color: #666;">Loading events...</td></tr>';

    try {
        const response = await fetch(`/setup/devices/${deviceId}/events`);
        const data = await response.json();
        const events = data.events;

        list.innerHTML = "";
        if (!events || events.length === 0) {
            list.innerHTML = '<tr><td colspan="3" style="padding: 20px; text-align: center; color: #666;">No events found for this device.</td></tr>';
            return;
        }

        // Sort events by time descending
        events.sort((a, b) => (b.time || "").localeCompare(a.time || ""));

        events.forEach((e) => {
            const tr = document.createElement("tr");
            tr.style.borderBottom = "1px solid #eee";

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

async function fetchParityMismatches() {
    const list = document.getElementById("parity-mismatches-list");
    list.innerHTML = '<tr><td colspan="5" style="padding: 20px; text-align: center; color: #666;">Loading mismatches...</td></tr>';

    try {
        const response = await fetch("/setup/parity-mismatches");
        const mismatches = await response.json();

        list.innerHTML = "";
        if (!mismatches || mismatches.length === 0) {
            list.innerHTML = '<tr><td colspan="5" style="padding: 20px; text-align: center; color: #666;">No parity mismatches detected yet.</td></tr>';
            return;
        }

        mismatches.forEach((m) => {
            const tr = document.createElement("tr");
            tr.style.borderBottom = "1px solid #eee";

            const time = m.timestamp || "";
            const method = m.method || "";
            const path = m.path || "";
            const reasons = (m.reasons || []).join(", ");

            tr.innerHTML = `
                <td style="padding: 8px; font-size: 0.8em;">${time}</td>
                <td style="padding: 8px; font-family: monospace;">${method}</td>
                <td style="padding: 8px; font-size: 0.9em;">${path}</td>
                <td style="padding: 8px; font-size: 0.85em; color: #c62828;">${reasons}</td>
                <td style="padding: 8px;"><button onclick='viewParityMismatch(${JSON.stringify(m)})'>View Diff</button></td>
            `;
            list.appendChild(tr);
        });
    } catch (error) {
        list.innerHTML = `<tr><td colspan="5" style="padding: 20px; text-align: center; color: #f44336;">Error loading mismatches: ${error.message}</td></tr>`;
    }
}

async function clearParityMismatches() {
    if (!confirm("Are you sure you want to clear all parity mismatch records?")) return;
    try {
        await fetch("/setup/parity-mismatches", {method: "DELETE"});
        fetchParityMismatches();
        document.getElementById("parity-diff-view").style.display = "none";
    } catch (error) {
        alert("Failed to clear mismatches: " + error.message);
    }
}

function viewParityMismatch(m, forceRichDiff = false) {
    document.getElementById("diff-path-display").innerText = m.method + " " + m.path;
    const reasonsList = document.getElementById("diff-reasons-list");
    reasonsList.innerHTML = "";
    (m.reasons || []).forEach((r) => {
        const li = document.createElement("li");
        li.innerText = r;
        reasonsList.appendChild(li);
    });

    document.getElementById("diff-local-meta").innerText = `Status: ${m.local.status}`;
    document.getElementById("diff-upstream-meta").innerText = `Status: ${m.upstream.status}`;

    const localCT = (m.local.headers && m.local.headers["Content-Type"]) ? m.local.headers["Content-Type"][0] : "";
    const upstreamCT = (m.upstream.headers && m.upstream.headers["Content-Type"]) ? m.upstream.headers["Content-Type"][0] : "";

    const localBody = formatBody(m.local.body, localCT);
    const upstreamBody = formatBody(m.upstream.body, upstreamCT);

    const diffSizeThreshold = 50000; // 50KB
    const isLarge = localBody.length > diffSizeThreshold || upstreamBody.length > diffSizeThreshold;
    const warningEl = document.getElementById("diff-size-warning");

    if (isLarge && !forceRichDiff) {
        warningEl.style.display = "block";
        const forceBtn = document.getElementById("force-rich-diff-btn");
        forceBtn.onclick = () => viewParityMismatch(m, true);

        document.getElementById("diff-local-body").innerText = localBody;
        document.getElementById("diff-upstream-body").innerText = upstreamBody;
    } else {
        warningEl.style.display = "none";
        if (typeof Diff !== 'undefined') {
            const diff = Diff.diffChars(localBody, upstreamBody);
            const localEl = document.getElementById("diff-local-body");
            const upstreamEl = document.getElementById("diff-upstream-body");

            localEl.innerHTML = "";
            upstreamEl.innerHTML = "";

            diff.forEach((part) => {
                const span = document.createElement('span');
                if (part.added) {
                    span.className = 'diff-added';
                    span.innerText = part.value;
                    upstreamEl.appendChild(span);
                } else if (part.removed) {
                    span.className = 'diff-removed';
                    span.innerText = part.value;
                    localEl.appendChild(span);
                } else {
                    localEl.appendChild(document.createTextNode(part.value));
                    upstreamEl.appendChild(document.createTextNode(part.value));
                }
            });
        } else {
            document.getElementById("diff-local-body").innerText = localBody;
            document.getElementById("diff-upstream-body").innerText = upstreamBody;
        }
    }

    document.getElementById("parity-diff-view").style.display = "block";
    document
        .getElementById("parity-diff-view")
        .scrollIntoView({behavior: "smooth"});
}

function formatBody(body, contentType) {
    if (!body) return "";
    contentType = (contentType || "").toLowerCase();
    if (contentType.includes("json")) {
        return formatJSON(body);
    }
    if (contentType.includes("xml")) {
        return formatXML(body);
    }
    return body;
}

function formatJSON(json) {
    if (!json) return "";
    try {
        const obj = typeof json === "string" ? JSON.parse(json) : json;
        return JSON.stringify(obj, null, 2);
    } catch (e) {
        return json;
    }
}

function formatXML(xml) {
    if (!xml) return "";
    try {
        let formatted = "";
        let reg = /(>)(<)(\/*)/g;
        xml = xml.replace(reg, "$1\r\n$2$3");
        let pad = 0;
        xml.split("\r\n").forEach(function (node) {
            let indent = 0;
            if (node.match(/.+<\/\w[^>]*>$/)) {
                indent = 0;
            } else if (node.match(/^<\/\w/)) {
                if (pad !== 0) pad -= 1;
            } else if (node.match(/^<\w[^>]*[^\/]>.*$/)) {
                indent = 1;
            } else {
                indent = 0;
            }

            let padding = "";
            for (let i = 0; i < pad; i++) padding += "  ";
            formatted += padding + node + "\r\n";
            pad += indent;
        });
        return formatted.trim();
    } catch (e) {
        return xml;
    }
}

document.addEventListener("DOMContentLoaded", () => {
    fetchSettings();
    fetchDevices();
    triggerDiscovery();
    fetchVersion();
    fetchParityMismatches();

    const syncBtn = document.getElementById("sync-now-btn");
    if (syncBtn) syncBtn.onclick = startSync;
});

async function addManualDevice() {
    const ip = document.getElementById("add-manual-ip").value.trim();
    if (!ip) {
        alert("Please enter an IP address");
        return;
    }

    try {
        const response = await fetch("/setup/devices", {
            method: "POST", headers: {"Content-Type": "application/json"}, body: JSON.stringify({ip: ip}),
        });

        if (response.ok) {
            document.getElementById("add-manual-ip").value = "";
            fetchDevices();
        } else {
            const err = await response.text();
            alert("Failed to add device: " + err);
        }
    } catch (error) {
        alert("Error adding device: " + error.message);
    }
}

async function removeDevice(deviceId, name) {
    if (!confirm(`Are you sure you want to remove device "${name}"?`)) {
        return;
    }

    try {
        const response = await fetch(`/setup/devices/${deviceId}`, {
            method: "DELETE",
        });

        if (response.ok) {
            fetchDevices();
        } else {
            const err = await response.text();
            alert("Failed to remove device: " + err);
        }
    } catch (error) {
        alert("Error removing device: " + error.message);
    }
}

async function triggerDiscovery() {
    const indicator = document.getElementById("discovery-indicator");
    if (indicator) indicator.style.display = "inline";
    try {
        const response = await fetch("/setup/discover", {method: "POST"});
        if (!response.ok) {
            const err = await response.text();
            alert("Failed to start discovery: " + err);
            if (indicator) indicator.style.display = "none";
            return;
        }
        pollDiscoveryStatus();
    } catch (error) {
        console.error("Failed to trigger discovery", error);
        if (indicator) indicator.style.display = "none";
    }
}

async function pollDiscoveryStatus() {
    const indicator = document.getElementById("discovery-indicator");
    try {
        const response = await fetch("/setup/discovery-status");
        const data = await response.json();
        if (data.discovering) {
            setTimeout(pollDiscoveryStatus, 2000);
        } else {
            if (indicator) indicator.style.display = "none";
            fetchDevices();
        }
    } catch (error) {
        console.error("Failed to check discovery status", error);
        if (indicator) indicator.style.display = "none";
    }
}

async function updateDeviceInfo(deviceId, ip) {
    try {
        const response = await fetch("/setup/info/" + encodeURIComponent(deviceId));
        if (!response.ok) return;
        const info = await response.json();

        const rowId = "device-row-" + deviceId;
        const row = document.getElementById(rowId);
        if (row) {
            const nameEl = row.querySelector(".col-name");
            if (nameEl && info.name) nameEl.innerText = info.name;

            const modelEl = row.querySelector(".col-model");
            if (modelEl && info.type) modelEl.innerText = info.type;

            const serialEl = row.querySelector(".col-serial");
            if (serialEl && info.serialNumber) serialEl.innerText = info.serialNumber;

            const firmwareEl = row.querySelector(".col-firmware");
            if (firmwareEl && info.softwareVersion) firmwareEl.innerText = info.softwareVersion;

            const deviceIdEl = row.querySelector(".col-deviceid");
            if (deviceIdEl && info.deviceID) deviceIdEl.innerText = info.deviceID;

            const accountIdEl = row.querySelector(".col-accountid");
            if (accountIdEl && info.margeAccountUUID) accountIdEl.innerText = info.margeAccountUUID;
        }
    } catch (error) {
        console.warn("Failed to fetch live info for " + ip, error);
    }
}

async function showSummary(deviceId) {
    if (!deviceId) {
        document.getElementById("migration-summary").style.display = "none";
        return;
    }
    const targetUrl = document.getElementById("target-domain").value;

    const opts = {
        marge: document.getElementById("opt-marge").value,
        stats: document.getElementById("opt-stats").value,
        sw_update: document.getElementById("opt-sw_update").value,
        bmx: document.getElementById("opt-bmx").value,
    };

    const statusDiv = document.getElementById("status");
    statusDiv.style.display = "block";
    statusDiv.style.backgroundColor = "#ffffcc";

    const display = getDeviceDisplayName(deviceId);
    statusDiv.innerHTML = "Fetching summary for " + display + "...";

    const outputBox = document.getElementById("command-output-box");
    if (outputBox) outputBox.style.display = "none";

    let query = "?target_url=" + encodeURIComponent(targetUrl);
    for (let k in opts) {
        query += "&" + k + "=" + encodeURIComponent(opts[k]);
    }

    try {
        const response = await fetch("/setup/summary/" + encodeURIComponent(deviceId) + query,);
        if (!response.ok) {
            const errorText = await response.text();
            throw new Error(errorText);
        }
        const summary = await response.json();

        statusDiv.style.display = "none";

        const ip = summary.ip_address || deviceId;
        const finalDisplay = summary.device_name ? `${summary.device_name} (${ip})` : ip;
        document.getElementById("summary-device-display").innerText = finalDisplay;
        // Keep deviceId hidden for subsequent calls
        document.getElementById("summary-device-id").value = deviceId;

        // Update table row if it exists
        const rowId = "device-row-" + deviceId;
        const row = document.getElementById(rowId);
        if (row) {
            const nameEl = row.querySelector(".col-name");
            if (nameEl && summary.device_name) nameEl.innerText = summary.device_name;

            const modelEl = row.querySelector(".col-model");
            if (modelEl && summary.device_model) modelEl.innerText = summary.device_model;

            const serialEl = row.querySelector(".col-serial");
            if (serialEl && summary.device_serial) serialEl.innerText = summary.device_serial;

            const firmwareEl = row.querySelector(".col-firmware");
            if (firmwareEl && summary.firmware_version) firmwareEl.innerText = summary.firmware_version;

            const deviceIdEl = row.querySelector(".col-deviceid");
            if (deviceIdEl && summary.device_id) deviceIdEl.innerText = summary.device_id;

            const accountIdEl = row.querySelector(".col-accountid");
            if (accountIdEl && summary.account_id) accountIdEl.innerText = summary.account_id;
        }

        document.getElementById("ssh-status").innerText = summary.ssh_success ? "✅ Success" : "❌ Failed";
        document.getElementById("ssh-status").style.color = summary.ssh_success ? "green" : "red";

        const migrationStatus = document.getElementById("migration-status");
        migrationStatus.innerText = summary.is_migrated ? "✅ Migrated to AfterTouch" : "❌ Not Migrated";
        migrationStatus.style.color = summary.is_migrated ? "green" : "red";
        migrationStatus.style.fontWeight = "bold";

        document.getElementById("original-config-status").style.display = summary.original_config ? "block" : "none";
        document.getElementById("no-original-config-status").style.display = summary.original_config ? "none" : "block";
        document.getElementById("original-config-content").innerText = summary.original_config || "";
        document.getElementById("original-config-pane").style.display = "none";

        if (summary.parsed_current_config) {
            document.getElementById("service-options").style.display = "block";
            document.getElementById("orig-marge").innerText = summary.parsed_current_config.margeServerUrl;
            document.getElementById("orig-stats").innerText = summary.parsed_current_config.statsServerUrl;
            document.getElementById("orig-sw_update").innerText = summary.parsed_current_config.swUpdateUrl;
            document.getElementById("orig-bmx").innerText = summary.parsed_current_config.bmxRegistryUrl;
        } else {
            document.getElementById("service-options").style.display = "none";
        }

        const remoteStatus = document.getElementById("remote-services-status");
        const remoteFound = document.getElementById("remote-services-found");
        if (summary.ssh_success) {
            if (summary.remote_services_enabled) {
                remoteStatus.innerText = summary.remote_services_persistent ? "✅ Yes" : "⚠️ Yes (non-persistent)";
                remoteStatus.style.color = summary.remote_services_persistent ? "green" : "orange";
            } else {
                remoteStatus.innerText = "❌ No";
                remoteStatus.style.color = "red";
            }
            remoteFound.innerText = summary.remote_services_found && summary.remote_services_found.length > 0 ? "(" + summary.remote_services_found.join(", ") + ")" : "";

            const caTrustStatus = document.getElementById("ca-trust-status");
            caTrustStatus.innerText = summary.ca_cert_trusted ? "✅ Yes" : "❌ No";
            caTrustStatus.style.color = summary.ca_cert_trusted ? "green" : "red";
            document.getElementById("trust-ca-btn").style.display = summary.ca_cert_trusted ? "none" : "inline-block";
            document.getElementById("trust-ca-btn").onclick = () => trustCA(deviceId, ip);
        } else {
            remoteStatus.innerText = "❓ Unknown";
            remoteStatus.style.color = "gray";
            remoteFound.innerText = "";

            const caTrustStatus = document.getElementById("ca-trust-status");
            caTrustStatus.innerText = "❓ Unknown";
            caTrustStatus.style.color = "gray";
        }

        const currentConfigElem = document.getElementById("current-config");
        currentConfigElem.innerText = summary.current_config;
        currentConfigElem.style.color = summary.ssh_success ? "black" : "red";

        document.getElementById("planned-config").innerText = summary.planned_config;
        document.getElementById("planned-hosts").innerText = summary.planned_hosts || "";
        document.getElementById("planned-resolv").innerText = summary.planned_resolv || "";

        const currentResolvElem = document.getElementById("current-resolv-content");
        if (currentResolvElem) {
            currentResolvElem.innerText = summary.current_resolv_conf || "Not available";
        }

        const testUrlElem = document.getElementById("test-url");
        testUrlElem.innerText = summary.server_https_url || "N/A";
        const testResultDiv = document.getElementById("test-result");
        testResultDiv.style.display = "none";
        testResultDiv.innerText = "";

        document.getElementById("test-connection-explicit-btn").onclick = () => testConnection(deviceId, true);
        document.getElementById("test-connection-trusted-btn").onclick = () => testConnection(deviceId, false);
        document.getElementById("test-hosts-btn").onclick = () => testHostsRedirection(deviceId);
        document.getElementById("test-dns-btn").onclick = () => testDNSRedirection(deviceId);

        toggleMigrationMethod();

        const migrateBtn = document.getElementById("confirm-migrate-btn");
        migrateBtn.onclick = () => migrate(deviceId, ip);
        migrateBtn.disabled = !summary.ssh_success;

        const revertBtn = document.getElementById("revert-migrate-btn");
        revertBtn.onclick = () => revert(deviceId, ip);
        revertBtn.disabled = !summary.ssh_success;
        revertBtn.style.display = summary.original_config ? "inline-block" : "none";

        const rebootBtn = document.getElementById("reboot-speaker-btn");
        rebootBtn.onclick = () => reboot(deviceId, ip);
        rebootBtn.disabled = !summary.ssh_success;
        rebootBtn.style.border = "none"; // Reset border if it was set during migration

        const remoteBtn = document.getElementById("ensure-remote-btn");
        remoteBtn.onclick = () => ensureRemoteServices(deviceId, ip);
        remoteBtn.disabled = !summary.ssh_success;

        const removeRemoteBtn = document.getElementById("remove-remote-btn");
        removeRemoteBtn.onclick = () => removeRemoteServices(deviceId, ip);
        removeRemoteBtn.disabled = !summary.ssh_success || !summary.remote_services_enabled;

        const backupBtn = document.getElementById("backup-config-btn");
        backupBtn.onclick = () => backupConfig(deviceId, ip);
        backupBtn.disabled = !summary.ssh_success || !!summary.original_config;

        document.getElementById("migration-summary").style.display = "block";
        document.getElementById("migration-summary").scrollIntoView();
    } catch (error) {
        statusDiv.style.backgroundColor = "#ffcccc";
        statusDiv.innerHTML = "Error fetching summary for " + display + ": " + error;
    }
}

function refreshSummary() {
    const deviceId = document.getElementById("summary-device-id").value;
    if (deviceId) {
        showSummary(deviceId);
    }
}

function showCommandOutput(result) {
    const outputBox = document.getElementById("command-output-box");
    const outputText = document.getElementById("command-output");
    if (outputBox && outputText && result.output) {
        outputBox.style.display = "block";
        outputText.innerText = result.output;
    } else if (outputBox) {
        outputBox.style.display = "none";
    }
}

async function revert(deviceId, ip) {
    if (!deviceId) {
        alert("Please select a device.");
        return;
    }
    const display = getDeviceDisplayName(deviceId);
    if (!confirm("Are you sure you want to revert " + display + " to Bose cloud defaults?",)) {
        return;
    }

    const summaryDiv = document.getElementById("migration-summary");
    summaryDiv.style.display = "none";

    const statusDiv = document.getElementById("status");
    statusDiv.style.display = "block";
    statusDiv.style.backgroundColor = "#ffffcc";
    statusDiv.innerHTML = "Reverting " + display + " to defaults...";

    try {
        const response = await fetch("/setup/revert/" + encodeURIComponent(deviceId), {method: "POST"},);
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = "#ccffcc";
            statusDiv.innerHTML = "Successfully started revert for " + display + ".";
        } else {
            statusDiv.style.backgroundColor = "#ffcccc";
            statusDiv.innerHTML = "Revert failed for " + display + ": " + (result.message || "Unknown error");
        }
    } catch (error) {
        statusDiv.style.backgroundColor = "#ffcccc";
        statusDiv.innerHTML = "Error reverting " + display + ": " + error;
    }
}

async function reboot(deviceId, ip) {
    if (!deviceId) {
        alert("Please select a device.");
        return;
    }
    const display = getDeviceDisplayName(deviceId);
    if (!confirm("Are you sure you want to reboot the speaker at " + display + "?")) {
        return;
    }

    const statusDiv = document.getElementById("status");
    statusDiv.style.display = "block";
    statusDiv.style.backgroundColor = "#ffffcc";
    statusDiv.innerHTML = "Rebooting " + display + "...";

    try {
        const response = await fetch("/setup/reboot/" + encodeURIComponent(deviceId), {method: "POST"},);
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = "#ccffcc";
            statusDiv.innerHTML = "Successfully started reboot for " + display + ".";
        } else {
            statusDiv.style.backgroundColor = "#ffcccc";
            statusDiv.innerHTML = "Reboot failed for " + display + ": " + (result.message || "Unknown error");
        }
    } catch (error) {
        statusDiv.style.backgroundColor = "#ffcccc";
        statusDiv.innerHTML = "Error rebooting " + display + ": " + error;
    }
}

async function migrate(deviceId, ip) {
    if (!deviceId) {
        alert("Please select a device.");
        return;
    }
    const targetUrl = document.getElementById("target-domain").value;
    const method = document.getElementById("migration-method").value;

    const opts = {
        marge: document.getElementById("opt-marge").value,
        stats: document.getElementById("opt-stats").value,
        sw_update: document.getElementById("opt-sw_update").value,
        bmx: document.getElementById("opt-bmx").value,
    };

    const summaryDiv = document.getElementById("migration-summary");
    summaryDiv.style.display = "none";

    const statusDiv = document.getElementById("status");
    statusDiv.style.display = "block";
    statusDiv.style.backgroundColor = "#ffffcc";
    const display = getDeviceDisplayName(deviceId);
    statusDiv.innerHTML = "Migrating " + display + " using " + method + "...";

    let query = "?method=" + encodeURIComponent(method) + "&target_url=" + encodeURIComponent(targetUrl);
    for (let k in opts) {
        query += "&" + k + "=" + encodeURIComponent(opts[k]);
    }

    try {
        const response = await fetch("/setup/migrate/" + encodeURIComponent(deviceId) + query, {method: "POST"},);
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = "#ccffcc";
            statusDiv.innerHTML = "Successfully started migration for " + display + ". <strong>Please reboot the device to activate the changes.</strong>";

            // Make reboot button available and prominent
            const rebootBtn = document.getElementById("reboot-speaker-btn");
            rebootBtn.style.display = "inline-block";
            rebootBtn.disabled = false;
            rebootBtn.style.border = "2px solid #000";

            // Re-show summary but with prominence on reboot
            summaryDiv.style.display = "block";
        } else {
            statusDiv.style.backgroundColor = "#ffcccc";
            statusDiv.innerHTML = "Migration failed for " + display + ": " + (result.message || "Unknown error");
        }
    } catch (error) {
        statusDiv.style.backgroundColor = "#ffcccc";
        statusDiv.innerHTML = "Error migrating " + display + ": " + error;
    }
}

async function trustCA(deviceId, ip) {
    if (!deviceId) {
        alert("Please select a device.");
        return;
    }
    const statusDiv = document.getElementById("status");
    statusDiv.style.display = "block";
    statusDiv.style.backgroundColor = "#ffffcc";
    const display = getDeviceDisplayName(deviceId);
    statusDiv.innerHTML = "Injecting Root CA into shared trust store on " + display + "...";

    try {
        const response = await fetch("/setup/trust-ca/" + encodeURIComponent(deviceId), {method: "POST"},);
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = "#ccffcc";
            statusDiv.innerHTML = "Successfully injected Root CA on " + display + ".";
            showSummary(deviceId); // Refresh to update status
        } else {
            statusDiv.style.backgroundColor = "#ffcccc";
            statusDiv.innerHTML = "Failed to trust CA on " + display + ": " + (result.message || "Unknown error");
        }
    } catch (error) {
        statusDiv.style.backgroundColor = "#ffcccc";
        statusDiv.innerHTML = "Error trusting CA on " + display + ": " + error;
    }
}

async function ensureRemoteServices(deviceId, ip) {
    if (!deviceId) {
        alert("Please select a device.");
        return;
    }
    const summaryDiv = document.getElementById("migration-summary");
    summaryDiv.style.display = "none";

    const statusDiv = document.getElementById("status");
    statusDiv.style.display = "block";
    statusDiv.style.backgroundColor = "#ffffcc";
    const display = getDeviceDisplayName(deviceId);
    statusDiv.innerHTML = "Ensuring remote services for " + display + "...";

    try {
        const response = await fetch("/setup/ensure-remote-services/" + encodeURIComponent(deviceId), {method: "POST"},);
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = "#ccffcc";
            statusDiv.innerHTML = "Successfully ensured remote services for " + display + ".";
        } else {
            statusDiv.style.backgroundColor = "#ffcccc";
            statusDiv.innerHTML = "Failed to ensure remote services for " + display + ": " + (result.message || "Unknown error");
        }
    } catch (error) {
        statusDiv.style.backgroundColor = "#ffcccc";
        statusDiv.innerHTML = "Error ensuring remote services for " + display + ": " + error;
    }
}

async function removeRemoteServices(deviceId, ip) {
    if (!deviceId) {
        alert("Please select a device.");
        return;
    }
    const display = getDeviceDisplayName(deviceId);
    if (!confirm("Are you sure you want to remove remote services from " + display + "?",)) {
        return;
    }
    const summaryDiv = document.getElementById("migration-summary");
    summaryDiv.style.display = "none";

    const statusDiv = document.getElementById("status");
    statusDiv.style.display = "block";
    statusDiv.style.backgroundColor = "#ffffcc";
    statusDiv.innerHTML = "Removing remote services for " + display + "...";

    try {
        const response = await fetch("/setup/remove-remote-services/" + encodeURIComponent(deviceId), {method: "POST"},);
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = "#ccffcc";
            statusDiv.innerHTML = "Successfully removed remote services from " + display + ".";
        } else {
            statusDiv.style.backgroundColor = "#ffcccc";
            statusDiv.innerHTML = "Failed to remove remote services for " + display + ": " + (result.message || "Unknown error");
        }
    } catch (error) {
        statusDiv.style.backgroundColor = "#ffcccc";
        statusDiv.innerHTML = "Error removing remote services for " + display + ": " + error;
    }
}

async function backupConfig(deviceId, ip) {
    if (!deviceId) {
        alert("Please select a device.");
        return;
    }
    const statusDiv = document.getElementById("status");
    statusDiv.style.display = "block";
    statusDiv.style.backgroundColor = "#ffffcc";
    const display = getDeviceDisplayName(deviceId);
    statusDiv.innerHTML = "Creating backup for " + display + "...";

    try {
        const response = await fetch("/setup/backup/" + encodeURIComponent(deviceId), {method: "POST"},);
        const result = await response.json();
        showCommandOutput(result);
        if (result.ok) {
            statusDiv.style.backgroundColor = "#ccffcc";
            statusDiv.innerHTML = "Successfully created backup for " + display + ".";
            showSummary(deviceId); // Refresh
        } else {
            statusDiv.style.backgroundColor = "#ffcccc";
            statusDiv.innerHTML = "Backup failed for " + display + ": " + (result.message || "Unknown error");
        }
    } catch (error) {
        statusDiv.style.backgroundColor = "#ffcccc";
        statusDiv.innerHTML = "Error creating backup for " + display + ": " + error;
    }
}

async function testConnection(deviceId, useExplicitCA) {
    const testUrl = document.getElementById("test-url").innerText;
    const testResultDiv = document.getElementById("test-result");
    const display = getDeviceDisplayName(deviceId);

    testResultDiv.style.display = "block";
    testResultDiv.style.backgroundColor = "#f0f0f0";
    testResultDiv.style.color = "black";
    testResultDiv.innerText = "Running connection test from " + display + "...\n(This may take a few seconds)";

    try {
        const query = `?target_url=${encodeURIComponent(testUrl)}&use_explicit_ca=${useExplicitCA}`;
        const response = await fetch(`/setup/test-connection/${encodeURIComponent(deviceId)}${query}`, {method: "POST"},);
        const result = await response.json();

        if (result.ok) {
            testResultDiv.style.backgroundColor = "#ccffcc";
            testResultDiv.innerText = "✅ " + result.message + "\n\nOutput:\n" + result.output;
        } else {
            testResultDiv.style.backgroundColor = "#ffcccc";
            testResultDiv.innerText = "❌ Connection failed: " + result.message + "\n\nOutput:\n" + result.output;
        }
    } catch (error) {
        testResultDiv.style.backgroundColor = "#ffcccc";
        testResultDiv.innerText = "❌ Error triggering test: " + error;
    }
}

async function testHostsRedirection(deviceId) {
    const targetUrl = document.getElementById("target-domain").value;
    const testResultDiv = document.getElementById("hosts-test-result");
    const display = getDeviceDisplayName(deviceId);

    testResultDiv.style.display = "block";
    testResultDiv.style.backgroundColor = "#f0f0f0";
    testResultDiv.style.color = "black";
    testResultDiv.innerText = "Running hosts redirection test from " + display + "...\n(This may take a few seconds)";

    try {
        const query = `?target_url=${encodeURIComponent(targetUrl)}`;
        const response = await fetch(`/setup/test-hosts/${encodeURIComponent(deviceId)}${query}`, {method: "POST"},);
        const result = await response.json();

        if (result.ok) {
            testResultDiv.style.backgroundColor = "#ccffcc";
            testResultDiv.innerText = "✅ " + result.message + "\n\nOutput:\n" + result.output;
        } else {
            testResultDiv.style.backgroundColor = "#ffcccc";
            testResultDiv.innerText = "❌ Test failed: " + result.message + "\n\nOutput:\n" + result.output;
        }
    } catch (error) {
        testResultDiv.style.backgroundColor = "#ffcccc";
        testResultDiv.innerText = "❌ Error triggering test: " + error;
    }
}

async function testDNSRedirection(deviceId) {
    const targetUrl = document.getElementById("target-domain").value;
    const testResultDiv = document.getElementById("dns-test-result");
    const display = getDeviceDisplayName(deviceId);

    testResultDiv.style.display = "block";
    testResultDiv.style.backgroundColor = "#f0f0f0";
    testResultDiv.style.color = "black";
    testResultDiv.innerText = "Running DNS redirection test from " + display + "...\n(This may take a few seconds)";

    try {
        const query = `?target_url=${encodeURIComponent(targetUrl)}`;
        const response = await fetch(`/setup/test-dns/${encodeURIComponent(deviceId)}${query}`, {method: "POST"},);
        const result = await response.json();

        if (result.ok) {
            testResultDiv.style.backgroundColor = "#ccffcc";
            testResultDiv.innerText = "✅ " + result.message + "\n\nOutput:\n" + result.output;
        } else {
            testResultDiv.style.backgroundColor = "#ffcccc";
            testResultDiv.innerText = "❌ Test failed: " + result.message + "\n\nOutput:\n" + result.output;
        }
    } catch (error) {
        testResultDiv.style.backgroundColor = "#ffcccc";
        testResultDiv.innerText = "❌ Error triggering test: " + error;
    }
}

function toggleOriginalConfig() {
    const pane = document.getElementById("original-config-pane");
    pane.style.display = pane.style.display === "none" ? "block" : "none";
}

async function toggleMigrationMethod() {
    const method = document.getElementById("migration-method").value;
    const xmlDiffPane = document.getElementById("xml-diff-pane");
    const plannedXmlPane = document.getElementById("planned-xml-pane");
    const plannedHostsPane = document.getElementById("planned-hosts-pane");
    const plannedResolvPane = document.getElementById("planned-resolv-pane");
    const currentResolvPane = document.getElementById("current-resolv-pane");
    const serviceOptions = document.getElementById("service-options");
    const hostsTestPane = document.getElementById("hosts-redirection-test");
    const dnsTestPane = document.getElementById("dns-redirection-test");

    const dnsWarning = document.getElementById("dns-port-warning");

    if (method === "hosts") {
        xmlDiffPane.style.display = "none";
        plannedXmlPane.style.display = "none";
        plannedHostsPane.style.display = "block";
        plannedResolvPane.style.display = "none";
        currentResolvPane.style.display = "none";
        serviceOptions.style.display = "none";
        hostsTestPane.style.display = "block";
        dnsTestPane.style.display = "none";
        if (dnsWarning) dnsWarning.style.display = "none";
    } else if (method === "resolv") {
        xmlDiffPane.style.display = "none";
        plannedXmlPane.style.display = "none";
        plannedHostsPane.style.display = "none";
        plannedResolvPane.style.display = "block";
        currentResolvPane.style.display = "none";
        serviceOptions.style.display = "none";
        hostsTestPane.style.display = "none";
        dnsTestPane.style.display = "block";

        const resolvNote = document.getElementById("resolv-note");
        if (resolvNote) {
            resolvNote.innerHTML = "<strong>Note:</strong> This method injects a persistent DNS priority hook into the DHCP logic (<code>/etc/udhcpc.d/50default</code>). It preserves your router's search domain and secondary DNS servers. It also injects the Local Root CA.";
        }

        // Check DNS settings
        try {
            const response = await fetch("/setup/settings");
            const settings = await response.json();
            const dnsBind = settings.dns_bind_addr || "";
            const isPort53 = dnsBind.endsWith(":53") || dnsBind === "53";
            const isEnabled = settings.dns_enabled;
            const isRunning = settings.dns_running;
            const actualBind = settings.dns_actual_bind;

            if (dnsWarning) {
                if (!isEnabled) {
                    dnsWarning.innerText = "⚠️ DNS Discovery is DISABLED in Settings. Migration will fail.";
                    dnsWarning.style.display = "block";
                } else if (!isPort53) {
                    dnsWarning.innerText = `⚠️ DNS Discovery is bound to ${dnsBind}, but port 53 is required for migration.`;
                    dnsWarning.style.display = "block";
                } else if (!isRunning) {
                    dnsWarning.innerText = `⚠️ DNS Discovery server is NOT RUNNING on ${dnsBind} (check for port conflicts/permissions). Migration will fail.`;
                    dnsWarning.style.display = "block";
                } else {
                    dnsWarning.style.display = "none";
                }
            }
        } catch (e) {
            console.error("Failed to check DNS settings", e);
        }
    } else {
        xmlDiffPane.style.display = "block";
        plannedXmlPane.style.display = "block";
        plannedHostsPane.style.display = "none";
        plannedResolvPane.style.display = "none";
        currentResolvPane.style.display = "none";
        hostsTestPane.style.display = "none";
        dnsTestPane.style.display = "none";
        // Only show service options if we have a parsed config
        const currentConfig = document.getElementById("current-config").innerText;
        if (currentConfig && !currentConfig.startsWith("Error") && currentConfig !== "loading...") {
            serviceOptions.style.display = "block";
        }
    }
}

document.addEventListener("DOMContentLoaded", () => {
    fetchDevices();
    fetchSettings();
    triggerDiscovery();
});
