import { apiURL } from './paths.js';
import { openResolvedMarkdownLink } from './canvas-markdown-links.js';
import { renderLocalGraphSection, type GraphTarget } from './canvas-local-graph.js';
import { renderBrainCanvasCardsSection } from './canvas-brain-cards.js';

const PANEL_ID = 'canvas-markdown-link-panel';

type RenderCanvas = (event: Record<string, unknown>) => void;

type CanvasGraphEvent = {
  kind?: string;
  path?: string;
  title?: string;
  artifact_id?: number;
  meta?: Record<string, unknown>;
};

function appState(): Record<string, unknown> {
  const app = (window as unknown as { _slopshellApp?: { getState?: () => Record<string, unknown> } })._slopshellApp;
  if (app && typeof app.getState === 'function') return app.getState();
  return {};
}

function activeWorkspaceID(): string {
  const state = appState();
  return String(state.activeWorkspaceId || 'active').trim() || 'active';
}

function activeBrainProjectRoot(): string {
  const state = appState();
  const projects = Array.isArray(state.projects) ? state.projects : [];
  const activeID = String(state.activeWorkspaceId || '').trim();
  const project = projects.find((entry) => String((entry as { id?: unknown })?.id || '') === activeID);
  if (!project) return '';
  const rootPath = String((project as { root_path?: unknown }).root_path || (project as { workspace_path?: unknown }).workspace_path || '').trim();
  if (!rootPath) return '';
  if (!/(?:^|[\\/])brain$/i.test(rootPath.replace(/[\\/]+$/, ''))) return '';
  return rootPath;
}

function isMarkdownArtifactPath(path: string): boolean {
  return /\.md$/i.test(String(path || '').trim());
}

function graphEntityRoot(path: string): string {
  const clean = String(path || '').trim();
  return /^(?:artifact|item|actor|label|source):[^:]+/i.test(clean) ? clean : '';
}

function eventArtifactID(event: CanvasGraphEvent): number {
  const raw = Number(event.meta?.artifact_id || event.artifact_id || 0);
  return Number.isFinite(raw) && raw > 0 ? raw : 0;
}

function graphTargetForCanvasEvent(event: CanvasGraphEvent): GraphTarget | null {
  const path = String(event.path || '').trim();
  if (isMarkdownArtifactPath(path)) return { sourcePath: path, label: path };
  const rootID = graphEntityRoot(path);
  if (rootID) return { rootID, label: rootID };
  const artifactID = eventArtifactID(event);
  if (artifactID > 0) return { artifactID, label: String(event.title || path || `artifact:${artifactID}`) };
  if (path) return { artifactPath: path, label: path };
  return null;
}

interface OutgoingRef {
  target: string;
  type: string;
  ok: boolean;
  blocked?: boolean;
  reason?: string;
  resolved_path?: string;
  vault_relative_path?: string;
  file_url?: string;
  kind?: string;
}

interface BacklinkEntry {
  source_path: string;
  link_type: string;
  link_target: string;
  excerpt?: string;
  file_url?: string;
}

interface PanelPayload {
  ok?: boolean;
  source_path?: string;
  outgoing?: OutgoingRef[];
  broken_count?: number;
  backlinks?: BacklinkEntry[];
  backlinks_truncated?: boolean;
  scan_limit_reached?: boolean;
  error?: string;
}

function panelHost(): HTMLElement | null {
  const column = document.getElementById('canvas-column');
  if (!(column instanceof HTMLElement)) return null;
  let panel = document.getElementById(PANEL_ID);
  if (!(panel instanceof HTMLElement)) {
    panel = document.createElement('aside');
    panel.id = PANEL_ID;
    panel.className = 'canvas-link-panel';
    panel.setAttribute('aria-label', 'Linked notes panel');
    panel.hidden = true;
    column.appendChild(panel);
  }
  return panel;
}

function hidePanel() {
  const panel = document.getElementById(PANEL_ID);
  if (panel instanceof HTMLElement) {
    panel.hidden = true;
    panel.replaceChildren();
  }
}

function setPanelLoading(panel: HTMLElement, sourcePath: string) {
  panel.hidden = false;
  panel.replaceChildren();
  const heading = document.createElement('header');
  heading.className = 'canvas-link-panel-header';
  heading.textContent = 'Loading links…';
  panel.appendChild(heading);
  panel.dataset.sourcePath = sourcePath;
}

function appendSection(panel: HTMLElement, title: string): HTMLElement {
  const section = document.createElement('section');
  section.className = 'canvas-link-panel-section';
  const heading = document.createElement('h4');
  heading.className = 'canvas-link-panel-heading';
  heading.textContent = title;
  section.appendChild(heading);
  panel.appendChild(section);
  return section;
}

