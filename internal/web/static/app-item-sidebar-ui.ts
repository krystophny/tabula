import * as env from './app-env.js';
import * as context from './app-context.js';
import {
  beginHorizontalSwipe,
  horizontalSwipeDelta,
  isHorizontalSwipeIntent,
} from './app-swipe.js';
import {
  deadlineLevelForItem,
  filterProjectItemsForSidebarView,
} from './app-item-sidebar-projects.js';

const { marked, apiURL, wsURL, renderCanvas, clearCanvas, getLocationFromSelection, clearLineHighlight, escapeHtml, sanitizeHtml, getActiveArtifactTitle, getActiveTextEventId, getPreviousArtifactText, getUiState, setUiMode, showIndicatorMode, hideIndicator, showTextInput, hideTextInput, showOverlay, hideOverlay, updateOverlay, isOverlayVisible, isTextInputVisible, isRecording, setRecording, getInputAnchor, setInputAnchor, getAnchorFromPoint, buildContextPrefix, getLastInputPosition, setLastInputPosition, configureLiveSession, getLiveSessionSnapshot, handleLiveSessionMessage, isLiveSessionListenActive, LIVE_SESSION_HOTWORD_DEFAULT, LIVE_SESSION_MODE_DIALOGUE, LIVE_SESSION_MODE_MEETING, onLiveSessionTTSPlaybackComplete, cancelLiveSessionListen, startLiveSession, stopLiveSession, initHotword, startHotwordMonitor, stopHotwordMonitor, isHotwordActive, onHotwordDetected, setHotwordThreshold, setHotwordAudioContext, getPreRollAudio, getHotwordMicStream, initVAD, ensureVADLoaded, float32ToWav } = env;
const { refs, state, getState, isVoiceTurn, COMPANION_VIEW_PATH_PREFIX, COMPANION_TRANSCRIPT_VIEW_PATH, COMPANION_SUMMARY_VIEW_PATH, COMPANION_REFERENCES_VIEW_PATH, MEETING_TRANSCRIPT_LABEL, MEETING_SUMMARY_LABEL, MEETING_REFERENCES_LABEL, MEETING_SUMMARY_ITEMS_PANEL_ID, CHAT_CTRL_LONG_PRESS_MS, ARTIFACT_EDIT_LONG_TAP_MS, ITEM_SIDEBAR_VIEWS, ITEM_SIDEBAR_GESTURE_CANCEL_PX, ITEM_SIDEBAR_GESTURE_COMMIT_PX, ITEM_SIDEBAR_GESTURE_LONG_PX, ITEM_SIDEBAR_DEFAULT_LATER_HOUR_UTC, ITEM_SIDEBAR_MENU_ID, DEV_UI_RELOAD_POLL_MS, ASSISTANT_ACTIVITY_POLL_MS, CHAT_WS_STALE_THRESHOLD_MS, ACTIVE_TURN_NO_ID_CLEAR_GRACE_MS, ACTIVE_TURN_ACTIVITY_CLEAR_GRACE_MS, PROJECT_CHAT_MODEL_ALIASES, PROJECT_CHAT_MODEL_REASONING_EFFORTS, TTS_SILENT_STORAGE_KEY, YOLO_MODE_STORAGE_KEY, SOMEDAY_REVIEW_NUDGE_ENABLED_STORAGE_KEY, SOMEDAY_REVIEW_NUDGE_LAST_SHOWN_STORAGE_KEY, SOMEDAY_REVIEW_NUDGE_INTERVAL_MS, ACTIVE_PROJECT_STORAGE_KEY, LAST_VIEW_STORAGE_KEY, RUNTIME_RELOAD_CONTEXT_STORAGE_KEY, SIDEBAR_IMAGE_EXTENSIONS, PANEL_MOTION_WATCH_QUERIES, VOICE_LIFECYCLE, COMPANION_IDLE_SURFACES, COMPANION_RUNTIME_STATES, TOOL_PALETTE_MODES } = context;

