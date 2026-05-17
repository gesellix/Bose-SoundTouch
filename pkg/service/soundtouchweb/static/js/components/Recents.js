import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

const SOURCE_ICONS = {
    TUNEIN: '📻', SPOTIFY: '🎵', AMAZON: '🎶', PANDORA: '🎸',
    DEEZER: '🎵', IHEART: '📻', BLUETOOTH: '📶', AUX: '🔌',
    LOCAL_MUSIC: '💽', STORED_MUSIC: '💽',
};

export function Recents({ deviceId }) {
    const [items, setItems] = useState(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        if (!deviceId) return;
        api.recents(deviceId).then(resp => {
            setItems(resp.data?.Items ?? []);
        }).catch(() => {
            setItems([]);
        }).finally(() => setLoading(false));
    }, [deviceId]);

    if (loading) return html`
        <div class="recents-section">
            <div class="section-title">Recents</div>
            <div class="loading-bar"></div>
        </div>
    `;

    if (!items || items.length === 0) return null;

    function play(item) {
        const ci = item.ContentItem;
        if (!ci?.Location) return;
        api.play(deviceId, {
            source: ci.Source,
            type: ci.Type,
            location: ci.Location,
            sourceAccount: ci.SourceAccount,
            itemName: ci.ItemName,
            containerArt: ci.ContainerArt,
            isPresetable: ci.IsPresetable,
        });
    }

    return html`
        <div class="recents-section">
            <div class="section-title">Recents</div>
            <div class="recents-list">
                ${items.map(item => {
                    const ci = item.ContentItem;
                    if (!ci) return null;
                    const icon = SOURCE_ICONS[ci.Source] ?? '♪';
                    return html`
                        <button class="recent-item" key=${item.ID || item.UTCTime} onClick=${() => play(item)}>
                            ${ci.ContainerArt
                                ? html`<img class="recent-art" src=${ci.ContainerArt} alt="" />`
                                : html`<div class="recent-art recent-art-empty">${icon}</div>`
                            }
                            <div class="recent-info">
                                <span class="recent-name">${ci.ItemName || ci.Source}</span>
                                <span class="recent-source">${ci.Source}</span>
                            </div>
                            <span class="recent-play">▶</span>
                        </button>
                    `;
                })}
            </div>
        </div>
    `;
}