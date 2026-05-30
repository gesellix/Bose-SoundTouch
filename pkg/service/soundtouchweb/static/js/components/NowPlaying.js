import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

function fmt(secs) {
    if (!secs || secs <= 0) return '0:00';
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
}

// PresetPicker — ★ star button in the top-right corner of the now-playing card.
// Translucent when nothing is mapped; golden when the current content already
// exists in one of the device's presets.  Click to open a slot picker (1–6).
function PresetPicker({ deviceId, nowPlaying, presets }) {
    const [open, setOpen] = useState(false);
    const [savingSlot, setSavingSlot] = useState(null);
    const [savedSlot, setSavedSlot] = useState(null);
    const wrapRef = useRef(null);

    // Detect whether the current content is already saved as any preset.
    const currentSource   = nowPlaying?.ContentItem?.Source;
    const currentLocation = nowPlaying?.ContentItem?.Location;
    const presetList = presets?.Preset ?? [];
    const isMapped = !!(currentLocation && presetList.some(p =>
        p.ContentItem?.Location === currentLocation &&
        p.ContentItem?.Source   === currentSource
    ));

    // Close popover on outside click.
    useEffect(() => {
        if (!open) return;
        function onDocClick(e) {
            if (!wrapRef.current?.contains(e.target)) setOpen(false);
        }
        document.addEventListener('click', onDocClick, true);
        return () => document.removeEventListener('click', onDocClick, true);
    }, [open]);

    function save(slotId) {
        setSavingSlot(slotId);
        api.storePreset(deviceId, slotId)
            .then(res => {
                setSavingSlot(null);
                setSavedSlot(slotId);
                setTimeout(() => {
                    setSavedSlot(null);
                    setOpen(false);
                }, 900);
            })
            .catch(() => {
                setSavingSlot(null);
                setOpen(false);
            });
    }

    return html`
        <div class="now-playing-fav-wrap" ref=${wrapRef}>
            <button
                class="now-playing-fav-btn ${isMapped ? 'mapped' : ''} ${open ? 'open' : ''}"
                onClick=${() => setOpen(o => !o)}
                title=${isMapped ? 'Saved as preset — save again to update' : 'Save as preset'}
                aria-label="Save as preset"
            >★</button>
            ${open && html`
                <div class="now-playing-fav-overlay">
                    <div class="preset-picker-label">Save as preset</div>
                    <div class="preset-picker-slots">
                        ${[1, 2, 3, 4, 5, 6].map(n => html`
                            <button
                                key=${n}
                                class="preset-picker-slot ${savingSlot === n ? 'saving' : savedSlot === n ? 'saved' : ''}"
                                onClick=${() => save(n)}
                                disabled=${savingSlot !== null}
                            >${n}</button>
                        `)}
                    </div>
                </div>
            `}
        </div>
    `;
}

export function NowPlaying({ nowPlaying, deviceId, presets }) {
    const [position, setPosition] = useState(0);

    useEffect(() => {
        const pos = nowPlaying?.Time?.Position ?? 0;
        setPosition(pos);
        if (nowPlaying?.PlayStatus !== 'PLAY_STATE') return;
        const id = setInterval(() => setPosition(p => p + 1), 1000);
        return () => clearInterval(id);
    }, [nowPlaying?.Time?.Position, nowPlaying?.PlayStatus,
        nowPlaying?.TrackID, nowPlaying?.ContentItem?.Location]);

    if (!nowPlaying || nowPlaying.Source === 'STANDBY') {
        return html`<div class="now-playing standby">Standby</div>`;
    }

    const title = nowPlaying.Track || nowPlaying.StationName || nowPlaying.Source;
    const artURL = nowPlaying.Art?.URL;
    const isBuffering = nowPlaying.PlayStatus === 'BUFFERING_STATE';
    const total = nowPlaying.Time?.Total ?? 0;
    const pct = total > 0 ? Math.min(100, (position / total) * 100) : 0;

    return html`
        <div class="now-playing">
            ${artURL && html`<img class="album-art" src=${artURL} alt="" />`}
            <div class="track-info">
                <div class="track-title">${title}</div>
                ${nowPlaying.Artist && html`<div class="track-artist">${nowPlaying.Artist}</div>`}
                ${nowPlaying.Album && html`<div class="track-album">${nowPlaying.Album}</div>`}
                <div class="track-meta">
                    <span class="track-source">${nowPlaying.Source}</span>
                    ${isBuffering && html`<span class="buffering-badge">Buffering…</span>`}
                </div>
                ${total > 0 && html`
                    <div class="progress-row">
                        <div class="progress-bar">
                            <div class="progress-fill" style="width:${pct}%"></div>
                        </div>
                        <span class="progress-time">${fmt(Math.min(position, total))} / ${fmt(total)}</span>
                    </div>
                `}
            </div>
            ${deviceId && html`
                <${PresetPicker}
                    deviceId=${deviceId}
                    nowPlaying=${nowPlaying}
                    presets=${presets}
                />
            `}
        </div>
    `;
}
