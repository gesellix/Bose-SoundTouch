import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { SourceIcon } from '../sourceIcons.js';
import { artworkFor, presetArtIndex } from '../recentsArt.mjs';

const html = htm.bind(h);

export function Recents({ deviceId, presets, command, commandBusy = false, onPlay }) {
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
        if (onPlay) {
            onPlay(item);
            return;
        }
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

    // The speaker sends no artwork with recents; a preset for the same content
    // is the one place the player can borrow it from. See recentsArt.mjs.
    const artIndex = presetArtIndex(presets);

    return html`
        <div class="recents-section">
            <div class="section-title">Recents</div>
            <div class="recents-list">
                ${items.map(item => {
                    const ci = item.ContentItem;
                    if (!ci) return null;
                    const art = artworkFor(ci, artIndex);
                    return html`
                        <button
                            class="recent-item"
                            key=${item.ID || item.UTCTime}
                            onClick=${() => play(item)}
                            disabled=${commandBusy}
                            aria-busy=${command?.action === 'recent' && commandBusy &&
                                command?.expected?.targetId === String(item.ID || item.UTCTime || ci.Location)
                                ? 'true' : null}
                        >
                            ${art
                                ? html`<img class="recent-art" src=${art} alt="" />`
                                : html`<div class="recent-art recent-art-empty"><${SourceIcon} source=${ci.Source} className="recent-source-icon" /></div>`
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
