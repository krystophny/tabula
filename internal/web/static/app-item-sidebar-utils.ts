import * as env from './app-env.js';
import * as context from './app-context.js';

const { marked, apiURL, wsURL, renderCanvas, clearCanvas, getLocationFromSelection, clearLineHighlight, escapeHtml, sanitizeHtml, getActiveArtifactTitle, getActiveTextEventId, getPreviousArtifactText, getUiState, setUiMode, showIndicatorMode, hideIndicator, showTextInput, hideTextInput, showOverlay, hideOverlay, updateOverlay, isOverlayVisible, isTextInputVisible, isRecording, setRecording, getInputAnchor, setInputAnchor, getAnchorFromPoint, buildContextPrefix, getLastInputPosition, setLastInputPosition, configureLiveSession, getLiveSessionSnapshot, handleLiveSessionMessage, isLiveSessionListenActive, LIVE_SESSION_HOTWORD_DEFAULT, LIVE_SESSION_MODE_DIALOGUE, LIVE_SESSION_MODE_MEETING, onLiveSessionTTSPlaybackComplete, cancelLiveSessionListen, startLiveSession, stopLiveSession, initHotword, startHotwordMonitor, stopHotwordMonitor, isHotwordActive, onHotwordDetected, setHotwordThreshold, setHotwordAudioContext, getPreRollAudio, getHotwordMicStream, initVAD, ensureVADLoaded, float32ToWav } = env;
const { refs, state, getState, isVoiceTurn, COMPANION_VIEW_PATH_PREFIX, COMPANION_TRANSCRIPT_VIEW_PATH, COMPANION_SUMMARY_VIEW_PATH, COMPANION_REFERENCES_VIEW_PATH, MEETING_TRANSCRIPT_LABEL, MEETING_SUMMARY_LABEL, MEETING_REFERENCES_LABEL, MEETING_SUMMARY_ITEMS_PANEL_ID, CHAT_CTRL_LONG_PRESS_MS, ARTIFACT_EDIT_LONG_TAP_MS, ITEM_SIDEBAR_VIEWS, ITEM_SIDEBAR_GESTURE_CANCEL_PX, ITEM_SIDEBAR_GESTURE_COMMIT_PX, ITEM_SIDEBAR_GESTURE_LONG_PX, ITEM_SIDEBAR_DEFAULT_LATER_HOUR_UTC, ITEM_SIDEBAR_MENU_ID, DEV_UI_RELOAD_POLL_MS, ASSISTANT_ACTIVITY_POLL_MS, CHAT_WS_STALE_THRESHOLD_MS, ACTIVE_TURN_NO_ID_CLEAR_GRACE_MS, ACTIVE_TURN_ACTIVITY_CLEAR_GRACE_MS, PROJECT_CHAT_MODEL_ALIASES, PROJECT_CHAT_MODEL_REASONING_EFFORTS, TTS_SILENT_STORAGE_KEY, YOLO_MODE_STORAGE_KEY, SOMEDAY_REVIEW_NUDGE_ENABLED_STORAGE_KEY, SOMEDAY_REVIEW_NUDGE_LAST_SHOWN_STORAGE_KEY, SOMEDAY_REVIEW_NUDGE_INTERVAL_MS, ACTIVE_PROJECT_STORAGE_KEY, LAST_VIEW_STORAGE_KEY, RUNTIME_RELOAD_CONTEXT_STORAGE_KEY, SIDEBAR_IMAGE_EXTENSIONS, PANEL_MOTION_WATCH_QUERIES, VOICE_LIFECYCLE, COMPANION_IDLE_SURFACES, COMPANION_RUNTIME_STATES, TOOL_PALETTE_MODES } = context;

const showStatus = (...args) => refs.showStatus(...args);
const loadItemSidebarView = (...args) => refs.loadItemSidebarView(...args);
const appendPlainMessage = (...args) => refs.appendPlainMessage(...args);
const applyCanvasArtifactEvent = (...args) => refs.applyCanvasArtifactEvent(...args);
const normalizeDisplayText = (...args) => refs.normalizeDisplayText(...args);
const normalizeActiveSphere = (...args) => refs.normalizeActiveSphere(...args);
const readSomedayReviewNudgeLastShownAt = (...args) => refs.readSomedayReviewNudgeLastShownAt(...args);
const persistSomedayReviewNudgeLastShownAt = (...args) => refs.persistSomedayReviewNudgeLastShownAt(...args);

function appendSphereQuery(path, sphere = state.activeSphere, allSpheres = false) {
  if (allSpheres) {
    return String(path || '');
  }
  const cleanSphere = normalizeActiveSphere(sphere);
  const separator = String(path || '').includes('?') ? '&' : '?';
  return `${path}${separator}sphere=${encodeURIComponent(cleanSphere)}`;
}

