package main

// embeddedUI is the single-page admin + chat interface served at /ui.
// Design: Neo-Editorial Minimalism (Cormorant Garamond, DM Sans, IBM Plex Mono,
// warm ivory/oxblood palette per UI_DESIGN_PATTERN.md).
const embeddedUI = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Freebuff Proxy</title>
<link rel="preconnect" href="https://fonts.googleapis.com">
<link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
<link href="https://fonts.googleapis.com/css2?family=Cormorant+Garamond:ital,wght@0,300;0,500;1,300&family=DM+Sans:wght@400;500;600&family=IBM+Plex+Mono:wght@400;600&display=swap" rel="stylesheet">
<style>
:root {
  --bg: #F7F4ED;
  --surface: #FCFBF7;
  --text: #11100D;
  --muted: #77736C;
  --accent: #861F1F;
  --accent-dark: #681414;
  --tag-bg: #F2EAD8;
  --border: #D8D2C7;
  --green: #2d7a3a;
  --red: #a03030;
}
@media (prefers-color-scheme: dark) {
  :root {
    --bg: #11100D;
    --surface: #1A1812;
    --text: #F5F1E8;
    --muted: #8a8580;
    --accent: #C94444;
    --accent-dark: #e05555;
    --tag-bg: #2a2520;
    --border: #3a3530;
    --green: #4caf50;
    --red: #e05555;
  }
}
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: 'DM Sans', sans-serif;
  background: var(--bg);
  color: var(--text);
  min-height: 100vh;
  display: flex;
  flex-direction: column;
}
/* Header */
.header {
  border-bottom: 1px solid var(--border);
  padding: 16px clamp(20px, 5vw, 80px);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.header h1 {
  font-family: 'Cormorant Garamond', serif;
  font-weight: 300;
  font-size: 28px;
  letter-spacing: -0.5px;
}
.header h1 em {
  font-style: italic;
  color: var(--accent);
}
.nav { display: flex; gap: 4px; }
.nav button {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  padding: 8px 16px;
  border: 1px solid var(--border);
  border-radius: 6px;
  background: transparent;
  color: var(--muted);
  cursor: pointer;
  transition: all 0.2s;
}
.nav button.active, .nav button:hover {
  background: var(--accent);
  color: white;
  border-color: var(--accent);
}
/* Main layout */
.main {
  flex: 1;
  display: flex;
  flex-direction: column;
  max-width: 900px;
  width: 100%;
  margin: 0 auto;
  padding: 0 clamp(16px, 4vw, 40px);
}
/* Panels */
.panel { display: none; flex-direction: column; flex: 1; }
.panel.active { display: flex; }

/* Chat panel */
.messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px 0;
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.msg {
  max-width: 85%;
  padding: 14px 18px;
  border-radius: 12px;
  font-size: 15px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
}
.msg.user {
  align-self: flex-end;
  background: var(--accent);
  color: white;
  border-bottom-right-radius: 4px;
}
.msg.assistant {
  align-self: flex-start;
  background: var(--surface);
  border: 1px solid var(--border);
  border-bottom-left-radius: 4px;
  line-height: 1.6;
}
.msg.assistant h1, .msg.assistant h2, .msg.assistant h3, .msg.assistant h4 {
  font-family: 'Cormorant Garamond', serif;
  font-weight: 500;
  margin: 12px 0 6px;
}
.msg.assistant h1 { font-size: 20px; }
.msg.assistant h2 { font-size: 17px; }
.msg.assistant h3 { font-size: 15px; }
.msg.assistant p { margin: 6px 0; }
.msg.assistant ul, .msg.assistant ol {
  margin: 6px 0;
  padding-left: 20px;
}
.msg.assistant li { margin: 2px 0; }
.msg.assistant code {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 13px;
  background: var(--tag-bg);
  padding: 1px 5px;
  border-radius: 4px;
}
.msg.assistant pre {
  background: var(--tag-bg);
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 12px 16px;
  margin: 8px 0;
  overflow-x: auto;
}
.msg.assistant pre code {
  background: none;
  padding: 0;
  font-size: 13px;
  line-height: 1.5;
}
.msg.assistant blockquote {
  border-left: 3px solid var(--accent);
  padding-left: 12px;
  margin: 8px 0;
  color: var(--muted);
  font-style: italic;
}
.msg.assistant strong { font-weight: 600; }
.msg.assistant a {
  color: var(--accent);
  text-decoration: underline;
}
.msg.assistant hr {
  border: none;
  border-top: 1px solid var(--border);
  margin: 12px 0;
}
.msg.assistant table {
  border-collapse: collapse;
  margin: 8px 0;
  font-size: 14px;
}
.msg.assistant th, .msg.assistant td {
  border: 1px solid var(--border);
  padding: 6px 10px;
  text-align: left;
}
.msg.assistant th {
  font-weight: 600;
  background: var(--tag-bg);
}
.msg.system-msg {
  align-self: center;
  background: var(--tag-bg);
  color: var(--muted);
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  padding: 8px 16px;
  border-radius: 20px;
}
.chat-input-row {
  display: flex;
  gap: 12px;
  padding: 16px 0 24px;
  border-top: 1px solid var(--border);
  align-items: stretch;
}
.model-select {
  position: relative;
  width: 220px;
  flex-shrink: 0;
}
.model-select-current {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  padding: 12px 14px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  user-select: none;
  height: 100%;
}
.model-select-current:hover { border-color: var(--accent); }
.model-select-current::after { content: '▾'; font-size: 10px; color: var(--muted); }
.model-select-dropdown {
  display: none;
  position: absolute;
  bottom: calc(100% + 4px);
  left: 0;
  right: 0;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  max-height: 300px;
  overflow-y: auto;
  z-index: 100;
  box-shadow: 0 8px 24px rgba(0,0,0,0.08);
}
.model-select-dropdown.open { display: block; }
.model-select-option {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  padding: 10px 14px;
  cursor: pointer;
  color: var(--text);
  border-bottom: 1px solid var(--border);
}
.model-select-option:last-child { border-bottom: none; }
.model-select-option:hover { background: var(--tag-bg); color: var(--accent); }
.model-select-option.selected { background: var(--accent); color: white; }
.chat-input {
  flex: 1;
  padding: 12px 16px;
  font-family: 'DM Sans', sans-serif;
  font-size: 15px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  resize: none;
  outline: none;
  transition: border-color 0.2s;
}
.chat-input:focus { border-color: var(--accent); }
.send-btn {
  padding: 12px 20px;
  background: var(--accent);
  color: white;
  border: none;
  border-radius: 8px;
  font-family: 'DM Sans', sans-serif;
  font-weight: 600;
  font-size: 14px;
  cursor: pointer;
  transition: background 0.2s;
  white-space: nowrap;
}
.send-btn:hover { background: var(--accent-dark); }
.send-btn:disabled { opacity: 0.5; cursor: not-allowed; }

/* Admin panels */
.section-title {
  font-family: 'IBM Plex Mono', monospace;
  font-weight: 600;
  font-size: 12px;
  text-transform: uppercase;
  letter-spacing: 0.1em;
  color: var(--accent);
  margin: 24px 0 12px;
}
.card {
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 12px;
  padding: 20px;
  margin-bottom: 12px;
}
.card-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 8px 0;
}
.card-row + .card-row { border-top: 1px solid var(--border); }
.mono {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 13px;
}
.tag {
  display: inline-block;
  padding: 4px 10px;
  background: var(--tag-bg);
  color: var(--accent);
  font-family: 'IBM Plex Mono', monospace;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  border-radius: 6px;
}
.tag.green { color: var(--green); }
.tag.red { color: var(--red); }
.btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 16px;
  font-family: 'DM Sans', sans-serif;
  font-weight: 500;
  font-size: 13px;
  border-radius: 8px;
  cursor: pointer;
  transition: all 0.2s;
  border: 1px solid transparent;
}
.btn-primary { background: var(--accent); color: white; }
.btn-primary:hover { background: var(--accent-dark); }
.btn-danger { background: transparent; color: var(--red); border-color: var(--red); }
.btn-danger:hover { background: var(--red); color: white; }
.btn-secondary { background: var(--surface); color: var(--text); border-color: var(--border); }
.btn-secondary:hover { border-color: var(--accent); color: var(--accent); }
.input-row {
  display: flex;
  gap: 8px;
  margin-top: 12px;
}
.input-row input {
  flex: 1;
  padding: 10px 14px;
  font-family: 'DM Sans', sans-serif;
  font-size: 14px;
  border: 1px solid var(--border);
  border-radius: 8px;
  background: var(--surface);
  color: var(--text);
  outline: none;
}
.input-row input:focus { border-color: var(--accent); }
.connection-box {
  background: var(--tag-bg);
  border-radius: 8px;
  padding: 16px;
  margin-top: 12px;
}
.connection-box code {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 13px;
  display: block;
  padding: 4px 0;
  color: var(--text);
  word-break: break-all;
}
.connection-box .label {
  font-family: 'IBM Plex Mono', monospace;
  font-size: 11px;
  font-weight: 600;
  text-transform: uppercase;
  letter-spacing: 0.08em;
  color: var(--muted);
  margin-bottom: 4px;
}
.models-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: 8px;
  margin-top: 8px;
}
.model-chip {
  padding: 8px 14px;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: 8px;
  font-family: 'IBM Plex Mono', monospace;
  font-size: 12px;
  color: var(--text);
}
.empty {
  text-align: center;
  padding: 40px;
  color: var(--muted);
  font-style: italic;
}
@media (max-width: 600px) {
  .chat-input-row { flex-wrap: wrap; }
  .model-select { min-width: 100%; }
}
</style>
</head>
<body>

