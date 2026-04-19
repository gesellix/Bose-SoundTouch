// SoundTouch Web UI - Application JavaScript
// Single Page Application functionality with WebSocket real-time updates

// Global variables
let ws = null;
let reconnectInterval = null;
let reconnectAttempts = 0;
let maxReconnectAttempts = 5;
let devices = {};
let currentDeviceId = null;
let tuneInNavStack = [];
let tuneInPendingPlay = null;

// Page navigation
function showPage(pageId) {
    document
        .querySelectorAll(".page")
        .forEach((page) => page.classList.remove("active"));
    document.getElementById(pageId + "-page").classList.add("active");

    if (pageId === "devices") {
        currentDeviceId = null;
        loadDevices();
    } else if (pageId === "tunein" && tuneInNavStack.length === 0) {
        tuneInBrowse();
    }
}

// ── TuneIn Browse ──────────────────────────────────────────────────────────────

function tuneInBrowse() {
    tuneInNavStack = [{ fetchUrl: "/api/tunein/navigate", label: "TuneIn" }];
    tuneInRenderBreadcrumb();
    tuneInFetchAndRender("/api/tunein/navigate");
}

function tuneInSearch(query) {
    if (!query || !query.trim()) return;
    const q = query.trim();
    document.getElementById("tunein-search-input").value = q;
    const url = "/api/tunein/search?q=" + encodeURIComponent(q);
    tuneInNavStack = [
        { fetchUrl: "/api/tunein/navigate", label: "TuneIn" },
        { fetchUrl: url, label: "Search: " + q },
    ];
    tuneInRenderBreadcrumb();
    tuneInFetchAndRender(url);
}

function tuneInNavigate(navPath, label) {
    const url = "/api/tunein/navigate/" + navPath;
    tuneInNavStack.push({ fetchUrl: url, label: label || "Browse" });
    tuneInRenderBreadcrumb();
    tuneInFetchAndRender(url);
}

function tuneInNavTo(index) {
    tuneInNavStack = tuneInNavStack.slice(0, index + 1);
    tuneInRenderBreadcrumb();
    tuneInFetchAndRender(tuneInNavStack[tuneInNavStack.length - 1].fetchUrl);
}

function tuneInRenderBreadcrumb() {
    const nav = document.getElementById("tunein-breadcrumb");
    if (tuneInNavStack.length <= 1) {
        nav.style.display = "none";
        return;
    }
    nav.style.display = "";
    const items = tuneInNavStack
        .map((entry, i) => {
            if (i === tuneInNavStack.length - 1) {
                return `<li class="breadcrumb-item active" aria-current="page">${escapeHtml(entry.label)}</li>`;
            }
            return `<li class="breadcrumb-item"><a href="#" onclick="tuneInNavTo(${i}); return false;">${escapeHtml(entry.label)}</a></li>`;
        })
        .join("");
    nav.innerHTML = `<ol class="breadcrumb mb-0">${items}</ol>`;
}

function tuneInFetchAndRender(url) {
    const el = document.getElementById("tunein-results");
    el.innerHTML = '<div class="loading-spinner mx-auto mt-4"></div>';
    fetch(url)
        .then((r) => r.json())
        .then((data) => {
            if (data.success) {
                renderTuneInResponse(data.data);
            } else {
                el.innerHTML = `<div class="alert alert-danger mt-3">${escapeHtml(data.error || "Failed to load TuneIn content")}</div>`;
            }
        })
        .catch(() => {
            el.innerHTML =
                '<div class="alert alert-danger mt-3">Failed to load TuneIn content. Check your connection.</div>';
        });
}

function renderTuneInResponse(data) {
    const el = document.getElementById("tunein-results");
    if (!data || !data.bmx_sections || data.bmx_sections.length === 0) {
        el.innerHTML =
            '<div class="text-center text-muted py-5"><i class="bi bi-music-note display-4"></i><p class="mt-2">No results found</p></div>';
        return;
    }
    el.innerHTML = data.bmx_sections.map(renderTuneInSection).join("");
}

function renderTuneInSection(section) {
    const layout = section.layout || "list";
    const items = section.items || [];
    if (items.length === 0) return "";

    const titleHtml = section.name
        ? `<h5 class="tunein-section-title">${escapeHtml(section.name)}</h5>`
        : "";

    let itemsHtml;
    if (layout === "ribbon") {
        itemsHtml = `<div class="tunein-ribbon">${items.map((item) => renderTuneInItem(item, "ribbon")).join("")}</div>`;
    } else if (layout === "hero") {
        itemsHtml = `<div class="tunein-hero">${items.map((item) => renderTuneInItem(item, "hero")).join("")}</div>`;
    } else if (layout === "responsiveGrid") {
        itemsHtml = `<div class="tunein-grid">${items.map((item) => renderTuneInItem(item, "grid")).join("")}</div>`;
    } else {
        itemsHtml = `<div class="tunein-list">${items.map((item) => renderTuneInItem(item, "list")).join("")}</div>`;
    }

    return `<div class="tunein-section">${titleHtml}${itemsHtml}</div>`;
}