export function defaultItemSidebarCounts() {
  return {
    inbox: 0,
    next: 0,
    waiting: 0,
    deferred: 0,
    someday: 0,
    review: 0,
    done: 0,
  };
}

export function normalizeItemSidebarView(rawView) {
  const value = String(rawView || '').trim().toLowerCase();
  if (ITEM_SIDEBAR_VIEWS.includes(value)) return value;
  return 'inbox';
}

export const SIDEBAR_SECTION_IDS = ['project_items', 'people', 'drift', 'dedup', 'recent_meetings'];

export function normalizeItemSidebarFilters(rawFilters = null) {
  const filters = rawFilters && typeof rawFilters === 'object' ? rawFilters : {};
  const source = String(filters.source || '').trim().toLowerCase();
  const sourceContainer = String(filters.source_container || '').trim();
  const labelIDRaw = Number(filters.label_id || 0);
  const labelID = Number.isFinite(labelIDRaw) && labelIDRaw > 0 ? Math.trunc(labelIDRaw) : null;
  const actorIDRaw = Number(filters.actor_id || 0);
  const actorID = Number.isFinite(actorIDRaw) && actorIDRaw > 0 ? Math.trunc(actorIDRaw) : null;
  const projectItemIDRaw = Number(filters.project_item_id || 0);
  const projectItemID = Number.isFinite(projectItemIDRaw) && projectItemIDRaw > 0 ? Math.trunc(projectItemIDRaw) : null;
  const allSpheres = filters.all_spheres === true;
  const workspaceRaw = filters.workspace_id;
  const workspaceUnassigned = String(workspaceRaw || '').trim().toLowerCase() === 'null'
    || filters.workspace_unassigned === true;
  let workspaceID = null;
  if (!workspaceUnassigned && Number.isFinite(Number(workspaceRaw)) && Number(workspaceRaw) > 0) {
    workspaceID = Math.trunc(Number(workspaceRaw));
  }
  const sectionRaw = String(filters.section || '').trim().toLowerCase();
  const section = SIDEBAR_SECTION_IDS.includes(sectionRaw) ? sectionRaw : '';
  const dueBefore = String(filters.due_before || '').trim();
  const dueAfter = String(filters.due_after || '').trim();
  const followUpBefore = String(filters.follow_up_before || '').trim();
  const followUpAfter = String(filters.follow_up_after || '').trim();
  return {
    all_spheres: allSpheres,
    source,
    source_container: sourceContainer,
    workspace_id: workspaceID,
    label_id: labelID,
    actor_id: actorID,
    project_item_id: projectItemID,
    workspace_unassigned: workspaceUnassigned,
    section,
    due_before: dueBefore,
    due_after: dueAfter,
    follow_up_before: followUpBefore,
    follow_up_after: followUpAfter,
  };
}

function appendItemSidebarQueryParam(path, key, value) {
  if (value === null || value === undefined || value === '') return path;
  const separator = String(path || '').includes('?') ? '&' : '?';
  return `${path}${separator}${key}=${encodeURIComponent(String(value))}`;
}

function appendItemSidebarFilterQuery(path, filters = state.itemSidebarFilters) {
  const normalized = normalizeItemSidebarFilters(filters);
  let nextPath = String(path || '');
  nextPath = appendItemSidebarQueryParam(nextPath, 'source', normalized.source);
  nextPath = appendItemSidebarQueryParam(nextPath, 'source_container', normalized.source_container);
  if (normalized.workspace_unassigned) {
    nextPath = appendItemSidebarQueryParam(nextPath, 'workspace_id', 'null');
  } else if (Number.isFinite(normalized.workspace_id) && normalized.workspace_id > 0) {
    nextPath = appendItemSidebarQueryParam(nextPath, 'workspace_id', normalized.workspace_id);
  }
  if (Number.isFinite(normalized.label_id) && normalized.label_id > 0) {
    nextPath = appendItemSidebarQueryParam(nextPath, 'label_id', normalized.label_id);
  }
  if (Number.isFinite(normalized.actor_id) && normalized.actor_id > 0) {
    nextPath = appendItemSidebarQueryParam(nextPath, 'actor_id', normalized.actor_id);
  }
  if (Number.isFinite(normalized.project_item_id) && normalized.project_item_id > 0) {
    nextPath = appendItemSidebarQueryParam(nextPath, 'project_item_id', normalized.project_item_id);
  }
  nextPath = appendItemSidebarQueryParam(nextPath, 'section', normalized.section);
  nextPath = appendItemSidebarQueryParam(nextPath, 'due_before', normalized.due_before);
  nextPath = appendItemSidebarQueryParam(nextPath, 'due_after', normalized.due_after);
  nextPath = appendItemSidebarQueryParam(nextPath, 'follow_up_before', normalized.follow_up_before);
  nextPath = appendItemSidebarQueryParam(nextPath, 'follow_up_after', normalized.follow_up_after);
  return nextPath;
}

