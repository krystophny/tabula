import { esc } from './utils.js';

const MAX_ENTRIES = 200;
let entries = [];
let visible = false;

export function initMcpLog() {
  const toggle = document.getElementById('btn-mcp-log');
  if (toggle) {
    toggle.addEventListener('click', () => {
      visible = !visible;
      const panel = document.getElementById('mcp-log-panel');
      panel.style.display = visible ? '' : 'none';
      toggle.textContent = visible ? 'Hide Log' : 'MCP Log';
    });
  }
}

export function logEvent(direction, data) {
  const ts = new Date().toISOString().slice(11, 23);
  let summary;
  if (typeof data === 'string') {
    try { data = JSON.parse(data); } catch { summary = data.slice(0, 120); }
  }
  if (!summary) {
    if (data.kind) {
      summary = `${data.kind}${data.title ? ': ' + data.title : ''}`;
    } else if (data.method) {
      summary = `${data.method}${data.params?.name ? ' → ' + data.params.name : ''}`;
    } else if (data.result) {
      summary = 'result';
      if (data.result.structuredContent?.kind) {
        summary = `result: ${data.result.structuredContent.kind}`;
      }
    } else {
      summary = JSON.stringify(data).slice(0, 120);
    }
  }

  const entry = { ts, direction, summary, full: JSON.stringify(data, null, 2) };
  entries.push(entry);
  if (entries.length > MAX_ENTRIES) entries.shift();

  renderLog();
}

function renderLog() {
  const container = document.getElementById('mcp-log-entries');
  if (!container) return;

  const entry = entries[entries.length - 1];
  if (!entry) return;

  const el = document.createElement('div');
  el.className = 'log-entry';
  const arrow = entry.direction === 'in' ? '←' : '→';
  const dirClass = entry.direction === 'in' ? 'log-in' : 'log-out';
  el.innerHTML = `<span class="log-ts">${entry.ts}</span> <span class="${dirClass}">${arrow}</span> <span class="log-summary">${esc(entry.summary)}</span>`;
  el.title = entry.full;
  el.addEventListener('click', () => {
    const detail = el.querySelector('.log-detail');
    if (detail) {
      detail.remove();
    } else {
      const d = document.createElement('pre');
      d.className = 'log-detail';
      d.textContent = entry.full;
      el.appendChild(d);
    }
  });

  container.appendChild(el);
  container.scrollTop = container.scrollHeight;
}