function tuneInNavPath(item) {
    const href = item._links?.bmx_navigate?.href;
    return href ? href.replace(/^\/v1\/navigate\/?/, "") : null;
}

function renderTuneInItem(item, layout) {
    const navPath = tuneInNavPath(item);
    const isNavigable = !!navPath;
    const playHref = item._links?.bmx_playback?.href;
    const playType = item._links?.bmx_playback?.type || "stationurl";
    const isPlayable = !!playHref;
    const name = item.name || "";
    const subtitle = item.subtitle || "";
    const imageUrl = item.imageUrl || "";

    const navAttrs = isNavigable
        ? `data-nav-path="${escapeHtml(navPath)}" data-nav-label="${escapeHtml(name)}" role="button" tabindex="0"`
        : "";
    const navClass = isNavigable ? " tunein-nav-item" : "";

    const playBtn = isPlayable
        ? `<button class="tunein-play-btn" data-play-location="${escapeHtml(playHref)}" data-play-name="${escapeHtml(name)}" data-play-type="${escapeHtml(playType)}" data-play-art="${escapeHtml(imageUrl)}" title="Play ${escapeHtml(name)}" aria-label="Play ${escapeHtml(name)}"><i class="bi bi-play-fill"></i></button>`
        : "";

    const imgHtml = imageUrl
        ? `<img src="${escapeHtml(imageUrl)}" alt="" class="tunein-item-image" loading="lazy" onerror="this.style.display='none'">`
        : `<div class="tunein-item-image tunein-item-placeholder"><i class="bi bi-music-note-beamed"></i></div>`;

    if (layout === "ribbon") {
        return `<div class="tunein-ribbon-item${navClass}" ${navAttrs}>${imgHtml}<div class="tunein-item-label">${escapeHtml(name)}</div>${playBtn}</div>`;
    }

    if (layout === "hero") {
        return `<div class="tunein-hero-item${navClass}" ${navAttrs}>${imgHtml}<div class="tunein-hero-overlay"><div class="tunein-hero-name">${escapeHtml(name)}</div>${subtitle ? `<div class="tunein-hero-subtitle">${escapeHtml(subtitle)}</div>` : ""}</div>${playBtn ? `<div class="tunein-hero-play">${playBtn}</div>` : ""}</div>`;
    }

    if (layout === "grid") {
        return `<div class="tunein-grid-item${navClass}" ${navAttrs}>${imgHtml}<div class="tunein-item-label">${escapeHtml(name)}</div>${subtitle ? `<div class="tunein-item-subtitle">${escapeHtml(subtitle)}</div>` : ""}${playBtn ? `<div class="mt-1 text-center">${playBtn}</div>` : ""}</div>`;
    }

    // list / shortList / default
    return `<div class="tunein-list-item${navClass}" ${navAttrs}>${imgHtml}<div class="tunein-item-info"><div class="tunein-item-name">${escapeHtml(name)}</div>${subtitle ? `<div class="tunein-item-subtitle">${escapeHtml(subtitle)}</div>` : ""}</div>${isNavigable ? '<i class="bi bi-chevron-right tunein-item-chevron ms-auto"></i>' : ""}${playBtn}</div>`;
}

function tuneInPlayClick(location, name, type, art) {
    const deviceIds = Object.keys(devices);
    if (deviceIds.length === 0) {
        showToast("No Devices", "No SoundTouch devices found. Try discovering devices first.", "warning");
        return;
    }
    if (deviceIds.length === 1) {
        tuneInPlay(deviceIds[0], location, name, type, art);
    } else {
        tuneInShowDevicePicker(location, name, type, art);
    }
}

