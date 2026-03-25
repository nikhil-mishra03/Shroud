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
    padding: 10px 20px;
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
  .badge-status { background: #1c2128; border: 1px solid var(--border); color: var(--dim); }
  .badge-status.connected { color: var(--green); border-color: #2d4a2d; background: #1a2f1a; }
  .dot { width: 7px; height: 7px; border-radius: 50%; background: currentColor; }
  .spacer { flex: 1; }

  /* Three-column layout */
  main {
    flex: 1;
    display: grid;
    grid-template-columns: 240px 1fr 280px;
    overflow: hidden;
  }
  .panel {
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--border);
    overflow: hidden;
  }
  .panel:last-child { border-right: none; }
  .panel-header {
    padding: 9px 16px;
    border-bottom: 1px solid var(--border);
    font-size: 10px;
    font-weight: 700;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: var(--dim);
    flex-shrink: 0;
  }
  .panel-body {
    flex: 1;
    overflow-y: auto;
    padding: 12px 16px;
  }
  .panel-body::-webkit-scrollbar { width: 6px; }
  .panel-body::-webkit-scrollbar-track { background: transparent; }
  .panel-body::-webkit-scrollbar-thumb { background: var(--border); border-radius: 3px; }

  /* Left panel: type aggregates */
  .type-row {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 12px;
  }
  .type-icon { font-size: 14px; width: 20px; text-align: center; flex-shrink: 0; }
  .type-label { width: 44px; font-size: 11px; font-weight: 700; color: var(--dim); flex-shrink: 0; }
  .type-bar-wrap { flex: 1; background: #21262d; border-radius: 3px; height: 5px; overflow: hidden; }
  .type-bar { height: 100%; border-radius: 3px; width: 0; transition: width 0.3s ease; }
  .bar-EMAIL { background: var(--blue); }
  .bar-KEY   { background: var(--red); }
  .bar-IP    { background: var(--purple); }
  .bar-ENV   { background: var(--green); }
  .bar-TOKEN { background: var(--yellow); }
  .bar-CRED  { background: var(--orange); }
  .type-count { font-size: 12px; font-weight: 700; color: var(--text); width: 24px; text-align: right; flex-shrink: 0; }
  .total-row {
    margin-top: 8px;
    padding-top: 12px;
    border-top: 1px solid var(--border);
    display: flex;
    justify-content: space-between;
    align-items: baseline;
  }
  .total-label { font-size: 10px; color: var(--dim); text-transform: uppercase; letter-spacing: 0.06em; }
  .total-value { font-size: 24px; font-weight: 700; color: var(--green); }
  .empty-msg { color: var(--dim); font-style: italic; font-size: 12px; }

  /* Middle panel: request blocks */
  .req-block {
    border: 1px solid var(--border);
    border-radius: 6px;
    margin-bottom: 8px;
    overflow: hidden;
    animation: fadeIn 0.2s ease;
  }
  @keyframes fadeIn { from { opacity: 0; transform: translateY(-3px); } to { opacity: 1; transform: none; } }
  .req-block.clean { opacity: 0.45; }
  .req-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    background: var(--surface);
    cursor: pointer;
    user-select: none;
    font-size: 11px;
  }
  .req-block.clean .req-header { cursor: default; }
  .req-arrow { color: var(--dim); font-size: 9px; flex-shrink: 0; }
  .req-id { font-size: 10px; font-weight: 700; background: #21262d; color: var(--dim); padding: 2px 6px; border-radius: 3px; flex-shrink: 0; }
  .req-summary { color: var(--dim); font-size: 11px; flex: 1; }
  .req-summary.has-secrets { color: var(--text); }
  .req-ts { color: var(--dim); font-size: 11px; flex-shrink: 0; }
  .req-body {
    padding: 8px 10px;
    font-size: 11px;
    line-height: 1.7;
    color: #c9d1d9;
    background: var(--bg);
    border-top: 1px solid var(--border);
    white-space: pre-wrap;
    word-break: break-word;
    max-height: 300px;
    overflow-y: auto;
  }
  /* Placeholder highlight colors — applied via JS innerHTML after HTML-escaping */
  .ph-email  { color: var(--blue);   font-weight: 700; }
  .ph-key    { color: var(--red);    font-weight: 700; }
  .ph-ip     { color: var(--purple); font-weight: 700; }
  .ph-env    { color: var(--green);  font-weight: 700; }
  .ph-token  { color: var(--yellow); font-weight: 700; }
  .ph-cred   { color: var(--orange); font-weight: 700; }

  /* Right panel: rehydrated events */
  .rh-row {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 6px 0;
    border-bottom: 1px solid #21262d;
    font-size: 12px;
    animation: fadeIn 0.2s ease;
  }
  .rh-check { color: var(--green); font-weight: 700; flex-shrink: 0; }
  .rh-ph { color: var(--green); font-weight: 600; }
  .rh-desc { color: var(--dim); font-size: 11px; }
  .rh-ts { color: var(--dim); font-size: 11px; margin-left: auto; flex-shrink: 0; }
  .rh-count { font-size: 20px; font-weight: 700; color: var(--text); margin-top: 12px; }
  .rh-count-label { font-size: 10px; color: var(--dim); text-transform: uppercase; letter-spacing: 0.06em; }
</style>
</head>
<body>
<header>
  <h1>🛡 <span>Shroud</span></h1>
  <div class="badge badge-tool"><span class="dot"></span><span id="tool-name">connecting...</span></div>
  <div class="spacer"></div>
  <div class="badge badge-status" id="conn-badge"><span class="dot"></span><span id="conn-status">connecting</span></div>
</header>

<main>
  <!-- Left: type aggregates -->
  <div class="panel">
    <div class="panel-header">Protected This Session</div>
    <div class="panel-body" id="left-panel">
      <div class="empty-msg" id="left-empty">Waiting for requests...</div>
      <div id="type-rows" style="display:none">
        <div class="type-row" id="row-EMAIL" style="display:none">
          <span class="type-icon">📧</span>
          <span class="type-label">EMAIL</span>
          <div class="type-bar-wrap"><div class="type-bar bar-EMAIL" id="bar-EMAIL"></div></div>
          <span class="type-count" id="count-EMAIL">0</span>
        </div>
        <div class="type-row" id="row-KEY" style="display:none">
          <span class="type-icon">🔑</span>
          <span class="type-label">KEY</span>
          <div class="type-bar-wrap"><div class="type-bar bar-KEY" id="bar-KEY"></div></div>
          <span class="type-count" id="count-KEY">0</span>
        </div>
        <div class="type-row" id="row-IP" style="display:none">
          <span class="type-icon">🌐</span>
          <span class="type-label">IP</span>
          <div class="type-bar-wrap"><div class="type-bar bar-IP" id="bar-IP"></div></div>
          <span class="type-count" id="count-IP">0</span>
        </div>
        <div class="type-row" id="row-ENV" style="display:none">
          <span class="type-icon">⚙️</span>
          <span class="type-label">ENV</span>
          <div class="type-bar-wrap"><div class="type-bar bar-ENV" id="bar-ENV"></div></div>
          <span class="type-count" id="count-ENV">0</span>
        </div>
        <div class="type-row" id="row-TOKEN" style="display:none">
          <span class="type-icon">🎟</span>
          <span class="type-label">TOKEN</span>
          <div class="type-bar-wrap"><div class="type-bar bar-TOKEN" id="bar-TOKEN"></div></div>
          <span class="type-count" id="count-TOKEN">0</span>
        </div>
        <div class="type-row" id="row-CRED" style="display:none">
          <span class="type-icon">🔒</span>
          <span class="type-label">CRED</span>
          <div class="type-bar-wrap"><div class="type-bar bar-CRED" id="bar-CRED"></div></div>
          <span class="type-count" id="count-CRED">0</span>
        </div>
        <div class="total-row">
          <span class="total-label">Unique secrets</span>
          <span class="total-value" id="total-count">0</span>
        </div>
      </div>
    </div>
  </div>

  <!-- Middle: outbound requests (masked prompt log) -->
  <div class="panel">
    <div class="panel-header">Outbound Requests (masked)</div>
    <div class="panel-body" id="mid-panel">
      <div class="empty-msg" id="mid-empty">Waiting for LLM requests...</div>
    </div>
  </div>

  <!-- Right: rehydrated responses -->
  <div class="panel">
    <div class="panel-header">Rehydrated in Response</div>
    <div class="panel-body" id="right-panel">
      <div class="empty-msg" id="right-empty">Responses will appear here...</div>
      <div id="rh-stats" style="display:none;margin-bottom:12px">
        <div class="rh-count-label">Rehydrated</div>
        <div class="rh-count" id="rh-total">0</div>
      </div>
    </div>
  </div>
</main>

<script>
  // Placeholder types known to Shroud — must match masker entity types exactly.
  const TYPES = ['EMAIL','KEY','IP','ENV','TOKEN','CRED'];
  const TYPE_CLASS = {EMAIL:'ph-email',KEY:'ph-key',IP:'ph-ip',ENV:'ph-env',TOKEN:'ph-token',CRED:'ph-cred'};

  // Counts per entity type across the session (unique secrets, not occurrences).
  const typeCounts = {EMAIL:0,KEY:0,IP:0,ENV:0,TOKEN:0,CRED:0};
  let totalUnique = 0;
  let rehydratedTotal = 0;
  let reqCounter = 0;

  // HTML-escape arbitrary text before inserting into the DOM.
  // This prevents prompt-injection XSS: a prompt containing <script> or
  // event handlers cannot execute in the browser dashboard.
  function escapeHtml(s) {
    return s
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

  // Highlight Shroud placeholders in an already-HTML-escaped string.
  // Regex matches only the 6 known entity types to avoid false positives
  // on user text that happens to look like [WORD_N].
  function highlightPlaceholders(escaped) {
    return escaped.replace(
      /\[(EMAIL|KEY|IP|ENV|TOKEN|CRED)_(\d+)\]/g,
      function(m, type) {
        return '<span class="' + TYPE_CLASS[type] + '">' + m + '</span>';
      }
    );
  }

  function fmt(ts) {
    return new Date(ts).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit',second:'2-digit'});
  }

  function fmtReqId(id) {
    return 'req_' + String(id).padStart(3, '0');
  }

  // Update left-panel type aggregate bars.
  function updateTypeBars() {
    const max = Math.max(...Object.values(typeCounts), 1);
    let anyVisible = false;
    for (const type of TYPES) {
      const count = typeCounts[type];
      if (count > 0) {
        document.getElementById('row-' + type).style.display = 'flex';
        document.getElementById('count-' + type).textContent = count;
        document.getElementById('bar-' + type).style.width = Math.round(count / max * 100) + '%';
        anyVisible = true;
      }
    }
    if (anyVisible) {
      document.getElementById('left-empty').style.display = 'none';
      document.getElementById('type-rows').style.display = 'block';
      document.getElementById('total-count').textContent = totalUnique;
    }
  }

  function handleMaskEvent(e) {
    const type = e.entity;
    if (!(type in typeCounts)) return;
    typeCounts[type]++;
    totalUnique++;
    updateTypeBars();
  }

  function handleRequestBody(e) {
    reqCounter++;
    const panel = document.getElementById('mid-panel');
    document.getElementById('mid-empty').style.display = 'none';

    const isClean = (e.masked_count || 0) === 0;
    const reqId = fmtReqId(e.request_id || reqCounter);
    const ts = fmt(e.ts);
    const block = document.createElement('div');
    block.className = 'req-block' + (isClean ? ' clean' : '');

    if (isClean) {
      // Clean request: single dim row, no expand.
      block.innerHTML =
        '<div class="req-header">' +
          '<span class="req-id">' + reqId + '</span>' +
          '<span class="req-summary">✓ no secrets detected</span>' +
          '<span class="req-ts">' + ts + '</span>' +
        '</div>';
    } else {
      // Masked request: collapsible. Default to collapsed; first one expands.
      const preview = escapeHtml((e.body || '').substring(0, 80).split('\n')[0]);
      const full = highlightPlaceholders(escapeHtml(e.body || ''));
      const count = e.masked_count || 0;
      const label = count + ' secret' + (count !== 1 ? 's' : '') + ' masked';
      // Expanded by default only for the very first masked request.
      const expanded = (panel.querySelectorAll('.req-block:not(.clean)').length === 0);

      block.innerHTML =
        '<div class="req-header">' +
          '<span class="req-arrow">' + (expanded ? '▼' : '▶') + '</span>' +
          '<span class="req-id">' + reqId + '</span>' +
          '<span class="req-summary has-secrets">' + label + '</span>' +
          '<span class="req-ts">' + ts + '</span>' +
        '</div>' +
        '<div class="req-body" style="display:' + (expanded ? 'block' : 'none') + '">' + full + '</div>';

      block.querySelector('.req-header').addEventListener('click', function() {
        const body = block.querySelector('.req-body');
        const arrow = block.querySelector('.req-arrow');
        const open = body.style.display !== 'none';
        body.style.display = open ? 'none' : 'block';
        arrow.textContent = open ? '▶' : '▼';
      });
    }

    panel.appendChild(block);
    panel.scrollTop = panel.scrollHeight;
  }

  function handleRehydrateEvent(e) {
    rehydratedTotal++;
    const panel = document.getElementById('right-panel');
    document.getElementById('right-empty').style.display = 'none';
    document.getElementById('rh-stats').style.display = 'block';
    document.getElementById('rh-total').textContent = rehydratedTotal;

    const row = document.createElement('div');
    row.className = 'rh-row';
    const ph = e.placeholder ? escapeHtml(e.placeholder) : '(chunk)';
    row.innerHTML =
      '<span class="rh-check">✓</span>' +
      '<div>' +
        '<div><span class="rh-ph">' + ph + '</span></div>' +
        '<div class="rh-desc">restored in response</div>' +
      '</div>' +
      '<span class="rh-ts">' + fmt(e.ts) + '</span>';
    panel.appendChild(row);
    panel.scrollTop = panel.scrollHeight;
  }

  function connect() {
    const ws = new WebSocket('ws://' + location.host + '/ws');
    const badge = document.getElementById('conn-badge');
    const status = document.getElementById('conn-status');

    ws.onopen = function() {
      badge.classList.add('connected');
      status.textContent = 'connected';
    };

    ws.onmessage = function(msg) {
      let e;
      try { e = JSON.parse(msg.data); } catch { return; }
      if (e.type === 'session_start') {
        document.getElementById('tool-name').textContent = e.tool || 'unknown';
      } else if (e.type === 'mask_event') {
        handleMaskEvent(e);
      } else if (e.type === 'request_body') {
        handleRequestBody(e);
      } else if (e.type === 'rehydrate_event') {
        handleRehydrateEvent(e);
      }
    };

    ws.onclose = function() {
      badge.classList.remove('connected');
      status.textContent = 'reconnecting...';
      setTimeout(connect, 1500);
    };

    ws.onerror = function() { ws.close(); };
  }

  connect();
</script>
</body>
</html>`
