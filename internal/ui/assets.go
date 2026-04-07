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
    --red-dim: #3d1a1a;
    --yellow-dim: #2d2410;
    --grey-dim: #1c2128;
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
    grid-template-columns: 220px 1fr 260px;
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
  .empty-msg { color: var(--dim); font-style: italic; font-size: 12px; }

  /* ── Left panel: severity-grouped protection summary ── */
  .sev-group {
    margin-bottom: 14px;
  }
  .sev-header {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-bottom: 6px;
    font-size: 10px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.08em;
  }
  .sev-dot {
    width: 8px; height: 8px; border-radius: 50%; flex-shrink: 0;
  }
  .sev-dot.critical { background: var(--red); }
  .sev-dot.moderate { background: var(--yellow); }
  .sev-dot.low      { background: var(--dim); }
  .sev-label.critical { color: var(--red); }
  .sev-label.moderate { color: var(--yellow); }
  .sev-label.low      { color: var(--dim); }
  .sev-count { margin-left: auto; font-size: 13px; font-weight: 700; }
  .sev-count.critical { color: var(--red); }
  .sev-count.moderate { color: var(--yellow); }
  .sev-count.low      { color: var(--dim); }

  .type-row {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-bottom: 5px;
    padding-left: 15px;
  }
  .type-icon { font-size: 12px; width: 18px; text-align: center; flex-shrink: 0; }
  .type-label { font-size: 11px; color: var(--dim); flex: 1; }
  .type-bar-wrap { width: 40px; background: #21262d; border-radius: 3px; height: 4px; overflow: hidden; }
  .type-bar { height: 100%; border-radius: 3px; width: 0; transition: width 0.3s ease; }
  .bar-EMAIL { background: var(--blue); }
  .bar-KEY   { background: var(--red); }
  .bar-IP    { background: var(--dim); }
  .bar-ENV   { background: var(--green); }
  .bar-TOKEN { background: var(--yellow); }
  .bar-CRED  { background: var(--orange); }
  .type-count { font-size: 11px; font-weight: 700; color: var(--text); width: 20px; text-align: right; flex-shrink: 0; }

  .total-row {
    margin-top: 10px;
    padding-top: 10px;
    border-top: 1px solid var(--border);
    display: flex;
    justify-content: space-between;
    align-items: baseline;
  }
  .total-label { font-size: 10px; color: var(--dim); text-transform: uppercase; letter-spacing: 0.06em; }
  .total-value { font-size: 22px; font-weight: 700; color: var(--green); }

  /* ── Middle panel: request summary cards ── */
  .req-block {
    border: 1px solid var(--border);
    border-radius: 6px;
    margin-bottom: 8px;
    overflow: hidden;
    animation: fadeIn 0.2s ease;
  }
  @keyframes fadeIn { from { opacity: 0; transform: translateY(-3px); } to { opacity: 1; transform: none; } }
  .req-block.clean { opacity: 0.4; }
  .req-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 7px 10px;
    background: var(--surface);
    cursor: pointer;
    user-select: none;
    font-size: 11px;
  }
  .req-block.clean .req-header { cursor: default; }
  .req-arrow { color: var(--dim); font-size: 9px; flex-shrink: 0; }
  .req-id { font-size: 10px; font-weight: 700; background: #21262d; color: var(--dim); padding: 2px 6px; border-radius: 3px; flex-shrink: 0; }
  .req-model { color: var(--purple); font-size: 11px; flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .req-ts { color: var(--dim); font-size: 11px; flex-shrink: 0; }

  /* Severity badge row inside req-header */
  .req-sev-badges {
    display: flex;
    gap: 5px;
    align-items: center;
    flex-shrink: 0;
  }
  .sev-badge {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 10px;
    font-weight: 700;
    padding: 1px 5px;
    border-radius: 10px;
  }
  .sev-badge.critical { background: var(--red-dim); color: var(--red); border: 1px solid #5a1d1d; }
  .sev-badge.moderate { background: var(--yellow-dim); color: var(--yellow); border: 1px solid #4a3a10; }
  .sev-badge.low      { background: var(--grey-dim); color: var(--dim); border: 1px solid var(--border); }
  .sev-badge-dot { width: 5px; height: 5px; border-radius: 50%; background: currentColor; }

  /* Meta line: system prompt size, tools, messages */
  .req-meta {
    display: flex;
    gap: 12px;
    padding: 4px 10px;
    background: #0f1419;
    border-top: 1px solid #21262d;
    font-size: 10px;
    color: var(--dim);
  }
  .req-meta-item { display: flex; gap: 4px; }
  .req-meta-label { color: #555; }
  .req-meta-value { color: var(--dim); }

  /* Expandable user content */
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
  .req-body-label {
    font-size: 9px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--dim);
    margin-bottom: 6px;
    padding-bottom: 5px;
    border-bottom: 1px solid #21262d;
  }
  .req-sev-scope {
    font-size: 9px;
    color: var(--dim);
    opacity: 0.6;
    margin-left: 2px;
    align-self: center;
  }
  .req-body-empty { color: var(--dim); font-style: italic; }

  /* Placeholder highlight colors */
  .ph-email  { color: var(--blue);   font-weight: 700; }
  .ph-key    { color: var(--red);    font-weight: 700; }
  .ph-ip     { color: var(--dim);    font-weight: 700; }
  .ph-env    { color: var(--green);  font-weight: 700; }
  .ph-token  { color: var(--yellow); font-weight: 700; }
  .ph-cred   { color: var(--orange); font-weight: 700; }

  /* ── Right panel: rehydrated events ── */
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
  .outbound-block { margin-bottom: 10px; border-left: 2px solid #30363d; padding-left: 8px; }
  .outbound-block.outbound-tool { border-left-color: #388bfd44; }
  .outbound-block-label { font-size: 10px; color: var(--dim); text-transform: uppercase; letter-spacing: 0.06em; margin-bottom: 4px; }
  .outbound-block-content { font-size: 12px; color: var(--text); white-space: pre-wrap; word-break: break-all; max-height: 300px; overflow-y: auto; }
  .rh-main { display: flex; align-items: center; gap: 6px; flex: 1; }
  .rh-arrow { color: var(--dim); }
  .rh-hint { color: var(--dim); font-size: 11px; font-family: monospace; }
  .sev-critical { color: #f85149; }
  .sev-moderate { color: #d29922; }
  .sev-low { color: var(--dim); }
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
  <!-- Left: severity-grouped protection summary -->
  <div class="panel">
    <div class="panel-header">Protection Summary</div>
    <div class="panel-body" id="left-panel">
      <div class="empty-msg" id="left-empty">Waiting for requests...</div>
      <div id="sev-groups" style="display:none">

        <!-- Critical group -->
        <div class="sev-group" id="grp-critical" style="display:none">
          <div class="sev-header">
            <span class="sev-dot critical"></span>
            <span class="sev-label critical">Critical</span>
            <span class="sev-count critical" id="sev-count-critical">0</span>
          </div>
          <div class="type-row" id="row-KEY" style="display:none">
            <span class="type-icon">🔑</span>
            <span class="type-label">KEY</span>
            <div class="type-bar-wrap"><div class="type-bar bar-KEY" id="bar-KEY"></div></div>
            <span class="type-count" id="count-KEY">0</span>
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
        </div>

        <!-- Moderate group -->
        <div class="sev-group" id="grp-moderate" style="display:none">
          <div class="sev-header">
            <span class="sev-dot moderate"></span>
            <span class="sev-label moderate">Moderate</span>
            <span class="sev-count moderate" id="sev-count-moderate">0</span>
          </div>
          <div class="type-row" id="row-EMAIL" style="display:none">
            <span class="type-icon">📧</span>
            <span class="type-label">EMAIL</span>
            <div class="type-bar-wrap"><div class="type-bar bar-EMAIL" id="bar-EMAIL"></div></div>
            <span class="type-count" id="count-EMAIL">0</span>
          </div>
          <div class="type-row" id="row-ENV" style="display:none">
            <span class="type-icon">⚙️</span>
            <span class="type-label">ENV</span>
            <div class="type-bar-wrap"><div class="type-bar bar-ENV" id="bar-ENV"></div></div>
            <span class="type-count" id="count-ENV">0</span>
          </div>
        </div>

        <!-- Low group -->
        <div class="sev-group" id="grp-low" style="display:none">
          <div class="sev-header">
            <span class="sev-dot low"></span>
            <span class="sev-label low">Ambient</span>
            <span class="sev-count low" id="sev-count-low">0</span>
          </div>
          <div class="type-row" id="row-IP" style="display:none">
            <span class="type-icon">🌐</span>
            <span class="type-label">IP</span>
            <div class="type-bar-wrap"><div class="type-bar bar-IP" id="bar-IP"></div></div>
            <span class="type-count" id="count-IP">0</span>
          </div>
        </div>

        <div class="total-row">
          <span class="total-label">Unique secrets</span>
          <span class="total-value" id="total-count">0</span>
        </div>
      </div>
    </div>
  </div>

  <!-- Middle: request summary cards -->
  <div class="panel">
    <div class="panel-header">Outbound Requests</div>
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
  const TYPES = ['EMAIL','KEY','IP','ENV','TOKEN','CRED'];
  const TYPE_CLASS = {EMAIL:'ph-email',KEY:'ph-key',IP:'ph-ip',ENV:'ph-env',TOKEN:'ph-token',CRED:'ph-cred'};
  // Severity mapping matches masker.Severity() in Go
  const TYPE_SEV = {KEY:'critical',TOKEN:'critical',CRED:'critical',EMAIL:'moderate',ENV:'moderate',IP:'low'};

  const typeCounts = {EMAIL:0,KEY:0,IP:0,ENV:0,TOKEN:0,CRED:0};
  const sevCounts  = {critical:0,moderate:0,low:0};
  let totalUnique = 0;
  let rehydratedTotal = 0;
  let reqCounter = 0;

  function escapeHtml(s) {
    return s
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;')
      .replace(/'/g, '&#39;');
  }

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

  function fmtLen(n) {
    if (!n) return '—';
    if (n >= 1000) return Math.round(n / 100) / 10 + 'k';
    return n + ' ch';
  }

  // hintMap: stores {placeholder → {hint, entity, severity}} for rehydrate events
  const hintMap = {};

  function updateLeftPanel() {
    const max = Math.max(...Object.values(typeCounts), 1);
    let anyVisible = false;

    for (const type of TYPES) {
      const count = typeCounts[type];
      const row = document.getElementById('row-' + type);
      if (count > 0) {
        row.style.display = 'flex';
        document.getElementById('count-' + type).textContent = count;
        document.getElementById('bar-' + type).style.width = Math.round(count / max * 100) + '%';
        anyVisible = true;
      }
    }

    // Show/hide severity groups
    for (const sev of ['critical','moderate','low']) {
      const grp = document.getElementById('grp-' + sev);
      const cnt = sevCounts[sev];
      grp.style.display = cnt > 0 ? 'block' : 'none';
      document.getElementById('sev-count-' + sev).textContent = cnt;
    }

    if (anyVisible) {
      document.getElementById('left-empty').style.display = 'none';
      document.getElementById('sev-groups').style.display = 'block';
      document.getElementById('total-count').textContent = totalUnique;
    }
  }

  function handleMaskEvent(e) {
    const type = e.entity;
    if (!(type in typeCounts)) return;

    // Store hint for later rehydrate event lookup
    if (e.placeholder) {
      hintMap[e.placeholder] = { hint: e.hint || e.placeholder, entity: e.entity, severity: e.severity };
    }
    typeCounts[type]++;
    totalUnique++;
    const sev = e.severity || TYPE_SEV[type] || 'low';
    sevCounts[sev] = (sevCounts[sev] || 0) + 1;
    updateLeftPanel();
  }

  function buildSevBadges(criticalCount, moderateCount, lowCount) {
    let html = '';
    if (criticalCount > 0) {
      html += '<span class="sev-badge critical"><span class="sev-badge-dot"></span>' + criticalCount + ' critical</span>';
    }
    if (moderateCount > 0) {
      html += '<span class="sev-badge moderate"><span class="sev-badge-dot"></span>' + moderateCount + ' moderate</span>';
    }
    if (lowCount > 0) {
      html += '<span class="sev-badge low"><span class="sev-badge-dot"></span>' + lowCount + ' ambient</span>';
    }
    return html;
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
      const modelLabel = e.model ? escapeHtml(e.model) : '—';
      block.innerHTML =
        '<div class="req-header">' +
          '<span class="req-id">' + reqId + '</span>' +
          '<span class="req-model">' + modelLabel + '</span>' +
          '<span style="color:var(--dim);font-size:11px">✓ clean</span>' +
          '<span class="req-ts">' + ts + '</span>' +
        '</div>';
    } else {
      const critical = e.critical_count || 0;
      const moderate = e.moderate_count || 0;
      const low      = e.low_count || 0;
      const modelLabel = e.model ? escapeHtml(e.model) : '—';

      function renderBlocks(blocks, emptyText) {
        let html = '';
        if (blocks.length > 0) {
          blocks.forEach(function(b) {
            const isToolResult = b.role === 'tool_result';
            const label = isToolResult ? 'Tool result · ' + escapeHtml(b.label) : 'User';
            const content = highlightPlaceholders(escapeHtml(b.content || ''));
            html +=
              '<div class="outbound-block' + (isToolResult ? ' outbound-tool' : '') + '">' +
                '<div class="outbound-block-label">' + label + '</div>' +
                '<div class="outbound-block-content">' + content + '</div>' +
              '</div>';
          });
        } else {
          html = '<span class="req-body-empty">' + emptyText + '</span>';
        }
        return html;
      }

      let changedBlocksHtml = '';
      let fullBlocksHtml = '';
      try {
        const changedBlocks = e.changed_outbound_blocks ? JSON.parse(e.changed_outbound_blocks) : [];
        const fullBlocks = e.outbound_blocks ? JSON.parse(e.outbound_blocks) : [];
        changedBlocksHtml = renderBlocks(changedBlocks, '(no changed outbound content detected)');
        fullBlocksHtml = renderBlocks(fullBlocks, '(no content extracted)');
      } catch (_) {
        changedBlocksHtml = '<span class="req-body-empty">(parse error)</span>';
        fullBlocksHtml = '<span class="req-body-empty">(parse error)</span>';
      }

      // Expanded by default only for the very first masked request
      const expanded = (panel.querySelectorAll('.req-block:not(.clean)').length === 0);

      block.innerHTML =
        '<div class="req-header">' +
          '<span class="req-arrow">' + (expanded ? '▼' : '▶') + '</span>' +
          '<span class="req-id">' + reqId + '</span>' +
          '<span class="req-model">' + modelLabel + '</span>' +
          '<div class="req-sev-badges">' + buildSevBadges(critical, moderate, low) + '<span class="req-sev-scope">in full request</span></div>' +
          '<span class="req-ts">' + ts + '</span>' +
        '</div>' +
        '<div class="req-meta">' +
          '<span class="req-meta-item"><span class="req-meta-label">sys</span><span class="req-meta-value">' + fmtLen(e.system_len) + '</span></span>' +
          '<span class="req-meta-item"><span class="req-meta-label">user</span><span class="req-meta-value">' + fmtLen(e.user_len) + '</span></span>' +
          (e.tool_count ? '<span class="req-meta-item"><span class="req-meta-label">tools</span><span class="req-meta-value">' + e.tool_count + '</span></span>' : '') +
          (e.msg_count  ? '<span class="req-meta-item"><span class="req-meta-label">msgs</span><span class="req-meta-value">' + e.msg_count + '</span></span>' : '') +
        '</div>' +
        '<div class="req-body" style="display:' + (expanded ? 'block' : 'none') + '">' +
          '<div class="req-body-label">Changed outbound to LLM</div>' +
          changedBlocksHtml +
          '<details style="margin-top:10px">' +
            '<summary style="cursor:pointer;color:var(--dim);font-size:11px">Full outbound (all extracted blocks)</summary>' +
            '<div style="margin-top:8px">' + fullBlocksHtml + '</div>' +
          '</details>' +
        '</div>';

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
    const info = e.placeholder ? hintMap[e.placeholder] : null;
    const hint = info ? escapeHtml(info.hint) : '';
    const sevClass = info ? ' sev-' + (info.severity || 'low') : '';

    row.innerHTML =
      '<span class="rh-check">✓</span>' +
      '<div class="rh-main">' +
        '<span class="rh-ph">' + ph + '</span>' +
        (hint ? '<span class="rh-arrow">→</span><span class="rh-hint' + sevClass + '">' + hint + '</span>' : '') +
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