<div class="header">
  <h1>Freebuff <em>Proxy</em></h1>
  <div class="nav">
    <button onclick="showPanel('chat')" id="tab-chat" class="active">Chat</button>
    <button onclick="showPanel('tokens')" id="tab-tokens">Tokens</button>
    <button onclick="showPanel('config')" id="tab-config">Config</button>
  </div>
</div>

<div class="main">
  <!-- Chat Panel -->
  <div id="panel-chat" class="panel active">
    <div class="messages" id="messages"></div>
    <div class="chat-input-row">
      <div class="model-select" id="model-select">
        <div class="model-select-current" id="model-select-current" onclick="toggleModelDropdown()">Select model...</div>
        <div class="model-select-dropdown" id="model-select-dropdown"></div>
      </div>
      <input type="hidden" id="model-select-value">
      <textarea class="chat-input" id="chat-input" rows="1" placeholder="Send a message..." onkeydown="if(event.key==='Enter'&&!event.shiftKey){event.preventDefault();sendMessage()}"></textarea>
      <button class="send-btn" id="send-btn" onclick="sendMessage()">Send</button>
    </div>
  </div>

  <!-- Tokens Panel -->
  <div id="panel-tokens" class="panel">
    <div class="section-title">Active Tokens</div>
    <div id="tokens-list"></div>
    <div class="section-title">Add Token</div>
    <div class="card">
      <p style="font-size:14px;color:var(--muted);margin-bottom:8px">Paste a Codebuff auth token to add it to the rotation pool.</p>
      <div class="input-row">
        <input type="text" id="new-token" placeholder="xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx">
        <button class="btn btn-primary" onclick="addToken()">Add</button>
      </div>
      <div style="margin-top:16px;font-size:14px;color:var(--muted)">
        <p><strong>How to get a new token:</strong></p>
        <p>Login to a different account on the Codebuff CLI:</p>
        <div class="connection-box">
          <code>npx -y codebuff@latest login</code>
        </div>
        <p style="margin-top:8px">This opens browser &rarr; sign in/up with a different email &rarr; it writes the token to <code>~/.config/manicode/credentials.json</code>.</p>
        <p style="margin-top:8px">Then grab it:</p>
        <div class="connection-box">
          <code>grep authToken ~/.config/manicode/credentials.json</code>
        </div>
      </div>
    </div>
  </div>

  <!-- Config Panel -->
  <div id="panel-config" class="panel">
    <div class="section-title">Connection Info</div>
    <div class="card">
      <div class="connection-box">
        <div class="label">OpenAI Base URL</div>
        <code id="openai-url"></code>
      </div>
      <div class="connection-box" style="margin-top:8px">
        <div class="label">Anthropic Base URL</div>
        <code id="anthropic-url"></code>
      </div>
    </div>
    <div class="section-title">API Keys</div>
    <div id="api-keys-list" class="card"></div>
    <div class="section-title">Available Models</div>
    <div id="models-list" class="models-grid"></div>
    <div class="section-title" style="margin-top:32px">System Status</div>
    <div id="status-card" class="card">
      <div class="empty">Loading...</div>
    </div>
  </div>
