import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

// Flat SVG icons using stroke/fill="currentColor" so they automatically
// follow the button's text colour in light mode, dark mode, and in the
// accent-inverted active state — no CSS filter needed.

function IconVolume({ muted = false, size = 20 }) {
    if (muted) {
        return html`<svg width=${size} height=${size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
            <line x1="23" y1="9" x2="17" y2="15"/>
            <line x1="17" y1="9" x2="23" y2="15"/>
        </svg>`;
    }
    return html`<svg width=${size} height=${size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <polygon points="11 5 6 9 2 9 2 15 6 15 11 19 11 5"/>
        <path d="M15.54 8.46a5 5 0 0 1 0 7.07"/>
    </svg>`;
}

function IconShuffle() {
    return html`<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <polyline points="16 3 21 3 21 8"/>
        <line x1="4" y1="20" x2="21" y2="3"/>
        <polyline points="21 16 21 21 16 21"/>
        <line x1="15" y1="15" x2="21" y2="21"/>
        <line x1="4" y1="4" x2="9" y2="9"/>
    </svg>`;
}

function IconRepeat({ one = false }) {
    return html`<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <polyline points="17 1 21 5 17 9"/>
        <path d="M3 11V9a4 4 0 0 1 4-4h14"/>
        <polyline points="7 23 3 19 7 15"/>
        <path d="M21 13v2a4 4 0 0 1-4 4H3"/>
        ${one && html`<text x="12" y="15" text-anchor="middle" font-size="8" font-weight="bold" stroke="none" fill="currentColor" font-family="sans-serif">1</text>`}
    </svg>`;
}

export function Controls({ deviceId, status }) {
    const np = status?.nowPlaying;
    const isPlaying = np?.PlayStatus === 'PLAY_STATE';
    const actualVolume = status?.volume?.ActualVolume ?? 0;
    const isMuted = status?.volume?.MuteEnabled ?? false;
    const shuffle = np?.ShuffleSetting ?? 'SHUFFLE_OFF';
    const repeat = np?.RepeatSetting ?? 'REPEAT_OFF';
    const actualBass = status?.bass?.TargetBass ?? 0;
    const hasBass = status?.bass != null;

    const [localVolume, setLocalVolume] = useState(actualVolume);
    const [localBass, setLocalBass] = useState(actualBass);

    useEffect(() => { setLocalVolume(actualVolume); }, [actualVolume]);
    useEffect(() => { setLocalBass(actualBass); }, [actualBass]);

    const send = (key) => api.key(deviceId, key);

    function onVolumeChange(e) {
        const val = parseInt(e.target.value, 10);
        setLocalVolume(val);
        api.volume(deviceId, val);
    }

    function onBassChange(e) {
        const val = parseInt(e.target.value, 10);
        setLocalBass(val);
        api.bass(deviceId, val);
    }

    function toggleShuffle() {
        send(shuffle === 'SHUFFLE_ON' ? 'SHUFFLE_OFF' : 'SHUFFLE_ON');
    }

    function cycleRepeat() {
        if (repeat === 'REPEAT_OFF') send('REPEAT_ALL');
        else if (repeat === 'REPEAT_ALL') send('REPEAT_ONE');
        else send('REPEAT_OFF');
    }

    return html`
        <div class="controls">
            <div class="transport">
                <button class="ctrl-btn" onClick=${() => send('PREV_TRACK')} title="Previous">⏮</button>
                <button class="ctrl-btn play-btn" onClick=${() => send(isPlaying ? 'PAUSE' : 'PLAY')}>
                    ${isPlaying ? '⏸' : '▶'}
                </button>
                <button class="ctrl-btn" onClick=${() => send('NEXT_TRACK')} title="Next">⏭</button>
                <button class="ctrl-btn ${isMuted ? 'active' : ''}" onClick=${() => send('MUTE')} title="Mute">
                    ${IconVolume({ muted: isMuted })}
                </button>
                <button class="ctrl-btn ${shuffle === 'SHUFFLE_ON' ? 'active' : ''}" onClick=${toggleShuffle} title="Shuffle">
                    ${IconShuffle()}
                </button>
                <button class="ctrl-btn ${repeat !== 'REPEAT_OFF' ? 'active' : ''}" onClick=${cycleRepeat} title=${repeat === 'REPEAT_ONE' ? 'Repeat one' : repeat === 'REPEAT_ALL' ? 'Repeat all' : 'Repeat'}>
                    ${IconRepeat({ one: repeat === 'REPEAT_ONE' })}
                </button>
            </div>
            <div class="volume-row">
                <span class="volume-icon">${IconVolume({ size: 16 })}</span>
                <input type="range" class="volume-slider" min="0" max="100"
                    value=${localVolume} onInput=${onVolumeChange} />
                <span class="volume-value">${localVolume}</span>
            </div>
            ${hasBass && html`
                <div class="bass-row">
                    <span class="bass-label">Bass</span>
                    <input type="range" class="volume-slider" min="-9" max="9"
                        value=${localBass} onInput=${onBassChange} />
                    <span class="volume-value">${localBass > 0 ? '+' : ''}${localBass}</span>
                </div>
            `}
        </div>
    `;
}
