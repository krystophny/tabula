import * as env from './app-env.js';
import * as context from './app-context.js';

const { clearCanvas } = env;
const { refs, state } = context;

const clearInkDraft = (...args) => refs.clearInkDraft(...args);
const refreshWorkspaceBrowser = (...args) => refs.refreshWorkspaceBrowser(...args);
const loadItemSidebarView = (...args) => refs.loadItemSidebarView(...args);
const setPrReviewDrawerOpen = (...args) => refs.setPrReviewDrawerOpen(...args);
const renderPrReviewFileList = (...args) => refs.renderPrReviewFileList(...args);
const hideCanvasColumn = (...args) => refs.hideCanvasColumn(...args);
const applyIPhoneFrameCorners = (...args) => refs.applyIPhoneFrameCorners(...args);
const isFocusedTextInput = (...args) => refs.isFocusedTextInput(...args);
const isIPhoneStandalone = (...args) => refs.isIPhoneStandalone(...args);
const setSyncKeyboardStateNow = (...args) => refs.setSyncKeyboardStateNow(...args);
const stepCanvasFile = (...args) => refs.stepCanvasFile(...args);

const EDGE_TAP_SIZE_PX = 30;
const EDGE_TAP_MOVE_THRESHOLD_PX = 10;
const EDGE_SWIPE_COMMIT_PX = 48;

export function getEdgeTapSizePx() {
  return EDGE_TAP_SIZE_PX;
}

export function getTopEdgeTapSizePx() {
  return EDGE_TAP_SIZE_PX;
}

export function edgePanelsAreOpen() {
  const edgeTop = document.getElementById('edge-top');
  const edgeRight = document.getElementById('edge-right');
  const topOpen = Boolean(edgeTop && edgeTop.classList.contains('edge-pinned'));
  const rightOpen = Boolean(edgeRight && edgeRight.classList.contains('edge-pinned'));
  return topOpen || rightOpen || state.prReviewDrawerOpen;
}

function togglePanel(panel) {
  if (!(panel instanceof HTMLElement)) return;
  if (panel.classList.contains('edge-pinned')) {
    panel.classList.remove('edge-pinned', 'edge-active');
  } else {
    panel.classList.add('edge-active', 'edge-pinned');
  }
}

export function toggleFileSidebarFromEdge() {
  if (!state.prReviewMode) {
    if (state.fileSidebarMode === 'workspace') {
      if (!state.workspaceBrowserLoading && state.workspaceBrowserEntries.length === 0 && !state.workspaceBrowserError) {
        void refreshWorkspaceBrowser(false);
      }
    } else if (!state.itemSidebarLoading && state.itemSidebarItems.length === 0 && !state.itemSidebarError) {
      void loadItemSidebarView(state.itemSidebarView);
    }
  }
  setPrReviewDrawerOpen(!state.prReviewDrawerOpen);
  renderPrReviewFileList();
}

export function toggleRightEdgeDrawer(edgeRight) {
  togglePanel(edgeRight);
}

export function toggleTopEdgeDrawer(edgeTop) {
  togglePanel(edgeTop);
}

export function handleRasaEdgeTap() {
  const hadOpenPanels = edgePanelsAreOpen();
  closeEdgePanels();
  if (hadOpenPanels) return;
  if (state.hasArtifact) {
    clearCanvas();
    hideCanvasColumn();
  }
}

export function isLeftEdgeTapCoordinate(clientX) {
  if (!state.prReviewDrawerOpen) {
    return clientX < EDGE_TAP_SIZE_PX;
  }
  const pane = document.getElementById('pr-file-pane');
  if (!(pane instanceof HTMLElement) || !pane.classList.contains('is-open')) {
    return clientX < EDGE_TAP_SIZE_PX;
  }
  const rect = pane.getBoundingClientRect();
  const zoneStart = Math.max(0, rect.right - EDGE_TAP_SIZE_PX);
  const zoneEnd = Math.min(window.innerWidth, rect.right);
  return clientX >= zoneStart && clientX <= zoneEnd;
}

