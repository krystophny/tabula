import { marked } from './vendor/marked.esm.js';

const els = {};

function getEls() {
  if (!els.empty) {
    els.empty = document.getElementById('canvas-empty');
    els.text = document.getElementById('canvas-text');
    els.image = document.getElementById('canvas-image');
    els.img = document.getElementById('canvas-img');
    els.pdf = document.getElementById('canvas-pdf');
    els.title = document.getElementById('canvas-title');
    els.mode = document.getElementById('canvas-mode');
  }
  return els;
}

function hideAll() {
  const e = getEls();
  e.empty.style.display = 'none';
  e.text.style.display = 'none';
  e.image.style.display = 'none';
  e.pdf.style.display = 'none';
}

export function renderCanvas(event) {
  const e = getEls();

  if (event.kind === 'text_artifact') {
    hideAll();
    e.text.style.display = '';
    e.text.innerHTML = marked.parse(event.text || '');
    e.title.textContent = event.title || 'Text';
    e.mode.textContent = 'review';
    e.mode.className = 'badge review';
    setupTextSelection(event.event_id);
  } else if (event.kind === 'image_artifact') {
    hideAll();
    e.image.style.display = '';
    const state = (window._tabulaApp || {}).getState ? window._tabulaApp.getState() : {};
    const sid = state.sessionId || '';
    e.img.src = `/api/files/${sid}/${encodeURIComponent(event.path)}`;
    e.img.alt = event.title || 'Image';
    e.title.textContent = event.title || 'Image';
    e.mode.textContent = 'review';
    e.mode.className = 'badge review';
  } else if (event.kind === 'pdf_artifact') {
    hideAll();
    e.pdf.style.display = '';
    const pdfState = (window._tabulaApp || {}).getState ? window._tabulaApp.getState() : {};
    const pdfSid = pdfState.sessionId || '';
    e.pdf.innerHTML = `<iframe src="/api/files/${pdfSid}/${encodeURIComponent(event.path)}" style="width:100%;height:100%;border:none;"></iframe>`;
    e.title.textContent = event.title || 'PDF';
    e.mode.textContent = 'review';
    e.mode.className = 'badge review';
  } else if (event.kind === 'clear_canvas') {
    clearCanvas();
  }
}

export function clearCanvas() {
  const e = getEls();
  if (e.text._selectionHandler) {
    document.removeEventListener('selectionchange', e.text._selectionHandler);
    e.text._selectionHandler = null;
  }
  hideAll();
  e.empty.style.display = '';
  e.title.textContent = 'Canvas';
  e.mode.textContent = 'prompt';
  e.mode.className = 'badge';
}

function setupTextSelection(eventId) {
  const e = getEls();
  if (e.text._selectionHandler) {
    document.removeEventListener('selectionchange', e.text._selectionHandler);
  }
  const handler = () => {
    const selection = window.getSelection();
    if (!selection || selection.isCollapsed) {
      sendSelectionFeedback(eventId, null);
      return;
    }
    const text = selection.toString();
    if (!text) {
      sendSelectionFeedback(eventId, null);
      return;
    }

    const range = selection.getRangeAt(0);
    const fullText = e.text.textContent || '';
    const lines = fullText.split('\n');

    let charCount = 0;
    let lineStart = 1;
    let lineEnd = 1;
    const startOffset = getTextOffset(e.text, range.startContainer, range.startOffset);
    const endOffset = getTextOffset(e.text, range.endContainer, range.endOffset);

    for (let i = 0; i < lines.length; i++) {
      if (charCount + lines[i].length >= startOffset) {
        lineStart = i + 1;
        break;
      }
      charCount += lines[i].length + 1;
    }
    charCount = 0;
    for (let i = 0; i < lines.length; i++) {
      if (charCount + lines[i].length >= endOffset) {
        lineEnd = i + 1;
        break;
      }
      charCount += lines[i].length + 1;
    }

    sendSelectionFeedback(eventId, { line_start: lineStart, line_end: lineEnd, text });
  };

  document.addEventListener('selectionchange', handler);
  e.text._selectionHandler = handler;
}

function getTextOffset(root, node, offset) {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let total = 0;
  while (walker.nextNode()) {
    if (walker.currentNode === node) return total + offset;
    total += walker.currentNode.textContent.length;
  }
  return total + offset;
}

function sendSelectionFeedback(eventId, selection) {
  const { getState } = window._tabulaApp || {};
  if (!getState) return;
  const state = getState();
  if (!state.canvasWs || state.canvasWs.readyState !== WebSocket.OPEN) return;

  const payload = {
    kind: 'text_selection',
    event_id: eventId,
  };
  if (selection) {
    payload.line_start = selection.line_start;
    payload.line_end = selection.line_end;
    payload.text = selection.text;
  } else {
    payload.line_start = null;
    payload.line_end = null;
    payload.text = null;
  }
  state.canvasWs.send(JSON.stringify(payload));
}
