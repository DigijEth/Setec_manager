/* Setec App Manager — Client JS */

// ── API helper ──
const api = {
    async get(url) {
        const r = await fetch(url, { credentials: 'same-origin' });
        if (r.status === 401) { window.location.href = '/login'; return null; }
        if (!r.ok) throw new Error(`GET ${url}: ${r.status}`);
        return r.json();
    },
    async post(url, body) {
        const r = await fetch(url, {
            method: 'POST',
            credentials: 'same-origin',
            headers: { 'Content-Type': 'application/json' },
            body: body ? JSON.stringify(body) : undefined
        });
        if (r.status === 401) { window.location.href = '/login'; return null; }
        if (!r.ok) throw new Error(`POST ${url}: ${r.status}`);
        return r.json();
    },
    async del(url) {
        const r = await fetch(url, { method: 'DELETE', credentials: 'same-origin' });
        if (r.status === 401) { window.location.href = '/login'; return null; }
        if (!r.ok) throw new Error(`DELETE ${url}: ${r.status}`);
        return r.json();
    }
};

// ── Active nav highlight ──
(function highlightNav() {
    const path = window.location.pathname;
    document.querySelectorAll('.nav-link').forEach(link => {
        const href = link.getAttribute('href');
        if (href === '/' && path === '/') {
            link.classList.add('active');
        } else if (href !== '/' && path.startsWith(href)) {
            link.classList.add('active');
        }
    });
})();

// ── Dashboard auto-refresh ──
(function dashboardRefresh() {
    if (window.location.pathname !== '/') return;

    const INTERVAL = 10000; // 10 seconds

    async function refresh() {
        try {
            const data = await api.get('/api/stats');
            if (!data) return;

            // Update stat card values if elements exist
            const updates = {
                cpuBar:   { style: `width: ${data.cpu || 0}%` },
                memBar:   { style: `width: ${data.mem_percent || 0}%` },
                diskBar:  { style: `width: ${data.disk_percent || 0}%` },
            };

            for (const [id, props] of Object.entries(updates)) {
                const el = document.getElementById(id);
                if (el && props.style) el.setAttribute('style', props.style);
                if (el && props.text) el.textContent = props.text;
            }
        } catch (e) {
            console.warn('Stats refresh failed:', e.message);
        }
    }

    setInterval(refresh, INTERVAL);
})();

// ── Monitor page auto-refresh ──
(function monitorRefresh() {
    if (window.location.pathname !== '/monitor') return;

    const INTERVAL = 5000;

    async function refresh() {
        try {
            const data = await api.get('/api/stats');
            if (!data) return;

            const bar = (id, pct) => {
                const el = document.getElementById(id);
                if (el) el.style.width = pct + '%';
            };
            const txt = (id, val) => {
                const el = document.getElementById(id);
                if (el) el.textContent = val;
            };

            bar('cpuBar', data.cpu || 0);
            txt('cpuText', (data.cpu || 0).toFixed(1) + '%');
            bar('memBar', data.mem_percent || 0);
            bar('diskBar', data.disk_percent || 0);

            if (data.mem_text)  txt('memText', data.mem_text);
            if (data.disk_text) txt('diskText', data.disk_text);
            if (data.net_in)    txt('netIn', data.net_in);
            if (data.net_out)   txt('netOut', data.net_out);
        } catch (e) {
            console.warn('Monitor refresh failed:', e.message);
        }
    }

    setInterval(refresh, INTERVAL);
})();
