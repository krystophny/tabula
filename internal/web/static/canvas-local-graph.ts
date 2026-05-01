import { apiURL } from './paths.js';
import { openResolvedMarkdownLink } from './canvas-markdown-links.js';

type RenderCanvas = (event: Record<string, unknown>) => void;

type GraphNode = {
  id: string;
  type: string;
  label: string;
  path?: string;
  file_url?: string;
  source?: string;
  sphere?: string;
};

type GraphEdge = {
  source: string;
  target: string;
  relation: string;
  label?: string;
};

type GraphPayload = {
  ok?: boolean;
  source_path?: string;
  nodes?: GraphNode[];
  edges?: GraphEdge[];
  truncated?: boolean;
  error?: string;
};

export type GraphTarget = {
  sourcePath?: string;
  artifactID?: number;
  artifactPath?: string;
  rootID?: string;
  label?: string;
};

const RELATIONS = [
  ['markdown_link', 'Links'],
  ['backlink', 'Backlinks'],
  ['artifact', 'Artifacts'],
  ['source_binding', 'Sources'],
  ['item_actor', 'Actors'],
  ['item_label', 'Labels'],
] as const;

function appState(): Record<string, any> {
  const app = (window as any)._slopshellApp;
  return typeof app?.getState === 'function' ? app.getState() : {};
}

function activeSphere(): string {
  const state = appState();
  const projects = Array.isArray(state.projects) ? state.projects : [];
  const activeID = String(state.activeWorkspaceId || '').trim();
  const project = projects.find((entry) => String(entry?.id || '') === activeID);
  return String(project?.sphere || state.activeSphere || 'work').trim().toLowerCase() || 'work';
}

function makeGraphSection(panel: HTMLElement): HTMLElement {
  const existing = panel.querySelector('.canvas-local-graph-section');
  if (existing instanceof HTMLElement) return existing;
  let section: HTMLElement;
  section = document.createElement('section');
  section.className = 'canvas-link-panel-section canvas-local-graph-section';
  const heading = document.createElement('h4');
  heading.className = 'canvas-link-panel-heading';
  heading.textContent = 'Graph';
  section.appendChild(heading);
  panel.appendChild(section);
  return section;
}

function relationParams(form: HTMLFormElement): string[] {
  const checked = Array.from(form.querySelectorAll<HTMLInputElement>('input[name="graph-relation"]:checked'));
  return checked.map((input) => input.value).filter(Boolean);
}

function normalizeGraphTarget(target: GraphTarget | string): GraphTarget {
  return typeof target === 'string' ? { sourcePath: target, label: target } : target;
}

function graphQuery(target: GraphTarget, form: HTMLFormElement): URLSearchParams {
  const params = new URLSearchParams();
  if (target.sourcePath) params.set('source', target.sourcePath);
  if (target.artifactID && Number.isFinite(target.artifactID)) params.set('artifact_id', String(target.artifactID));
  if (target.artifactPath) params.set('artifact_path', target.artifactPath);
  if (target.rootID) params.set('root', target.rootID);
  for (const relation of relationParams(form)) params.append('relation', relation);
  const source = String(new FormData(form).get('source_filter') || '').trim();
  const label = String(new FormData(form).get('label') || '').trim();
  const sphere = String(new FormData(form).get('sphere') || '').trim();
  if (source) params.set('source_filter', source);
  if (label) params.set('label', label);
  if (sphere) params.set('sphere', sphere);
  return params;
}

