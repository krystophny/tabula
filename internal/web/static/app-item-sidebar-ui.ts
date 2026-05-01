import * as env from './app-env.js';
import * as context from './app-context.js';
import {
  beginHorizontalSwipe,
  horizontalSwipeDelta,
  isHorizontalSwipeIntent,
} from './app-swipe.js';

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
const openPersonOpenLoops = (...args) => refs.openPersonOpenLoops(...args);
const openSidebarArtifactItem = (...args) => refs.openSidebarArtifactItem(...args);
const isMobileViewport = (...args) => refs.isMobileViewport(...args);
const suppressSyntheticClick = (...args) => refs.suppressSyntheticClick(...args);
const showItemSidebarDelegateMenu = (...args) => refs.showItemSidebarDelegateMenu(...args);
const performItemSidebarTriage = (...args) => refs.performItemSidebarTriage(...args);
const performItemSidebarGesture = (...args) => refs.performItemSidebarGesture(...args);
const performItemSidebarStateUpdate = (...args) => refs.performItemSidebarStateUpdate(...args);
const showItemSidebarLabelFilterMenu = (...args) => refs.showItemSidebarLabelFilterMenu(...args);
const applyItemSidebarLabelFilter = (...args) => refs.applyItemSidebarLabelFilter(...args);
const normalizeWorkspaceBrowserPath = (...args) => refs.normalizeWorkspaceBrowserPath(...args);
const loadWorkspaceBrowserPath = (...args) => refs.loadWorkspaceBrowserPath(...args);
const parentWorkspaceBrowserPath = (...args) => refs.parentWorkspaceBrowserPath(...args);
const workspaceCompanionEntries = (...args) => refs.workspaceCompanionEntries(...args);
const openWorkspaceSidebarFile = (...args) => refs.openWorkspaceSidebarFile(...args);
const openScanImportPicker = (...args) => refs.openScanImportPicker(...args);
const launchNewMailAuthoring = (...args) => refs.launchNewMailAuthoring(...args);
const launchReplyAuthoring = (...args) => refs.launchReplyAuthoring(...args);
const launchReplyAllAuthoring = (...args) => refs.launchReplyAllAuthoring(...args);
const launchForwardAuthoring = (...args) => refs.launchForwardAuthoring(...args);

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
    const [itemsPayload, countsResp] = await Promise.all([
      projectItemList
        ? fetchItemSidebarProjectItemReview(normalizedFilters)
        : peopleList
          ? fetchItemSidebarPeopleDashboard(normalizedFilters)
          : personDetail
            ? fetchItemSidebarPersonDashboard(normalizedFilters.actor_id, normalizedFilters)
        : fetch(apiURL(itemSidebarEndpoint(normalizedView, normalizedFilters)), { cache: 'no-store' }),
      fetch(apiURL(itemSidebarCountsEndpoint(normalizedFilters)), { cache: 'no-store' }),
    ]);
    if (!countsResp.ok) {
      const detail = (await countsResp.text()).trim() || `HTTP ${countsResp.status}`;
      throw new Error(detail);
    }
    const countsPayload = await countsResp.json();
    const sidebarItems = projectItemList || peopleList
      ? (Array.isArray(itemsPayload) ? itemsPayload : [])
      : personDetail
        ? []
      : (itemsPayload.ok ? await itemsPayload.json() : null);
    if (!projectItemList && !peopleList && !personDetail && !itemsPayload.ok) {
      const detail = (await itemsPayload.text()).trim() || `HTTP ${itemsPayload.status}`;
      throw new Error(detail);
    }
    if (workspaceID !== String(state.activeWorkspaceId || '').trim()) return false;
    if (loadSeq !== Number(state.itemSidebarLoadSeq || 0)) return false;
    state.itemSidebarPersonDashboard = personDetail ? itemsPayload : null;
    state.itemSidebarItems = projectItemList || peopleList
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
  const nextFilters = {
    all_spheres: Boolean(state.itemSidebarFilters?.all_spheres),
    project_item_id: projectItemID,
    section: '',
  };
  await loadItemSidebarView('next', nextFilters);
  showStatus(`project item: ${String(item?.title || '').trim() || projectItemID}`);
  return true;
}

