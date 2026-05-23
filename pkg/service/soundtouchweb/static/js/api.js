const JSON_HEADERS = { 'Content-Type': 'application/json' };

async function req(url, opts = {}) {
    const r = await fetch(url, opts);
    return r.json();
}

export const api = {
    devices: () => req('/api/devices'),
    device: (id) => req(`/api/device/${id}`),
    discover: () => req('/api/discover', { method: 'POST' }),
    key: (id, key) => req(`/api/device-key/${id}/${key}`, { method: 'POST' }),
    volume: (id, level) => req(`/api/device-volume/${id}/${level}`, { method: 'POST' }),
    bass: (id, level) => req(`/api/control/${id}/bass`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ level }),
    }),
    power: (id) => req(`/api/device-power/${id}`, { method: 'POST' }),
    recents: (id) => req(`/api/device-recents/${id}`),
    zone: (id) => req(`/api/zone/${id}`),
    zoneAdd: (masterId, slaveId) => req(`/api/zone/${masterId}/add/${slaveId}`, { method: 'POST' }),
    zoneRemove: (masterId, slaveId) => req(`/api/zone/${masterId}/remove/${slaveId}`, { method: 'POST' }),
    zoneDissolve: (id) => req(`/api/zone/${id}/dissolve`, { method: 'POST' }),
    zoneLeave: (id) => req(`/api/zone/${id}/leave`, { method: 'POST' }),
    play: (id, item) => req(`/api/device-play/${id}`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(item),
    }),
    tuneInBrowse: (path) => req(path ? `/api/tunein/navigate/${path}` : '/api/tunein/navigate'),
    tuneInSearch: (q) => req(`/api/tunein/search?q=${encodeURIComponent(q)}`),
    tuneInSearchNext: (cursor) => req(`/api/tunein/search/next?cursor=${encodeURIComponent(cursor)}`),
    control: (id, action, presetId) => req(`/api/control/${id}/${action}?id=${presetId}`),
    selectSource: (id, source, account) => req(`/api/control/${id}/source?name=${encodeURIComponent(source)}&account=${encodeURIComponent(account || '')}`),
    tuneInPlay: (deviceId, item) => req(`/api/tunein/play/${deviceId}`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(item),
    }),
    radioBrowserSearch: (q) => req(`/api/radiobrowser/search?q=${encodeURIComponent(q)}`),
    radioBrowserPlay: (deviceId, item) => req(`/api/radiobrowser/play/${deviceId}`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(item),
    }),
};
