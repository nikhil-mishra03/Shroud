package ui

// DashboardHTML is the single-page dashboard served at the UI endpoint.
// Self-contained: no CDN, no external deps, inline CSS + JS.
const DashboardHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Shroud — Live Session</title>
<style>
  * { box-sizing: border-box; margin: 0; padding: 0; }
  :root {
    --bg: #0d1117;
    --surface: #161b22;
    --border: #30363d;
    --text: #e6edf3;
    --dim: #8b949e;
    --red: #f85149;
    --green: #3fb950;
    --yellow: #d29922;
    --blue: #58a6ff;
    --purple: #bc8cff;
    --orange: #ffa657;
  }
  body {
    background: var(--bg);
    color: var(--text);
    font-family: 'SF Mono', 'Fira Code', 'Cascadia Code', monospace;
    font-size: 13px;
    height: 100vh;
    display: flex;
    flex-direction: column;
  }
  header {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 12px 20px;
    border-bottom: 1px solid var(--border);
    background: var(--surface);
    flex-shrink: 0;
  }
  header h1 { font-size: 15px; font-weight: 600; color: var(--text); }
  header h1 span { color: var(--purple); }
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 10px;
    border-radius: 20px;
    font-size: 12px;
    font-weight: 500;
  }
  .badge-tool { background: #1c2128; border: 1px solid var(--border); color: var(--dim); }
  .badge-count { background: #1a2f1a; border: 1px solid #2d4a2d; color: var(--green); }
  .badge-status { background: #1c2128; border: 1px solid var(--border); color: var(--dim); }
  .badge-status.connected { color: var(--green); border-color: #2d4a2d; background: #1a2f1a; }
  .dot { width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
  .spacer { flex: 1; }
  main {
    flex: 1;
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 0;
    overflow: hidden;
  }
  .panel {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    border-right: 1px solid var(--border);
  }
  .panel:last-child { border-right: none; }
  .panel-header {
    padding: 10px 16px;
    border-bottom: 1px solid var(--border);
    font-size: 11px;
    font-weight: 600;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--dim);
    flex-shrink: 0;
  }
  .panel-body {
    flex: 1;
    overflow-y: auto;
    padding: 8px 0;
  }
  .panel-body::-webkit-scrollbar { width: 6px; }
  .panel-body::-webkit-scrollbar-track { background: transparent; }
  .panel-body::-webkit-scrollbar-thumb { background: var(--border); border-radius: 3px; }
  .event-row {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 16px;
    border-bottom: 1px solid transparent;
    transition: background 0.1s;
    animation: fadeIn 0.2s ease;
  }
  @keyframes fadeIn { from { opacity: 0; transform: translateY(-4px); } to { opacity: 1; transform: none; } }
  .event-row:hover { background: var(--surface); }
  .entity-badge {
    font-size: 10px;
    font-weight: 700;
    padding: 2px 7px;
    border-radius: 4px;
    min-width: 52px;
    text-align: center;
    flex-shrink: 0;
  }
  .entity-EMAIL  { background: #1e2d3d; color: var(--blue); border: 1px solid #1f3a52; }
  .entity-IP     { background: #2d1e3d; color: var(--purple); border: 1px solid #3d1f52; }
  .entity-KEY    { background: #3d1e1e; color: var(--red); border: 1px solid #521f1f; }
  .entity-TOKEN  { background: #2d2a1e; color: var(--yellow); border: 1px solid #52421f; }
  .entity-ENV    { background: #1e2d1e; color: var(--green); border: 1px solid #1f521f; }
  .placeholder { color: var(--red); font-weight: 600; }
  .ts { color: var(--dim); font-size: 11px; flex-shrink: 0; }
  .arrow { color: var(--dim); }
  .req-id { color: var(--dim); font-size: 11px; margin-left: auto; }
  .empty { color: var(--dim); padding: 24px 16px; font-style: italic; }
  .rehydrate-row { opacity: 0.6; }
  .rehydrate-ph { color: var(--green); font-weight: 600; }

  /* Stats panel */
  .stats-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 1px;
    background: var(--border);
    flex-shrink: 0;
    border-top: 1px solid var(--border);
  }
  .stat-cell {
    background: var(--bg);
    padding: 12px 16px;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .stat-label { font-size: 10px; color: var(--dim); text-transform: uppercase; letter-spacing: 0.06em; }
  .stat-value { font-size: 20px; font-weight: 700; color: var(--text); }
</style>
</head>
<body>
<header>
  <h1>🛡 <span>Shroud</span></h1>
  <div class="badge badge-tool"><span class="dot"></span><span id="tool-name">connecting...</span></div>
  <div class="badge badge-count"><span class="dot"></span><span id="masked-count">0</span> masked</div>
  <div class="spacer"></div>
  <div class="badge badge-status" id="conn-badge"><span class="dot"></span><span id="conn-status">connecting</span></div>
</header>

<main>
  <div class="panel">
    <div class="panel-header">Secrets Intercepted</div>
    <div class="panel-body" id="mask-list">
      <div class="empty" id="mask-empty">Waiting for LLM requests...</div>
    </div>
  </div>
  <div class="panel">
    <div class="panel-header">Rehydrated in Response</div>
    <div class="panel-body" id="rehydrate-list">
      <div class="empty" id="rehydrate-empty">Responses will appear here...</div>
    </div>
  </div>
</main>

<div class="stats-grid">
  <div class="stat-cell">
    <div class="stat-label">Secrets Masked</div>
    <div class="stat-value" id="stat-masked">0</div>
  </div>
  <div class="stat-cell">
    <div class="stat-label">Rehydrated</div>
    <div class="stat-value" id="stat-rehydrated">0</div>
  </div>
  <div class="stat-cell">
    <div class="stat-label">LLM Requests</div>
    <div class="stat-value" id="stat-requests">0</div>
  </div>
  <div class="stat-cell">
    <div class="stat-label">Session</div>
    <div class="stat-value" id="stat-session">—</div>
  </div>
</div>

<script>
  let maskedCount = 0, rehydratedCount = 0, requestCount = 0;
  const wsUrl = 'ws://' + location.host + '/ws';

  function fmt(ts) {
    return new Date(ts).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit',second:'2-digit'});
  }

  function addMaskRow(e) {
    const list = document.getElementById('mask-list');
    document.getElementById('mask-empty').style.display = 'none';
    const row = document.createElement('div');
    row.className = 'event-row';
    row.innerHTML =
      '<span class="entity-badge entity-' + e.entity + '">' + e.entity + '</span>' +
      '<span class="arrow">→</span>' +
      '<span class="placeholder">' + e.placeholder + '</span>' +
      '<span class="req-id">' + fmt(e.ts) + '</span>';
    list.appendChild(row);
    list.scrollTop = list.scrollHeight;
    maskedCount++;
    document.getElementById('masked-count').textContent = maskedCount;
    document.getElementById('stat-masked').textContent = maskedCount;
  }

  function addRehydrateRow(e) {
    const list = document.getElementById('rehydrate-list');
    document.getElementById('rehydrate-empty').style.display = 'none';
    const row = document.createElement('div');
    row.className = 'event-row rehydrate-row';
    row.innerHTML =
      '<span class="rehydrate-ph">' + (e.placeholder || '(chunk)') + '</span>' +
      '<span class="arrow">✓ rehydrated</span>' +
      '<span class="req-id">' + fmt(e.ts) + '</span>';
    list.appendChild(row);
    list.scrollTop = list.scrollHeight;
    rehydratedCount++;
    document.getElementById('stat-rehydrated').textContent = rehydratedCount;
  }

  function connect() {
    const ws = new WebSocket(wsUrl);
    const badge = document.getElementById('conn-badge');
    const status = document.getElementById('conn-status');

    ws.onopen = () => {
      badge.classList.add('connected');
      status.textContent = 'connected';
    };

    ws.onmessage = (msg) => {
      let e;
      try { e = JSON.parse(msg.data); } catch { return; }
      if (e.type === 'session_start') {
        document.getElementById('tool-name').textContent = e.tool || 'unknown';
        document.getElementById('stat-session').textContent = fmt(e.ts);
      } else if (e.type === 'mask_event') {
        addMaskRow(e);
      } else if (e.type === 'rehydrate_event') {
        addRehydrateRow(e);
      } else if (e.type === 'request') {
        requestCount++;
        document.getElementById('stat-requests').textContent = requestCount;
      }
    };

    ws.onclose = () => {
      badge.classList.remove('connected');
      status.textContent = 'reconnecting...';
      setTimeout(connect, 1500);
    };

    ws.onerror = () => ws.close();
  }

  connect();
</script>
</body>
</html>`
