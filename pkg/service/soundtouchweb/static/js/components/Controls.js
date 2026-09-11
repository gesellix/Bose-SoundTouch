import { h } from 'preact';
import { useState, useEffect, useMemo } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { throttleTrailing, SLIDER_WRITE_INTERVAL_MS } from '../throttle.mjs';

const html = htm.bind(h);

// Flat SVG icons using stroke/fill="currentColor" so they automatically
// follow the button's text colour in light mode, dark mode, and in the
// accent-inverted active state — no CSS filter needed.

function IconPrev() {
    return html`<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <polygon points="19 20 9 12 19 4 19 20"/>
        <line x1="5" y1="19" x2="5" y2="5"/>
    </svg>`;
}

function IconNext() {
    return html`<svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <polygon points="5 4 15 12 5 20 5 4"/>
        <line x1="19" y1="5" x2="19" y2="19"/>
    </svg>`;
}

function IconPlay() {
    return html`<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <polygon points="6 3 20 12 6 21 6 3"/>
    </svg>`;
}

function IconPause() {
    return html`<svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
        <rect x="6" y="4" width="4" height="16"/>
        <rect x="14" y="4" width="4" height="16"/>
    </svg>`;
}

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

// balanceLabel renders a level as a side rather than a bare signed number:
// "L3" reads better than "-3" on a control whose whole point is left/right.
function balanceLabel(level) {
    if (level === 0) return 'C';
    return level < 0 ? `L${-level}` : `R${level}`;
}

