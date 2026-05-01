import * as env from './app-env.js';
import * as context from './app-context.js';

const { apiURL } = env;
const { refs, state } = context;

const showStatus = (...args) => refs.showStatus(...args);
const loadItemSidebarView = (...args) => refs.loadItemSidebarView(...args);
const isEmailSidebarItem = (...args) => refs.isEmailSidebarItem(...args);
const defaultItemSidebarLaterVisibleAfter = (...args) => refs.defaultItemSidebarLaterVisibleAfter(...args);

const ITEM_SIDEBAR_GESTURE_UNDO_TIMEOUT_MS = 6000;

// performItemSidebarGesture executes a swipe gesture against the
// /api/items/{id}/gesture endpoint and stages an undo affordance for the user.
// `complete`, `drop`, and `defer` are direct one-shot calls; `delegate` falls
// through to the existing actor-picker, which then invokes this function with
// the chosen actor_id so undo state is captured the same way.
export async function performItemSidebarGesture(item, gesture, options: Record<string, any> = {}) {
  const itemID = Number(item?.id || 0);
  const normalizedGesture = String(gesture || '').trim().toLowerCase();
  if (itemID <= 0 || !normalizedGesture) return false;
  const body: Record<string, any> = { action: normalizedGesture };
  let actorName = '';
  if (normalizedGesture === 'defer') {
    body.follow_up_at = String(options.followUpAt || defaultItemSidebarLaterVisibleAfter(options.now || new Date()));
  } else if (normalizedGesture === 'delegate') {
    const actorID = Number(options.actorID || 0);
    if (actorID <= 0) return false;
    body.actor_id = actorID;
    actorName = String(options.actorName || '').trim();
    if (options.followUpAt) {
      body.follow_up_at = String(options.followUpAt);
    }
  } else if (normalizedGesture === 'drop' && options.dropUpstream) {
    body.drop_upstream = true;
  }
  try {
    const resp = await fetch(apiURL(`items/${encodeURIComponent(String(itemID))}/gesture`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    const payload = await resp.json();
    const data = payload && typeof payload === 'object' && payload.data && typeof payload.data === 'object' ? payload.data : payload;
    state.itemSidebarActiveItemID = itemID;
    await loadItemSidebarView(state.itemSidebarView);
    const undoSnapshot = (data && typeof data === 'object' && data.undo && typeof data.undo === 'object') ? data.undo : null;
    showItemSidebarGestureUndo(item, normalizedGesture, undoSnapshot, actorName);
    return true;
  } catch (err) {
    showStatus(`gesture failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function performItemSidebarGestureUndo(item, undoSnapshot) {
  const itemID = Number(item?.id || 0);
  if (itemID <= 0 || !undoSnapshot || typeof undoSnapshot !== 'object') return false;
  try {
    const resp = await fetch(apiURL(`items/${encodeURIComponent(String(itemID))}/gesture/undo`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ undo: undoSnapshot }),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    state.itemSidebarActiveItemID = itemID;
    await loadItemSidebarView(state.itemSidebarView);
    showStatus('action undone');
    return true;
  } catch (err) {
    showStatus(`undo failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

function gestureUndoLabel(gesture, item, actorName) {
  const normalized = String(gesture || '').trim().toLowerCase();
  if (normalized === 'complete') return isEmailSidebarItem(item) ? 'archived' : 'completed';
  if (normalized === 'drop') return 'dropped';
  if (normalized === 'defer') return 'deferred';
  if (normalized === 'delegate') return actorName ? `delegated to ${actorName}` : 'delegated';
  return 'updated';
}

export function showItemSidebarGestureUndo(item, gesture, undoSnapshot, actorName = '') {
  const label = gestureUndoLabel(gesture, item, actorName);
  if (!undoSnapshot) {
    showStatus(label);
    return;
  }
  const banner = ensureItemSidebarUndoBanner();
  banner.dataset.gesture = String(gesture || '');
  const text = banner.querySelector('.item-sidebar-undo-banner-text');
  if (text instanceof HTMLElement) {
    text.textContent = label;
  }
  const undoButton = banner.querySelector('.item-sidebar-undo-banner-action');
  if (undoButton instanceof HTMLButtonElement) {
    undoButton.onclick = () => {
      hideItemSidebarUndoBanner();
      void performItemSidebarGestureUndo(item, undoSnapshot);
    };
  }
  banner.classList.add('is-open');
  banner.setAttribute('aria-hidden', 'false');
  if (state.itemSidebarUndoTimer) {
    window.clearTimeout(state.itemSidebarUndoTimer);
  }
  state.itemSidebarUndoTimer = window.setTimeout(hideItemSidebarUndoBanner, ITEM_SIDEBAR_GESTURE_UNDO_TIMEOUT_MS);
  showStatus(`${label} (undo available)`);
}

export function hideItemSidebarUndoBanner() {
  const banner = document.getElementById('item-sidebar-undo-banner');
  if (!(banner instanceof HTMLElement)) return;
  banner.classList.remove('is-open');
  banner.setAttribute('aria-hidden', 'true');
  if (state.itemSidebarUndoTimer) {
    window.clearTimeout(state.itemSidebarUndoTimer);
    state.itemSidebarUndoTimer = 0;
  }
}

function ensureItemSidebarUndoBanner() {
  let banner = document.getElementById('item-sidebar-undo-banner');
  if (banner instanceof HTMLElement) return banner;
  banner = document.createElement('div');
  banner.id = 'item-sidebar-undo-banner';
  banner.className = 'item-sidebar-undo-banner';
  banner.setAttribute('role', 'status');
  banner.setAttribute('aria-live', 'polite');
  banner.setAttribute('aria-hidden', 'true');
  const text = document.createElement('span');
  text.className = 'item-sidebar-undo-banner-text';
  banner.appendChild(text);
  const action = document.createElement('button');
  action.type = 'button';
  action.className = 'item-sidebar-undo-banner-action';
  action.textContent = 'Undo';
  banner.appendChild(action);
  document.body.appendChild(banner);
  return banner;
}
