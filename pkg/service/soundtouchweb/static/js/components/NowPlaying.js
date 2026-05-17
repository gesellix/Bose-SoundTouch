import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';

const html = htm.bind(h);

function fmt(secs) {
    if (!secs || secs <= 0) return '0:00';
    const m = Math.floor(secs / 60);
    const s = secs % 60;
    return `${m}:${s.toString().padStart(2, '0')}`;
}

export function NowPlaying({ nowPlaying }) {
    const [position, setPosition] = useState(0);

    useEffect(() => {
        const pos = nowPlaying?.Time?.Position ?? 0;
        setPosition(pos);
        if (nowPlaying?.PlayStatus !== 'PLAY_STATE') return;
        const id = setInterval(() => setPosition(p => p + 1), 1000);
        return () => clearInterval(id);
    }, [nowPlaying?.Time?.Position, nowPlaying?.PlayStatus]);

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
                        <span class="progress-time">${fmt(position)} / ${fmt(total)}</span>
                    </div>
                `}
            </div>
        </div>
    `;
}