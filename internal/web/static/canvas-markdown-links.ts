import { apiURL } from './paths.js';
import { normalizeCanvasPath } from './canvas-visual.js';
import { createLinkedWorkspaceAtPath } from './app-workspace-runtime.js';

function currentWorkspaceID() {
  const state = (window._slopshellApp || {}).getState ? window._slopshellApp.getState() : {};
  return String(state.activeWorkspaceId || 'active').trim() || 'active';
}

function currentWorkspaceProject() {
  const state = (window._slopshellApp || {}).getState ? window._slopshellApp.getState() : {};
  const projects = Array.isArray(state.projects) ? state.projects : [];
  const activeWorkspaceId = String(state.activeWorkspaceId || '').trim();
  return projects.find((project) => String(project?.id || '') === activeWorkspaceId) || null;
}

function parentPath(rawPath) {
  const cleaned = String(rawPath || '').trim().replace(/[\\/]+$/, '');
  if (!cleaned) return '';
  return cleaned.replace(/[\\/]+[^\\/]+$/, '');
}

function joinWorkspacePath(basePath, relativePath) {
  const base = String(basePath || '').trim().replace(/[\\/]+$/, '');
  const rel = String(relativePath || '').trim().replace(/[\\/]+/g, '/').replace(/^\/+/, '');
  if (!base) return rel;
  if (!rel) return base;
  const sep = base.includes('\\') ? '\\' : '/';
  return `${base}${sep}${rel.replaceAll('/', sep)}`;
}

function resolveLinkedWorkspacePath(vaultRelativePath) {
  const project = currentWorkspaceProject();
  const rootPath = String(project?.root_path || project?.workspace_path || '').trim();
  if (!rootPath) throw new Error('workspace root unavailable');
  if (!/brain$/i.test(rootPath.replace(/[\\/]+$/, ''))) {
    throw new Error('linked folders can only open from a brain workspace');
  }
  const vaultRoot = parentPath(rootPath);
  if (!vaultRoot) throw new Error('vault root unavailable');
  return joinWorkspacePath(vaultRoot, vaultRelativePath);
}

function canvasMarkdownSourcePath(event) {
  return normalizeCanvasPath(event?.path || event?.title || '');
}

function isLocalMarkdownHref(raw) {
  const href = String(raw || '').trim();
  if (!href || href.startsWith('#')) return false;
  if (href.toLowerCase().startsWith('slopshell-wiki:')) return true;
  try {
    const parsed = new URL(href, window.location.href);
    return parsed.origin === window.location.origin
      && !/^[a-z][a-z0-9+.-]*:/i.test(href);
  } catch (_) {
    return !/^[a-z][a-z0-9+.-]*:/i.test(href);
  }
}

function clearMarkdownLinkReason(link) {
  link.classList.remove('markdown-link-blocked');
  delete link.dataset.blockedReason;
  const next = link.nextElementSibling;
  if (next instanceof HTMLElement && next.classList.contains('markdown-link-blocked-reason')) {
    next.remove();
  }
}

function showMarkdownLinkBlocked(link, reasonRaw) {
  const reason = String(reasonRaw || 'link blocked').trim();
  link.classList.add('markdown-link-blocked');
  link.dataset.blockedReason = reason;
  link.title = reason;
  let note = link.nextElementSibling;
  if (!(note instanceof HTMLElement) || !note.classList.contains('markdown-link-blocked-reason')) {
    note = document.createElement('span');
    note.className = 'markdown-link-blocked-reason';
    link.insertAdjacentElement('afterend', note);
  }
  note.textContent = ` ${reason}`;
}

async function openResolvedMarkdownLink(resolution, renderCanvas) {
  const link = resolution?.link || resolution || {};
  const kind = String(link.kind || 'text').trim();
  const path = String(link.vault_relative_path || link.resolved_path || '').trim();
  const title = path || 'Linked note';
  if (kind === 'folder') {
    const linkedWorkspacePath = resolveLinkedWorkspacePath(path);
    await createLinkedWorkspaceAtPath(linkedWorkspacePath);
    return;
  }
  const fileURL = String(link.file_url || '').trim();
  if (!fileURL) throw new Error('link target unavailable');
  if (kind === 'image') {
    renderCanvas({
      kind: 'image_artifact',
      event_id: `markdown-link-${Date.now()}`,
      title,
      path,
      url: fileURL,
    });
    return;
  }
  if (kind === 'pdf') {
    renderCanvas({
      kind: 'pdf_artifact',
      event_id: `markdown-link-${Date.now()}`,
      title,
      path,
      url: fileURL,
    });
    return;
  }
  const resp = await fetch(fileURL, { cache: 'no-store' });
  if (!resp.ok) {
    const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
    throw new Error(detail);
  }
  const text = await resp.text();
  renderCanvas({
    kind: 'text_artifact',
    event_id: `markdown-link-${Date.now()}`,
    title,
    path,
    text,
  });
}

export function hydrateMarkdownArtifactLinks(root, event, renderCanvas) {
  if (!(root instanceof HTMLElement)) return;
  const source = canvasMarkdownSourcePath(event);
  if (!source || typeof renderCanvas !== 'function') return;
  root.querySelectorAll('a[href]').forEach((node) => {
    if (!(node instanceof HTMLAnchorElement)) return;
    const rawHref = node.getAttribute('href') || '';
    if (!isLocalMarkdownHref(rawHref)) return;
    const isWikilink = rawHref.toLowerCase().startsWith('slopshell-wiki:');
    node.dataset.markdownLinkTarget = rawHref;
    node.dataset.markdownLinkSource = source;
    node.addEventListener('click', (ev) => {
      ev.preventDefault();
      clearMarkdownLinkReason(node);
      const workspaceID = currentWorkspaceID();
      const params = new URLSearchParams({
        source,
        target: rawHref,
      });
      if (isWikilink) params.set('type', 'wikilink');
      void fetch(apiURL(`workspaces/${encodeURIComponent(workspaceID)}/markdown-link/resolve?${params.toString()}`), { cache: 'no-store' })
        .then(async (resp) => {
          if (!resp.ok) {
            const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
            throw new Error(detail);
          }
          return resp.json();
        })
        .then(async (payload) => {
          if (!payload?.ok) {
            showMarkdownLinkBlocked(node, payload?.reason || 'link blocked');
            return;
          }
          await openResolvedMarkdownLink(payload, renderCanvas);
        })
        .catch((err) => {
          showMarkdownLinkBlocked(node, String(err?.message || err || 'link blocked'));
        });
    });
  });
}
