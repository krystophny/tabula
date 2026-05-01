import { apiURL } from './paths.js';
import { openResolvedMarkdownLink } from './canvas-markdown-links.js';

const SECTION_CLASS = 'canvas-brain-cards-section';
const BOARD_CLASS = 'canvas-brain-cards-board';
const CARD_CLASS = 'canvas-brain-card';
const HANDLE_CLASS = 'canvas-brain-card-resize';

const MIN_DIMENSION = 80;
const MIN_BOARD_WIDTH = 360;
const MIN_BOARD_HEIGHT = 240;
const PADDING = 24;

type RenderCanvas = (event: Record<string, unknown>) => void;

export interface BrainCanvasBinding {
  kind: string;
  id?: number;
  path?: string;
  url?: string;
  provider?: string;
  ref?: string;
}

export interface BrainCanvasCard {
  id: string;
  type?: string;
  x: number;
  y: number;
  width: number;
  height: number;
  color?: string;
  title: string;
  body?: string;
  open_url?: string;
  stale?: boolean;
  reason?: string;
  binding: BrainCanvasBinding;
}

interface BrainCanvasPayload {
  ok?: boolean;
  name?: string;
  cards?: BrainCanvasCard[];
  error?: string;
}

interface BrainCanvasOpenPayload {
  ok?: boolean;
  kind?: string;
  open_url?: string;
  title?: string;
  body?: string;
  binding?: BrainCanvasBinding;
  error?: string;
}

interface BrainCanvasContext {
  workspaceID: string;
  panelSourcePath: string;
  renderCanvas: RenderCanvas;
}

function brainCanvasSection(panel: HTMLElement): HTMLElement {
  const existing = panel.querySelector(`.${SECTION_CLASS}`);
  if (existing instanceof HTMLElement) return existing;
  const section = document.createElement('section');
  section.className = `canvas-link-panel-section ${SECTION_CLASS}`;
  const heading = document.createElement('h4');
  heading.className = 'canvas-link-panel-heading';
  heading.textContent = 'Canvas cards';
  section.appendChild(heading);
  panel.appendChild(section);
  return section;
}

function brainCanvasBoard(section: HTMLElement): HTMLElement {
  const existing = section.querySelector(`.${BOARD_CLASS}`);
  if (existing instanceof HTMLElement) return existing;
  const board = document.createElement('div');
  board.className = BOARD_CLASS;
  board.setAttribute('role', 'application');
  board.setAttribute('aria-label', 'Brain canvas cards');
  section.appendChild(board);
  return board;
}

function emptyMessage(text: string): HTMLElement {
  const message = document.createElement('p');
  message.className = 'canvas-link-panel-empty';
  message.textContent = text;
  return message;
}

function expandBoardForCard(board: HTMLElement, card: BrainCanvasCard) {
  const right = card.x + card.width + PADDING;
  const bottom = card.y + card.height + PADDING;
  const targetWidth = Math.max(MIN_BOARD_WIDTH, right);
  const targetHeight = Math.max(MIN_BOARD_HEIGHT, bottom);
  if (board.clientWidth < targetWidth) board.style.minWidth = `${Math.ceil(targetWidth)}px`;
  if (board.clientHeight < targetHeight) board.style.minHeight = `${Math.ceil(targetHeight)}px`;
}

function patchCanvasCard(workspaceID: string, cardID: string, body: Record<string, unknown>): Promise<Response> {
  return fetch(apiURL(`workspaces/${encodeURIComponent(workspaceID)}/brain-canvas/cards/${encodeURIComponent(cardID)}`), {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  });
}

function fetchCanvasOpen(workspaceID: string, cardID: string): Promise<BrainCanvasOpenPayload> {
  return fetch(apiURL(`workspaces/${encodeURIComponent(workspaceID)}/brain-canvas/cards/${encodeURIComponent(cardID)}/open`), {
    cache: 'no-store',
  }).then(async (resp) => {
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    return (await resp.json()) as BrainCanvasOpenPayload;
  });
}

function applyCardLayout(card: BrainCanvasCard, element: HTMLElement) {
  element.style.transform = `translate(${card.x}px, ${card.y}px)`;
  element.style.width = `${card.width}px`;
  element.style.height = `${card.height}px`;
}

