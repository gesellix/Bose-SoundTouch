import { h } from 'preact';
import { useState, useEffect } from 'preact/hooks';
import htm from 'htm';

const html = htm.bind(h);

// Announcements renders dismissible banners for the player (target=app),
// mirroring the admin UI's banner (pkg/service/handlers/web/js/script.js,
// fetchAnnouncements/dismissAnnouncement) against the same backend endpoints
// added for #419 — see _/i419/design-admin-area-auth-gate.md. Dismissal is
// recorded server-side (not a client-only localStorage flag), so it stays
// dismissed across sessions/devices.
export function Announcements() {
    const [announcements, setAnnouncements] = useState([]);

    useEffect(() => {
        fetch('/api/announcements?target=app')
            .then(res => res.ok ? res.json() : { announcements: [] })
            .then(data => setAnnouncements(data.announcements || []))
            .catch(err => console.error('Failed to fetch announcements:', err));
    }, []);

    async function dismiss(id) {
        try {
            await fetch(`/api/announcements/${encodeURIComponent(id)}/dismiss`, { method: 'POST' });
        } catch (err) {
            console.error('Failed to dismiss announcement:', err);
        }
        setAnnouncements(prev => prev.filter(a => a.id !== id));
    }

    if (announcements.length === 0) return null;

    return html`
        <div class="announcements-banner">
            ${announcements.map(a => html`
                <div class="announcement announcement-${a.level || 'info'}" key=${a.id}>
                    <span>
                        ${a.message}
                        ${a.link_url ? html` <a href=${a.link_url} target="_blank" rel="noopener">${a.link_text || a.link_url}</a>` : null}
                    </span>
                    <button class="announcement-dismiss" onClick=${() => dismiss(a.id)} title="Dismiss">×</button>
                </div>
            `)}
        </div>
    `;
}