function renderGraphControls(section: HTMLElement, onApply: () => void): HTMLFormElement {
  const form = document.createElement('form');
  form.className = 'canvas-local-graph-controls';
  for (const [value, label] of RELATIONS) {
    const wrapper = document.createElement('label');
    wrapper.className = 'canvas-local-graph-check';
    const input = document.createElement('input');
    input.type = 'checkbox';
    input.name = 'graph-relation';
    input.value = value;
    input.checked = true;
    wrapper.append(input, document.createTextNode(label));
    form.appendChild(wrapper);
  }
  form.appendChild(graphTextInput('source_filter', 'source'));
  form.appendChild(graphTextInput('label', 'label'));
  const sphere = document.createElement('select');
  sphere.name = 'sphere';
  for (const value of ['work', 'private']) {
    const option = document.createElement('option');
    option.value = value;
    option.textContent = value;
    option.selected = value === activeSphere();
    sphere.appendChild(option);
  }
  form.appendChild(sphere);
  const apply = document.createElement('button');
  apply.type = 'submit';
  apply.textContent = 'Apply';
  form.appendChild(apply);
  form.addEventListener('submit', (ev) => {
    ev.preventDefault();
    onApply();
  });
  section.appendChild(form);
  return form;
}

function graphTextInput(name: string, placeholder: string): HTMLInputElement {
  const input = document.createElement('input');
  input.name = name;
  input.placeholder = placeholder;
  input.autocomplete = 'off';
  return input;
}

function renderGraphMessage(host: HTMLElement, text: string) {
  host.replaceChildren();
  appendGraphNote(host, text);
}

function appendGraphNote(host: HTMLElement, text: string) {
  const note = document.createElement('p');
  note.className = 'canvas-link-panel-empty';
  note.textContent = text;
  host.appendChild(note);
}

function nodeColor(type: string): string {
  if (type === 'item') return '#0f766e';
  if (type === 'actor') return '#7c2d12';
  if (type === 'label') return '#854d0e';
  if (type === 'source') return '#4338ca';
  if (type === 'artifact') return '#0369a1';
  return '#475569';
}

function graphPositions(nodes: GraphNode[], width: number, height: number): Map<string, { x: number; y: number }> {
  const positions = new Map<string, { x: number; y: number }>();
  const center = { x: width / 2, y: height / 2 };
  if (!nodes.length) return positions;
  positions.set(nodes[0].id, center);
  const radius = Math.max(70, Math.min(width, height) * 0.34);
  nodes.slice(1).forEach((node, index) => {
    const angle = (-Math.PI / 2) + (index * 2 * Math.PI / Math.max(1, nodes.length - 1));
    positions.set(node.id, {
      x: center.x + Math.cos(angle) * radius,
      y: center.y + Math.sin(angle) * radius,
    });
  });
  return positions;
}

function appendEdge(svg: SVGSVGElement, edge: GraphEdge, positions: Map<string, { x: number; y: number }>) {
  const from = positions.get(edge.source);
  const to = positions.get(edge.target);
  if (!from || !to) return;
  const line = document.createElementNS('http://www.w3.org/2000/svg', 'line');
  line.setAttribute('x1', String(from.x));
  line.setAttribute('y1', String(from.y));
  line.setAttribute('x2', String(to.x));
  line.setAttribute('y2', String(to.y));
  line.setAttribute('class', `canvas-local-graph-edge relation-${edge.relation}`);
  svg.appendChild(line);
}

function appendNode(svg: SVGSVGElement, node: GraphNode, point: { x: number; y: number }, renderCanvas: RenderCanvas) {
  const group = document.createElementNS('http://www.w3.org/2000/svg', 'g');
  group.setAttribute('class', `canvas-local-graph-node type-${node.type}`);
  group.setAttribute('role', 'button');
  group.setAttribute('tabindex', '0');
  group.setAttribute('aria-label', `${node.type}: ${node.label}`);
  group.dataset.nodeId = node.id;

  const circle = document.createElementNS('http://www.w3.org/2000/svg', 'circle');
  circle.setAttribute('cx', String(point.x));
  circle.setAttribute('cy', String(point.y));
  circle.setAttribute('r', node.type === 'note' ? '14' : '11');
  circle.setAttribute('fill', nodeColor(node.type));
  group.appendChild(circle);

  const text = document.createElementNS('http://www.w3.org/2000/svg', 'text');
  text.setAttribute('x', String(point.x));
  text.setAttribute('y', String(point.y + 27));
  text.textContent = node.label;
  group.appendChild(text);

  const open = () => openGraphNode(node, renderCanvas);
  group.addEventListener('click', open);
  group.addEventListener('keydown', (ev) => {
    if (ev.key === 'Enter' || ev.key === ' ') {
      ev.preventDefault();
      open();
    }
  });
  svg.appendChild(group);
}