function renderCardTitle(card: BrainCanvasCard, ctx: BrainCanvasContext, host: HTMLElement) {
  const title = document.createElement('div');
  title.className = 'canvas-brain-card-title';
  title.textContent = card.title;
  if (canEditTitle(card.binding.kind)) {
    title.contentEditable = 'plaintext-only';
    title.spellcheck = false;
    title.dataset.field = 'title';
    title.addEventListener('blur', () => {
      const next = title.textContent || '';
      if (next.trim() === card.title.trim()) return;
      void patchCanvasCard(ctx.workspaceID, card.id, { title: next })
        .then((resp) => {
          if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
          card.title = next;
        })
        .catch(() => {
          title.textContent = card.title;
        });
    });
    title.addEventListener('keydown', (event) => {
      if (event.key === 'Enter' && !event.shiftKey) {
        event.preventDefault();
        title.blur();
      }
    });
  }
  host.appendChild(title);
}

function canEditTitle(kind: string): boolean {
  return kind === 'artifact' || kind === 'item' || kind === 'link';
}

function renderCardBody(card: BrainCanvasCard, ctx: BrainCanvasContext, host: HTMLElement) {
  const body = document.createElement('div');
  body.className = 'canvas-brain-card-body';
  if (card.binding.kind === 'note') {
    const editor = document.createElement('textarea');
    editor.className = 'canvas-brain-card-text';
    editor.value = card.body ?? '';
    editor.spellcheck = false;
    editor.dataset.field = 'body';
    editor.addEventListener('blur', () => {
      if (editor.value === (card.body ?? '')) return;
      const next = editor.value;
      void patchCanvasCard(ctx.workspaceID, card.id, { body: next })
        .then((resp) => {
          if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
          card.body = next;
        })
        .catch(() => {
          editor.value = card.body ?? '';
        });
    });
    body.appendChild(editor);
  } else if (card.body) {
    const detail = document.createElement('div');
    detail.className = 'canvas-brain-card-detail';
    detail.textContent = card.body;
    body.appendChild(detail);
  }
  host.appendChild(body);
}

function renderCardFooter(card: BrainCanvasCard, ctx: BrainCanvasContext, host: HTMLElement) {
  const footer = document.createElement('div');
  footer.className = 'canvas-brain-card-footer';
  const kind = document.createElement('span');
  kind.className = 'canvas-brain-card-kind';
  kind.textContent = card.binding.kind;
  footer.appendChild(kind);
  const open = document.createElement('button');
  open.type = 'button';
  open.className = 'canvas-brain-card-open';
  open.textContent = 'Open';
  open.addEventListener('click', (event) => {
    event.preventDefault();
    void openBackingObject(card, ctx).catch((err) => {
      open.dataset.error = String((err as Error)?.message || err || 'open failed');
    });
  });
  footer.appendChild(open);
  host.appendChild(footer);
}

function openBackingObject(card: BrainCanvasCard, ctx: BrainCanvasContext): Promise<void> {
  return fetchCanvasOpen(ctx.workspaceID, card.id).then((open) => {
    if (!open || !open.ok) {
      throw new Error(open?.error || 'card backing object unavailable');
    }
    if (open.kind === 'note' && open.open_url) {
      const vaultPath = vaultRelativeNotePath(card, open);
      return openResolvedMarkdownLink(
        {
          ok: true,
          kind: 'text',
          file_url: open.open_url,
          vault_relative_path: vaultPath,
          resolved_path: vaultPath,
          source_path: ctx.panelSourcePath,
        },
        ctx.renderCanvas,
      );
    }
    if (open.open_url) {
      window.open(open.open_url, '_blank', 'noopener');
      return;
    }
    ctx.renderCanvas({
      event_id: `brain-canvas-${card.id}`,
      kind: 'text_artifact',
      title: open.title || card.title,
      text: composeFallbackText(open),
    });
  });
}

function vaultRelativeNotePath(card: BrainCanvasCard, open: BrainCanvasOpenPayload): string {
  const candidate = (open.binding?.path || card.binding.path || '').trim();
  if (!candidate) return '';
  if (/^brain(?:[\\/]|$)/i.test(candidate)) return candidate;
  return `brain/${candidate.replace(/^\/+/, '')}`;
}

function composeFallbackText(open: BrainCanvasOpenPayload): string {
  const lines: string[] = [];
  if (open.binding) {
    lines.push(`Type: ${open.binding.kind}`);
    if (open.binding.id) lines.push(`ID: ${open.binding.id}`);
    if (open.binding.provider) lines.push(`Provider: ${open.binding.provider}`);
    if (open.binding.ref) lines.push(`Ref: ${open.binding.ref}`);
  }
  if (open.body) {
    lines.push('');
    lines.push(open.body);
  }
  return lines.join('\n');
}