export function itemSidebarEndpoint(view, filters = state.itemSidebarFilters) {
  const normalized = normalizeItemSidebarView(view);
  const normalizedFilters = normalizeItemSidebarFilters(filters);
  if (normalized === 'done') return appendItemSidebarFilterQuery(appendSphereQuery(`items/${normalized}?limit=50`, state.activeSphere, normalizedFilters.all_spheres), normalizedFilters);
  return appendItemSidebarFilterQuery(appendSphereQuery(`items/${normalized}`, state.activeSphere, normalizedFilters.all_spheres), normalizedFilters);
}

export function itemSidebarCountsEndpoint(filters = state.itemSidebarFilters) {
  const normalizedFilters = normalizeItemSidebarFilters(filters);
  return appendItemSidebarFilterQuery(appendSphereQuery('items/counts', state.activeSphere, normalizedFilters.all_spheres), normalizedFilters);
}

export function normalizeItemSidebarCounts(rawCounts) {
  const counts = defaultItemSidebarCounts();
  if (!rawCounts || typeof rawCounts !== 'object') return counts;
  ITEM_SIDEBAR_VIEWS.forEach((view) => {
    const value = Number(rawCounts[view] ?? 0);
    counts[view] = Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0;
  });
  return counts;
}

export function setInboxTriggerCount(count) {
  const edgeLeftTap = document.getElementById('edge-left-tap');
  if (!(edgeLeftTap instanceof HTMLElement)) return;
  const normalizedCount = Number.isFinite(Number(count)) && Number(count) > 0
    ? Math.trunc(Number(count))
    : 0;
  if (normalizedCount > 0) {
    edgeLeftTap.dataset.inboxCount = String(normalizedCount);
    edgeLeftTap.classList.add('has-inbox-count');
    return;
  }
  edgeLeftTap.dataset.inboxCount = '';
  edgeLeftTap.classList.remove('has-inbox-count');
}

export function defaultItemSidebarSectionCounts() {
  return {
    project_items_open: 0,
    people_open: 0,
    drift_review: 0,
    dedup_review: 0,
    recent_meetings: 0,
  };
}

export function normalizeItemSidebarSectionCounts(rawSections) {
  const out = defaultItemSidebarSectionCounts();
  if (!rawSections || typeof rawSections !== 'object') return out;
  const fields = ['project_items_open', 'people_open', 'drift_review', 'dedup_review', 'recent_meetings'];
  for (const field of fields) {
    const raw = Number(rawSections[field] ?? 0);
    if (Number.isFinite(raw) && raw > 0) {
      out[field] = Math.trunc(raw);
    }
  }
  return out;
}

export function applyItemSidebarCounts(rawCounts, rawSections = null) {
  state.itemSidebarCounts = normalizeItemSidebarCounts(rawCounts);
  state.itemSidebarSectionCounts = normalizeItemSidebarSectionCounts(rawSections);
  setInboxTriggerCount(state.itemSidebarCounts.inbox);
  maybeShowSomedayReviewNudge();
}

export function maybeShowSomedayReviewNudge() {
  if (!state.somedayReviewNudgeEnabled) return false;
  const somedayCount = Number(state.itemSidebarCounts?.someday || 0);
  if (somedayCount <= 0) return false;
  if (state.fileSidebarMode === 'items' && state.itemSidebarView === 'someday' && state.prReviewDrawerOpen) {
    persistSomedayReviewNudgeLastShownAt();
    return false;
  }
  const lastShownAt = readSomedayReviewNudgeLastShownAt();
  if (lastShownAt > 0 && (Date.now() - lastShownAt) < SOMEDAY_REVIEW_NUDGE_INTERVAL_MS) {
    return false;
  }
  const suffix = somedayCount === 1 ? '' : 's';
  appendPlainMessage('system', `You have ${somedayCount} item${suffix} in someday. Say "review my someday list" to open them.`);
  showStatus('review someday list');
  persistSomedayReviewNudgeLastShownAt();
  return true;
}

export async function refreshItemSidebarCounts() {
  const workspaceID = String(state.activeWorkspaceId || '').trim();
  if (!workspaceID) {
    applyItemSidebarCounts(defaultItemSidebarCounts(), null);
    return false;
  }
  const resp = await fetch(apiURL(itemSidebarCountsEndpoint(state.itemSidebarFilters)), { cache: 'no-store' });
  if (!resp.ok) {
    const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
    throw new Error(detail);
  }
  const payload = await resp.json();
  if (workspaceID !== String(state.activeWorkspaceId || '').trim()) return false;
  applyItemSidebarCounts(payload?.counts, payload?.sections);
  return true;
}

export function isEmailSidebarItem(item) {
  return ['email', 'email_thread', 'email_draft'].includes(String(item?.artifact_kind || '').trim().toLowerCase());
}