function emptyNote(text: string): HTMLElement {
  const note = document.createElement('p');
  note.className = 'canvas-link-panel-empty';
  note.textContent = text;
  return note;
}

function setBlockedReason(item: HTMLElement, reason: string) {
  item.classList.add('is-blocked');
  const detail = item.querySelector('.canvas-link-panel-detail');
  const message = String(reason || '').trim() || 'link blocked';
  if (detail instanceof HTMLElement) {
    detail.textContent = message;
  }
}

function makeLinkAnchor(label: string): HTMLAnchorElement {
  const link = document.createElement('a');
  link.href = '#';
  link.className = 'canvas-link-panel-target canvas-link-panel-link';
  link.textContent = label;
  return link;
}

function bindResolverClick(
  anchor: HTMLAnchorElement,
  item: HTMLElement,
  load: () => Promise<void>,
) {
  anchor.addEventListener('click', (ev) => {
    ev.preventDefault();
    if (anchor.dataset.loading === '1') return;
    anchor.dataset.loading = '1';
    item.classList.remove('is-blocked');
    void load()
      .catch((err) => {
        setBlockedReason(item, String((err as Error)?.message || err || 'link blocked'));
      })
      .finally(() => {
        delete anchor.dataset.loading;
      });
  });
}

function renderOutgoingItem(
  ref: OutgoingRef,
  panelSourcePath: string,
  renderCanvas: RenderCanvas,
): HTMLElement {
  const item = document.createElement('li');
  item.className = 'canvas-link-panel-item';
  const label = ref.type === 'wikilink' ? `[[${ref.target}]]` : ref.target;
  const detail = document.createElement('span');
  detail.className = 'canvas-link-panel-detail';
  if (ref.ok) {
    const anchor = makeLinkAnchor(label);
    item.appendChild(anchor);
    detail.textContent = ref.vault_relative_path || ref.resolved_path || '';
    item.appendChild(detail);
    bindResolverClick(anchor, item, () =>
      openResolvedMarkdownLink(
        {
          ok: true,
          kind: ref.kind,
          file_url: ref.file_url,
          vault_relative_path: ref.vault_relative_path,
          resolved_path: ref.resolved_path,
          source_path: panelSourcePath,
        },
        renderCanvas,
      ),
    );
    return item;
  }
  const span = document.createElement('span');
  span.className = 'canvas-link-panel-target';
  span.textContent = label;
  item.appendChild(span);
  detail.textContent = ref.reason || 'broken link';
  item.appendChild(detail);
  item.classList.add('is-blocked');
  return item;
}

function renderBacklinkItem(
  entry: BacklinkEntry,
  panelSourcePath: string,
  renderCanvas: RenderCanvas,
): HTMLElement {
  const item = document.createElement('li');
  item.className = 'canvas-link-panel-item';
  const fileURL = String(entry.file_url || '').trim();
  if (!fileURL) {
    const span = document.createElement('span');
    span.className = 'canvas-link-panel-target';
    span.textContent = entry.source_path;
    item.appendChild(span);
  } else {
    const anchor = makeLinkAnchor(entry.source_path);
    item.appendChild(anchor);
    bindResolverClick(anchor, item, () =>
      openResolvedMarkdownLink(
        {
          ok: true,
          kind: 'text',
          file_url: fileURL,
          vault_relative_path: entry.source_path,
          resolved_path: entry.source_path,
          source_path: panelSourcePath,
        },
        renderCanvas,
      ),
    );
  }
  if (entry.excerpt) {
    const excerpt = document.createElement('span');
    excerpt.className = 'canvas-link-panel-excerpt';
    excerpt.textContent = entry.excerpt;
    item.appendChild(excerpt);
  }
  const meta = document.createElement('span');
  meta.className = 'canvas-link-panel-detail';
  meta.textContent = entry.link_type === 'wikilink'
    ? `[[${entry.link_target}]]`
    : entry.link_target;
  item.appendChild(meta);
  return item;
}

