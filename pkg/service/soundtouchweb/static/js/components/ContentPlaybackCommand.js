import { h } from 'preact';
import { useEffect, useRef } from 'preact/hooks';
import htm from 'htm';
import { useDiscreteCommand } from '../discreteCommand.js';

const html = htm.bind(h);

export function ContentPlaybackCommand({
    request,
    devices,
    onStatusReadback,
    onStateChange,
    onClear,
}) {
    const started = useRef(false);
    const observedCommand = useRef(false);
    const device = request ? devices[request.deviceId] : null;
    const { command, busy, statusText, run } = useDiscreteCommand({
        deviceId: request?.deviceId,
        status: device?.status,
        onStatusReadback,
        readbackDelays: request?.readbackDelays,
        targetIdentity: request?.targetIdentity,
        currentTargetIdentity: device?.info?.device_id,
    });

    useEffect(() => {
        if (!request || started.current) return;
        started.current = true;
        run(request.action, request.invoke, request.expected, {
            expectedFromResponse: request.expectedFromResponse,
        });
    }, [request]);

    useEffect(() => {
        if (command) {
            observedCommand.current = true;
            onStateChange?.({ busy, outcome: command.outcome, error: command.error || '' });
        } else if (started.current && observedCommand.current) {
            onClear?.();
        }
    }, [busy, command, onClear, onStateChange]);

    if (!request) return null;
    const deviceName = device?.info?.name || request.deviceId;
    return html`
        <div class="content-command-status ${command?.outcome || 'pending'}" role="status" aria-live="polite">
            <span>
                <strong>${deviceName}</strong>: ${statusText || 'Starting playback'}
                ${command?.error ? html`<small class="content-command-error">${command.error}</small>` : null}
            </span>
            <button
                class="btn-secondary"
                onClick=${onClear}
                disabled=${busy}
                title=${busy ? 'Waiting for device readback' : 'Dismiss'}
            >Dismiss</button>
        </div>
    `;
}