export function isGitHubPRSidebarItem(item) {
  return String(item?.artifact_kind || '').trim().toLowerCase() === 'github_pr';
}

export function itemSidebarActionLabel(action, item = null) {
  const normalized = String(action || '').trim().toLowerCase();
  if (normalized === 'done') {
    return isEmailSidebarItem(item) ? 'Archive' : 'Done';
  }
  if (normalized === 'inbox') return 'Back to Inbox';
  if (normalized === 'next') return 'Clarify...';
  if (normalized === 'delete') return 'Delete';
  if (normalized === 'delegate') return 'Delegate';
  if (normalized === 'later') return 'Later';
  if (normalized === 'someday') return 'Someday';
  return '';
}

export function itemSidebarStatusText(action, item = null, actorName = '') {
  const normalized = String(action || '').trim().toLowerCase();
  const label = itemSidebarActionLabel(action, item).toLowerCase();
  if (normalized === 'delegate' && String(actorName || '').trim()) {
    return `delegated to ${String(actorName || '').trim()}`;
  }
  if (!label) return 'updated';
  if (label === 'back to inbox') return 'returned to inbox';
  if (normalized === 'next') return 'moved to next';
  if (normalized === 'later') return 'moved to later';
  if (normalized === 'someday') return 'moved to someday';
  return `${label}d`;
}

export function defaultItemSidebarLaterVisibleAfter(now = new Date()) {
  const base = new Date(now);
  base.setUTCDate(base.getUTCDate() + 1);
  base.setUTCHours(ITEM_SIDEBAR_DEFAULT_LATER_HOUR_UTC, 0, 0, 0);
  return base.toISOString();
}

export function itemSidebarGestureAction(dx) {
  const offset = Number(dx) || 0;
  if (offset >= ITEM_SIDEBAR_GESTURE_LONG_PX) {
    return { action: 'delete', label: 'Delete' };
  }
  if (offset >= ITEM_SIDEBAR_GESTURE_COMMIT_PX) {
    return { action: 'done', label: 'Done' };
  }
  if (offset <= -ITEM_SIDEBAR_GESTURE_LONG_PX) {
    return { action: 'later', label: 'Later' };
  }
  if (offset <= -ITEM_SIDEBAR_GESTURE_COMMIT_PX) {
    return { action: 'delegate', label: 'Delegate' };
  }
  return null;
}

export function itemSidebarMenuEl() {
  let menu = document.getElementById(ITEM_SIDEBAR_MENU_ID);
  if (menu instanceof HTMLElement) return menu;
  menu = document.createElement('div');
  menu.id = ITEM_SIDEBAR_MENU_ID;
  menu.className = 'item-sidebar-menu';
  menu.setAttribute('role', 'menu');
  menu.setAttribute('aria-hidden', 'true');
  document.body.appendChild(menu);
  return menu;
}

export function hideItemSidebarMenu() {
  const menu = document.getElementById(ITEM_SIDEBAR_MENU_ID);
  if (!(menu instanceof HTMLElement)) return;
  menu.innerHTML = '';
  menu.classList.remove('is-open');
  menu.setAttribute('aria-hidden', 'true');
  state.itemSidebarMenuOpen = false;
}

export function positionItemSidebarMenu(menu, x, y) {
  if (!(menu instanceof HTMLElement)) return;
  menu.style.left = '0px';
  menu.style.top = '0px';
  menu.style.maxHeight = `${Math.max(160, window.innerHeight - 24)}px`;
  menu.classList.add('is-open');
  menu.setAttribute('aria-hidden', 'false');
  const rect = menu.getBoundingClientRect();
  const maxLeft = Math.max(12, window.innerWidth - rect.width - 12);
  const maxTop = Math.max(12, window.innerHeight - rect.height - 12);
  const left = Math.min(Math.max(12, Number(x) || 12), maxLeft);
  const top = Math.min(Math.max(12, Number(y) || 12), maxTop);
  menu.style.left = `${left}px`;
  menu.style.top = `${top}px`;
  state.itemSidebarMenuOpen = true;
}

export function showItemSidebarMenu(entries, x, y) {
  const items = Array.isArray(entries) ? entries.filter((entry) => entry && entry.label) : [];
  if (items.length === 0) {
    hideItemSidebarMenu();
    return;
  }
  const menu = itemSidebarMenuEl();
  menu.innerHTML = '';
  items.forEach((entry) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'item-sidebar-menu-item';
    if (entry.action) {
      button.dataset.action = String(entry.action);
    }
    if (entry.disabled) {
      button.disabled = true;
      button.classList.add('is-disabled');
    }
    button.textContent = String(entry.label || '');
    button.addEventListener('click', (event) => {
      event.preventDefault();
      if (entry.disabled) return;
      const handler = typeof entry.onClick === 'function' ? entry.onClick : null;
      hideItemSidebarMenu();
      if (handler) {
        void Promise.resolve(handler());
      }
    });
    menu.appendChild(button);
  });
  positionItemSidebarMenu(menu, x, y);
}