function renderCard(card: BrainCanvasCard, ctx: BrainCanvasContext, board: HTMLElement) {
  const element = document.createElement('article');
  element.className = CARD_CLASS;
  element.dataset.cardId = card.id;
  element.dataset.bindingKind = card.binding.kind;
  if (card.color) element.style.borderColor = card.color;
  if (card.stale) element.dataset.stale = '1';
  applyCardLayout(card, element);
  enableDrag(element, card, ctx, board);
  renderCardTitle(card, ctx, element);
  renderCardBody(card, ctx, element);
  renderCardFooter(card, ctx, element);
  enableResize(element, card, ctx, board);
  board.appendChild(element);
  expandBoardForCard(board, card);
}

function enableDrag(element: HTMLElement, card: BrainCanvasCard, ctx: BrainCanvasContext, board: HTMLElement) {
  element.addEventListener('pointerdown', (event) => {
    if (!(event.target instanceof HTMLElement)) return;
    if (event.target.closest('button, textarea, input, [contenteditable="plaintext-only"]')) return;
    if (event.target.classList.contains(HANDLE_CLASS)) return;
    event.preventDefault();
    const startX = event.clientX;
    const startY = event.clientY;
    const originX = card.x;
    const originY = card.y;
    element.classList.add('is-dragging');
    const move = (ev: PointerEvent) => {
      card.x = Math.max(0, originX + (ev.clientX - startX));
      card.y = Math.max(0, originY + (ev.clientY - startY));
      applyCardLayout(card, element);
      expandBoardForCard(board, card);
    };
    const up = () => {
      element.classList.remove('is-dragging');
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
      void patchCanvasCard(ctx.workspaceID, card.id, { x: card.x, y: card.y });
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  });
}

function enableResize(element: HTMLElement, card: BrainCanvasCard, ctx: BrainCanvasContext, board: HTMLElement) {
  const handle = document.createElement('span');
  handle.className = HANDLE_CLASS;
  handle.setAttribute('aria-label', 'Resize card');
  handle.addEventListener('pointerdown', (event) => {
    event.preventDefault();
    event.stopPropagation();
    const startX = event.clientX;
    const startY = event.clientY;
    const originW = card.width;
    const originH = card.height;
    element.classList.add('is-resizing');
    const move = (ev: PointerEvent) => {
      card.width = Math.max(MIN_DIMENSION, originW + (ev.clientX - startX));
      card.height = Math.max(MIN_DIMENSION, originH + (ev.clientY - startY));
      applyCardLayout(card, element);
      expandBoardForCard(board, card);
    };
    const up = () => {
      element.classList.remove('is-resizing');
      window.removeEventListener('pointermove', move);
      window.removeEventListener('pointerup', up);
      window.removeEventListener('pointercancel', up);
      void patchCanvasCard(ctx.workspaceID, card.id, { width: card.width, height: card.height });
    };
    window.addEventListener('pointermove', move);
    window.addEventListener('pointerup', up);
    window.addEventListener('pointercancel', up);
  });
  element.appendChild(handle);
}

export async function renderBrainCanvasCardsSection(
  panel: HTMLElement,
  workspaceID: string,
  panelSourcePath: string,
  renderCanvas: RenderCanvas,
): Promise<void> {
  const id = String(workspaceID || '').trim();
  if (!id) return;
  const section = brainCanvasSection(panel);
  const board = brainCanvasBoard(section);
  board.replaceChildren();
  let payload: BrainCanvasPayload;
  try {
    const resp = await fetch(apiURL(`workspaces/${encodeURIComponent(id)}/brain-canvas`), { cache: 'no-store' });
    if (!resp.ok) {
      board.appendChild(emptyMessage(`canvas unavailable: HTTP ${resp.status}`));
      return;
    }
    payload = (await resp.json()) as BrainCanvasPayload;
  } catch (err) {
    board.appendChild(emptyMessage(String((err as Error)?.message || err || 'canvas unavailable')));
    return;
  }
  if (!payload.ok) {
    board.appendChild(emptyMessage(payload.error || 'canvas unavailable'));
    return;
  }
  const cards = Array.isArray(payload.cards) ? payload.cards : [];
  if (!cards.length) {
    board.appendChild(emptyMessage('no canvas cards yet'));
    return;
  }
  const ctx: BrainCanvasContext = { workspaceID: id, panelSourcePath, renderCanvas };
  cards.forEach((card) => renderCard(card, ctx, board));
}
