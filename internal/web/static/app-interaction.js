import * as context from './app-context.js';

const { refs, state, TOOL_PALETTE_MODES } = context;

const clearInkDraft = (...args) => refs.clearInkDraft(...args);
const renderInkControls = (...args) => refs.renderInkControls(...args);
const updateRuntimePreferences = (...args) => refs.updateRuntimePreferences(...args);
const syncInteractionBodyState = (...args) => refs.syncInteractionBodyState(...args);

export function normalizeInteractionTool(modeRaw) {
  const mode = String(modeRaw || '').trim().toLowerCase();
  if (mode === 'highlight' || mode === 'select') return 'highlight';
  if (mode === 'ink' || mode === 'draw' || mode === 'pen') return 'ink';
  if (mode === 'text_note' || mode === 'text-note' || mode === 'text' || mode === 'note' || mode === 'keyboard' || mode === 'typing') return 'text_note';
  if (mode === 'prompt' || mode === 'voice' || mode === 'talk' || mode === 'mic') return 'prompt';
  return 'pointer';
}

export function normalizeInteractionSurface(modeRaw) {
  return String(modeRaw || '').trim().toLowerCase() === 'editor' ? 'editor' : 'annotate';
}

export function interactionConversationMode() {
  if (state.liveSessionActive
    && (state.liveSessionMode === 'dialogue' || state.liveSessionMode === 'meeting')) {
    return 'continuous_dialogue';
  }
  return 'push_to_talk';
}

export function isInkTool() {
  return state.interaction.tool === 'ink';
}

export function prefersTextComposer() {
  return state.interaction.surface === 'editor' || state.interaction.tool === 'text_note';
}

export function isPromptTool() {
  return state.interaction.tool === 'prompt';
}

export function currentCanvasPaneId() {
  const pane = document.querySelector('#canvas-viewport .canvas-pane.is-active');
  return pane instanceof HTMLElement ? pane.id : '';
}

export function canToggleInteractionSurface() {
  return state.hasArtifact && currentCanvasPaneId() === 'canvas-text' && !state.prReviewMode;
}

export function interactionSurfaceDefaultForPane(paneId) {
  if (state.prReviewMode) return 'annotate';
  return paneId === 'canvas-text' ? 'editor' : 'annotate';
}

export function applyInteractionDefaultsForPane(paneId) {
  state.interaction.conversation = interactionConversationMode();
  state.interaction.surface = interactionSurfaceDefaultForPane(paneId);
  if (state.interaction.surface !== 'annotate') {
    clearInkDraft();
  }
  syncInteractionBodyState();
  renderInkControls();
  renderInteractionSurfaceToggle();
  renderToolPalette();
}

export function setInteractionSurface(surface) {
  const nextSurface = normalizeInteractionSurface(surface);
  if (state.interaction.surface === nextSurface) return;
  state.interaction.surface = nextSurface;
  if (nextSurface !== 'annotate') {
    clearInkDraft();
  }
  syncInteractionBodyState();
  renderInkControls();
  renderInteractionSurfaceToggle();
  renderToolPalette();
}

export function renderInteractionSurfaceToggle() {
  const host = document.getElementById('surface-toggle');
  if (!(host instanceof HTMLButtonElement)) return;
  if (!canToggleInteractionSurface()) {
    host.style.display = 'none';
    return;
  }
  host.style.display = '';
  const nextSurface = state.interaction.surface === 'editor' ? 'annotate' : 'editor';
  host.dataset.surface = state.interaction.surface;
  host.textContent = nextSurface === 'annotate' ? 'Annotate' : 'Editor';
  host.setAttribute('aria-label', `Switch to ${nextSurface}`);
  host.setAttribute('title', `Switch to ${nextSurface}`);
  host.setAttribute('aria-pressed', state.interaction.surface === 'annotate' ? 'true' : 'false');
}

export async function selectInteractionTool(tool) {
  const nextTool = normalizeInteractionTool(tool);
  if (state.interaction.surface !== 'annotate') {
    state.interaction.surface = 'annotate';
  }
  await updateRuntimePreferences({ tool: nextTool });
}

export function renderToolPalette() {
  const host = document.getElementById('tool-palette');
  if (!(host instanceof HTMLElement)) return;
  if (state.interaction.surface !== 'annotate') {
    host.replaceChildren();
    host.style.display = 'none';
    return;
  }
  host.style.display = '';
  host.replaceChildren();
  const disabled = state.projectSwitchInFlight || state.projectModelSwitchInFlight;
  for (const mode of TOOL_PALETTE_MODES) {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = 'tool-palette-btn';
    button.dataset.mode = mode.id;
    button.setAttribute('aria-label', mode.label);
    button.setAttribute('title', mode.label);
    button.setAttribute('aria-pressed', state.interaction.tool === mode.id ? 'true' : 'false');
    if (state.interaction.tool === mode.id) {
      button.classList.add('is-active');
    }
    button.disabled = disabled;
    button.innerHTML = mode.icon;
    button.addEventListener('click', () => {
      updateRuntimePreferences({ tool: mode.id })
        .then(() => {
          if (mode.id !== 'ink') {
            clearInkDraft();
          }
          renderInkControls();
          refs.showStatus(`${mode.id.replace('_', ' ')} tool on`);
        })
        .catch((err) => {
          refs.showStatus(`tool switch failed: ${String(err?.message || err || 'unknown error')}`);
        });
    });
    host.appendChild(button);
  }
}