function openGraphNode(node: GraphNode, renderCanvas: RenderCanvas) {
  if (node.file_url || node.path) {
    void openResolvedMarkdownLink({
      ok: true,
      kind: node.type === 'artifact' ? 'text' : node.type,
      file_url: node.file_url,
      vault_relative_path: node.path,
      resolved_path: node.path,
    }, renderCanvas);
    return;
  }
  renderCanvas({
    kind: 'text_artifact',
    event_id: `local-graph-node-${Date.now()}`,
    title: node.label,
    path: node.id,
    text: [`# ${node.label}`, '', `Type: ${node.type}`, node.source ? `Source: ${node.source}` : '', node.sphere ? `Sphere: ${node.sphere}` : ''].filter(Boolean).join('\n'),
  });
}

function renderGraph(host: HTMLElement, payload: GraphPayload, renderCanvas: RenderCanvas) {
  host.replaceChildren();
  if (!payload.ok) {
    renderGraphMessage(host, payload.error || 'graph unavailable');
    return;
  }
  const nodes = Array.isArray(payload.nodes) ? payload.nodes : [];
  const edges = Array.isArray(payload.edges) ? payload.edges : [];
  if (!nodes.length) {
    renderGraphMessage(host, 'no graph nodes');
    return;
  }
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('class', 'canvas-local-graph');
  svg.setAttribute('viewBox', '0 0 520 300');
  svg.setAttribute('role', 'img');
  svg.setAttribute('aria-label', `Local graph for ${payload.source_path || 'active note'}`);
  const positions = graphPositions(nodes, 520, 300);
  edges.forEach((edge) => appendEdge(svg, edge, positions));
  nodes.forEach((node) => {
    const point = positions.get(node.id);
    if (point) appendNode(svg, node, point, renderCanvas);
  });
  host.appendChild(svg);
  if (payload.truncated) appendGraphNote(host, 'graph truncated');
}

export async function renderLocalGraphSection(
  panel: HTMLElement,
  workspaceID: string,
  target: GraphTarget | string,
  renderCanvas: RenderCanvas,
) {
  const graphTarget = normalizeGraphTarget(target);
  const section = makeGraphSection(panel);
  const heading = section.querySelector('.canvas-link-panel-heading') || document.createElement('h4');
  heading.className = 'canvas-link-panel-heading';
  heading.textContent = 'Graph';
  section.replaceChildren(heading);
  let graphHost: HTMLElement;
  const form = renderGraphControls(section, () => {
    void loadGraph(workspaceID, graphTarget, form, graphHost, renderCanvas);
  });
  graphHost = document.createElement('div');
  graphHost.className = 'canvas-local-graph-host';
  section.appendChild(graphHost);
  await loadGraph(workspaceID, graphTarget, form, graphHost, renderCanvas);
}

async function loadGraph(workspaceID: string, target: GraphTarget, form: HTMLFormElement, host: HTMLElement, renderCanvas: RenderCanvas) {
  renderGraphMessage(host, 'Loading graph...');
  const params = graphQuery(target, form);
  const resp = await fetch(apiURL(`workspaces/${encodeURIComponent(workspaceID)}/graph?${params.toString()}`), { cache: 'no-store' });
  if (!resp.ok) {
    renderGraphMessage(host, (await resp.text()).trim() || `HTTP ${resp.status}`);
    return;
  }
  renderGraph(host, await resp.json() as GraphPayload, renderCanvas);
}
