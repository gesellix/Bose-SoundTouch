import { h } from 'preact';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

const SOURCE_ICONS = {
    TUNEIN: '📻', SPOTIFY: '🎵', AMAZON: '🛒', PANDORA: '🎶',
    BLUETOOTH: '📶', AUX: '🔌', OPTICAL: '💡', HDMI: '📺',
    IHEARTRADIO: '❤️', DEEZER: '🎼', LOCAL_INTERNET_RADIO: '📡',
    AIRPLAY: '📡', PRODUCT: '🔊',
};

export function Sources({ deviceId, status }) {
    const items = status?.sources?.SourceItem ?? [];
    const currentSource = status?.nowPlaying?.Source;
    const currentAccount = status?.nowPlaying?.SourceAccount;

    const ready = items.filter(s => s.Status === 'READY');
    if (ready.length === 0) return null;

    function select(src) {
        api.selectSource(deviceId, src.Source, src.SourceAccount ?? '');
    }

    return html`
        <div class="sources-section">
            <h3 class="section-title">Sources</h3>
            <div class="source-list">
                ${ready.map(src => {
                    const isActive = src.Source === currentSource &&
                        (!src.SourceAccount || src.SourceAccount === currentAccount);
                    return html`
                        <button
                            key=${src.Source + (src.SourceAccount || '')}
                            class="source-btn ${isActive ? 'active' : ''} ${src.IsLocal ? 'local' : ''}"
                            onClick=${() => select(src)}
                            title=${src.Source}
                        >
                            <span class="source-icon">${SOURCE_ICONS[src.Source] || '🔊'}</span>
                            <span class="source-name">${src.DisplayName || src.Source}</span>
                        </button>
                    `;
                })}
            </div>
        </div>
    `;
}