export function Controls({
    deviceId,
    status,
    command,
    commandBusy = false,
    commandStatus = '',
    onTogglePlayback,
    onToggleMute,
    onToggleShuffle,
    onCycleRepeat,
    onPreviousTrack,
    onNextTrack,
}) {
    const np = status?.nowPlaying;
    const isPlaying = np?.PlayStatus === 'PLAY_STATE';
    const actualVolume = status?.volume?.ActualVolume ?? 0;
    const isMuted = status?.volume?.MuteEnabled ?? false;
    const shuffle = np?.ShuffleSetting ?? 'SHUFFLE_OFF';
    const repeat = np?.RepeatSetting ?? 'REPEAT_OFF';
    const actualBass = status?.bass?.TargetBass ?? 0;
    const hasBass = status?.bass != null;

    // Balance belongs to a stereo pair and lives on its master, so the server
    // only reports it when it actually applies here. The bounds come from the
    // speaker (a SoundTouch 10 pair reports -7..7) rather than being assumed.
    const balance = status?.balance;
    const hasBalance = balance != null && balance.Available;
    const actualBalance = balance?.Target ?? 0;
    const balanceMin = balance?.Min ?? 0;
    const balanceMax = balance?.Max ?? 0;

    const [localVolume, setLocalVolume] = useState(actualVolume);
    const [localBass, setLocalBass] = useState(actualBass);
    const [localBalance, setLocalBalance] = useState(actualBalance);

    useEffect(() => { setLocalVolume(actualVolume); }, [actualVolume]);
    useEffect(() => { setLocalBass(actualBass); }, [actualBass]);
    useEffect(() => { setLocalBalance(actualBalance); }, [actualBalance]);

    const send = (key) => api.key(deviceId, key);
    const transportCommand = command?.action === 'play' || command?.action === 'pause'
        ? command : null;
    const muteCommand = command?.action?.startsWith('mute-') ? command : null;
    const shuffleCommand = command?.action?.startsWith('shuffle-') ? command : null;
    const repeatCommand = command?.action?.startsWith('repeat-') ? command : null;
    const previousCommand = command?.action === 'previous-track' ? command : null;
    const nextCommand = command?.action === 'next-track' ? command : null;

    // One throttle per slider per mount. The local state still updates on every
    // input, so the handle keeps up with the pointer; only the network write is
    // rate-limited.
    const writeVolume = useMemo(() => throttleTrailing(
        (id, val) => api.volume(id, val), SLIDER_WRITE_INTERVAL_MS), []);
    const writeBass = useMemo(() => throttleTrailing(
        (id, val) => api.bass(id, val), SLIDER_WRITE_INTERVAL_MS), []);
    const writeBalance = useMemo(() => throttleTrailing(
        (id, val) => api.balance(id, val), SLIDER_WRITE_INTERVAL_MS), []);

    function onVolumeChange(e) {
        const val = parseInt(e.target.value, 10);
        setLocalVolume(val);
        writeVolume(deviceId, val);
    }

    function onBassChange(e) {
        const val = parseInt(e.target.value, 10);
        setLocalBass(val);
        writeBass(deviceId, val);
    }

    function onBalanceChange(e) {
        const val = parseInt(e.target.value, 10);
        setLocalBalance(val);
        writeBalance(deviceId, val);
    }

    return html`
        <div class="controls">
            <div class="transport">
                <button
                    class="ctrl-btn command-btn previous-btn ${previousCommand?.outcome || ''}"
                    onClick=${onPreviousTrack || (() => send('PREV_TRACK'))}
                    disabled=${commandBusy}
                    aria-busy=${previousCommand && commandBusy ? 'true' : null}
                    title=${previousCommand && commandBusy ? commandStatus : 'Previous'}
                    aria-label="Previous"
                >
                    ${IconPrev()}
                </button>
                <button
                    class="ctrl-btn play-btn command-btn ${transportCommand?.outcome || ''}"
                    onClick=${onTogglePlayback || (() => send(isPlaying ? 'PAUSE' : 'PLAY'))}
                    disabled=${commandBusy}
                    aria-busy=${transportCommand && commandBusy ? 'true' : null}
                    title=${transportCommand && commandBusy ? commandStatus : (isPlaying ? 'Pause' : 'Play')}
                    aria-label=${isPlaying ? 'Pause' : 'Play'}
                >
                    ${isPlaying ? IconPause() : IconPlay()}
                </button>
                <button
                    class="ctrl-btn command-btn next-btn ${nextCommand?.outcome || ''}"
                    onClick=${onNextTrack || (() => send('NEXT_TRACK'))}
                    disabled=${commandBusy}
                    aria-busy=${nextCommand && commandBusy ? 'true' : null}
                    title=${nextCommand && commandBusy ? commandStatus : 'Next'}
                    aria-label="Next"
                >
                    ${IconNext()}
                </button>
                <button
                    class="ctrl-btn command-btn mute-btn ${isMuted ? 'active' : ''} ${muteCommand?.outcome || ''}"
                    onClick=${onToggleMute || (() => send('MUTE'))}
                    disabled=${commandBusy}
                    aria-busy=${muteCommand && commandBusy ? 'true' : null}
                    title=${muteCommand && commandBusy ? commandStatus : (isMuted ? 'Unmute' : 'Mute')}
                    aria-label=${isMuted ? 'Unmute' : 'Mute'}
                    aria-pressed=${isMuted}
                >
                    ${IconVolume({ muted: isMuted })}
                </button>
                <button
                    class="ctrl-btn command-btn shuffle-btn ${shuffle === 'SHUFFLE_ON' ? 'active' : ''} ${shuffleCommand?.outcome || ''}"
                    onClick=${onToggleShuffle || (() => send(shuffle === 'SHUFFLE_ON' ? 'SHUFFLE_OFF' : 'SHUFFLE_ON'))}
                    disabled=${commandBusy}
                    aria-busy=${shuffleCommand && commandBusy ? 'true' : null}
                    title=${shuffleCommand && commandBusy ? commandStatus : 'Shuffle'}
                    aria-label="Shuffle"
                    aria-pressed=${shuffle === 'SHUFFLE_ON'}
                >
                    ${IconShuffle()}
                </button>
                <button
                    class="ctrl-btn command-btn repeat-btn ${repeat !== 'REPEAT_OFF' ? 'active' : ''} ${repeatCommand?.outcome || ''}"
                    onClick=${onCycleRepeat || (() => {
                        if (repeat === 'REPEAT_OFF') send('REPEAT_ALL');
                        else if (repeat === 'REPEAT_ALL') send('REPEAT_ONE');
                        else send('REPEAT_OFF');
                    })}
                    disabled=${commandBusy}
                    aria-busy=${repeatCommand && commandBusy ? 'true' : null}
                    title=${repeatCommand && commandBusy ? commandStatus : (repeat === 'REPEAT_ONE' ? 'Repeat one' : repeat === 'REPEAT_ALL' ? 'Repeat all' : 'Repeat')}
                    aria-label=${repeat === 'REPEAT_ONE' ? 'Repeat one' : repeat === 'REPEAT_ALL' ? 'Repeat all' : 'Repeat'}
                    aria-pressed=${repeat !== 'REPEAT_OFF'}
                >
                    ${IconRepeat({ one: repeat === 'REPEAT_ONE' })}
                </button>
            </div>
            <div class="discrete-command-status" role="status" aria-live="polite">
                ${commandStatus}
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
            ${hasBalance && html`
                <div class="bass-row">
                    <span class="bass-label" title="Left / right balance for this stereo pair">L/R</span>
                    <input type="range" class="volume-slider"
                        min=${balanceMin} max=${balanceMax}
                        value=${localBalance} onInput=${onBalanceChange} />
                    <span class="volume-value">${balanceLabel(localBalance)}</span>
                </div>
            `}
        </div>
    `;
}
