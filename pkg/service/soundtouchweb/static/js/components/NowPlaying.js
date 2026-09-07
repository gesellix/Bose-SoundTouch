import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { SourceIcon } from '../sourceIcons.js';

const html = htm.bind(h);

function fmt(secs) {
    if (!secs || secs <= 0) return '0:00';
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
}

// PresetPicker — ★ star button in the top-right corner of the now-playing card.
// Translucent when nothing is mapped; golden when the current content already
// exists in one of the device's presets, in which case the tooltip names the
// slot.  Click to open a slot picker (1–6).
function PresetPicker({ deviceId, nowPlaying, presets }) {
    const [open, setOpen] = useState(false);
    const [savingSlot, setSavingSlot] = useState(null);
    const [savedSlot, setSavedSlot] = useState(null);
    const [errorSlot, setErrorSlot] = useState(null);
    const wrapRef = useRef(null);

    // Detect whether the current content is already saved as any preset.
    const currentSource   = nowPlaying?.ContentItem?.Source;
    const currentLocation = nowPlaying?.ContentItem?.Location;
    const presetList = presets?.Preset ?? [];
    // Keep the matching preset, not just a boolean, so the tooltip can name
    // the slot it landed in.
    const mappedPreset = currentLocation
        ? presetList.find(p =>
            p.ContentItem?.Location === currentLocation &&
            p.ContentItem?.Source   === currentSource)
        : undefined;
    const isMapped = !!mappedPreset;
    const favTitle = isMapped
        ? `Saved as preset ${mappedPreset.ID}. Save again to update.`
        : 'Save as preset';

    // Close popover on outside click.
    useEffect(() => {
        if (!open) return;
        function onDocClick(e) {
            if (!wrapRef.current?.contains(e.target)) setOpen(false);
        }
        document.addEventListener('click', onDocClick, true);
        return () => document.removeEventListener('click', onDocClick, true);
    }, [open]);

    // Mirrors the feedback the preset tiles give (see Presets.js): a failed
    // save used to close the overlay silently, which looked identical to a
    // successful one.
    function finish(setter, slotId) {
        setSavingSlot(null);
        setter(slotId);
        setTimeout(() => {
            setter(null);
            setOpen(false);
        }, 900);
    }

    function save(slotId) {
        setSavingSlot(slotId);
        api.storePreset(deviceId, slotId)
            .then(res => finish(res.success ? setSavedSlot : setErrorSlot, slotId))
            .catch(() => finish(setErrorSlot, slotId));
    }

    return html`
        <div class="now-playing-fav-wrap" ref=${wrapRef}>
            <button
                class="now-playing-fav-btn ${isMapped ? 'mapped' : ''} ${open ? 'open' : ''}"
                onClick=${() => setOpen(o => !o)}
                title=${favTitle}
                aria-label=${favTitle}
            >★</button>
            ${open && html`
                <div class="now-playing-fav-overlay">
                    <div class="preset-picker-label">Save as preset</div>
                    <div class="preset-picker-slots">
                        ${[1, 2, 3, 4, 5, 6].map(n => html`
                            <button
                                key=${n}
                                class="preset-picker-slot ${savingSlot === n ? 'saving' : savedSlot === n ? 'saved' : errorSlot === n ? 'error' : ''}"
                                onClick=${() => save(n)}
                                disabled=${savingSlot !== null}
                            >${savedSlot === n ? '✓' : errorSlot === n ? '✗' : n}</button>
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
    const longMetadata = [title, nowPlaying.Artist, nowPlaying.Album]
        .some(value => value && value.length > 80);
    const isRAOP = nowPlaying.Source === 'AIRPLAY' || nowPlaying.Source === 'RAOP';
    const showFullMetadata = isRAOP || longMetadata;
    // Same precedence as the model's GetArtworkURL (nowplaying.go): the <art>
    // element first, then the ContentItem's own artwork, which is where radio
    // stations carry their logo.
    const artURL = nowPlaying.Art?.URL || nowPlaying.ContentItem?.ContainerArt;
    const isBuffering = nowPlaying.PlayStatus === 'BUFFERING_STATE';
    const total = nowPlaying.Time?.Total ?? 0;
    const pct = total > 0 ? Math.min(100, (position / total) * 100) : 0;

    return html`
        <div class="now-playing">
            ${artURL
                ? html`<img class="album-art" src=${artURL} alt="" />`
                : html`<div class="album-art album-art-empty" data-source=${nowPlaying.Source ?? ''}>
                    <${SourceIcon} source=${nowPlaying.Source} className="now-playing-source-icon" />
                </div>`
            }
            <div class="track-info">
                <div class="track-title" title=${title}>${title}</div>
                ${nowPlaying.Artist && html`<div class="track-artist" title=${nowPlaying.Artist}>${nowPlaying.Artist}</div>`}
                ${nowPlaying.Album && html`<div class="track-album" title=${nowPlaying.Album}>${nowPlaying.Album}</div>`}
                <div class="track-meta">
                    <span class="track-source">${nowPlaying.Source}</span>
                    ${isBuffering && html`<span class="buffering-badge">Buffering…</span>`}
                </div>
                ${showFullMetadata && html`
                    <details class="track-details">
                        <summary>Full details</summary>
                        <dl>
                            <dt>Title</dt><dd>${title}</dd>
                            ${nowPlaying.Artist && html`<dt>Artist</dt><dd>${nowPlaying.Artist}</dd>`}
                            ${nowPlaying.Album && html`<dt>Album</dt><dd>${nowPlaying.Album}</dd>`}
                        </dl>
                    </details>
                `}
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