function tuneInPlay(deviceId, location, name, type, art) {
    fetch(`/api/tunein/play/${deviceId}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ location, name, type, containerArt: art }),
    })
        .then((r) => r.json())
        .then((data) => {
            if (data.success) {
                showToast("Now Playing", data.data?.message || name, "success");
            } else {
                showToast("Playback Failed", data.error || "Could not play station", "error");
            }
        })
        .catch(() => showToast("Playback Failed", "Could not reach device", "error"));
}

function tuneInShowDevicePicker(location, name, type, art) {
    tuneInPendingPlay = { location, name, type, art };
    const list = document.getElementById("devicePickerList");
    list.innerHTML = Object.entries(devices)
        .map(
            ([id, dev]) =>
                `<button class="btn btn-outline-secondary w-100 text-start mb-1" data-device-id="${escapeHtml(id)}" onclick="tuneInPlayOnDevice('${escapeHtml(id)}')"><i class="bi bi-speaker me-2"></i>${escapeHtml(dev.info?.Name || id)}</button>`,
        )
        .join("");
    new bootstrap.Modal(document.getElementById("devicePickerModal")).show();
}

function tuneInPlayOnDevice(deviceId) {
    if (!tuneInPendingPlay) return;
    const { location, name, type, art } = tuneInPendingPlay;
    tuneInPendingPlay = null;
    bootstrap.Modal.getInstance(document.getElementById("devicePickerModal")).hide();
    tuneInPlay(deviceId, location, name, type, art);
}

function escapeHtml(s) {
    return String(s)
        .replace(/&/g, "&amp;")
        .replace(/</g, "&lt;")
        .replace(/>/g, "&gt;")
        .replace(/"/g, "&quot;");
}

// WebSocket connection management
function connectWebSocket() {
    const protocol = location.protocol === "https:" ? "wss:" : "ws:";
    const wsUrl = `${protocol}//${location.host}/ws`;

    ws = new WebSocket(wsUrl);

    ws.onopen = function () {
        console.log("WebSocket connected");
        reconnectAttempts = 0;
        clearInterval(reconnectInterval);
        hideConnectionError();
    };

    ws.onmessage = function (event) {
        const data = JSON.parse(event.data);
        handleWebSocketMessage(data);
    };

    ws.onclose = function () {
        console.log("WebSocket disconnected");
        if (reconnectAttempts < maxReconnectAttempts) {
            showConnectionError();
            scheduleReconnect();
        } else {
            showMaxReconnectError();
        }
    };

    ws.onerror = function (error) {
        console.error("WebSocket error:", error);
    };
}

function scheduleReconnect() {
    reconnectAttempts++;
    // Exponential backoff: 1s, 2s, 4s, 8s, 16s, max 30s
    const delay = Math.min(1000 * Math.pow(2, reconnectAttempts - 1), 30000);
    console.log(
        `Reconnect attempt ${reconnectAttempts}/${maxReconnectAttempts} in ${delay}ms`,
    );

    clearInterval(reconnectInterval);
    reconnectInterval = setTimeout(connectWebSocket, delay);
}

function handleWebSocketMessage(data) {
    switch (data.type) {
        case "devices":
            devices = data.data;
            renderDeviceList();
            break;
        case "status_update":
            if (devices[data.deviceId]) {
                devices[data.deviceId].status = data.data;
                if (currentDeviceId === data.deviceId) {
                    updateDeviceStatus(data.data);
                } else {
                    renderDeviceList();
                }
            }
            break;
        case "discovery_status":
            handleDiscoveryStatus(data.data);
            break;
    }
}

// Device management functions
function loadDevices() {
    document.getElementById("devices-loading").style.display = "block";
    document.getElementById("devices-list").style.display = "none";
    document.getElementById("no-devices").style.display = "none";

    fetch("/api/devices")
        .then((response) => response.json())
        .then((data) => {
            if (data.success) {
                devices = data.data;
                // Fetch power status for each device after getting device list
                fetchPowerStatesForAllDevices();
                renderDeviceList();
            } else {
                showToast(
                    "Error",
                    data.error || "Failed to load devices",
                    "error",
                );
                showNoDevices();
            }
        })
        .catch((error) => {
            console.error("Error loading devices:", error);
            showToast("Error", "Failed to load devices", "error");
            showNoDevices();
        });
}

function fetchPowerStatesForAllDevices() {
    Object.keys(devices).forEach((deviceId) => {
        fetchDevicePowerStatus(deviceId);
    });
}

function fetchDevicePowerStatus(deviceId) {
    fetch(`/api/device-power-status/${deviceId}`)
        .then((response) => response.json())
        .then((data) => {
            if (data.success && devices[deviceId]) {
                // Update device status with power information
                if (!devices[deviceId].status) {
                    devices[deviceId].status = {};
                }
                if (!devices[deviceId].status.nowPlaying) {
                    devices[deviceId].status.nowPlaying = {};
                }
                devices[deviceId].status.nowPlaying.source = data.data.source;
                devices[deviceId].status.isConnected = true;

                // Re-render device list with updated power status
                renderDeviceList();
            }
        })
        .catch((error) => {
            console.log(`Failed to get power status for ${deviceId}:`, error);
        });
}

function renderDeviceList() {
    const devicesContainer = document.getElementById("devices-list");
    const loadingSpinner = document.getElementById("devices-loading");
    const noDevicesMsg = document.getElementById("no-devices");

    loadingSpinner.style.display = "none";

    if (!devices || Object.keys(devices).length === 0) {
        showNoDevices();
        return;
    }

    noDevicesMsg.style.display = "none";
    devicesContainer.style.display = "block";

    let html = "";
    for (const [deviceId, device] of Object.entries(devices)) {
        const isConnected = device.status?.isConnected || false;
        const nowPlaying = device.status?.nowPlaying;
        const isPoweredOn =
            isConnected && nowPlaying && nowPlaying.source !== "STANDBY";

        const statusClass = isConnected
            ? isPoweredOn
                ? "connected"
                : "standby"
            : "disconnected";
        const statusText = isConnected
            ? isPoweredOn
                ? "On"
                : "Standby"
            : "Disconnected";
        const powerIcon = isPoweredOn
            ? "bi-power text-success"
            : "bi-power text-muted";

        const lastSeen = device.lastSeen
            ? formatTimeAgo(new Date(device.lastSeen))
            : "Never";

        html += `
            <div class="col-md-6 col-lg-4 mb-4">
                <div class="card device-card ${statusClass}">
                    <div class="card-body">
                        <h5 class="card-title d-flex align-items-center justify-content-between">
                            <span>
                                <span class="status-indicator status-${isConnected ? "connected" : "disconnected"}"></span>
                                ${device.info?.Name || "Unknown Device"}
                            </span>
                            <i class="bi ${powerIcon}" title="Power Status"></i>
                        </h5>
                        <p class="card-text">
                            <strong>Type:</strong> ${device.info?.Type || "Unknown"}<br>
                            <strong>Status:</strong> ${statusText}<br>
                            <strong>Last Seen:</strong> ${lastSeen}
                        </p>
                        <div class="d-flex gap-2">
                            <button class="btn btn-primary flex-grow-1" onclick="showDevice('${deviceId}')">
                                <i class="bi bi-gear"></i>
                                Control
                            </button>
                            <button class="btn ${isPoweredOn ? "btn-outline-danger" : "btn-success"} power-btn" onclick="toggleDevicePowerFromCard('${deviceId}', this)" title="${isPoweredOn ? "Power Off" : "Power On"}">
                                <i class="bi bi-power"></i>
                            </button>
                        </div>
                    </div>
                </div>
            </div>
        `;
    }

    devicesContainer.innerHTML = html;
}

function showNoDevices() {
    document.getElementById("devices-loading").style.display = "none";
    document.getElementById("devices-list").style.display = "none";
    document.getElementById("no-devices").style.display = "block";
}

function showDevice(deviceId) {
    currentDeviceId = deviceId;
    const device = devices[deviceId];

    if (!device) {
        showToast("Error", "Device not found", "error");
        return;
    }

    renderDeviceControl(deviceId, device);
    showPage("device");
}

function renderDeviceControl(deviceId, device) {
    const container = document.getElementById("device-content");
    const info = device.info || {};
    const status = device.status || {};
    const nowPlaying = status.nowPlaying || {};
    const volume = status.volume || {};
    const bass = status.bass || {};

    const isPoweredOn =
        status.isConnected && nowPlaying && nowPlaying.Source !== "STANDBY";
    const powerButtonClass = isPoweredOn ? "btn-outline-danger" : "btn-success";
    const powerButtonTitle = isPoweredOn ? "Power Off" : "Power On";

    const html = `
        <div class="d-flex justify-content-between align-items-center mb-3">
            <div>
                <h2 class="mb-1">${info.Name || "Unknown Device"}</h2>
                <p class="text-muted mb-0">${info.Type || "Unknown Type"}</p>
            </div>
            <button class="btn ${powerButtonClass} btn-sm device-header-power" onclick="toggleDevicePower('${deviceId}')" title="${powerButtonTitle}">
                <i class="bi bi-power"></i>
            </button>
        </div>

        <!-- Now Playing Section -->
        <div class="card now-playing mb-4">
            <div class="card-body">
                <div class="row align-items-center">
                    <div class="col-auto">
                        <div class="artwork bg-secondary d-flex align-items-center justify-content-center">
                            <i class="bi bi-music-note-beamed text-white fs-2"></i>
                        </div>
                    </div>
                    <div class="col">
                        <h6 class="mb-1" id="track-title">${nowPlaying.Track || nowPlaying.track || "No track playing"}</h6>
                        <p class="mb-1" id="track-artist">${nowPlaying.Artist || nowPlaying.artist || "Unknown artist"}</p>
                        <small id="track-album">${nowPlaying.Album || nowPlaying.album || "Unknown album"}</small>
                    </div>
                </div>
            </div>
        </div>

        <!-- Transport Controls -->
        <div class="control-panel">
            <h5>Transport Controls</h5>
            <div class="d-flex justify-content-center gap-3 mb-3">
                <button class="btn btn-outline-primary" onclick="controlDevice('${deviceId}', 'previous')">
                    <i class="bi bi-skip-backward"></i>
                </button>
                <button class="btn btn-primary btn-lg" onclick="controlDevice('${deviceId}', 'play')">
                    <i class="bi bi-play"></i>
                </button>
                <button class="btn btn-outline-primary" onclick="controlDevice('${deviceId}', 'pause')">
                    <i class="bi bi-pause"></i>
                </button>
                <button class="btn btn-outline-primary" onclick="controlDevice('${deviceId}', 'stop')">
                    <i class="bi bi-stop"></i>
                </button>
                <button class="btn btn-outline-primary" onclick="controlDevice('${deviceId}', 'next')">
                    <i class="bi bi-skip-forward"></i>
                </button>
            </div>
        </div>

        <!-- Volume Control -->
        <div class="control-panel">
            <h5>Volume Control</h5>
            <div class="row align-items-center">
                <div class="col">
                    <input type="range" class="form-range volume-slider"
                           min="0" max="100" value="${volume.ActualVolume || volume.TargetVolume || 50}"
                           id="volume-slider-${deviceId.replace(/\./g, "-")}"
                           oninput="updateVolume('${deviceId}', this.value)">
                </div>
                <div class="col-auto">
                    <button class="btn btn-outline-secondary" onclick="controlDevice('${deviceId}', 'mute')">
                        <i class="bi bi-volume-mute"></i>
                    </button>
                </div>
            </div>
            <div class="text-center mt-2">
                <span id="volume-display">${volume.ActualVolume || volume.TargetVolume || 50}%</span>
            </div>
        </div>

        <!-- Bass Control -->
        <div class="control-panel">
            <h5>Bass Control</h5>
            <div class="row align-items-center">
                <div class="col">
                    <input type="range" class="form-range bass-slider"
                           min="-9" max="9" value="${bass.ActualBass || bass.TargetBass || 0}"
                           id="bass-slider-${deviceId.replace(/\./g, "-")}"
                           oninput="updateBass('${deviceId}', this.value)">
                </div>
            </div>
            <div class="text-center mt-2">
                <span id="bass-display">${bass.ActualBass || bass.TargetBass || 0}</span>
            </div>
        </div>

        <!-- Presets -->
        <div class="control-panel">
            <h5>Presets</h5>
            <div class="preset-grid" id="presets-grid">
                ${[1, 2, 3, 4, 5, 6]
                    .map(
                        (i) => `
                    <button class="preset-btn" onclick="controlDevice('${deviceId}', 'preset', null, '?id=${i}')">
                        <i class="bi bi-bookmark"></i>
                        <span>Preset ${i}</span>
                    </button>
                `,
                    )
                    .join("")}
            </div>
        </div>

        <!-- Sources -->
        <div class="control-panel">
            <h5>Sources</h5>
            <div class="source-grid">
                <button class="source-btn" onclick="controlDevice('${deviceId}', 'source', null, '?name=SPOTIFY')">
                    <i class="bi bi-spotify"></i>
                    <span>Spotify</span>
                </button>
                <button class="source-btn" onclick="controlDevice('${deviceId}', 'source', null, '?name=BLUETOOTH')">
                    <i class="bi bi-bluetooth"></i>
                    <span>Bluetooth</span>
                </button>
                <button class="source-btn" onclick="controlDevice('${deviceId}', 'source', null, '?name=AUX')">
                    <i class="bi bi-input-cursor"></i>
                    <span>AUX</span>
                </button>
                <button class="source-btn" onclick="controlDevice('${deviceId}', 'source', null, '?name=INTERNET_RADIO')">
                    <i class="bi bi-radio"></i>
                    <span>Radio</span>
                </button>
            </div>
        </div>
    `;

    container.innerHTML = html;
}

function updateDeviceStatus(status) {
    // Update volume display if we have volume info
    if (status.volume !== undefined) {
        const volumeSlider = document.getElementById("volume-slider");
        const volumeDisplay = document.getElementById("volume-display");
        if (volumeSlider && volumeDisplay) {
            volumeSlider.value = status.volume;
            volumeDisplay.textContent = status.volume + "%";
        }
    }

    // Update bass display if we have bass info
    if (status.bass !== undefined) {
        const bassSlider = document.getElementById("bass-slider");
        const bassDisplay = document.getElementById("bass-display");
        if (bassSlider && bassDisplay) {
            bassSlider.value = status.bass;
            bassDisplay.textContent = status.bass;
        }
    }

    // Update now playing info
    if (status.nowPlaying) {
        const np = status.nowPlaying;
        const titleEl = document.getElementById("track-title");
        const artistEl = document.getElementById("track-artist");
        const albumEl = document.getElementById("track-album");

        if (titleEl) titleEl.textContent = np.track || "No track playing";
        if (artistEl) artistEl.textContent = np.artist || "Unknown artist";
        if (albumEl) albumEl.textContent = np.album || "Unknown album";
    }
}

// Device control functions
function updateVolume(deviceId, level) {
    document.getElementById("volume-display").textContent = level + "%";
    // Debounce the actual API call
    clearTimeout(window.volumeTimeout);
    window.volumeTimeout = setTimeout(() => {
        setDeviceVolume(deviceId, parseInt(level));
    }, 300);
}

function updateBass(deviceId, level) {
    document.getElementById("bass-display").textContent = level;
    // Debounce the actual API call
    clearTimeout(window.bassTimeout);
    window.bassTimeout = setTimeout(() => {
        controlDevice(deviceId, "bass", { level: parseInt(level) });
    }, 300);
}

// Enhanced device control functions
function toggleDevicePower(deviceId) {
    const url = `/api/device-power/${deviceId}`;

    // Show loading state on button
    const powerBtn = event.target.closest("button");
    const originalContent = powerBtn.innerHTML;
    powerBtn.disabled = true;
    powerBtn.classList.add("power-loading");
    powerBtn.innerHTML = '<i class="bi bi-hourglass-split"></i>';

    fetch(url, { method: "POST" })
        .then((response) => response.json())
        .then((result) => {
            if (result.success) {
                showToast("Success", result.data.message, "success");
                // Trigger device power status refresh after power command
                setTimeout(() => {
                    fetchDevicePowerStatus(deviceId);
                }, 2000);
            } else {
                showToast(
                    "Error",
                    result.error || "Power toggle failed",
                    "error",
                );
            }
        })
        .catch((error) => {
            console.error("Power toggle error:", error);
            showToast("Error", "Failed to toggle power", "error");
        })
        .finally(() => {
            // Restore button state
            powerBtn.disabled = false;
            powerBtn.classList.remove("power-loading");
            powerBtn.innerHTML = originalContent;
        });
}

function toggleDevicePowerFromCard(deviceId, buttonElement) {
    const url = `/api/device-power/${deviceId}`;

    // Show loading state on button
    const originalContent = buttonElement.innerHTML;
    buttonElement.disabled = true;
    buttonElement.classList.add("power-loading");
    buttonElement.innerHTML = '<i class="bi bi-hourglass-split"></i>';

    fetch(url, { method: "POST" })
        .then((response) => response.json())
        .then((result) => {
            if (result.success) {
                showToast("Success", result.data.message, "success");
                // Trigger device power status refresh after power command
                setTimeout(() => {
                    fetchDevicePowerStatus(deviceId);
                }, 2000);
            } else {
                showToast(
                    "Error",
                    result.error || "Power toggle failed",
                    "error",
                );
            }
        })
        .catch((error) => {
            console.error("Power toggle error:", error);
            showToast("Error", "Failed to toggle power", "error");
        })
        .finally(() => {
            // Restore button state
            buttonElement.disabled = false;
            buttonElement.classList.remove("power-loading");
            buttonElement.innerHTML = originalContent;
        });
}

function sendDeviceKey(deviceId, key) {
    const url = `/api/device-key/${deviceId}/${key}`;

    fetch(url, { method: "POST" })
        .then((response) => response.json())
        .then((result) => {
            if (result.success) {
                showToast("Success", result.data.message, "success");
            } else {
                showToast(
                    "Error",
                    result.error || "Key command failed",
                    "error",
                );
            }
        })
        .catch((error) => {
            console.error("Key command error:", error);
            showToast("Error", "Failed to send key command", "error");
        });
}

function setDeviceVolume(deviceId, level) {
    const url = `/api/device-volume/${deviceId}/${level}`;

    fetch(url, { method: "POST" })
        .then((response) => response.json())
        .then((result) => {
            if (result.success) {
                showToast("Success", result.data.message, "success");
            } else {
                showToast(
                    "Error",
                    result.error || "Volume command failed",
                    "error",
                );
            }
        })
        .catch((error) => {
            console.error("Volume command error:", error);
            showToast("Error", "Failed to set volume", "error");
        });
}

function controlDevice(deviceId, action, data = null, queryParams = "") {
    let url = `/api/control/${deviceId}/${action}${queryParams}`;
    let options = {
        method: action === "volume" || action === "bass" ? "POST" : "GET",
    };

    if (data) {
        options.headers = {
            "Content-Type": "application/json",
        };
        options.body = JSON.stringify(data);
    }

    fetch(url, options)
        .then((response) => response.json())
        .then((result) => {
            if (result.success) {
                showToast("Success", result.data.message, "success");
            } else {
                showToast(
                    "Error",
                    result.error || "Control command failed",
                    "error",
                );
            }
        })
        .catch((error) => {
            console.error("Control error:", error);
            showToast("Error", "Failed to send control command", "error");
        });
}

function discoverDevices() {
    fetch("/api/discover", {
        method: "POST",
    })
        .then((response) => response.json())
        .then((data) => {
            if (data.success) {
                showToast(
                    "Discovery Started",
                    "Searching for SoundTouch devices...",
                    "info",
                );
                // Real-time updates will come via WebSocket, no need for setTimeout
            } else {
                showToast(
                    "Discovery Failed",
                    data.error || "Failed to start discovery",
                    "error",
                );
            }
        })
        .catch((error) => {
            console.error("Discovery error:", error);
            showToast(
                "Discovery Error",
                "Failed to start device discovery",
                "error",
            );
        });
}

function handleDiscoveryStatus(data) {
    const status = data.status;
    const deviceCount = data.deviceCount;

    switch (status) {
        case "starting":
            showToast(
                "Discovery Starting",
                "Searching for SoundTouch devices...",
                "info",
            );
            break;
        case "completed":
            const message =
                deviceCount === 0
                    ? "No devices found"
                    : `Discovery completed - found ${deviceCount} device${deviceCount === 1 ? "" : "s"}`;
            showToast(
                "Discovery Completed",
                message,
                deviceCount === 0 ? "warning" : "success",
            );
            break;
        case "failed":
            showToast(
                "Discovery Failed",
                "Failed to discover devices",
                "error",
            );
            break;
    }
}

// Utility functions
function formatTimeAgo(date) {
    const now = new Date();
    const diff = now - date;
    const minutes = Math.floor(diff / 60000);
    const hours = Math.floor(diff / 3600000);
    const days = Math.floor(diff / 86400000);

    if (days > 0) return `${days}d ago`;
    if (hours > 0) return `${hours}h ago`;
    if (minutes > 0) return `${minutes}m ago`;
    return "Just now";
}

function showToast(title, message, type = "info") {
    const toastContainer = document.querySelector(".toast-container");
    const toastId = "toast-" + Date.now();

    const toastHTML = `
        <div class="toast" id="${toastId}" role="alert" aria-live="assertive" aria-atomic="true">
            <div class="toast-header">
                <i class="bi bi-${getToastIcon(type)} me-2"></i>
                <strong class="me-auto">${title}</strong>
                <button type="button" class="btn-close" data-bs-dismiss="toast"></button>
            </div>
            <div class="toast-body">${message}</div>
        </div>
    `;

    toastContainer.insertAdjacentHTML("beforeend", toastHTML);
    const toast = new bootstrap.Toast(document.getElementById(toastId));
    toast.show();

    document
        .getElementById(toastId)
        .addEventListener("hidden.bs.toast", function () {
            this.remove();
        });
}

function getToastIcon(type) {
    switch (type) {
        case "success":
            return "check-circle-fill text-success";
        case "error":
            return "exclamation-triangle-fill text-danger";
        case "warning":
            return "exclamation-triangle-fill text-warning";
        default:
            return "info-circle-fill text-info";
    }
}

function showConnectionError() {
    // Check if connection error is already shown to avoid spam
    const existingError = document.getElementById("connection-error");
    if (existingError) {
        // Update existing error message with attempt count
        const messageDiv = existingError.querySelector("div");
        messageDiv.textContent = `Connection lost. Reconnecting... (${reconnectAttempts}/${maxReconnectAttempts})`;
        return;
    }

    // Create a persistent notification instead of toast
    const errorDiv = document.createElement("div");
    errorDiv.id = "connection-error";
    errorDiv.className =
        "alert alert-warning d-flex align-items-center position-fixed top-0 start-50 translate-middle-x mt-3";
    errorDiv.style.zIndex = "9999";
    errorDiv.innerHTML = `
        <i class="bi bi-exclamation-triangle-fill me-2"></i>
        <div>Connection lost. Reconnecting... (${reconnectAttempts}/${maxReconnectAttempts})</div>
        <button type="button" class="btn-close ms-auto" onclick="this.parentElement.remove()"></button>
    `;

    document.body.appendChild(errorDiv);
}

function showMaxReconnectError() {
    const existingError = document.getElementById("connection-error");
    if (existingError) {
        existingError.remove();
    }

    const errorDiv = document.createElement("div");
    errorDiv.id = "connection-error";
    errorDiv.className =
        "alert alert-danger d-flex align-items-center position-fixed top-0 start-50 translate-middle-x mt-3";
    errorDiv.style.zIndex = "9999";
    errorDiv.innerHTML = `
        <i class="bi bi-exclamation-triangle-fill me-2"></i>
        <div>Connection failed. Please refresh the page to retry.</div>
        <button type="button" class="btn btn-outline-light btn-sm ms-3" onclick="window.location.reload()">Refresh</button>
        <button type="button" class="btn-close ms-2" onclick="this.parentElement.remove()"></button>
    `;

    document.body.appendChild(errorDiv);
}

function hideConnectionError() {
    const existingError = document.getElementById("connection-error");
    if (existingError) {
        existingError.remove();
    }
}

// Theme management functions
function initializeTheme() {
    // Check for saved theme preference or default to 'auto'
    const savedTheme = localStorage.getItem("theme") || "auto";
    applyTheme(savedTheme);
    updateThemeIcon(savedTheme);
}

function toggleTheme() {
    const themeToggle = document.querySelector(".theme-toggle");

    // Add animation class
    themeToggle.classList.add("switching");

    const currentTheme = document.documentElement.getAttribute("data-theme");
    const systemPrefersDark = window.matchMedia(
        "(prefers-color-scheme: dark)",
    ).matches;

    let newTheme;
    if (currentTheme === null || currentTheme === "auto") {
        // If auto or no theme set, switch to opposite of system preference
        newTheme = systemPrefersDark ? "light" : "dark";
    } else if (currentTheme === "light") {
        newTheme = "dark";
    } else {
        newTheme = "light";
    }

    // Apply theme changes after a short delay for animation
    setTimeout(() => {
        applyTheme(newTheme);
        updateThemeIcon(newTheme);
        localStorage.setItem("theme", newTheme);

        // Remove animation class after animation completes
        setTimeout(() => {
            themeToggle.classList.remove("switching");
        }, 100);
    }, 200);
}

function applyTheme(theme) {
    if (theme === "auto") {
        document.documentElement.removeAttribute("data-theme");
    } else {
        document.documentElement.setAttribute("data-theme", theme);
    }
}

function updateThemeIcon(theme) {
    const themeIcon = document.getElementById("theme-icon");
    const systemPrefersDark = window.matchMedia(
        "(prefers-color-scheme: dark)",
    ).matches;

    if (theme === "dark" || (theme === "auto" && systemPrefersDark)) {
        themeIcon.className = "bi bi-sun";
    } else {
        themeIcon.className = "bi bi-moon";
    }
}

// Listen for system theme changes
window
    .matchMedia("(prefers-color-scheme: dark)")
    .addEventListener("change", (e) => {
        const currentTheme = localStorage.getItem("theme") || "auto";
        if (currentTheme === "auto") {
            updateThemeIcon("auto");
        }
    });

// Initialize application
document.addEventListener("DOMContentLoaded", function () {
    initializeTheme();
    connectWebSocket();
    loadDevices();

    // TuneIn: keyboard search
    document
        .getElementById("tunein-search-input")
        .addEventListener("keydown", function (e) {
            if (e.key === "Enter") tuneInSearch(this.value);
        });

    // TuneIn: event delegation — play buttons take priority over navigation
    document
        .getElementById("tunein-results")
        .addEventListener("click", function (e) {
            const playBtn = e.target.closest(".tunein-play-btn");
            if (playBtn) {
                e.preventDefault();
                e.stopPropagation();
                tuneInPlayClick(
                    playBtn.dataset.playLocation,
                    playBtn.dataset.playName,
                    playBtn.dataset.playType || "stationurl",
                    playBtn.dataset.playArt || "",
                );
                return;
            }
            const item = e.target.closest("[data-nav-path]");
            if (item) {
                e.preventDefault();
                tuneInNavigate(item.dataset.navPath, item.dataset.navLabel || "");
            }
        });

    // TuneIn: keyboard activation for navigable items
    document
        .getElementById("tunein-results")
        .addEventListener("keydown", function (e) {
            if (e.key === "Enter" || e.key === " ") {
                const item = e.target.closest("[data-nav-path]");
                if (item) {
                    e.preventDefault();
                    tuneInNavigate(item.dataset.navPath, item.dataset.navLabel || "");
                }
            }
        });
});