export async function fetchItemSidebarActors() {
  const resp = await fetch(apiURL('actors'), { cache: 'no-store' });
  if (!resp.ok) {
    const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
    throw new Error(detail);
  }
  const payload = await resp.json();
  const actors = Array.isArray(payload?.actors) ? payload.actors : [];
  return actors
    .map((actor) => ({
      id: Number(actor?.id || 0),
      name: String(actor?.name || '').trim(),
    }))
    .filter((actor) => actor.id > 0 && actor.name);
}

export async function fetchItemSidebarWorkspaces() {
  const resp = await fetch(apiURL(appendSphereQuery('workspaces')), { cache: 'no-store' });
  if (!resp.ok) {
    const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
    throw new Error(detail);
  }
  const payload = await resp.json();
  const workspaces = Array.isArray(payload?.workspaces) ? payload.workspaces : [];
  return workspaces
    .map((workspace) => ({
      id: Number(workspace?.id || 0),
      name: String(workspace?.name || '').trim(),
    }))
    .filter((workspace) => workspace.id > 0 && workspace.name);
}

export async function fetchItemSidebarProjectItems() {
  const resp = await fetch(apiURL(appendSphereQuery('items?section=project_items')), { cache: 'no-store' });
  if (!resp.ok) {
    const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
    throw new Error(detail);
  }
  const payload = await resp.json();
  const items = Array.isArray(payload?.items) ? payload.items : [];
  return items
    .map((item) => ({
      id: Number(item?.id || 0),
      title: String(item?.title || '').trim(),
      state: String(item?.state || '').trim().toLowerCase(),
    }))
    .filter((item) => item.id > 0 && item.title && item.state !== 'done');
}

export async function fetchItemSidebarLabels() {
  const resp = await fetch(apiURL('labels'), { cache: 'no-store' });
  if (!resp.ok) {
    const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
    throw new Error(detail);
  }
  const payload = await resp.json();
  const labels = Array.isArray(payload?.labels) ? payload.labels : [];
  const normalized = labels
    .map((entry) => ({
      id: Number(entry?.id || 0),
      name: String(entry?.name || '').trim(),
      parent_id: Number(entry?.parent_id || 0) > 0 ? Number(entry.parent_id) : null,
    }))
    .filter((entry) => entry.id > 0 && entry.name);
  const parentByID = new Map(normalized.map((entry) => [entry.id, entry.parent_id]));
  const depthFor = (id, seen = new Set()) => {
    if (seen.has(id)) return 0;
    seen.add(id);
    const parentID = parentByID.get(id);
    if (!Number.isFinite(parentID) || Number(parentID) <= 0) return 0;
    return depthFor(Number(parentID), seen) + 1;
  };
  const withDepth = normalized.map((entry) => ({
    ...entry,
    depth: depthFor(entry.id),
  }));
  const childrenByParent = withDepth.reduce((acc, entry) => {
    const parentID = Number(entry?.parent_id || 0);
    if (!acc.has(parentID)) acc.set(parentID, []);
    acc.get(parentID).push(entry);
    return acc;
  }, new Map());
  childrenByParent.forEach((entries) => {
    entries.sort((left, right) => {
      const nameCompare = String(left?.name || '').localeCompare(String(right?.name || ''), undefined, { sensitivity: 'base' });
      if (nameCompare !== 0) return nameCompare;
      return Number(left?.id || 0) - Number(right?.id || 0);
    });
  });
  const ordered = [];
  const visit = (parentID = 0) => {
    const children = childrenByParent.get(parentID) || [];
    children.forEach((entry) => {
      ordered.push(entry);
      visit(entry.id);
    });
  };
  visit(0);
  return ordered;
}

export async function applyItemSidebarLabelFilter(labelID = 0, labelName = '') {
  const normalizedLabelID = Number.isFinite(Number(labelID)) && Number(labelID) > 0
    ? Math.trunc(Number(labelID))
    : 0;
  state.itemSidebarLabelName = normalizedLabelID > 0
    ? (String(labelName || '').trim() || `Label ${normalizedLabelID}`)
    : '';
  const nextFilters = {
    ...state.itemSidebarFilters,
    label_id: normalizedLabelID > 0 ? normalizedLabelID : null,
  };
  await loadItemSidebarView(state.itemSidebarView, nextFilters);
  showStatus(normalizedLabelID > 0
    ? `label filter: ${state.itemSidebarLabelName}`
    : 'label filter cleared');
  return true;
}