export async function openSidebarItem(item) {
  state.itemSidebarActiveItemID = Number(item?.id || 0);
  renderPrReviewFileList();
  if (String(item?.kind || '').trim().toLowerCase() === 'person_dashboard') {
    await openPersonOpenLoops(item);
    return;
  }
  if (String(item?.kind || '').trim().toLowerCase() === 'project') {
    await openProjectItemQueue(item);
    return;
  }
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

export function itemKindLabel(item) {
  if (String(item?.kind || '').trim().toLowerCase() === 'person_dashboard') return 'person';
  if (String(item?.kind || '').trim().toLowerCase() === 'project') return 'project item';
  const artifactKind = String(item?.artifact_kind || '').trim().toLowerCase();
  if (artifactKind === 'idea_note') return 'idea';
  if (artifactKind === 'email' || artifactKind === 'email_thread' || artifactKind === 'email_draft') return 'email';
  if (artifactKind === 'github_pr') return 'review';
  if (artifactKind === 'github_issue') return 'task';
  if (artifactKind === 'plan_note') return 'task';
  const source = itemSourceLabel(item);
  if (source === 'github') return 'task';
  return 'task';
}

export function itemIconForRow(item) {
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

export function buildItemSidebarSubtitle(item) {
  const parts = [];
  const artifactTitle = String(item?.artifact_title || '').trim();
  if (artifactTitle) parts.push(artifactTitle);
  const actorName = String(item?.actor_name || '').trim();
  if (actorName) parts.push(actorName);
  return parts.join(' · ');
}

export function buildItemSidebarBadges(item) {
  const badges = [];
  const kind = itemKindLabel(item);
  if (kind) badges.push(kind);
  if (String(item?.kind || '').trim().toLowerCase() === 'person_dashboard') {
    return badges.concat(personOpenLoopBadges(item));
  }
  if (String(item?.kind || '').trim().toLowerCase() === 'project') {
    return badges.concat(projectItemChildBadges(item));
  }
  const source = itemSourceLabel(item);
  if (source) badges.push(source);
  const artifactKind = normalizeDisplayText(item?.artifact_kind).toLowerCase();
  if (artifactKind && artifactKind !== kind) badges.push(artifactKind);
  return badges.filter((badge, index, all) => badge && all.indexOf(badge) === index);
}

function personOpenLoopBadges(item) {
  const counts = item && typeof item === 'object' && item.counts && typeof item.counts === 'object'
    ? item.counts
    : {};
  const count = (field) => {
    const value = Number(counts[field] || 0);
    return Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0;
  };
  const badges = [
    `waiting ${count('waiting_on_them')}`,
    `owed ${count('i_owe_them')}`,
    `closed ${count('recently_closed')}`,
  ];
  const diagnostics = Array.isArray(item?.diagnostics) ? item.diagnostics : [];
  if (diagnostics.some((entry) => String(entry || '').trim().startsWith('needs_person_note:'))) {
    badges.push('needs person note');
  }
  return badges;
}

function projectItemChildBadges(item) {
  const children = item && typeof item === 'object' && item.children && typeof item.children === 'object'
    ? item.children
    : {};
  const count = (field) => {
    const value = Number(children[field] || 0);
    return Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0;
  };
  return [
    `next ${count('next')}`,
    `waiting ${count('waiting')}`,
    `deferred ${count('deferred')}`,
    `someday ${count('someday')}`,
    `recently closed ${count('done')}`,
  ];
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
    const icon = itemIconForRow(item);
    list.appendChild(renderSidebarRow({
      icon: icon.icon,
      iconText: icon.text,
      label: String(item?.title || 'Untitled item'),
      subtitle: buildItemSidebarSubtitle(item),
      badges: buildItemSidebarBadges(item),
      meta: formatSidebarAge(item?.updated_at || item?.created_at),
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
      badges: ['diagnostic'],
      onClick: () => {},
    }));
  }
  renderPersonOpenLoopRows(list, 'Waiting on them', dashboard.waiting_on_them);
  renderPersonOpenLoopRows(list, 'I owe them', dashboard.i_owe_them);
  renderPersonOpenLoopRows(list, 'Recently closed', dashboard.recently_closed);
  const projectItems = Array.isArray(dashboard?.project_items) ? dashboard.project_items : [];
  if (projectItems.length > 0) {
    renderPersonOpenLoopRows(list, 'Project items', projectItems);
  }
  return true;
}

function createSidebarActionButton(id, label, onClick) {
  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'edge-btn';
  button.id = id;
  button.textContent = label;
  button.addEventListener('click', onClick);
  return button;
}

function activeSidebarMailItem(items) {
  const activeItem = items.find((entry) => Number(entry?.id || 0) === Number(state.itemSidebarActiveItemID || 0)) || null;
  const artifactKind = String(activeItem?.artifact_kind || '').trim().toLowerCase();
  return ['email', 'email_thread'].includes(artifactKind) ? activeItem : null;
}

function appendMailSidebarActions(actions, item) {
  if (!item) return;
  actions.appendChild(createSidebarActionButton('reply-mail-trigger', 'Reply', () => {
    void launchReplyAuthoring(item);
  }));
  actions.appendChild(createSidebarActionButton('reply-all-mail-trigger', 'Reply All', () => {
    void launchReplyAllAuthoring(item);
  }));
  actions.appendChild(createSidebarActionButton('forward-mail-trigger', 'Forward', () => {
    void launchForwardAuthoring(item);
  }));
}

function renderItemSidebarActions(list, items) {
  const actions = document.createElement('div');
  actions.className = 'sidebar-actions';
  const activeLabelID = Number(state.itemSidebarFilters?.label_id || 0);
  const labelFilter = activeLabelID > 0
    ? `Label: ${String(state.itemSidebarLabelName || '').trim() || `#${activeLabelID}`}`
    : 'Filter by Label';
  const labelFilterButton = createSidebarActionButton('item-sidebar-label-filter', labelFilter, () => {
    const rect = labelFilterButton.getBoundingClientRect();
    void showItemSidebarLabelFilterMenu(rect.left, rect.bottom + 8);
  });
  actions.appendChild(labelFilterButton);
  if (activeLabelID > 0) {
    actions.appendChild(createSidebarActionButton('item-sidebar-label-clear', 'Clear Label Filter', () => {
      void applyItemSidebarLabelFilter(0, '');
    }));
  }
  actions.appendChild(createSidebarActionButton('new-mail-trigger', 'New Mail', () => {
    void launchNewMailAuthoring();
  }));
  appendMailSidebarActions(actions, activeSidebarMailItem(items));
  actions.appendChild(createSidebarActionButton('scan-upload-trigger', 'Scan Notes', () => {
    openScanImportPicker();
  }));
  list.appendChild(actions);
}

function renderEmptyItemSidebarList(list) {
  const section = String(state.itemSidebarFilters?.section || '').trim().toLowerCase();
  let emptyLabel = `No ${sidebarTabLabel(state.itemSidebarView).toLowerCase()} items.`;
  if (section === 'project_items') emptyLabel = 'No project items.';
  if (section === 'people') emptyLabel = 'No people with open loops.';
  list.appendChild(renderSidebarRow({
    icon: 'symbol',
    iconText: '0',
    label: emptyLabel,
    onClick: () => {},
  }));
}

function renderItemSidebarRows(list, items) {
  items.forEach((item) => {
    const icon = itemIconForRow(item);
    const triageEnabled = state.itemSidebarView === 'inbox' || state.itemSidebarView === 'next';
    list.appendChild(renderSidebarRow({
      icon: icon.icon,
      iconText: icon.text,
      label: String(item?.title || 'Untitled item'),
      subtitle: buildItemSidebarSubtitle(item),
      badges: buildItemSidebarBadges(item),
      meta: formatSidebarAge(item?.updated_at || item?.created_at),
      active: Number(item?.id || 0) === Number(state.itemSidebarActiveItemID || 0),
      item,
      triageEnabled,
      onClick: () => { void openSidebarItem(item); },
    }));
  });
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
  renderItemSidebarActions(list, items);
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