</div>

<script>
const BASE = location.origin;
let chatHistory = [];
let streaming = false;
let selectedModel = '';

function toggleModelDropdown() {
  const dd = document.getElementById('model-select-dropdown');
  dd.classList.toggle('open');
  // Close on outside click
  if (dd.classList.contains('open')) {
    setTimeout(() => document.addEventListener('click', closeModelDropdown), 0);
  }
}

function closeModelDropdown(e) {
  const container = document.getElementById('model-select');
  if (!container.contains(e.target)) {
    document.getElementById('model-select-dropdown').classList.remove('open');
    document.removeEventListener('click', closeModelDropdown);
  }
}

function selectModel(model) {
  selectedModel = model;
  document.getElementById('model-select-current').textContent = model;
  document.getElementById('model-select-value').value = model;
  document.getElementById('model-select-dropdown').classList.remove('open');
  document.getElementById('model-select-dropdown').querySelectorAll('.model-select-option').forEach(el => {
    el.classList.toggle('selected', el.dataset.model === model);
  });
  document.removeEventListener('click', closeModelDropdown);
}

function renderMarkdown(text) {
  // Escape HTML entities first
  let html = text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;');

  // Code blocks (backtick-backtick-backtick...backtick-backtick-backtick) - preserve newlines inside
  html = html.replace(/\x60\x60\x60(\w*)\n([\s\S]*?)\x60\x60\x60/g, '<pre><code class="lang-$1">$2</code></pre>');

  // Inline code (single backtick)
  html = html.replace(/\x60([^\x60]+)\x60/g, '<code>$1</code>');

  // Headers (###, ##, #)
  html = html.replace(/^#### (.+)$/gm, '<h4>$1</h4>');
  html = html.replace(/^### (.+)$/gm, '<h3>$1</h3>');
  html = html.replace(/^## (.+)$/gm, '<h2>$1</h2>');
  html = html.replace(/^# (.+)$/gm, '<h1>$1</h1>');

  // Bold + italic
  html = html.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>');
  html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
  html = html.replace(/\*(.+?)\*/g, '<em>$1</em>');

  // Blockquote
  html = html.replace(/^&gt; (.+)$/gm, '<blockquote>$1</blockquote>');

  // Unordered lists
  html = html.replace(/^[\-\*] (.+)$/gm, '<li>$1</li>');
  html = html.replace(/(<li>.*<\/li>)/gs, '<ul>$1</ul>');
  html = html.replace(/<\/ul>\s*<ul>/g, '');

  // Ordered lists
  html = html.replace(/^\d+\. (.+)$/gm, '<li>$1</li>');

  // Horizontal rule
  html = html.replace(/^---+$/gm, '<hr>');

  // Links
  html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>');

  // Tables
  html = html.replace(/^\|(.+)\|$/gm, (match, content) => {
    const cells = content.split('|').map(c => c.trim());
    if (cells.every(c => /^[\-\s:]+$/.test(c))) return '';
    return '<tr>' + cells.map(c => '<td>' + c + '</td>').join('') + '</tr>';
  });
  html = html.replace(/((<tr>.*<\/tr>\n?)+)/g, '<table>$1</table>');

  // Split on code blocks first to protect them
  const parts = html.split(/(<pre><code[\s\S]*?<\/code><\/pre>)/g);

  const processed = parts.map((part, i) => {
    // Odd indices are code blocks, skip processing
    if (part.startsWith('<pre>')) return part;

    // Paragraphs (double newline)
    part = part.replace(/\n\n+/g, '</p><p>');
    // Single newlines → <br>
    part = part.replace(/\n/g, '<br>');
    return part;
  });

  return processed.join('');
}

function showPanel(name) {
  document.querySelectorAll('.panel').forEach(p => p.classList.remove('active'));
  document.querySelectorAll('.nav button').forEach(b => b.classList.remove('active'));
  document.getElementById('panel-' + name).classList.add('active');
  document.getElementById('tab-' + name).classList.add('active');
  if (name === 'tokens') loadTokens();
  if (name === 'config') loadConfig();
}

// --- Chat ---
function addMsg(role, content) {
  const el = document.createElement('div');
  el.className = 'msg ' + (role === 'user' ? 'user' : role === 'system' ? 'system-msg' : 'assistant');
  if (role === 'assistant') {
    el.innerHTML = renderMarkdown(content);
  } else {
    el.textContent = content;
  }
  document.getElementById('messages').appendChild(el);
  el.scrollIntoView({ behavior: 'smooth' });
  return el;
}

async function sendMessage() {
  if (streaming) return;
  const input = document.getElementById('chat-input');
  const text = input.value.trim();
  if (!text) return;
  input.value = '';

  chatHistory.push({ role: 'user', content: text });
  addMsg('user', text);

  const model = selectedModel;
  const sendBtn = document.getElementById('send-btn');
  sendBtn.disabled = true;
  sendBtn.textContent = '...';
  streaming = true;

  const msgEl = addMsg('assistant', '');
  let full = '';

  try {
    const resp = await fetch(BASE + '/v1/chat/completions', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ model, messages: chatHistory, stream: true })
    });

    if (!resp.ok) {
      const raw = await resp.text();
      let errMsg = raw;
      try { const j = JSON.parse(raw); errMsg = j.error?.message || j.error || raw; } catch(e) {}
      msgEl.className = 'msg system-msg';
      msgEl.textContent = errMsg;
      chatHistory.pop();
      return;
    }

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = '';

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buf += decoder.decode(value, { stream: true });
      const lines = buf.split('\n');
      buf = lines.pop();
      for (const line of lines) {
        if (!line.startsWith('data: ')) continue;
        const data = line.slice(6);
        if (data === '[DONE]') continue;
        try {
          const chunk = JSON.parse(data);
          const delta = chunk.choices?.[0]?.delta?.content;
          if (delta) {
            full += delta;
            msgEl.innerHTML = renderMarkdown(full);
            msgEl.scrollIntoView({ behavior: 'smooth' });
          }
        } catch(e) {}
      }
    }
    if (full) chatHistory.push({ role: 'assistant', content: full });
  } catch(e) {
    msgEl.className = 'msg system-msg';
    msgEl.textContent = 'Network error: ' + e.message;
    chatHistory.pop();
  } finally {
    streaming = false;
    sendBtn.disabled = false;
    sendBtn.textContent = 'Send';
  }
}

