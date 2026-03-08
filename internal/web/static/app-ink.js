import * as env from './app-env.js';
import * as context from './app-context.js';

const { marked, apiURL, wsURL, renderCanvas, clearCanvas, getLocationFromSelection, clearLineHighlight, escapeHtml, sanitizeHtml, getActiveArtifactTitle, getActiveTextEventId, getPreviousArtifactText, getUiState, setUiMode, showIndicatorMode, hideIndicator, showTextInput, hideTextInput, showOverlay, hideOverlay, updateOverlay, isOverlayVisible, isTextInputVisible, isRecording, setRecording, getInputAnchor, setInputAnchor, getAnchorFromPoint, buildContextPrefix, getLastInputPosition, setLastInputPosition, configureLiveSession, getLiveSessionSnapshot, handleLiveSessionMessage, isLiveSessionListenActive, LIVE_SESSION_HOTWORD_DEFAULT, LIVE_SESSION_MODE_DIALOGUE, LIVE_SESSION_MODE_MEETING, onLiveSessionTTSPlaybackComplete, cancelLiveSessionListen, startLiveSession, stopLiveSession, initHotword, startHotwordMonitor, stopHotwordMonitor, isHotwordActive, onHotwordDetected, setHotwordThreshold, setHotwordAudioContext, getPreRollAudio, getHotwordMicStream, initVAD, ensureVADLoaded, float32ToWav } = env;
const { refs, state, getState, isVoiceTurn, COMPANION_VIEW_PATH_PREFIX, COMPANION_TRANSCRIPT_VIEW_PATH, COMPANION_SUMMARY_VIEW_PATH, COMPANION_REFERENCES_VIEW_PATH, MEETING_TRANSCRIPT_LABEL, MEETING_SUMMARY_LABEL, MEETING_REFERENCES_LABEL, MEETING_SUMMARY_ITEMS_PANEL_ID, CHAT_CTRL_LONG_PRESS_MS, ARTIFACT_EDIT_LONG_TAP_MS, ITEM_SIDEBAR_VIEWS, ITEM_SIDEBAR_GESTURE_CANCEL_PX, ITEM_SIDEBAR_GESTURE_COMMIT_PX, ITEM_SIDEBAR_GESTURE_LONG_PX, ITEM_SIDEBAR_DEFAULT_LATER_HOUR_UTC, ITEM_SIDEBAR_MENU_ID, DEV_UI_RELOAD_POLL_MS, ASSISTANT_ACTIVITY_POLL_MS, CHAT_WS_STALE_THRESHOLD_MS, ACTIVE_TURN_NO_ID_CLEAR_GRACE_MS, ACTIVE_TURN_ACTIVITY_CLEAR_GRACE_MS, PROJECT_CHAT_MODEL_ALIASES, PROJECT_CHAT_MODEL_REASONING_EFFORTS, TTS_SILENT_STORAGE_KEY, YOLO_MODE_STORAGE_KEY, SOMEDAY_REVIEW_NUDGE_ENABLED_STORAGE_KEY, SOMEDAY_REVIEW_NUDGE_LAST_SHOWN_STORAGE_KEY, SOMEDAY_REVIEW_NUDGE_INTERVAL_MS, ACTIVE_PROJECT_STORAGE_KEY, LAST_VIEW_STORAGE_KEY, RUNTIME_RELOAD_CONTEXT_STORAGE_KEY, SIDEBAR_IMAGE_EXTENSIONS, PANEL_MOTION_WATCH_QUERIES, VOICE_LIFECYCLE, COMPANION_IDLE_SURFACES, COMPANION_RUNTIME_STATES, TOOL_PALETTE_MODES } = context;

const showStatus = (...args) => refs.showStatus(...args);
const renderEdgeTopModelButtons = (...args) => refs.renderEdgeTopModelButtons(...args);
const renderEdgeTopProjects = (...args) => refs.renderEdgeTopProjects(...args);
const openWorkspaceSidebarFile = (...args) => refs.openWorkspaceSidebarFile(...args);
const activeProject = (...args) => refs.activeProject(...args);
const normalizeProjectRunState = (...args) => refs.normalizeProjectRunState(...args);
const isPenInputMode = (...args) => refs.isPenInputMode(...args);

export function activeArtifactKindForInk() {
  const activePane = document.querySelector('#canvas-viewport .canvas-pane.is-active');
  if (!(activePane instanceof HTMLElement)) return 'text';
  if (activePane.id === 'canvas-pdf') return 'pdf';
  if (activePane.id === 'canvas-image') return 'image';
  return 'text';
}

export function resetInkDraftState() {
  state.inkDraft.activePointerId = null;
  state.inkDraft.activePointerType = '';
  state.inkDraft.activePath = null;
}