export async function showItemSidebarLabelFilterMenu(x, y) {
  try {
    const labels = await fetchItemSidebarLabels();
    const currentLabelID = Number(state.itemSidebarFilters?.label_id || 0);
    const entries = [{
      label: currentLabelID > 0 ? 'All labels' : 'All labels (current)',
      action: 'clear_label_filter',
      onClick: () => applyItemSidebarLabelFilter(0, ''),
    }];
    labels.forEach((entry) => {
      const prefix = entry.depth > 0 ? `${'  '.repeat(entry.depth)}↳ ` : '';
      entries.push({
        label: entry.id === currentLabelID ? `${prefix}${entry.name} (current)` : `${prefix}${entry.name}`,
        action: 'set_label_filter',
        onClick: () => applyItemSidebarLabelFilter(entry.id, entry.name),
      });
    });
    showItemSidebarMenu(entries, x, y);
    return true;
  } catch (err) {
    showStatus(`label filter failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function performItemSidebarSphereUpdate(item, nextSphere) {
  const itemID = Number(item?.id || 0);
  const sphere = normalizeActiveSphere(nextSphere);
  if (itemID <= 0 || !sphere) return false;
  try {
    const resp = await fetch(apiURL(`items/${encodeURIComponent(String(itemID))}`), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ sphere }),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    state.itemSidebarActiveItemID = itemID;
    await loadItemSidebarView(state.itemSidebarView);
    showStatus(`moved to ${sphere}`);
    return true;
  } catch (err) {
    showStatus(`sphere move failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function performItemSidebarReviewDispatch(item, target, value = '') {
  const itemID = Number(item?.id || 0);
  const cleanTarget = String(target || '').trim().toLowerCase();
  if (itemID <= 0 || !cleanTarget) return false;
  const body: Record<string, any> = { target: cleanTarget };
  let label = cleanTarget;
  if (cleanTarget === 'github') {
    const reviewer = String(value || window.prompt('GitHub reviewer', '') || '').trim();
    if (!reviewer) return false;
    body.reviewer = reviewer;
    label = `github:${reviewer}`;
  } else if (cleanTarget === 'email') {
    const email = String(value || window.prompt('Reviewer email', '') || '').trim();
    if (!email) return false;
    body.email = email;
    label = `email:${email}`;
  }
  try {
    const resp = await fetch(apiURL(`items/${encodeURIComponent(String(itemID))}/dispatch-review`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    state.itemSidebarActiveItemID = itemID;
    await loadItemSidebarView(state.itemSidebarView);
    showStatus(`review dispatched: ${label}`);
    return true;
  } catch (err) {
    showStatus(`review dispatch failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function performItemSidebarWorkspaceUpdate(item, workspaceID = null, workspaceName = '') {
  const itemID = Number(item?.id || 0);
  if (itemID <= 0) return false;
  const body = { workspace_id: workspaceID };
  try {
    const resp = await fetch(apiURL(`items/${encodeURIComponent(String(itemID))}/workspace`), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    const payload = await resp.json();
    state.itemSidebarActiveItemID = itemID;
    await loadItemSidebarView(state.itemSidebarView);
    const warning = String(payload?.warning || '').trim();
    const label = workspaceID ? `workspace set to ${String(workspaceName || '').trim() || 'selected workspace'}` : 'workspace cleared';
    showStatus(warning ? `${label}. ${warning}` : label);
    return true;
  } catch (err) {
    showStatus(`workspace picker failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function performItemSidebarProjectItemLink(item, projectItemID, projectTitle = '', role = 'next_action') {
  const itemID = Number(item?.id || 0);
  const parentID = Number(projectItemID || 0);
  if (itemID <= 0 || parentID <= 0 || itemID === parentID) return false;
  try {
    const resp = await fetch(apiURL(`items/${encodeURIComponent(String(itemID))}/project-item-link`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        project_item_id: parentID,
        role: String(role || 'next_action').trim() || 'next_action',
      }),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    state.itemSidebarActiveItemID = itemID;
    await loadItemSidebarView(state.itemSidebarView);
    showStatus(`linked to ${String(projectTitle || '').trim() || 'project item'}`);
    return true;
  } catch (err) {
    showStatus(`project link failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function showItemSidebarProjectItemMenu(item, x, y) {
  try {
    const itemID = Number(item?.id || 0);
    const projectItems = (await fetchItemSidebarProjectItems()).filter((entry) => entry.id !== itemID);
    if (projectItems.length === 0) {
      showStatus('no project items available');
      return false;
    }
    showItemSidebarMenu(projectItems.map((projectItem) => ({
      label: projectItem.title,
      action: 'link_project_item',
      onClick: () => performItemSidebarProjectItemLink(item, projectItem.id, projectItem.title),
    })), x, y);
    return true;
  } catch (err) {
    showStatus(`project picker failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export function sourceURLForItem(item) {
  const direct = String(item?.source_url || item?.url || '').trim();
  if (/^https?:\/\//i.test(direct)) return direct;
  const ref = String(item?.source_ref || '').trim();
  if (/^https?:\/\//i.test(ref)) return ref;
  return '';
}

export function openItemSource(item) {
  const url = sourceURLForItem(item);
  if (!url) {
    showStatus('source link unavailable');
    return false;
  }
  window.open(url, '_blank', 'noopener,noreferrer');
  showStatus('source opened');
  return true;
}

export async function materializeSidebarItemArtifact(item) {
  const artifactID = Number(item?.artifact_id || 0);
  if (artifactID <= 0) return false;
  const workspaceID = Number(item?.workspace_id || 0);
  try {
    const body: Record<string, any> = {};
    if (workspaceID > 0) {
      body.workspace_id = workspaceID;
    }
    const resp = await fetch(apiURL(`artifacts/${encodeURIComponent(String(artifactID))}/materialize`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    const payload = await resp.json();
    const relativePath = String(payload?.data?.relative_path || payload?.relative_path || '').trim();
    showStatus(relativePath ? `materialized: ${relativePath}` : 'materialized');
    return true;
  } catch (err) {
    showStatus(`materialize failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function performItemSidebarTriage(item, action, options: Record<string, any> = {}) {
  const itemID = Number(item?.id || 0);
  if (itemID <= 0) return false;
  const normalizedAction = String(action || '').trim().toLowerCase();
  if (!normalizedAction) return false;
  const body: Record<string, any> = { action: normalizedAction };
  let actorName = '';
  if (normalizedAction === 'later') {
    body.visible_after = defaultItemSidebarLaterVisibleAfter(options.now || new Date());
  } else if (normalizedAction === 'delegate') {
    const actorID = Number(options.actorID || 0);
    if (actorID <= 0) return false;
    body.actor_id = actorID;
    actorName = String(options.actorName || '').trim();
  }
  try {
    const resp = await fetch(apiURL(`items/${encodeURIComponent(String(itemID))}/triage`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    if (normalizedAction === 'delete') {
      if (state.itemSidebarActiveItemID === itemID) {
        state.itemSidebarActiveItemID = 0;
      }
    } else {
      state.itemSidebarActiveItemID = itemID;
    }
    await loadItemSidebarView(state.itemSidebarView);
    showStatus(itemSidebarStatusText(normalizedAction, item, actorName));
    return true;
  } catch (err) {
    showStatus(`item action failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function performItemSidebarStateUpdate(item, nextState) {
  const itemID = Number(item?.id || 0);
  const normalizedState = normalizeItemSidebarView(nextState);
  if (itemID <= 0 || !normalizedState) return false;
  try {
    const resp = await fetch(apiURL(`items/${encodeURIComponent(String(itemID))}/state`), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ state: normalizedState }),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    state.itemSidebarActiveItemID = itemID;
    const targetView = state.itemSidebarView;
    state.itemSidebarView = targetView;
    await loadItemSidebarView(targetView);
    showStatus(itemSidebarStatusText(normalizedState, item));
    return true;
  } catch (err) {
    showStatus(`item update failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function showItemSidebarDelegateMenu(item, x, y) {
  try {
    const actors = await fetchItemSidebarActors();
    if (actors.length === 0) {
      showStatus('no actors available');
      return false;
    }
    showItemSidebarMenu(
      actors.map((actor) => ({
        label: actor.name,
        action: 'delegate',
        onClick: () => performItemSidebarTriage(item, 'delegate', {
          actorID: actor.id,
          actorName: actor.name,
        }),
      })),
      x,
      y,
    );
    return true;
  } catch (err) {
    showStatus(`delegate picker failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function showItemSidebarWorkspaceMenu(item, x, y) {
  try {
    const workspaces = await fetchItemSidebarWorkspaces();
    if (workspaces.length === 0) {
      showStatus('no workspaces available');
      return false;
    }
    const currentWorkspaceID = Number(item?.workspace_id || 0);
    const entries = [];
    if (currentWorkspaceID > 0) {
      entries.push({
        label: 'Clear workspace',
        action: 'clear_workspace',
        onClick: () => performItemSidebarWorkspaceUpdate(item, null, ''),
      });
    }
    workspaces.forEach((workspace) => {
      entries.push({
        label: workspace.id === currentWorkspaceID ? `${workspace.name} (current)` : workspace.name,
        action: 'reassign_workspace',
        onClick: () => performItemSidebarWorkspaceUpdate(item, workspace.id, workspace.name),
      });
    });
    showItemSidebarMenu(entries, x, y);
    return true;
  } catch (err) {
    showStatus(`workspace picker failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export function showItemSidebarReviewMenu(item, x, y) {
  if (!isGitHubPRSidebarItem(item)) {
    showStatus('review dispatch only works for PR items');
    return false;
  }
  showItemSidebarMenu([
    {
      label: 'Agent Review',
      action: 'review_agent',
      onClick: () => performItemSidebarReviewDispatch(item, 'agent'),
    },
    {
      label: 'GitHub Reviewer...',
      action: 'review_github',
      onClick: () => performItemSidebarReviewDispatch(item, 'github'),
    },
    {
      label: 'Email Reviewer...',
      action: 'review_email',
      onClick: () => performItemSidebarReviewDispatch(item, 'email'),
    },
  ], x, y);
  return true;
}

function itemSidebarReviewEntries(item, x, y) {
  if (!isGitHubPRSidebarItem(item)) return [];
  return [{
    label: 'Review...',
    action: 'review_dispatch',
    onClick: () => showItemSidebarReviewMenu(item, x, y),
  }];
}

function itemSidebarSourceEntries(item) {
  const sourceLabel = normalizeDisplayText(item?.source || '').toLowerCase();
  if (!sourceLabel) return [];
  const sourceAction = sourceURLForItem(item)
    ? {
        label: 'Open source',
        action: 'open_source',
        onClick: () => openItemSource(item),
      }
    : {
        label: 'Open source unavailable',
        action: 'open_source_unavailable',
        disabled: true,
      };
  return [{
    label: `Source: ${sourceLabel}`,
    action: 'source_authority',
    disabled: true,
  }, sourceAction];
}

function itemSidebarMaterializeEntries(item) {
  if (Number(item?.artifact_id || 0) <= 0) return [];
  return [{
    label: 'Materialize artifact...',
    action: 'materialize_artifact',
    onClick: () => materializeSidebarItemArtifact(item),
  }];
}

function itemSidebarSphereEntries(item) {
  const nextSphere = normalizeActiveSphere(item?.sphere) === 'work' ? 'private' : 'work';
  if (Number(item?.workspace_id || 0) > 0) return [];
  return [{
    label: nextSphere === 'work' ? 'Move to Work' : 'Move to Private',
    action: 'move_sphere',
    onClick: () => performItemSidebarSphereUpdate(item, nextSphere),
  }];
}

function itemSidebarSharedEntries(item, x, y) {
  return [
    {
      label: 'Workspace...',
      action: 'workspace',
      onClick: () => showItemSidebarWorkspaceMenu(item, x, y),
    },
    {
      label: 'Project item...',
      action: 'project_item',
      onClick: () => showItemSidebarProjectItemMenu(item, x, y),
    },
    ...itemSidebarSourceEntries(item),
    ...itemSidebarMaterializeEntries(item),
    ...itemSidebarSphereEntries(item),
  ];
}

function itemSidebarTriageEntry(item, action, onClick = null) {
  return {
    label: itemSidebarActionLabel(action, item),
    action,
    onClick: onClick || (() => performItemSidebarTriage(item, action)),
  };
}

function itemSidebarStateEntries(item, itemState, x, y) {
  const reopenEntry = {
    label: itemSidebarActionLabel('inbox', item),
    action: 'inbox',
    onClick: () => performItemSidebarStateUpdate(item, 'inbox'),
  };
  const shared = itemSidebarSharedEntries(item, x, y);
  const review = itemSidebarReviewEntries(item, x, y);
  if (itemState === 'done') {
    return [
      reopenEntry,
      ...review,
      ...shared,
      itemSidebarTriageEntry(item, 'delete'),
    ];
  }
  if (itemState === 'someday') {
    return [
      reopenEntry,
      ...review,
      itemSidebarTriageEntry(item, 'done'),
      ...shared,
      itemSidebarTriageEntry(item, 'delete'),
    ];
  }
  if (itemState === 'waiting' || itemState === 'deferred' || itemState === 'review') {
    return [
      reopenEntry,
      ...review,
      itemSidebarTriageEntry(item, 'next'),
      itemSidebarTriageEntry(item, 'done'),
      ...shared,
      itemSidebarTriageEntry(item, 'someday'),
      itemSidebarTriageEntry(item, 'delete'),
    ];
  }
  return [
    ...review,
    itemSidebarTriageEntry(item, 'next'),
    itemSidebarTriageEntry(item, 'done'),
    ...shared,
    itemSidebarTriageEntry(item, 'later'),
    itemSidebarTriageEntry(item, 'delegate', () => showItemSidebarDelegateMenu(item, x, y)),
    itemSidebarTriageEntry(item, 'someday'),
    itemSidebarTriageEntry(item, 'delete'),
  ];
}

export function showItemSidebarActionMenu(item, x, y) {
  const itemState = normalizeItemSidebarView(item?.state || state.itemSidebarView);
  const entries = itemSidebarStateEntries(item, itemState, x, y);
  showItemSidebarMenu(entries, x, y);
}
