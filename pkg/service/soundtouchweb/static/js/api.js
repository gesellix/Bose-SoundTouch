const JSON_HEADERS = { 'Content-Type': 'application/json' };

async function req(url, opts = {}) {
    const r = await fetch(url, opts);
    return r.json();
}

export const api = {
    devices: () => req('/api/control/devices'),
    device: (id) => req(`/api/control/devices/${id}`),
    discover: () => req('/api/control/discover', { method: 'POST' }),
    key: (id, key) => req(`/api/control/devices/${id}/key/${key}`, { method: 'POST' }),
    volume: (id, level) => req(`/api/control/devices/${id}/volume/${level}`, { method: 'POST' }),
    bass: (id, level) => req(`/api/control/devices/${id}/action/bass`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ level }),
    }),
    power: (id) => req(`/api/control/devices/${id}/power`, { method: 'POST' }),
    recents: (id) => req(`/api/control/devices/${id}/recents`),
    zone: (id) => req(`/api/control/devices/${id}/zone`),
    zoneAdd: (masterId, slaveId) => req(`/api/control/devices/${masterId}/zone/add/${slaveId}`, { method: 'POST' }),
    zoneRemove: (masterId, slaveId) => req(`/api/control/devices/${masterId}/zone/remove/${slaveId}`, { method: 'POST' }),
    zoneDissolve: (id) => req(`/api/control/devices/${id}/zone/dissolve`, { method: 'POST' }),
    zoneLeave: (id) => req(`/api/control/devices/${id}/zone/leave`, { method: 'POST' }),
    play: (id, item) => req(`/api/control/devices/${id}/play`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(item),
    }),
    tuneInBrowse: (path) => req(path ? `/api/control/tunein/navigate/${path}` : '/api/control/tunein/navigate'),
    tuneInSearch: (q) => req(`/api/control/tunein/search?q=${encodeURIComponent(q)}`),
    tuneInSearchNext: (cursor) => req(`/api/control/tunein/search/next?cursor=${encodeURIComponent(cursor)}`),
    control: (id, action, presetId) => req(`/api/control/devices/${id}/action/${action}?id=${presetId}`),
    storePreset: (id, slotId) => req(`/api/control/devices/${id}/action/storepreset?id=${slotId}`),
    selectSource: (id, source, account) => req(`/api/control/devices/${id}/action/source?name=${encodeURIComponent(source)}&account=${encodeURIComponent(account || '')}`),
    tuneInPlay: (deviceId, item) => req(`/api/control/devices/${deviceId}/tunein/play`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(item),
    }),
    radioBrowserSearch: (q) => req(`/api/control/radiobrowser/search?q=${encodeURIComponent(q)}`),
    radioBrowserPlay: (deviceId, item) => req(`/api/control/devices/${deviceId}/radiobrowser/play`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(item),
    }),
    playURL: (deviceId, url, name, imageUrl, serviceUrl) => req(`/api/control/devices/${deviceId}/play-url`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ url, name, imageUrl, serviceUrl }),
    }),
    speak: (deviceId, text) => req(`/api/control/devices/${deviceId}/speak`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ text }),
    }),
};