export function inkLayerEl() {
  const node = document.getElementById('ink-layer');
  return node instanceof SVGSVGElement ? node : null;
}

export function renderInkControls() {
  const controls = document.getElementById('ink-controls');
  if (!(controls instanceof HTMLElement)) return;
  const visible = isPenInputMode() && state.inkDraft.dirty;
  controls.style.display = visible ? '' : 'none';
  document.body.classList.toggle('ink-controls-visible', visible);
  const submit = document.getElementById('ink-submit');
  const clear = document.getElementById('ink-clear');
  if (submit instanceof HTMLButtonElement) submit.disabled = state.inkSubmitInFlight;
  if (clear instanceof HTMLButtonElement) clear.disabled = state.inkSubmitInFlight;
}

export function syncInputModeBodyState() {
  document.body.classList.toggle('pen-input-mode', isPenInputMode());
}

export function setPenInkingState(active) {
  document.body.classList.toggle('pen-inking', Boolean(active));
}

export function clearInkDraft() {
  const layer = inkLayerEl();
  if (layer) layer.innerHTML = '';
  state.inkDraft.strokes = [];
  state.inkDraft.dirty = false;
  resetInkDraftState();
  setPenInkingState(false);
  renderInkControls();
}

export function syncInkLayerSize() {
  const layer = inkLayerEl();
  const viewport = document.getElementById('canvas-viewport');
  if (!(layer instanceof SVGSVGElement) || !(viewport instanceof HTMLElement)) return;
  const rect = viewport.getBoundingClientRect();
  const width = Math.max(1, Math.round(rect.width));
  const height = Math.max(1, Math.round(rect.height));
  layer.setAttribute('viewBox', `0 0 ${width} ${height}`);
  layer.setAttribute('width', `${width}`);
  layer.setAttribute('height', `${height}`);
}

export function pointForViewportEvent(clientX, clientY) {
  const viewport = document.getElementById('canvas-viewport');
  if (!(viewport instanceof HTMLElement)) {
    return { x: clientX, y: clientY };
  }
  const rect = viewport.getBoundingClientRect();
  return {
    x: clientX - rect.left + viewport.scrollLeft,
    y: clientY - rect.top + viewport.scrollTop,
  };
}

export function appendInkPointToPath(pathEl, stroke) {
  if (!(pathEl instanceof SVGPathElement) || !stroke || !Array.isArray(stroke.points) || stroke.points.length === 0) return;
  const d = stroke.points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' ');
  pathEl.setAttribute('d', d);
}

export function beginInkStroke(pointerEvent) {
  const layer = inkLayerEl();
  if (!(layer instanceof SVGSVGElement)) return false;
  syncInkLayerSize();
  const point = pointForViewportEvent(pointerEvent.clientX, pointerEvent.clientY);
  const stroke = {
    pointer_type: String(pointerEvent.pointerType || 'pen').trim().toLowerCase() || 'pen',
    width: Math.max(1.5, Number(pointerEvent.pressure) > 0 ? 1.8 + Number(pointerEvent.pressure) * 2.8 : 2.4),
    points: [{
      x: point.x,
      y: point.y,
      pressure: Number(pointerEvent.pressure) || 0,
    }],
  };
  const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
  path.setAttribute('stroke-width', stroke.width.toFixed(2));
  appendInkPointToPath(path, stroke);
  layer.appendChild(path);
  state.inkDraft.strokes.push(stroke);
  state.inkDraft.activePointerId = pointerEvent.pointerId;
  state.inkDraft.activePointerType = stroke.pointer_type;
  state.inkDraft.activePath = path;
  state.inkDraft.dirty = true;
  renderInkControls();
  return true;
}

export function extendInkStroke(pointerEvent) {
  if (state.inkDraft.activePointerId !== pointerEvent.pointerId) return false;
  const stroke = state.inkDraft.strokes[state.inkDraft.strokes.length - 1];
  const path = state.inkDraft.activePath;
  if (!stroke || !(path instanceof SVGPathElement)) return false;
  const point = pointForViewportEvent(pointerEvent.clientX, pointerEvent.clientY);
  stroke.points.push({
    x: point.x,
    y: point.y,
    pressure: Number(pointerEvent.pressure) || 0,
  });
  appendInkPointToPath(path, stroke);
  return true;
}

