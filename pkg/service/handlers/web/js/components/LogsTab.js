import { h } from 'preact';
import { useEffect, useLayoutEffect, useRef, useState } from 'preact/hooks';
import htm from 'htm';
import { formatLogLines } from '../logsState.mjs';

const html = htm.bind(h);

export function LogsTab({ controller }) {
    const [state, setState] = useState(controller.snapshot());
    const view = useRef(null);
    useEffect(() => controller.subscribe(setState), [controller]);
    const lines = formatLogLines(state.entries, state.filter);
    useLayoutEffect(() => {
        if (state.followTail && view.current) view.current.scrollTop = view.current.scrollHeight;
    }, [lines, state.followTail]);

    const onScroll = () => {
        if (!view.current) return;
        const remaining = view.current.scrollHeight - view.current.scrollTop - view.current.clientHeight;
        if ((remaining < 8) !== state.followTail) controller.setFollowTail(remaining < 8);
    };
    return html`<div data-admin-ui="preact-logs">
        <div style="display: flex; justify-content: space-between; align-items: center; flex-wrap: wrap; gap: 10px;">
            <h2 style="margin: 0;">Service Logs</h2>
            <div style="display: flex; gap: 10px; align-items: center; flex-wrap: wrap;">
                <input type="text" id="logs-filter" placeholder="Filter (substring, case-insensitive)"
                    style="padding: 4px 8px; min-width: 260px;" value=${state.filter}
                    onInput=${event => controller.setFilter(event.currentTarget.value)} />
                <label style="font-size: 0.9em;">
                    <input type="checkbox" id="logs-follow" checked=${state.followTail}
                        onChange=${event => controller.setFollowTail(event.currentTarget.checked)} /> Follow tail
                </label>
            </div>
        </div>
        <p style="font-size: 0.9em; color: #555;">
            Live mirror of the service's stderr log. Stderr still
            receives every line — this is a read-only view for
            convenience.
        </p>
        <div id="logs-status" style="font-size: 0.8em; color: #888; margin-bottom: 6px;">${state.status}</div>
        <pre id="logs-view" ref=${view} onScroll=${onScroll}
            style="background: #111; color: #ddd; padding: 10px; border-radius: 4px; max-height: 60vh; overflow-y: auto; font-size: 0.8em; line-height: 1.35; margin: 0; white-space: pre-wrap; word-break: break-all;">${lines}</pre>
    </div>`;
}
