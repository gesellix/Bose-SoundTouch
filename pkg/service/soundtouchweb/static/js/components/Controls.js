import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

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

    const repeatIcon = repeat === 'REPEAT_ONE' ? '🔂' : '🔁';

    return html`
        <div class="controls">
            <div class="transport">
                <button class="ctrl-btn" onClick=${() => send('PREV_TRACK')} title="Previous">⏮</button>
                <button class="ctrl-btn play-btn" onClick=${() => send(isPlaying ? 'PAUSE' : 'PLAY')}>
                    ${isPlaying ? '⏸' : '▶'}
                </button>
                <button class="ctrl-btn" onClick=${() => send('NEXT_TRACK')} title="Next">⏭</button>
                <button class="ctrl-btn ${isMuted ? 'active' : ''}" onClick=${() => send('MUTE')} title="Mute">
                    ${isMuted ? '🔇' : '🔊'}
                </button>
                <button class="ctrl-btn ${shuffle === 'SHUFFLE_ON' ? 'active' : ''}" onClick=${toggleShuffle} title="Shuffle">🔀</button>
                <button class="ctrl-btn ${repeat !== 'REPEAT_OFF' ? 'active' : ''}" onClick=${cycleRepeat} title="Repeat">${repeatIcon}</button>
            </div>
            <div class="volume-row">
                <span class="volume-icon">🔈</span>
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