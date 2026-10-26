// Keep the admin API separate from the player control API. A dedicated
// client for each migrated tab can be added as the migration progresses.
export const adminApi = {
    async logs(since, signal) {
        const response = await fetch(`/api/setup/logs?since=${encodeURIComponent(since)}`, { signal });
        if (!response.ok) throw new Error(`HTTP ${response.status}`);
        return response.json();
    },
};