function renderPanelContent(panel: HTMLElement, payload: PanelPayload, renderCanvas: RenderCanvas) {
  panel.replaceChildren();
  panel.hidden = false;

  const header = document.createElement('header');
  header.className = 'canvas-link-panel-header';
  header.textContent = payload.source_path
    ? `Links for ${payload.source_path}`
    : 'Links';
  panel.appendChild(header);

  if (!payload.ok) {
    panel.appendChild(emptyNote(payload.error || 'links unavailable'));
    return;
  }

  const sourcePath = String(payload.source_path || '').trim();
  const outgoing = Array.isArray(payload.outgoing) ? payload.outgoing : [];
  const broken = outgoing.filter((ref) => !ref.ok);
  const working = outgoing.filter((ref) => ref.ok);
  const backlinks = Array.isArray(payload.backlinks) ? payload.backlinks : [];

  const outgoingSection = appendSection(panel, `Outgoing (${working.length})`);
  if (!working.length) {
    outgoingSection.appendChild(emptyNote('no outgoing links'));
  } else {
    const list = document.createElement('ul');
    list.className = 'canvas-link-panel-list';
    working.forEach((ref) => list.appendChild(renderOutgoingItem(ref, sourcePath, renderCanvas)));
    outgoingSection.appendChild(list);
  }

  const brokenSection = appendSection(panel, `Broken (${broken.length})`);
  if (!broken.length) {
    brokenSection.appendChild(emptyNote('no broken links'));
  } else {
    const list = document.createElement('ul');
    list.className = 'canvas-link-panel-list';
    broken.forEach((ref) => list.appendChild(renderOutgoingItem(ref, sourcePath, renderCanvas)));
    brokenSection.appendChild(list);
  }

  const backlinksSection = appendSection(panel, `Backlinks (${backlinks.length}${payload.backlinks_truncated ? '+' : ''})`);
  if (!backlinks.length) {
    backlinksSection.appendChild(emptyNote('no backlinks found'));
  } else {
    const list = document.createElement('ul');
    list.className = 'canvas-link-panel-list';
    backlinks.forEach((entry) => list.appendChild(renderBacklinkItem(entry, sourcePath, renderCanvas)));
    backlinksSection.appendChild(list);
  }
  if (payload.scan_limit_reached) {
    panel.appendChild(emptyNote('Backlink scan stopped at the file cap; results may be incomplete.'));
  }
}

export function clearMarkdownLinkPanel() {
  hidePanel();
}

export async function renderMarkdownLinkPanelForCanvasEvent(
  event: CanvasGraphEvent | null | undefined,
  renderCanvas: RenderCanvas,
) {
  if (!event) {
    hidePanel();
    return;
  }
  const target = graphTargetForCanvasEvent(event);
  if (!target) {
    hidePanel();
    return;
  }
  if (!activeBrainProjectRoot()) {
    hidePanel();
    return;
  }
  if (target.sourcePath) {
    await renderMarkdownLinkPanel(activeWorkspaceID(), target.sourcePath, renderCanvas);
    return;
  }
  await renderGraphOnlyPanel(activeWorkspaceID(), target, renderCanvas);
  void renderBrainCanvasCardsSection(panelHost() ?? document.createElement('div'), activeWorkspaceID(), target.sourcePath || '', renderCanvas);
}

async function renderGraphOnlyPanel(workspaceID: string, target: GraphTarget, renderCanvas: RenderCanvas) {
  const panel = panelHost();
  if (!panel) return;
  const label = String(target.label || target.rootID || target.artifactPath || target.artifactID || 'active artifact');
  panel.replaceChildren();
  panel.hidden = false;
  const header = document.createElement('header');
  header.className = 'canvas-link-panel-header';
  header.textContent = `Links for ${label}`;
  panel.appendChild(header);
  await renderLocalGraphSection(panel, workspaceID, target, renderCanvas);
}

export async function renderMarkdownLinkPanel(workspaceID: string, sourcePath: string, renderCanvas: RenderCanvas) {
  const cleanSource = String(sourcePath || '').trim();
  if (!cleanSource) {
    hidePanel();
    return;
  }
  const id = String(workspaceID || '').trim() || 'active';
  const panel = panelHost();
  if (!panel) return;
  setPanelLoading(panel, cleanSource);
  try {
    const params = new URLSearchParams({ source: cleanSource });
    const resp = await fetch(
      apiURL(`workspaces/${encodeURIComponent(id)}/markdown-link/panel?${params.toString()}`),
      { cache: 'no-store' },
    );
    if (panel.dataset.sourcePath !== cleanSource) return;
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      renderPanelContent(panel, { ok: false, source_path: cleanSource, error: detail }, renderCanvas);
      return;
    }
    const payload = (await resp.json()) as PanelPayload;
    if (panel.dataset.sourcePath !== cleanSource) return;
    renderPanelContent(panel, payload, renderCanvas);
    void renderLocalGraphSection(panel, id, cleanSource, renderCanvas);
    void renderBrainCanvasCardsSection(panel, id, cleanSource, renderCanvas);
  } catch (err) {
    if (panel.dataset.sourcePath !== cleanSource) return;
    renderPanelContent(panel, {
      ok: false,
      source_path: cleanSource,
      error: String((err as Error)?.message || err || 'links unavailable'),
    }, renderCanvas);
  }
}