const showStatus = (...args) => refs.showStatus(...args);
const refreshWorkspaceBrowser = (...args) => refs.refreshWorkspaceBrowser(...args);
const setPrReviewDrawerOpen = (...args) => refs.setPrReviewDrawerOpen(...args);
const renderPrReviewFileList = (...args) => refs.renderPrReviewFileList(...args);
const normalizeItemSidebarView = (...args) => refs.normalizeItemSidebarView(...args);
const normalizeItemSidebarFilters = (...args) => refs.normalizeItemSidebarFilters(...args);
const showItemSidebarActionMenu = (...args) => refs.showItemSidebarActionMenu(...args);
const defaultItemSidebarCounts = (...args) => refs.defaultItemSidebarCounts(...args);
const closeEdgePanels = (...args) => refs.closeEdgePanels(...args);
const itemSidebarGestureAction = (...args) => refs.itemSidebarGestureAction(...args);
const itemSidebarActionLabel = (...args) => refs.itemSidebarActionLabel(...args);
const hideItemSidebarMenu = (...args) => refs.hideItemSidebarMenu(...args);
const applyItemSidebarCounts = (...args) => refs.applyItemSidebarCounts(...args);
const itemSidebarEndpoint = (...args) => refs.itemSidebarEndpoint(...args);
const itemSidebarCountsEndpoint = (...args) => refs.itemSidebarCountsEndpoint(...args);
const fetchItemSidebarProjectItemReview = (...args) => refs.fetchItemSidebarProjectItemReview(...args);
const fetchItemSidebarPeopleDashboard = (...args) => refs.fetchItemSidebarPeopleDashboard(...args);
const fetchItemSidebarPersonDashboard = (...args) => refs.fetchItemSidebarPersonDashboard(...args);
const fetchItemSidebarDedupReview = (...args) => refs.fetchItemSidebarDedupReview(...args);
const isDedupCandidateGroup = (...args) => refs.isDedupCandidateGroup(...args);
const renderDedupCandidateRow = (...args) => refs.renderDedupCandidateRow(...args);
const openPersonOpenLoops = (...args) => refs.openPersonOpenLoops(...args);
const openSidebarArtifactItem = (...args) => refs.openSidebarArtifactItem(...args);
const isMobileViewport = (...args) => refs.isMobileViewport(...args);
const suppressSyntheticClick = (...args) => refs.suppressSyntheticClick(...args);
const showItemSidebarDelegateMenu = (...args) => refs.showItemSidebarDelegateMenu(...args);
const performItemSidebarTriage = (...args) => refs.performItemSidebarTriage(...args);
const performItemSidebarGesture = (...args) => refs.performItemSidebarGesture(...args);
const performItemSidebarStateUpdate = (...args) => refs.performItemSidebarStateUpdate(...args);
const normalizeWorkspaceBrowserPath = (...args) => refs.normalizeWorkspaceBrowserPath(...args);
const loadWorkspaceBrowserPath = (...args) => refs.loadWorkspaceBrowserPath(...args);
const parentWorkspaceBrowserPath = (...args) => refs.parentWorkspaceBrowserPath(...args);
const workspaceCompanionEntries = (...args) => refs.workspaceCompanionEntries(...args);
const openWorkspaceSidebarFile = (...args) => refs.openWorkspaceSidebarFile(...args);
const switchProject = (...args) => refs.switchProject(...args);

export async function openItemSidebarView(view = state.itemSidebarView, filters = null) {
  state.fileSidebarMode = 'items';
  if (!state.prReviewDrawerOpen) {
    setPrReviewDrawerOpen(true);
  }
  renderPrReviewFileList();
  return loadItemSidebarView(view, filters);
}

export function activeItemSidebarShortcutTarget() {
  const view = normalizeItemSidebarView(state.itemSidebarView);
  if (state.prReviewMode || state.fileSidebarMode !== 'items' || (view !== 'inbox' && view !== 'someday')) {
    return null;
  }
  const items = Array.isArray(state.itemSidebarItems) ? state.itemSidebarItems : [];
  if (items.length === 0) return null;
  const activeID = Number(state.itemSidebarActiveItemID || 0);
  return items.find((item) => Number(item?.id || 0) === activeID) || items[0];
}

function activeCanvasSidebarItemIndex() {
  if (!state.hasArtifact || state.prReviewMode || state.fileSidebarMode !== 'items') {
    return -1;
  }
  const items = Array.isArray(state.itemSidebarItems) ? state.itemSidebarItems : [];
  if (items.length <= 1) return -1;
  const activeID = Number(state.itemSidebarActiveItemID || 0);
  if (activeID <= 0) return -1;
  const activeIndex = items.findIndex((item) => Number(item?.id || 0) === activeID);
  if (activeIndex < 0) return -1;
  const activeItem = items[activeIndex];
  const currentTitle = String(state.currentCanvasArtifact?.title || '').trim();
  if (!currentTitle) return -1;
  const expectedTitles = [
    String(activeItem?.artifact_title || '').trim(),
    String(activeItem?.title || '').trim(),
  ].filter(Boolean);
  return expectedTitles.includes(currentTitle) ? activeIndex : -1;
}

export function stepItemSidebarItem(delta) {
  const shift = Number(delta);
  if (!Number.isFinite(shift) || shift === 0) return false;
  const items = Array.isArray(state.itemSidebarItems) ? state.itemSidebarItems : [];
  if (items.length <= 1) return false;
  const currentIndex = activeCanvasSidebarItemIndex();
  if (currentIndex < 0) return false;
  const nextIndex = ((currentIndex + Math.trunc(shift)) % items.length + items.length) % items.length;
  if (nextIndex === currentIndex) return false;
  const nextItem = items[nextIndex];
  if (!nextItem) return false;
  state.itemSidebarActiveItemID = Number(nextItem?.id || 0);
  renderPrReviewFileList();
  void openSidebarItem(nextItem);
  return true;
}

