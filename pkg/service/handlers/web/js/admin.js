import { h, render } from 'preact';
import { LogsTab } from './components/LogsTab.js';
import { adminApi } from './api.js';
import { createLogsController } from './logsState.mjs';

const root = document.getElementById('tab-logs');
const controller = createLogsController({ request: adminApi.logs });

// Legacy navigation still owns the tab containers. A single bridge hands
// lifecycle transitions to Preact without changing the other eight tabs.
window.adminLogs = {
    activate() {
        if (!root || !controller.start()) return;
        render(h(LogsTab, { controller }), root);
    },
    deactivate() {
        controller.stop();
        if (root) render(null, root);
    },
};

// A direct #tab-logs visit can be activated by the classic script before this
// deferred module has finished loading. Reconcile with the actual tab state.
if (root?.classList.contains('active')) window.adminLogs.activate();
