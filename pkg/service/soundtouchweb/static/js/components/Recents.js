import { h } from 'preact';
import { useState, useEffect, useRef } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';
import { SourceIcon } from '../sourceIcons.js';
import { artworkFor, presetArtIndex } from '../recentsArt.mjs';

const html = htm.bind(h);

export function Recents(props) {
    if (!props.deviceId) return null;
    // Remount before rendering: old items must never acquire a new target's
    // onPlay handler while effect cleanup is still pending.
    const owner = JSON.stringify([props.deviceId, props.physicalId || '', props.epoch || 0]);
    return html`<${RecentItems} key=${owner} ...${props} />`;
}

function RecentItems({ deviceId, presets, command, commandBusy = false, onPlay,
    revision = 0, refreshToken = 0 }) {
    const [items, setItems] = useState(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState(null);
    const refresh = useRef(null);
    const trigger = JSON.stringify([revision, refreshToken]);
    const lastTrigger = useRef(trigger);

    useEffect(() => {
        let disposed = false;
        let running = false;
        let pending = false;
        const controller = new AbortController();

        async function load() {
            if (disposed) return;
            if (running) {
                pending = true;
                return;
            }
            running = true;
            setLoading(true);
            try {
                do {
                    pending = false;
                    try {
                        const response = await api.recents(deviceId, { signal: controller.signal });
                        if (disposed) return;
                        if (pending) continue;
                        if (!response?.success) throw new Error('Recents unavailable');
                        const next = response.data?.Items ?? [];
                        if (!Array.isArray(next)) throw new Error('Invalid recents response');
                        setItems(next);
                        setError(null);
                    } catch (_) {
                        if (!disposed && !pending) setError('Could not load recents.');
                    }
                } while (pending && !disposed);
            } finally {
                running = false;
                if (!disposed) setLoading(false);
            }
        }

        refresh.current = load;
        void load();
        const resume = () => {
            if (document.visibilityState === 'visible') void load();
        };
        document.addEventListener('visibilitychange', resume);
        return () => {
            disposed = true;
            refresh.current = null;
            controller.abort();
            document.removeEventListener('visibilitychange', resume);
        };
    }, [deviceId]);

    useEffect(() => {
        if (lastTrigger.current === trigger) return;
        lastTrigger.current = trigger;
        void refresh.current?.();
    }, [trigger]);

    if (loading && items === null && !error) return html`
        <div class="recents-section">
            <div class="section-title">Recents</div>
            <div class="loading-bar"></div>
        </div>
    `;

    if (!items?.length && !error) return null;

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
            ${error ? html`<div role="alert">
                ${error}
                <button type="button" class="btn-secondary" disabled=${loading}
                        onClick=${() => refresh.current?.()}>Retry recents</button>
            </div>` : null}
            <div class="recents-list">
                ${(items || []).map(item => {
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