export async function loadItemSidebarView(view = state.itemSidebarView, filters = null) {
  const normalizedView = normalizeItemSidebarView(view);
  const normalizedFilters = normalizeItemSidebarFilters(filters === null ? state.itemSidebarFilters : filters);
  const workspaceID = String(state.activeWorkspaceId || '').trim();
  const loadSeq = Number(state.itemSidebarLoadSeq || 0) + 1;
  state.itemSidebarLoadSeq = loadSeq;
  hideItemSidebarMenu();
  state.itemSidebarView = normalizedView;
  state.itemSidebarFilters = normalizedFilters;
  if (!(Number(state.itemSidebarFilters?.label_id || 0) > 0)) {
    state.itemSidebarLabelName = '';
  }
  state.itemSidebarLoading = true;
  state.itemSidebarError = '';
  if (!state.prReviewMode) {
    state.fileSidebarMode = 'items';
  }
  renderPrReviewFileList();
  if (!workspaceID) {
    try {
      const wsResp = await fetch(apiURL('runtime/workspaces'));
      if (wsResp.ok) {
        const wsList = await wsResp.json();
        const active = (Array.isArray(wsList) ? wsList : []).find((w) => w?.is_active);
        const resolvedID = String(active?.id || '').trim();
        if (resolvedID) {
          state.activeWorkspaceId = resolvedID;
          return loadItemSidebarView(view, filters);
        }
      }
    } catch (_) {}
    state.itemSidebarItems = [];
    state.itemSidebarPersonDashboard = null;
    state.itemSidebarLoading = false;
    applyItemSidebarCounts(defaultItemSidebarCounts(), null);
    renderPrReviewFileList();
    return false;
  }
  try {
    const projectItemList = normalizedFilters.section === 'project_items';
    const peopleList = normalizedFilters.section === 'people' && !(Number(normalizedFilters.actor_id || 0) > 0);
    const personDetail = normalizedFilters.section === 'people' && Number(normalizedFilters.actor_id || 0) > 0;
    const dedupList = normalizedFilters.section === 'dedup';
    const [itemsPayload, countsResp] = await Promise.all([
      projectItemList
        ? fetchItemSidebarProjectItemReview(normalizedFilters)
        : peopleList
          ? fetchItemSidebarPeopleDashboard(normalizedFilters)
          : personDetail
            ? fetchItemSidebarPersonDashboard(normalizedFilters.actor_id, normalizedFilters)
            : dedupList
              ? fetchItemSidebarDedupReview(normalizedFilters)
        : fetch(apiURL(itemSidebarEndpoint(normalizedView, normalizedFilters)), { cache: 'no-store' }),
      fetch(apiURL(itemSidebarCountsEndpoint(normalizedFilters)), { cache: 'no-store' }),
    ]);
    if (!countsResp.ok) {
      const detail = (await countsResp.text()).trim() || `HTTP ${countsResp.status}`;
      throw new Error(detail);
    }
    const countsPayload = await countsResp.json();
    const sidebarItems = projectItemList || peopleList || dedupList
      ? (Array.isArray(itemsPayload) ? itemsPayload : [])
      : personDetail
        ? []
      : (itemsPayload.ok ? await itemsPayload.json() : null);
    if (!projectItemList && !peopleList && !personDetail && !dedupList && !itemsPayload.ok) {
      const detail = (await itemsPayload.text()).trim() || `HTTP ${itemsPayload.status}`;
      throw new Error(detail);
    }
    if (workspaceID !== String(state.activeWorkspaceId || '').trim()) return false;
    if (loadSeq !== Number(state.itemSidebarLoadSeq || 0)) return false;
    state.itemSidebarPersonDashboard = personDetail ? itemsPayload : null;
    state.itemSidebarItems = projectItemList
      ? filterProjectItemsForSidebarView(sidebarItems, normalizedView)
      : peopleList || dedupList
        ? sidebarItems
      : (Array.isArray(sidebarItems?.items) ? sidebarItems.items : []);
    state.itemSidebarLoading = false;
    state.itemSidebarError = '';
    applyItemSidebarCounts(countsPayload?.counts, countsPayload?.sections);
    renderPrReviewFileList();
    return true;
  } catch (err) {
    if (workspaceID !== String(state.activeWorkspaceId || '').trim()) return false;
    if (loadSeq !== Number(state.itemSidebarLoadSeq || 0)) return false;
    state.itemSidebarItems = [];
    state.itemSidebarPersonDashboard = null;
    state.itemSidebarLoading = false;
    state.itemSidebarError = String(err?.message || err || 'item list unavailable');
    renderPrReviewFileList();
    return false;
  }
}

export function sidebarTabLabel(view) {
  if (view === 'projects') return 'Active';
  if (view === 'next') return 'Next';
  if (view === 'waiting') return 'Waiting';
  if (view === 'deferred') return 'Deferred';
  if (view === 'someday') return 'Someday';
  if (view === 'review') return 'Review';
  if (view === 'done') return 'Done';
  return 'Inbox';
}

export function normalizeDisplayText(raw) {
  return String(raw || '')
    .trim()
    .replace(/[._-]+/g, ' ')
    .replace(/\s+/g, ' ');
}

export function itemSourceLabel(item) {
  const source = normalizeDisplayText(item?.source);
  if (source) return source.toLowerCase();
  const sourceRef = String(item?.source_ref || '').trim();
  if (sourceRef.includes('#PR-')) return 'github';
  return '';
}

