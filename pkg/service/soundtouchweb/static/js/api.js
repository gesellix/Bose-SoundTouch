const JSON_HEADERS = { 'Content-Type': 'application/json' };

async function req(url, opts = {}) {
    const r = await fetch(url, opts);
    return r.json();
}

// checkedReq turns a failed request into a thrown Error, tagging whether the
// failure is DEFINITIVE, meaning proof the command never reached the speaker.
//
// Only 4xx qualifies. Every 4xx on the control endpoints is produced before
// any speaker call (missing/unknown device, unparseable body, empty source,
// unknown action), so nothing was sent onward. A 5xx is NOT proof of anything:
// handleSourceControl reports a failed Client.SelectSource through
// sendControlResponse, which maps any speaker-call error to 500, and a request
// that timed out after the speaker already switched looks exactly like one it
// never received. Transport errors are ambiguous for the same reason.
//
// Callers that verify by readback must keep verifying unless the failure is
// definitive.
async function checkedReq(url, opts = {}) {
    const r = await fetch(url, opts);
    const definitive = r.status >= 400 && r.status < 500;
    let response;
    try {
        response = await r.json();
    } catch (_) {
        throw Object.assign(new Error(`Request failed (${r.status})`), { definitive });
    }
    if (!r.ok || response?.success === false) {
        throw Object.assign(
            new Error(response?.error || `Request failed (${r.status})`),
            { definitive },
        );
    }
    return response;
}

export const api = {
    devices: () => req('/api/control/devices'),
    device: (id) => req(`/api/control/devices/${id}`),
    // Refreshes only /now_playing. Used by the source-selection readback,
    // which would otherwise poll every field to answer one question.
    deviceNowPlaying: (id) => req(`/api/control/devices/${id}/now-playing`),
    removeDevice: (id) => req(`/api/control/devices/${id}`, { method: 'DELETE' }),
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
    zoneCandidates: (id) => req(`/api/control/devices/${id}/zone/candidates`),
    zoneAdd: (masterId, slaveId) => req(`/api/control/devices/${masterId}/zone/add/${slaveId}`, { method: 'POST' }),
    zoneRemove: (masterId, slaveId) => req(`/api/control/devices/${masterId}/zone/remove/${slaveId}`, { method: 'POST' }),
    zoneDissolve: (id) => req(`/api/control/devices/${id}/zone/dissolve`, { method: 'POST' }),
    zoneLeave: (id) => req(`/api/control/devices/${id}/zone/leave`, { method: 'POST' }),
    stereoPair: (id) => req(`/api/control/devices/${id}/stereo-pair/`),
    stereoPairCreate: (leftId, rightId, name) => req(`/api/control/devices/${leftId}/stereo-pair/`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ rightId, name }),
    }),
    stereoPairRename: (id, groupId, name) => req(`/api/control/devices/${id}/stereo-pair/`, {
        method: 'PATCH',
        headers: JSON_HEADERS,
        body: JSON.stringify({ groupId, name }),
    }),
    stereoPairDissolve: (id, groupId, group) => req(`/api/control/devices/${id}/stereo-pair/`, {
        method: 'DELETE',
        headers: JSON_HEADERS,
        body: JSON.stringify({ groupId, group }),
    }),
    play: (id, item) => req(`/api/control/devices/${id}/play`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(item),
    }),
    tuneInBrowse: (path) => req(path ? `/api/control/providers/tunein/navigate/${path}` : '/api/control/providers/tunein/navigate'),
    tuneInSearch: (q) => req(`/api/control/providers/tunein/search?q=${encodeURIComponent(q)}`),
    tuneInSearchNext: (cursor) => req(`/api/control/providers/tunein/search/next?cursor=${encodeURIComponent(cursor)}`),
    control: (id, action, presetId) => req(`/api/control/devices/${id}/action/${action}?id=${presetId}`),
    storePreset: (id, slotId) => req(`/api/control/devices/${id}/action/storepreset?id=${slotId}`),
    selectSource: (id, source, account) => checkedReq(`/api/control/devices/${id}/action/source`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ source, account: account ?? '' }),
    }),
    tuneInPlay: (deviceId, item) => req(`/api/control/devices/${deviceId}/providers/tunein/play`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(item),
    }),
    radioBrowserSearch: (q) => req(`/api/control/providers/radiobrowser/search?q=${encodeURIComponent(q)}`),
    radioBrowserPlay: (deviceId, item) => req(`/api/control/devices/${deviceId}/providers/radiobrowser/play`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify(item),
    }),
    playURL: (deviceId, url, name, imageUrl, serviceUrl) => req(`/api/control/devices/${deviceId}/providers/url/play`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ url, name, imageUrl, serviceUrl }),
    }),
    speak: (deviceId, text) => req(`/api/control/devices/${deviceId}/providers/tts/play`, {
        method: 'POST',
        headers: JSON_HEADERS,
        body: JSON.stringify({ text }),
    }),
    libraryDiscover: (timeout) => req(`/api/control/providers/library/servers${timeout ? `?timeout=${timeout}` : ''}`),
    libraryServers: (id) => req(`/api/control/devices/${id}/library/servers`),
    libraryAddServer: (id, body) => req(`/api/control/devices/${id}/library/servers`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(body) }),
    libraryRemoveServer: (id, account) => req(`/api/control/devices/${id}/library/servers/${encodeURIComponent(account)}`, { method: 'DELETE' }),
    libraryBrowse: (id, { account, location, type, start, count }) => {
        const qs = [
            `account=${encodeURIComponent(account)}`,
            location !== undefined && location !== '' ? `location=${encodeURIComponent(location)}` : null,
            type ? `type=${encodeURIComponent(type)}` : null,
            start !== undefined ? `start=${encodeURIComponent(start)}` : null,
            count !== undefined ? `count=${encodeURIComponent(count)}` : null,
        ].filter(Boolean).join('&');
        return req(`/api/control/devices/${id}/library/browse?${qs}`);
    },
    libraryPlay: (id, body) => req(`/api/control/devices/${id}/library/play`, { method: 'POST', headers: JSON_HEADERS, body: JSON.stringify(body) }),
};