// --- Tokens ---
async function loadTokens() {
  try {
    const resp = await fetch(BASE + '/admin/tokens');
    const data = await resp.json();
    const list = document.getElementById('tokens-list');
    if (!data.tokens?.length) {
      list.innerHTML = '<div class="card"><div class="empty">No tokens configured</div></div>';
      return;
    }
    list.innerHTML = data.tokens.map(t => ` + "`" + `
      <div class="card">
        <div class="card-row">
          <div>
            <span class="tag">${t.name}</span>
            <span class="mono" style="margin-left:12px">${t.token}</span>
          </div>
          <div style="display:flex;gap:8px;align-items:center">
            ${t.user_id ? '<span class="tag green">Connected</span>' : '<span class="tag red">No User ID</span>'}
            <button class="btn btn-danger" onclick="removeToken('${t.name}')">Remove</button>
          </div>
        </div>
        ${t.user_id ? '<div style="font-size:12px;color:var(--muted);margin-top:4px;font-family:IBM Plex Mono,monospace">User: ' + t.user_id + '</div>' : ''}
      </div>
    ` + "`" + `).join('');
  } catch(e) {
    document.getElementById('tokens-list').innerHTML = '<div class="card"><div class="empty">Failed to load: ' + e.message + '</div></div>';
  }
}

async function addToken() {
  const input = document.getElementById('new-token');
  const token = input.value.trim();
  if (!token) return;
  try {
    const resp = await fetch(BASE + '/admin/tokens', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token })
    });
    const data = await resp.json();
    if (data.error) { alert(data.error); return; }
    input.value = '';
    loadTokens();
  } catch(e) { alert('Failed: ' + e.message); }
}