export function initEdgePanels() {
  const edgeTop = document.getElementById('edge-top');
  const edgeRight = document.getElementById('edge-right');
  const edgeLeftTap = document.getElementById('edge-left-tap');
  const edgeTopTap = document.getElementById('edge-top-tap');
  const edgeRightTap = document.getElementById('edge-right-tap');

  // Tabula Rasa button
  const rasaBtn = document.getElementById('btn-edge-rasa');
  if (rasaBtn) {
    rasaBtn.addEventListener('click', () => {
      clearInkDraft();
      clearCanvas();
      hideCanvasColumn();
      if (edgeTop) edgeTop.classList.remove('edge-active', 'edge-pinned');
    });
  }

  // Edge tap buttons: tap to toggle (prevent double-fire from touch+click)
  const bindEdgeTap = (btn, action, options: Record<string, any> = {}) => {
    if (!btn) return;
    let lastTouchAt = 0;
    let touchState = null;

    const resetTouchState = () => {
      touchState = null;
    };

    btn.addEventListener('touchstart', (ev) => {
      if (ev.touches.length !== 1) {
        resetTouchState();
        return;
      }
      const touch = ev.touches[0];
      touchState = {
        startX: touch.clientX,
        startY: touch.clientY,
        moved: false,
        swiped: false,
      };
    }, { passive: true });

    btn.addEventListener('touchmove', (ev) => {
      if (!touchState || ev.touches.length !== 1) return;
      const touch = ev.touches[0];
      const dx = touch.clientX - touchState.startX;
      const dy = touch.clientY - touchState.startY;
      if (!touchState.moved && Math.hypot(dx, dy) > EDGE_TAP_MOVE_THRESHOLD_PX) {
        touchState.moved = true;
      }
      if (!options.allowHorizontalCanvasSwipe || touchState.swiped || !state.hasArtifact) return;
      if (Math.abs(dx) < EDGE_SWIPE_COMMIT_PX) return;
      if (Math.abs(dx) <= Math.abs(dy) * 1.25) return;
      if (!stepCanvasFile(dx < 0 ? 1 : -1)) return;
      touchState.swiped = true;
      lastTouchAt = Date.now();
      ev.preventDefault();
    }, { passive: false });

    btn.addEventListener('touchend', (ev) => {
      ev.preventDefault();
      if (touchState?.swiped || touchState?.moved) {
        lastTouchAt = Date.now();
        resetTouchState();
        return;
      }
      lastTouchAt = Date.now();
      resetTouchState();
      action();
    }, { passive: false });
    btn.addEventListener('touchcancel', resetTouchState, { passive: true });
    btn.addEventListener('click', (ev) => {
      ev.preventDefault();
      if (Date.now() - lastTouchAt < 500) return;
      action();
    });
  };
  bindEdgeTap(edgeLeftTap, () => toggleFileSidebarFromEdge());
  bindEdgeTap(edgeRightTap, () => toggleRightEdgeDrawer(edgeRight), {
    allowHorizontalCanvasSwipe: true,
  });
  bindEdgeTap(edgeTopTap, () => toggleTopEdgeDrawer(edgeTop));

  // Blur chat input when app goes to background
  document.addEventListener('visibilitychange', () => {
    if (document.hidden) {
      const cpInput = document.getElementById('chat-pane-input');
      if (cpInput && document.activeElement === cpInput) cpInput.blur();
    }
  });

  // Keyboard state tracking for mobile
  if (window.visualViewport) {
    const inputRow = document.querySelector('.chat-pane-input-row');
    if (inputRow) {
      const root = document.documentElement;
      const setKeyboardOpen = (keyboardOpen) => {
        inputRow.classList.toggle('keyboard-open', keyboardOpen);
        document.body.classList.toggle('keyboard-open', keyboardOpen);
        if (!isIPhoneStandalone()) return;
        if (keyboardOpen) {
          root.style.setProperty('--cue-corner-radius', '0 0 0 0');
        } else {
          applyIPhoneFrameCorners();
        }
      };
      let baselineHeight = Math.max(
        window.innerHeight,
        window.visualViewport.height + Math.max(0, window.visualViewport.offsetTop || 0),
      );
      const syncKeyboardState = () => {
        const vv = window.visualViewport;
        if (!vv) return;
        const offsetTop = Math.max(0, Number(vv.offsetTop) || 0);
        const viewportExtent = vv.height + offsetTop;
        if (viewportExtent > baselineHeight) baselineHeight = viewportExtent;
        const focused = isFocusedTextInput();
        const shifted = offsetTop > 1;
        const shrunkenWhileFocused = focused && viewportExtent < baselineHeight - 100;
        const keyboardOpen = shifted || shrunkenWhileFocused;
        setKeyboardOpen(keyboardOpen);
        if (!keyboardOpen) {
          baselineHeight = Math.max(window.innerHeight, viewportExtent);
        }
      };
      window.visualViewport.addEventListener('resize', syncKeyboardState);
      window.visualViewport.addEventListener('scroll', syncKeyboardState);
      window.addEventListener('orientationchange', () => {
        baselineHeight = Math.max(
          window.innerHeight,
          window.visualViewport
            ? (window.visualViewport.height + Math.max(0, window.visualViewport.offsetTop || 0))
            : window.innerHeight,
        );
        window.setTimeout(syncKeyboardState, 80);
      });
      document.addEventListener('focusin', syncKeyboardState, true);
      document.addEventListener('focusout', () => {
        window.setTimeout(syncKeyboardState, 80);
        window.setTimeout(syncKeyboardState, 260);
      }, true);
      setSyncKeyboardStateNow(syncKeyboardState);
      syncKeyboardState();
    }
  }
}

export function closeEdgePanels() {
  const edgeTop = document.getElementById('edge-top');
  const edgeRight = document.getElementById('edge-right');
  if (edgeTop) edgeTop.classList.remove('edge-active', 'edge-pinned');
  if (edgeRight) edgeRight.classList.remove('edge-active', 'edge-pinned');
  if (state.prReviewDrawerOpen) {
    setPrReviewDrawerOpen(false);
  }
}
