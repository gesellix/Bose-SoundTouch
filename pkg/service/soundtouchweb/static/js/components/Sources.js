import { h } from 'preact';
import { useEffect, useRef, useState } from 'preact/hooks';
import htm from 'htm';
import { api } from '../api.js';

const html = htm.bind(h);

const SOURCE_ICONS = {
    TUNEIN: '📻', SPOTIFY: '🎵', AMAZON: '🛒', PANDORA: '🎶',
    BLUETOOTH: '📶', AUX: '🔌', OPTICAL: '💡', HDMI: '📺',
    IHEARTRADIO: '❤️', DEEZER: '🎼', LOCAL_INTERNET_RADIO: '📡',
    AIRPLAY: '📡', PRODUCT: '🔊',
};

const SOURCE_READBACK_DELAYS_MS = [2000, 5000, 10000];

function isErrorSource(source) {
    return source === 'INVALID_SOURCE' || source?.endsWith('_ERROR');
}

function sourceAccountIdentity(source, account) {
    return account && account !== source ? account : '';
}

function sourceAccountsMatch(source, left, right) {
    return sourceAccountIdentity(source, left) === sourceAccountIdentity(source, right);
}

export function Sources({
    deviceId,
    status,
    onStatusReadback,
    readbackDelays = SOURCE_READBACK_DELAYS_MS,
}) {
    const [command, setCommand] = useState(null);
    const commandRef = useRef({ generation: 0, active: null, timers: [] });
    const mountedRef = useRef(false);
    const items = status?.sources?.SourceItem ?? [];
    const currentSource = status?.nowPlaying?.Source;
    const currentAccount = status?.nowPlaying?.SourceAccount;
    const nowPlayingRevision = Number.isSafeInteger(status?.nowPlayingRevision)
        ? status.nowPlayingRevision : null;
    const sourcesStale = status?.sourcesStale === true;

    function clearReadbacks() {
        commandRef.current.timers.forEach(clearTimeout);
        commandRef.current.timers = [];
    }

    useEffect(() => {
        if (!mountedRef.current) {
            mountedRef.current = true;
            return;
        }

        commandRef.current.generation += 1;
        commandRef.current.active = null;
        clearReadbacks();
        setCommand(null);
    }, [deviceId]);

    useEffect(() => {
        return () => {
            commandRef.current.generation += 1;
            commandRef.current.active = null;
            clearReadbacks();
        };
    }, []);

    useEffect(() => {
        if (!command || nowPlayingRevision === null || command.startNowPlayingRevision === null ||
            nowPlayingRevision <= command.startNowPlayingRevision ||
            command.outcome === 'failed' || command.outcome === 'unverified') return;

        const matches = currentSource === command.source &&
            sourceAccountsMatch(command.source, currentAccount, command.account);
        if (command.outcome === 'final-confirmed') {
            if (nowPlayingRevision > command.confirmedRevision && !matches) {
                setCommand(previous => previous?.generation === command.generation
                    ? null : previous);
            }
            return;
        }
        if (isErrorSource(currentSource)) {
            clearReadbacks();
            commandRef.current.active = null;
            setCommand(previous => previous?.generation === command.generation
                ? { ...previous, outcome: 'failed', error: currentSource }
                : previous);
        } else if (matches && command.outcome === 'pending') {
            setCommand(previous => previous?.generation === command.generation
                ? { ...previous, outcome: 'provisional-confirmed' }
                : previous);
        }
    }, [command, currentSource, currentAccount, nowPlayingRevision]);

    const ready = items.filter(s => s.Status === 'READY');
    // Nothing to show and nothing to say: a device that has not been polled
    // yet has no inventory to call out of date. Only render once there is
    // either a list to offer or a list we are refusing to act on.
    if (ready.length === 0 && !sourcesStale) return null;

    const availabilityMessage = sourcesStale
        ? 'Source list out of date'
        : '';
    const availabilityId = sourcesStale ? 'source-stale-status' : null;

    async function select(src) {
        clearReadbacks();
        const generation = commandRef.current.generation + 1;
        const target = { source: src.Source, account: src.SourceAccount ?? '' };
        commandRef.current.generation = generation;
        const active = { generation, latestReadback: -1, writeError: null };
        commandRef.current.active = active;
        setCommand({
            ...target,
            generation,
            outcome: 'pending',
            startNowPlayingRevision: nowPlayingRevision,
        });
        const startedAt = Date.now();

        readbackDelays.forEach((delay, index) => {
            const timer = setTimeout(async () => {
                if (commandRef.current.active !== active) return;
                active.latestReadback = index;

                try {
                    const response = await api.device(deviceId);
                    if (commandRef.current.active !== active || active.latestReadback !== index) return;

                    const readbackStatus = response?.data?.status;
                    const nowPlaying = readbackStatus?.nowPlaying;
                    const readbackRevision = readbackStatus?.nowPlayingRevision;
                    const revisionIsNewer = nowPlayingRevision !== null &&
                        Number.isSafeInteger(readbackRevision) &&
                        readbackRevision > nowPlayingRevision;
                    onStatusReadback?.(readbackStatus);
                    if (revisionIsNewer && isErrorSource(nowPlaying?.Source)) {
                        clearReadbacks();
                        commandRef.current.active = null;
                        setCommand({
                            ...target,
                            generation,
                            outcome: 'failed',
                            error: nowPlaying.Source,
                            startNowPlayingRevision: nowPlayingRevision,
                        });
                    } else if (revisionIsNewer && nowPlaying?.Source === target.source &&
                        sourceAccountsMatch(target.source, nowPlaying?.SourceAccount, target.account)) {
                        const isFinalReadback = index === readbackDelays.length - 1;
                        setCommand({
                            ...target,
                            generation,
                            outcome: isFinalReadback ? 'final-confirmed' : 'provisional-confirmed',
                            startNowPlayingRevision: nowPlayingRevision,
                            confirmedRevision: readbackRevision,
                        });
                        if (isFinalReadback) {
                            clearReadbacks();
                            commandRef.current.active = null;
                        }
                    } else if (index === readbackDelays.length - 1) {
                        commandRef.current.active = null;
                        setCommand({
                            ...target,
                            generation,
                            outcome: 'unverified',
                            error: active.writeError?.message,
                            startNowPlayingRevision: nowPlayingRevision,
                        });
                    }
                } catch (_) {
                    if (commandRef.current.active === active && active.latestReadback === index &&
                        index === readbackDelays.length - 1) {
                        commandRef.current.active = null;
                        setCommand({
                            ...target,
                            generation,
                            outcome: 'unverified',
                            error: active.writeError?.message,
                            startNowPlayingRevision: nowPlayingRevision,
                        });
                    }
                }
            }, Math.max(0, delay - (Date.now() - startedAt)));
            commandRef.current.timers.push(timer);
        });

        try {
            await api.selectSource(deviceId, target.source, target.account);
        } catch (error) {
            if (commandRef.current.active === active) active.writeError = error;
        }
    }

    const projectsTarget = command && (command.outcome === 'pending' ||
        command.outcome === 'provisional-confirmed' || command.outcome === 'final-confirmed');
    const projectedSource = projectsTarget
        ? command.source : currentSource;
    const projectedAccount = projectsTarget
        ? command.account : (currentAccount ?? '');

    const outcomeText = {
        pending: 'Selecting source',
        'provisional-confirmed': 'Source selected, confirming',
        'final-confirmed': 'Source selected',
        unverified: 'Source selection unverified',
        failed: 'Source selection failed',
    };

    return html`
        <div class="sources-section">
            <h3 class="section-title">Sources</h3>
            <div class="source-list">
                ${ready.map(src => {
                    const account = src.SourceAccount ?? '';
                    const isTarget = command?.source === src.Source && command?.account === account;
                    const isActive = src.Source === projectedSource &&
                        sourceAccountsMatch(src.Source, account, projectedAccount);
                    const outcome = isTarget ? command.outcome : '';
                    return html`
                        <button
                            key=${src.Source + account}
                            class="source-btn ${isActive ? 'active' : ''} ${src.IsLocal ? 'local' : ''} ${outcome}"
                            onClick=${() => select(src)}
                            disabled=${sourcesStale}
                            title=${availabilityMessage || (outcome ? outcomeText[outcome] : src.Source)}
                            aria-describedby=${availabilityId}
                            aria-busy=${outcome === 'pending' || outcome === 'provisional-confirmed' ? 'true' : null}
                        >
                            <span class="source-icon">${SOURCE_ICONS[src.Source] || '🔊'}</span>
                            <span class="source-name">${src.DisplayName || src.Source}</span>
                        </button>
                    `;
                })}
            </div>
            <div
                id=${availabilityId}
                class="source-command-status ${availabilityMessage ? 'availability' : ''}"
                role="status"
                aria-live="polite"
            >
                ${availabilityMessage || (command ? outcomeText[command.outcome] : '')}
            </div>
        </div>
    `;
}