export function parsePullRequestNumberFromSourceRef(raw) {
  const match = String(raw || '').trim().match(/#PR-(\d+)$/i);
  if (!match) return 0;
  const number = Number(match[1]);
  return Number.isInteger(number) && number > 0 ? number : 0;
}

export async function openSidebarPRReview(prNumber) {
  if (!Number.isInteger(Number(prNumber)) || Number(prNumber) <= 0 || !state.chatSessionId) {
    return false;
  }
  state.prReviewAwaitingArtifact = true;
  try {
    const resp = await fetch(apiURL(`chat/sessions/${encodeURIComponent(state.chatSessionId)}/commands`), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ command: `/pr ${Number(prNumber)}` }),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    const payload = await resp.json();
    const commandName = String(payload?.result?.name || '').trim().toLowerCase();
    if (payload?.kind === 'command' && commandName === 'pr') {
      return true;
    }
    state.prReviewAwaitingArtifact = false;
    throw new Error('unexpected PR review response');
  } catch (err) {
    state.prReviewAwaitingArtifact = false;
    showStatus(`review open failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  }
}

export async function openProjectItemQueue(item) {
  const projectItemID = Number(item?.id || 0);
  if (projectItemID <= 0) return false;
  state.itemSidebarActiveItemID = projectItemID;
  await resolveWorkspaceForSidebarItem(item);
  const nextFilters = {
    all_spheres: Boolean(state.itemSidebarFilters?.all_spheres),
    project_item_id: projectItemID,
    section: '',
  };
  await loadItemSidebarView('next', nextFilters);
  await activatePreferredProjectAction(item);
  showStatus(`project: ${String(item?.title || '').trim() || projectItemID}`);
  return true;
}

export async function openSidebarItem(item) {
  state.itemSidebarActiveItemID = Number(item?.id || 0);
  renderPrReviewFileList();
  if (isDedupCandidateGroup(item)) return;
  if (isDriftSidebarItem(item)) return;
  if (String(item?.kind || '').trim().toLowerCase() === 'person_dashboard') {
    await openPersonOpenLoops(item);
    return;
  }
  if (String(item?.kind || '').trim().toLowerCase() === 'project') {
    await openProjectItemQueue(item);
    return;
  }
  await resolveWorkspaceForSidebarItem(item);
  state.itemSidebarActiveItemID = Number(item?.id || 0);
  renderPrReviewFileList();
  if (String(item?.artifact_kind || '').trim().toLowerCase() !== 'github_pr') {
    try {
      const opened = await openSidebarArtifactItem(item);
      if (opened && isMobileViewport()) {
        closeEdgePanels();
      }
    } catch (err) {
      showStatus(`item open failed: ${String(err?.message || err || 'unknown error')}`);
    }
    return;
  }
  const prNumber = parsePullRequestNumberFromSourceRef(item?.source_ref);
  if (prNumber <= 0) {
    return;
  }
  const opened = await openSidebarPRReview(prNumber);
  if (opened && isMobileViewport()) {
    closeEdgePanels();
  }
}

function itemWorkspaceID(item) {
  const id = Number(item?.workspace_id || 0);
  return Number.isFinite(id) && id > 0 ? String(Math.trunc(id)) : '';
}

function workspaceExists(workspaceID) {
  const id = String(workspaceID || '').trim();
  if (!id) return false;
  return (Array.isArray(state.projects) ? state.projects : []).some((project) => String(project?.id || '').trim() === id);
}

function fallbackWorkspaceID() {
  const defaultID = String(state.defaultWorkspaceId || '').trim();
  if (workspaceExists(defaultID)) return defaultID;
  const brain = (Array.isArray(state.projects) ? state.projects : []).find((project) => {
    const name = String(project?.name || '').trim().toLowerCase();
    const root = String(project?.root_path || project?.workspace_path || '').trim().toLowerCase();
    return Boolean(project?.is_default) || name === 'brain' || root.endsWith('/brain');
  });
  if (brain) return String(brain.id || '').trim();
  return String(state.projects?.[0]?.id || '').trim();
}

function rememberableWorkspaceID() {
  const current = String(state.activeWorkspaceId || '').trim();
  if (!current || current === fallbackWorkspaceID()) return '';
  if (!/^\d+$/.test(current)) return '';
  return current;
}

function updateCachedItemWorkspaceID(itemID, workspaceID) {
  const id = Number(itemID || 0);
  if (id <= 0) return;
  const nextWorkspaceID = workspaceID === null ? null : Number(workspaceID || 0);
  (Array.isArray(state.itemSidebarItems) ? state.itemSidebarItems : []).forEach((item) => {
    if (Number(item?.id || 0) === id) item.workspace_id = nextWorkspaceID;
  });
}

async function rememberSidebarItemWorkspace(item, workspaceID) {
  const itemID = Number(item?.id || 0);
  const numericWorkspaceID = Number(workspaceID || 0);
  if (itemID <= 0 || !Number.isFinite(numericWorkspaceID) || numericWorkspaceID <= 0) return false;
  try {
    const resp = await fetch(apiURL(`items/${encodeURIComponent(String(itemID))}/workspace`), {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ workspace_id: Math.trunc(numericWorkspaceID) }),
    });
    if (!resp.ok) return false;
    item.workspace_id = Math.trunc(numericWorkspaceID);
    updateCachedItemWorkspaceID(itemID, Math.trunc(numericWorkspaceID));
    return true;
  } catch (_) {
    return false;
  }
}

async function switchToItemWorkspace(workspaceID) {
  const target = String(workspaceID || '').trim();
  if (!target || target === String(state.activeWorkspaceId || '').trim()) return true;
  if (!workspaceExists(target)) return false;
  await switchProject(target);
  return target === String(state.activeWorkspaceId || '').trim();
}

async function resolveWorkspaceForSidebarItem(item) {
  const linkedWorkspaceID = itemWorkspaceID(item);
  if (linkedWorkspaceID) {
    if (await switchToItemWorkspace(linkedWorkspaceID)) return linkedWorkspaceID;
  }
  const currentWorkspaceID = rememberableWorkspaceID();
  if (currentWorkspaceID) {
    await rememberSidebarItemWorkspace(item, currentWorkspaceID);
    return currentWorkspaceID;
  }
  const fallbackID = fallbackWorkspaceID();
  if (fallbackID) {
    await switchToItemWorkspace(fallbackID);
  }
  return fallbackID;
}

async function activatePreferredProjectAction(projectItem) {
  const preferredID = Number(projectItem?.next_action?.id || 0);
  const rows = Array.isArray(state.itemSidebarItems) ? state.itemSidebarItems : [];
  const preferred = preferredID > 0
    ? rows.find((item) => Number(item?.id || 0) === preferredID)
    : null;
  const firstAction = preferred || rows.find((item) => String(item?.kind || 'action').trim().toLowerCase() !== 'project');
  if (!firstAction) return false;
  state.itemSidebarActiveItemID = Number(firstAction?.id || 0);
  renderPrReviewFileList();
  await resolveWorkspaceForSidebarItem(firstAction);
  state.itemSidebarActiveItemID = Number(firstAction?.id || 0);
  renderPrReviewFileList();
  return true;
}

export function itemIconForRow(item) {
  if (isDedupCandidateGroup(item)) return { icon: 'symbol', text: '=' };
  if (isDriftSidebarItem(item)) return { icon: 'symbol', text: 'D' };
  if (String(item?.kind || '').trim().toLowerCase() === 'person_dashboard') return { icon: 'symbol', text: '@' };
  if (String(item?.kind || '').trim().toLowerCase() === 'project') return { icon: 'symbol', text: 'P' };
  const artifactKind = String(item?.artifact_kind || '').trim().toLowerCase();
  const source = itemSourceLabel(item);
  if (artifactKind === 'github_pr') return { icon: 'symbol', text: 'R' };
  if (artifactKind === 'email') return { icon: 'symbol', text: '@' };
  if (artifactKind === 'email_draft') return { icon: 'symbol', text: 'M' };
  if (artifactKind === 'idea_note') return { icon: 'symbol', text: 'I' };
  if (source === 'github') return { icon: 'symbol', text: '#' };
  return { icon: 'symbol', text: 'T' };
}

export function parseSidebarTimestamp(value) {
  const text = String(value || '').trim();
  if (!text) return null;
  let normalized = text;
  if (/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(normalized)) {
    normalized = `${normalized.replace(' ', 'T')}Z`;
  }
  const parsed = Date.parse(normalized);
  return Number.isFinite(parsed) ? parsed : null;
}

export function formatSidebarAge(value) {
  const parsed = parseSidebarTimestamp(value);
  if (parsed === null) return '';
  const seconds = Math.max(0, Math.floor((Date.now() - parsed) / 1000));
  if (seconds < 60) return 'now';
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`;
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h`;
  return `${Math.floor(seconds / 86400)}d`;
}

export function formatSidebarTimestamp(value) {
  const parsed = parseSidebarTimestamp(value);
  if (parsed === null) return '';
  return `${new Date(parsed).toISOString().slice(0, 16).replace('T', ' ')} UTC`;
}

export function renderSidebarRow({
  icon,
  iconText = '',
  label,
  active = false,
  meta = '',
  subtitle = '',
  badges = [],
  item = null,
  workspaceEntry = null,
  triageEnabled = false,
  onClick,
}) {
  const button = createSidebarRowButton(active, item);
  appendSidebarRowContent(button, { icon, iconText, label, subtitle, badges, meta });
  const recentTouch = addSidebarRowTouchHandlers(button, item, triageEnabled, onClick);
  addSidebarRowFocusHandler(button, item, workspaceEntry);
  addSidebarRowClickHandler(button, workspaceEntry, recentTouch, onClick);
  return button;
}

function createSidebarRowButton(active, item) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'pr-file-item';
  if (active) button.classList.add('is-active');
  if (item && Number(item?.id || 0) > 0) {
    button.dataset.itemId = String(Number(item.id));
  }
  const deadlineLevel = deadlineLevelForItem(item);
  if (deadlineLevel) {
    button.dataset.deadline = deadlineLevel;
  }
  return button;
}

function appendSidebarRowContent(button, { icon, iconText, label, subtitle, badges, meta }) {
  button.appendChild(sidebarRowIcon(icon, iconText));
  button.appendChild(sidebarRowBody(label, subtitle, badges));
  if (!meta) return;
  const metaEl = document.createElement('span');
  metaEl.className = 'pr-file-status';
  metaEl.textContent = String(meta);
  button.appendChild(metaEl);
}

function sidebarRowIcon(icon, iconText) {
  const iconEl = document.createElement('span');
  iconEl.className = `chooser-icon icon-${icon}`;
  iconEl.textContent = String(iconText || '');
  return iconEl;
}

function sidebarRowBody(label, subtitle, badges) {
  const bodyEl = document.createElement('span');
  bodyEl.className = 'sidebar-row-main';
  const labelEl = document.createElement('span');
  labelEl.className = 'pr-file-name';
  labelEl.textContent = String(label || '');
  bodyEl.appendChild(labelEl);
  const secondaryEl = sidebarRowSecondary(subtitle, badges);
  if (secondaryEl) bodyEl.appendChild(secondaryEl);
  return bodyEl;
}

function sidebarRowSecondary(subtitle, badges) {
  if (!subtitle && badges.length === 0) return null;
  const secondaryEl = document.createElement('span');
  secondaryEl.className = 'sidebar-row-secondary';
  if (subtitle) {
    const subtitleEl = document.createElement('span');
    subtitleEl.className = 'sidebar-row-subtitle';
    subtitleEl.textContent = String(subtitle);
    secondaryEl.appendChild(subtitleEl);
  }
  if (badges.length > 0) secondaryEl.appendChild(sidebarRowBadges(badges));
  return secondaryEl;
}

function sidebarRowBadges(badges) {
  const badgesEl = document.createElement('span');
  badgesEl.className = 'sidebar-row-badges';
  badges.forEach((badgeText) => {
    const badgeEl = document.createElement('span');
    badgeEl.className = 'sidebar-badge';
    badgeEl.textContent = String(badgeText);
    badgesEl.appendChild(badgeEl);
  });
  return badgesEl;
}

function addSidebarRowTouchHandlers(button, item, triageEnabled, onClick) {
  const resetSwipeUi = () => {
    button.style.removeProperty('--swipe-offset');
    delete button.dataset.triageAction;
    delete button.dataset.triageLabel;
  };

  const applySwipeUi = (dx) => {
    const limited = Math.max(-220, Math.min(220, Number(dx) || 0));
    const action = itemSidebarGestureAction(limited);
    button.style.setProperty('--swipe-offset', `${limited}px`);
    if (action) {
      button.dataset.triageAction = action.action;
      button.dataset.triageLabel = itemSidebarActionLabel(action.action, item);
    } else {
      delete button.dataset.triageAction;
      delete button.dataset.triageLabel;
    }
    return action;
  };

  let lastTouchAt = 0;
  let touchState = null;
  button.addEventListener('touchstart', (ev) => {
    const t = ev.touches && ev.touches[0];
    if (!t) return;
    hideItemSidebarMenu();
    touchState = {
      ...beginHorizontalSwipe(t),
      currentX: t.clientX,
      currentY: t.clientY,
      swiping: false,
    };
    resetSwipeUi();
  }, { passive: true });
  if (triageEnabled && item) {
    button.addEventListener('touchmove', (ev) => {
      if (!touchState) return;
      const t = ev.touches && ev.touches[0];
      if (!t) return;
      touchState.currentX = t.clientX;
      touchState.currentY = t.clientY;
      const { dx, dy } = horizontalSwipeDelta(touchState, t);
      if (!touchState.swiping) {
        if (!isHorizontalSwipeIntent(dx, dy, ITEM_SIDEBAR_GESTURE_CANCEL_PX, 1)) {
          return;
        }
        touchState.swiping = true;
      }
      ev.preventDefault();
      applySwipeUi(dx);
    }, { passive: false });
    button.addEventListener('touchcancel', () => {
      touchState = null;
      resetSwipeUi();
    });
  }
  addSidebarRowContextMenu(button, item);
  button.addEventListener('touchend', (ev) => {
    const t = ev.changedTouches && ev.changedTouches[0];
    const current = touchState;
    touchState = null;
    if (!t || !current) {
      return;
    }
    const dx = t.clientX - current.startX;
    const dy = t.clientY - current.startY;
    const gestureAction = triageEnabled ? itemSidebarGestureAction(dx) : null;
    if (gestureAction && current.swiping) {
      ev.preventDefault();
      ev.stopPropagation();
      lastTouchAt = Date.now();
      suppressSyntheticClick();
      resetSwipeUi();
      const swipeAction = String(gestureAction.gesture || gestureAction.action || '').toLowerCase();
      if (swipeAction === 'delegate') {
        void showItemSidebarDelegateMenu(item, t.clientX, t.clientY);
      } else {
        void performItemSidebarGesture(item, swipeAction);
      }
      return;
    }
    resetSwipeUi();
    if (Math.abs(dx) > ITEM_SIDEBAR_GESTURE_CANCEL_PX || Math.abs(dy) > 10) {
      return;
    }
    ev.preventDefault();
    ev.stopPropagation();
    lastTouchAt = Date.now();
    onClick(ev);
  }, { passive: false });
  return () => Date.now() - lastTouchAt < 700;
}

function addSidebarRowContextMenu(button, item) {
  if (!item) return;
  button.addEventListener('contextmenu', (ev) => {
    ev.preventDefault();
    ev.stopPropagation();
    state.itemSidebarActiveItemID = Number(item?.id || 0);
    showItemSidebarActionMenu(item, ev.clientX, ev.clientY);
  });
}

function addSidebarRowFocusHandler(button, item, workspaceEntry) {
  if (item) {
    button.addEventListener('focus', () => {
      state.itemSidebarActiveItemID = Number(item?.id || 0);
    });
  } else if (workspaceEntry) {
    button.addEventListener('focus', () => {
      state.workspaceBrowserActivePath = String(workspaceEntry?.path || '').trim();
      state.workspaceBrowserActiveIsDir = Boolean(workspaceEntry?.is_dir);
    });
  }
}

function addSidebarRowClickHandler(button, workspaceEntry, recentTouch, onClick) {
  button.addEventListener('click', (ev) => {
    if (recentTouch()) {
      ev.preventDefault();
      return;
    }
    if (Date.now() - Number(state.sidebarEdgeTapAt || 0) < 600) return;
    if (workspaceEntry) {
      state.workspaceBrowserActivePath = String(workspaceEntry?.path || '').trim();
      state.workspaceBrowserActiveIsDir = Boolean(workspaceEntry?.is_dir);
    }
    onClick(ev);
  });
}

function renderPersonGroupHeading(label, count) {
  const heading = document.createElement('div');
  heading.className = 'sidebar-group-heading';
  heading.textContent = `${label} (${Number(count || 0)})`;
  return heading;
}

function renderPersonOpenLoopRows(list, label, rows) {
	const items = Array.isArray(rows) ? rows : [];
	list.appendChild(renderPersonGroupHeading(label, items.length));
	items.forEach((item) => {
		list.appendChild(renderSidebarRow({
			icon: 'symbol',
			label: String(item?.title || 'Untitled item'),
			active: Number(item?.id || 0) === Number(state.itemSidebarActiveItemID || 0),
			item,
			onClick: () => { void openSidebarItem(item); },
    }));
  });
}

function renderPersonDashboard(list, dashboard) {
  if (!dashboard) return false;
  const diagnostics = Array.isArray(dashboard?.diagnostics) ? dashboard.diagnostics : [];
  if (diagnostics.length > 0) {
    list.appendChild(renderSidebarRow({
      icon: 'symbol',
      iconText: '!',
      label: diagnostics.join('; '),
      onClick: () => {},
    }));
  }
  renderPersonOpenLoopRows(list, 'Waiting on them', dashboard.waiting_on_them);
  renderPersonOpenLoopRows(list, 'I owe them', dashboard.i_owe_them);
  renderPersonOpenLoopRows(list, 'Recently closed', dashboard.recently_closed);
  const projectItems = Array.isArray(dashboard?.project_items) ? dashboard.project_items : [];
  if (projectItems.length > 0) {
    renderPersonOpenLoopRows(list, 'Projects', projectItems);
  }
  return true;
}

function renderEmptyItemSidebarList(list) {
  const section = String(state.itemSidebarFilters?.section || '').trim().toLowerCase();
  let emptyLabel = `No ${sidebarTabLabel(state.itemSidebarView).toLowerCase()} items.`;
  if (section === 'project_items') emptyLabel = 'No active projects.';
  if (section === 'people') emptyLabel = 'No people with open loops.';
  if (section === 'dedup') emptyLabel = 'No duplicate candidates.';
  list.appendChild(renderSidebarRow({
    icon: 'symbol',
    iconText: '0',
    label: emptyLabel,
    onClick: () => {},
  }));
}

function renderItemSidebarRows(list, items) {
	items.forEach((item) => {
		if (isDedupCandidateGroup(item)) {
			list.appendChild(renderDedupCandidateRow(item));
			return;
		}
		const triageEnabled = state.itemSidebarView === 'inbox' || state.itemSidebarView === 'next';
		const row = renderSidebarRow({
			icon: 'symbol',
			label: String(item?.title || 'Untitled item'),
			active: Number(item?.id || 0) === Number(state.itemSidebarActiveItemID || 0),
			item,
			triageEnabled,
      onClick: () => { void openSidebarItem(item); },
    });
    if (isDriftSidebarItem(item)) appendDriftActions(row, item);
    list.appendChild(row);
  });
}

function isDriftSidebarItem(item) {
  return Number(item?.drift_id || 0) > 0 || String(item?.kind || '').trim().toLowerCase() === 'drift';
}

function appendDriftActions(row, item) {
  const actions = document.createElement('span');
  actions.className = 'sidebar-row-badges';
  const buttons = [
    ['keep_local', 'Keep local'],
    ['take_upstream', 'Take upstream'],
    ['reingest_source', 'Re-ingest'],
    ['dismiss', 'Dismiss'],
  ];
  buttons.forEach(([action, label]) => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'sidebar-badge';
    button.textContent = label;
    button.addEventListener('click', (ev) => {
      ev.preventDefault();
      ev.stopPropagation();
      void performDriftAction(item, action);
    });
    actions.appendChild(button);
  });
  row.querySelector('.sidebar-row-secondary')?.appendChild(actions);
}

async function performDriftAction(item, action) {
  const driftID = Number(item?.drift_id || 0);
  if (driftID <= 0) return false;
  const resp = await fetch(apiURL(`items/drift/${driftID}/${encodeURIComponent(action)}`), { method: 'POST' });
  if (!resp.ok) {
    const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
    showStatus(`drift action failed: ${detail}`);
    return false;
  }
  showStatus(`drift ${String(action).replace(/_/g, ' ')}`);
  await loadItemSidebarView(state.itemSidebarView, state.itemSidebarFilters);
  return true;
}

export function renderItemSidebarList(list) {
  if (state.itemSidebarLoading) {
    list.appendChild(renderSidebarRow({
      icon: 'symbol',
      iconText: '…',
      label: 'Loading items...',
      onClick: () => {},
    }));
    return;
  }
  if (state.itemSidebarError) {
    list.appendChild(renderSidebarRow({
      icon: 'symbol',
      iconText: '!',
      label: `Error: ${state.itemSidebarError}`,
      onClick: () => {},
    }));
    return;
  }
  const items = Array.isArray(state.itemSidebarItems) ? state.itemSidebarItems : [];
  if (state.itemSidebarPersonDashboard) {
    renderPersonDashboard(list, state.itemSidebarPersonDashboard);
    return;
  }
  if (items.length === 0) {
    renderEmptyItemSidebarList(list);
    return;
  }
  renderItemSidebarRows(list, items);
}

export function handleItemSidebarKeyboardShortcut(ev) {
  const sidebarTarget = activeItemSidebarShortcutTarget();
  if (!sidebarTarget) return false;
  if (!document.body.classList.contains('file-sidebar-open')) return false;
  const key = String(ev.key || '');
  let action = '';
  const view = normalizeItemSidebarView(state.itemSidebarView);
  if (key === 'Backspace') {
    action = 'delete';
  } else if (key === 'x' || key === 'X') {
    action = 'complete';
  } else if (key === 'd' || key === 'D') {
    action = 'done';
  } else if ((view === 'inbox' || view === 'next') && (key === 'l' || key === 'L')) {
    action = 'later';
  } else if ((view === 'inbox' || view === 'next') && (key === 'g' || key === 'G')) {
    action = 'delegate';
  } else if ((view === 'inbox' || view === 'next') && (key === 's' || key === 'S')) {
    action = 'someday';
  } else if (view !== 'inbox' && (key === 'a' || key === 'A')) {
    action = 'inbox';
  } else {
    return false;
  }
  ev.preventDefault();
  if (action === 'complete') {
    void performItemSidebarGesture(sidebarTarget, 'complete');
    return true;
  }
  if (action === 'delegate') {
    const row = document.querySelector(`#pr-file-list .pr-file-item[data-item-id="${Number(sidebarTarget.id)}"]`);
    const rect = row instanceof HTMLElement ? row.getBoundingClientRect() : null;
    const x = rect ? rect.right - 12 : 24;
    const y = rect ? rect.top + Math.min(rect.height, 48) : 24;
    void showItemSidebarDelegateMenu(sidebarTarget, x, y);
    return true;
  }
  if (action === 'inbox') {
    void performItemSidebarStateUpdate(sidebarTarget, 'inbox');
    return true;
  }
  void performItemSidebarTriage(sidebarTarget, action);
  return true;
}

export function renderWorkspaceFileList(list) {
  if (state.workspaceBrowserLoading) {
    list.appendChild(renderSidebarRow({
      icon: 'folder',
      label: 'Loading...',
      onClick: () => {},
    }));
    return;
  }
  if (state.workspaceBrowserError) {
    list.appendChild(renderSidebarRow({
      icon: 'file',
      label: `Error: ${state.workspaceBrowserError}`,
      onClick: () => {},
    }));
    return;
  }
  const currentPath = normalizeWorkspaceBrowserPath(state.workspaceBrowserPath);
  const activeWorkspaceFilePath = normalizeWorkspaceBrowserPath(state.workspaceOpenFilePath);
  const activeWorkspaceSelectionPath = normalizeWorkspaceBrowserPath(state.workspaceBrowserActivePath);
  if (currentPath) {
    const parentPath = parentWorkspaceBrowserPath(currentPath);
    list.appendChild(renderSidebarRow({
      icon: 'parent',
      label: '..',
      active: activeWorkspaceSelectionPath === parentPath,
      workspaceEntry: { path: parentPath, is_dir: true },
      onClick: () => {
        void loadWorkspaceBrowserPath(parentPath);
      },
    }));
  }
  const entries = Array.isArray(state.workspaceBrowserEntries) ? state.workspaceBrowserEntries : [];
  const rows = currentPath ? entries : workspaceCompanionEntries().concat(entries);
  rows.forEach((entry) => {
    const isDir = Boolean(entry?.is_dir);
    const entryPath = normalizeWorkspaceBrowserPath(entry?.path || '');
    const entryName = String(entry?.name || entryPath || '(item)');
    list.appendChild(renderSidebarRow({
      icon: isDir ? 'folder' : 'file',
      label: entryName,
      active: activeWorkspaceSelectionPath
        ? entryPath === activeWorkspaceSelectionPath
        : (!isDir && activeWorkspaceFilePath && entryPath === activeWorkspaceFilePath),
      workspaceEntry: entry,
      onClick: () => {
        if (isDir) {
          void loadWorkspaceBrowserPath(entryPath);
          return;
        }
        void openWorkspaceSidebarFile(entryPath);
      },
    }));
  });
}
