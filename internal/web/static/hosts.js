import { showMain } from './app.js';
import { esc } from './utils.js';

export async function loadHosts() {
  const container = document.getElementById('hosts-list');
  try {
    const resp = await fetch('/api/hosts');
    if (!resp.ok) { container.innerHTML = '<p class="error">Failed to load hosts</p>'; return; }
    const hosts = await resp.json();
    if (hosts.length === 0) {
      container.innerHTML = '<p style="color:var(--text-dim)">No hosts configured yet.</p>';
      return;
    }
    container.innerHTML = hosts.map(h => `
      <div class="host-item" data-id="${h.id}">
        <div class="host-info">
          <div class="host-name">${esc(h.name)}</div>
          <div class="host-detail">${esc(h.username)}@${esc(h.hostname)}:${h.port} - ${esc(h.project_dir)}</div>
        </div>
        <button class="btn-delete-host" data-id="${h.id}">Delete</button>
      </div>
    `).join('');

    container.querySelectorAll('.btn-delete-host').forEach(btn => {
      btn.addEventListener('click', async () => {
        if (!confirm('Delete this host?')) return;
        await fetch('/api/hosts/' + btn.dataset.id, { method: 'DELETE' });
        loadHosts();
      });
    });
  } catch (e) {
    container.innerHTML = '<p class="error">Error: ' + esc(e.message) + '</p>';
  }
}

export function initHostsView() {
  document.getElementById('btn-hosts-back').addEventListener('click', showMain);

  document.getElementById('host-form').addEventListener('submit', async (e) => {
    e.preventDefault();
    const form = e.target;
    const data = {
      name: form.name.value,
      hostname: form.hostname.value,
      port: parseInt(form.port.value) || 22,
      username: form.username.value,
      key_path: form.key_path.value,
      project_dir: form.project_dir.value || '~',
    };

    try {
      const resp = await fetch('/api/hosts', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(data),
      });
      if (!resp.ok) {
        alert('Failed: ' + await resp.text());
        return;
      }
      form.reset();
      form.port.value = '22';
      form.project_dir.value = '~';
      loadHosts();
    } catch (err) {
      alert('Error: ' + err.message);
    }
  });
}