export function buildInkSVGMarkup() {
  const layer = inkLayerEl();
  if (!(layer instanceof SVGSVGElement)) return '';
  syncInkLayerSize();
  const viewBox = layer.getAttribute('viewBox') || '0 0 1 1';
  return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="${viewBox}">${layer.innerHTML}</svg>`;
}

export function buildInkPNGBase64() {
  syncInkLayerSize();
  const layer = inkLayerEl();
  if (!(layer instanceof SVGSVGElement)) return '';
  const viewBox = String(layer.getAttribute('viewBox') || '').trim();
  const parts = viewBox.split(/\s+/).map((part) => Number(part));
  const width = Math.max(1, Math.round(parts[2] || Number(layer.getAttribute('width')) || 1));
  const height = Math.max(1, Math.round(parts[3] || Number(layer.getAttribute('height')) || 1));
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  const ctx = canvas.getContext('2d');
  if (!ctx) return '';
  ctx.fillStyle = '#ffffff';
  ctx.fillRect(0, 0, width, height);
  ctx.lineCap = 'round';
  ctx.lineJoin = 'round';
  ctx.strokeStyle = '#111827';
  for (const stroke of state.inkDraft.strokes) {
    const points = Array.isArray(stroke?.points) ? stroke.points : [];
    if (points.length === 0) continue;
    ctx.beginPath();
    ctx.lineWidth = Math.max(1.5, Number(stroke?.width) || 2.4);
    ctx.moveTo(Number(points[0]?.x) || 0, Number(points[0]?.y) || 0);
    for (let i = 1; i < points.length; i += 1) {
      ctx.lineTo(Number(points[i]?.x) || 0, Number(points[i]?.y) || 0);
    }
    if (points.length === 1) {
      ctx.lineTo((Number(points[0]?.x) || 0) + 0.01, Number(points[0]?.y) || 0);
    }
    ctx.stroke();
  }
  return canvas.toDataURL('image/png').replace(/^data:image\/png;base64,/, '');
}

export async function submitInkDraft() {
  if (state.inkSubmitInFlight || state.inkDraft.strokes.length === 0) return false;
  const project = activeProject();
  if (!project?.id) return false;
  const wasBlankCanvas = !state.hasArtifact;
  state.inkSubmitInFlight = true;
  renderInkControls();
  try {
    const payload = {
      project_id: project.id,
      artifact_kind: activeArtifactKindForInk(),
      artifact_title: String(getActiveArtifactTitle() || ''),
      artifact_path: String(state.workspaceOpenFilePath || ''),
      strokes: state.inkDraft.strokes.map((stroke) => ({
        pointer_type: stroke.pointer_type,
        width: stroke.width,
        points: stroke.points.map((point) => ({
          x: point.x,
          y: point.y,
          pressure: point.pressure,
        })),
      })),
      svg: buildInkSVGMarkup(),
      png_base64: buildInkPNGBase64(),
    };
    const resp = await fetch(apiURL('ink/submit'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    const result = await resp.json();
    const pngPath = String(result?.ink_png_path || '').trim();
    const summaryPath = String(result?.summary_path || '').trim();
    const inkPath = String(result?.ink_svg_path || '').trim();
    const revisionHistoryPath = String(result?.revision_history_path || '').trim();
    clearInkDraft();
    if (revisionHistoryPath) {
      showStatus(`ink saved: ${revisionHistoryPath}`);
    } else if (summaryPath) {
      showStatus(`ink saved: ${summaryPath}`);
    } else if (inkPath) {
      showStatus(`ink saved: ${inkPath}`);
    } else {
      showStatus('ink saved');
    }
    if (pngPath) {
      await openWorkspaceSidebarFile(pngPath);
    } else if (summaryPath) {
      await openWorkspaceSidebarFile(summaryPath);
    } else if (inkPath) {
      await openWorkspaceSidebarFile(inkPath);
    }
    if (wasBlankCanvas && pngPath) {
      showStatus(`ink saved as image: ${pngPath}`);
    }
    return true;
  } catch (err) {
    showStatus(`ink submit failed: ${String(err?.message || err || 'unknown error')}`);
    return false;
  } finally {
    state.inkSubmitInFlight = false;
    renderInkControls();
  }
}

export async function fetchProjects() {
  const resp = await fetch(apiURL('projects'), { cache: 'no-store' });
  if (!resp.ok) throw new Error(`projects list failed: HTTP ${resp.status}`);
  const payload = await resp.json();
  const projects = Array.isArray(payload?.projects) ? payload.projects : [];
  state.projects = projects.map((project) => ({
    ...project,
    id: String(project?.id || ''),
    chat_mode: String(project?.chat_mode || 'chat'),
    chat_model_reasoning_effort: String(project?.chat_model_reasoning_effort || '').trim().toLowerCase(),
    run_state: normalizeProjectRunState(project?.run_state),
    unread: Boolean(project?.unread),
    review_pending: Boolean(project?.review_pending),
  })).filter((project) => project.id);
  state.defaultProjectId = String(payload?.default_project_id || '').trim();
  state.serverActiveProjectId = String(payload?.active_project_id || '').trim();
  renderEdgeTopProjects();
  renderEdgeTopModelButtons();
}