async function removeToken(name) {
  if (!confirm('Remove ' + name + '?')) return;
  try {
    const resp = await fetch(BASE + '/admin/tokens', {
      method: 'DELETE',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ token: name })
    });
    const data = await resp.json();
    if (data.error) { alert(data.error); return; }
    loadTokens();
  } catch(e) { alert('Failed: ' + e.message); }
}

// --- Config ---
async function loadConfig() {
  try {
    const [configResp, healthResp] = await Promise.all([
      fetch(BASE + '/admin/config'),
      fetch(BASE + '/healthz')
    ]);
    const config = await configResp.json();
    const health = await healthResp.json();

    const host = location.hostname;
    const port = location.port || (location.protocol === 'https:' ? '443' : '80');
    document.getElementById('openai-url').textContent = location.origin + '/v1';
    document.getElementById('anthropic-url').textContent = location.origin + '/v1';

    // API keys
    const keysEl = document.getElementById('api-keys-list');
    if (config.api_keys?.length) {
      keysEl.innerHTML = config.api_keys.map(k =>
        '<div class="card-row"><span class="mono">' + k + '</span></div>'
      ).join('');
    } else {
      keysEl.innerHTML = '<div class="empty">No API keys configured (open access)</div>';
    }

    // Models
    const modelsEl = document.getElementById('models-list');
    modelsEl.innerHTML = (config.models || []).map(m =>
      '<div class="model-chip">' + m + '</div>'
    ).join('');

    // Status
    const statusEl = document.getElementById('status-card');
    const uptime = health.uptime_sec;
    const h = Math.floor(uptime / 3600);
    const m = Math.floor((uptime % 3600) / 60);
    statusEl.innerHTML = ` + "`" + `
      <div class="card-row">
        <span>Status</span>
        <span class="tag green">Online</span>
      </div>
      <div class="card-row">
        <span>Uptime</span>
        <span class="mono">${h}h ${m}m</span>
      </div>
      <div class="card-row">
        <span>Tokens</span>
        <span class="mono">${health.token_state?.length || 0}</span>
      </div>
    ` + "`" + `;
  } catch(e) {
    document.getElementById('status-card').innerHTML = '<div class="empty">Failed: ' + e.message + '</div>';
  }
}

// --- Init ---
async function init() {
  try {
    const resp = await fetch(BASE + '/admin/config?_=' + Date.now());
    const config = await resp.json();
    const dd = document.getElementById('model-select-dropdown');
    dd.innerHTML = '';
    (config.models || []).forEach(m => {
      const opt = document.createElement('div');
      opt.className = 'model-select-option';
      opt.dataset.model = m;
      opt.textContent = m;
      opt.onclick = () => selectModel(m);
      dd.appendChild(opt);
    });
    if (config.models?.length) selectModel(config.models[0]);
    console.log('Loaded', config.models?.length, 'models');
  } catch(e) { console.error('init failed:', e); }
}
init();
</script>
</body>
</html>
`
