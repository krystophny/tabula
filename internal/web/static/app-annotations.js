import { refs, state } from './app-context.js';

const acquireMicStream = (...args) => refs.acquireMicStream(...args);
const newMediaRecorder = (...args) => refs.newMediaRecorder(...args);
const showStatus = (...args) => refs.showStatus(...args);
const sttStart = (...args) => refs.sttStart(...args);
const sttSendBlob = (...args) => refs.sttSendBlob(...args);
const sttStop = (...args) => refs.sttStop(...args);
const sttCancel = (...args) => refs.sttCancel(...args);

const ANNOTATION_STORAGE_KEY = 'tabura.annotations.v1';
const HIGHLIGHT_COLOR = 'rgba(253, 230, 138, 0.72)';

let annotationsReady = false;
let activeDescriptor = null;
let bubbleState = null;
let activeVoiceNote = null;

function safeText(value) {
  return String(value == null ? '' : value).trim();
}

function cloneJSON(value, fallback) {
  try {
    return JSON.parse(JSON.stringify(value));
  } catch (_) {
    return fallback;
  }
}

function annotationStore() {
  try {
    const raw = window.localStorage.getItem(ANNOTATION_STORAGE_KEY);
    const parsed = JSON.parse(raw || '{}');
    return parsed && typeof parsed === 'object' ? parsed : {};
  } catch (_) {
    return {};
  }
}

function persistAnnotationStore(next) {
  try {
    window.localStorage.setItem(ANNOTATION_STORAGE_KEY, JSON.stringify(next || {}));
  } catch (_) {}
}

function activeAnnotationKey() {
  if (!activeDescriptor) return '';
  const kind = safeText(activeDescriptor.kind || state.currentCanvasArtifact?.kind || '');
  const title = safeText(activeDescriptor.title || state.currentCanvasArtifact?.title || '');
  const path = safeText(activeDescriptor.path);
  const eventID = safeText(activeDescriptor.event_id || activeDescriptor.eventId);
  const stableID = path || title || eventID;
  if (!stableID) return '';
  return `${kind || 'artifact'}:${stableID}`;
}

function listActiveAnnotations() {
  const key = activeAnnotationKey();
  if (!key) return [];
  const store = annotationStore();
  const entries = store[key];
  return Array.isArray(entries) ? entries : [];
}

function saveActiveAnnotations(entries) {
  const key = activeAnnotationKey();
  if (!key) return;
  const store = annotationStore();
  if (Array.isArray(entries) && entries.length > 0) {
    store[key] = entries;
  } else {
    delete store[key];
  }
  persistAnnotationStore(store);
}

function updateActiveAnnotation(annotationID, updater) {
  const annotations = listActiveAnnotations();
  const index = annotations.findIndex((entry) => safeText(entry?.id) === safeText(annotationID));
  if (index < 0) return null;
  const current = annotations[index];
  const updated = updater(cloneJSON(current, current));
  if (!updated) return null;
  annotations[index] = updated;
  saveActiveAnnotations(annotations);
  return updated;
}

function removeActiveAnnotation(annotationID) {
  const remaining = listActiveAnnotations().filter((entry) => safeText(entry?.id) !== safeText(annotationID));
  saveActiveAnnotations(remaining);
}

function normalizeRects(rects) {
  if (!Array.isArray(rects)) return [];
  return rects
    .map((rect) => ({
      x: Number(rect?.x),
      y: Number(rect?.y),
      width: Number(rect?.width),
      height: Number(rect?.height),
    }))
    .filter((rect) => [rect.x, rect.y, rect.width, rect.height].every((value) => Number.isFinite(value) && value >= 0));
}

function createAnnotationID() {
  return `ann-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

function createNoteID() {
  return `note-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 8)}`;
}

function collectNormalizedClientRects(range, root, options = {}) {
  if (!(range instanceof Range) || !(root instanceof HTMLElement)) return [];
  const rootRect = root.getBoundingClientRect();
  const width = options.scrollable
    ? Math.max(root.scrollWidth, root.clientWidth, 1)
    : Math.max(rootRect.width, 1);
  const height = options.scrollable
    ? Math.max(root.scrollHeight, root.clientHeight, 1)
    : Math.max(rootRect.height, 1);
  return Array.from(range.getClientRects())
    .filter((rect) => rect.width > 0 && rect.height > 0)
    .map((rect) => ({
      x: (rect.left - rootRect.left + (options.scrollable ? root.scrollLeft : 0)) / width,
      y: (rect.top - rootRect.top + (options.scrollable ? root.scrollTop : 0)) / height,
      width: rect.width / width,
      height: rect.height / height,
    }))
    .filter((rect) => rect.width > 0 && rect.height > 0);
}

function normalizeDescriptor(detail) {
  if (!detail || typeof detail !== 'object') return null;
  return {
    kind: safeText(detail.kind),
    title: safeText(detail.title),
    path: safeText(detail.path),
    event_id: safeText(detail.event_id || detail.eventId),
  };
}

function annotationClientRects(annotation) {
  if (!annotation || !Array.isArray(annotation.rects)) return [];
  if (annotation.target === 'pdf') {
    const page = document.querySelector(`.canvas-pdf-page[data-page="${safeText(annotation.page)}"] .canvas-pdf-page-inner`);
    if (!(page instanceof HTMLElement)) return [];
    const bounds = page.getBoundingClientRect();
    const width = Math.max(bounds.width, 1);
    const height = Math.max(bounds.height, 1);
    return normalizeRects(annotation.rects).map((rect) => ({
      left: bounds.left + (rect.x * width),
      top: bounds.top + (rect.y * height),
      width: rect.width * width,
      height: rect.height * height,
    }));
  }
  const pane = document.getElementById('canvas-text');
  if (!(pane instanceof HTMLElement)) return [];
  const bounds = pane.getBoundingClientRect();
  const width = Math.max(pane.scrollWidth, pane.clientWidth, 1);
  const height = Math.max(pane.scrollHeight, pane.clientHeight, 1);
  return normalizeRects(annotation.rects).map((rect) => ({
    left: bounds.left + (rect.x * width) - pane.scrollLeft,
    top: bounds.top + (rect.y * height) - pane.scrollTop,
    width: rect.width * width,
    height: rect.height * height,
  }));
}

function annotationAnchorRect(annotation) {
  const rects = annotationClientRects(annotation);
  return rects[0] || null;
}

function clearRenderedAnnotations() {
  document.querySelectorAll('.canvas-annotation-layer, .canvas-annotation-badge').forEach((node) => node.remove());
}

function ensureTextAnnotationLayer() {
  const pane = document.getElementById('canvas-text');
  if (!(pane instanceof HTMLElement) || !pane.classList.contains('is-active')) return null;
  let layer = pane.querySelector('.canvas-annotation-layer');
  if (!(layer instanceof HTMLElement)) {
    layer = document.createElement('div');
    layer.className = 'canvas-annotation-layer canvas-annotation-layer-text';
    pane.appendChild(layer);
  }
  layer.style.width = `${Math.max(pane.scrollWidth, pane.clientWidth, 1)}px`;
  layer.style.height = `${Math.max(pane.scrollHeight, pane.clientHeight, 1)}px`;
  return layer;
}

function ensurePdfAnnotationLayer(pageNumber) {
  const page = document.querySelector(`.canvas-pdf-page[data-page="${safeText(pageNumber)}"] .canvas-pdf-page-inner`);
  if (!(page instanceof HTMLElement)) return null;
  let layer = page.querySelector('.canvas-annotation-layer');
  if (!(layer instanceof HTMLElement)) {
    layer = document.createElement('div');
    layer.className = 'canvas-annotation-layer canvas-annotation-layer-pdf';
    page.appendChild(layer);
  }
  return layer;
}

function openAnnotationBubble(annotationID) {
  const key = activeAnnotationKey();
  if (!key || !annotationID) return;
  bubbleState = { key, annotationID };
  renderAnnotationBubble();
}

function closeAnnotationBubble() {
  bubbleState = null;
  stopAnnotationVoiceNote(true);
  const bubble = document.getElementById('annotation-bubble');
  if (bubble instanceof HTMLElement) {
    bubble.hidden = true;
  }
}

function ensureAnnotationBubble() {
  let bubble = document.getElementById('annotation-bubble');
  if (bubble instanceof HTMLElement) return bubble;
  bubble = document.createElement('section');
  bubble.id = 'annotation-bubble';
  bubble.className = 'annotation-bubble';
  bubble.hidden = true;
  document.body.appendChild(bubble);
  return bubble;
}

function renderAnnotationBubble() {
  const bubble = ensureAnnotationBubble();
  if (!bubbleState || bubbleState.key !== activeAnnotationKey()) {
    bubble.hidden = true;
    return;
  }
  const annotation = listActiveAnnotations().find((entry) => safeText(entry?.id) === safeText(bubbleState.annotationID));
  if (!annotation) {
    bubble.hidden = true;
    return;
  }
  const anchor = annotationAnchorRect(annotation);
  if (!anchor) {
    bubble.hidden = true;
    return;
  }

  bubble.replaceChildren();
  const preview = document.createElement('div');
  preview.className = 'annotation-bubble-preview';
  preview.textContent = safeText(annotation.text) || 'Highlight';
  bubble.appendChild(preview);

  const notes = document.createElement('div');
  notes.className = 'annotation-bubble-notes';
  const annotationNotes = Array.isArray(annotation.notes) ? annotation.notes : [];
  if (annotationNotes.length === 0) {
    const empty = document.createElement('div');
    empty.className = 'annotation-bubble-empty';
    empty.textContent = 'No notes yet.';
    notes.appendChild(empty);
  } else {
    annotationNotes.forEach((note) => {
      const node = document.createElement('div');
      node.className = 'annotation-bubble-note';
      node.dataset.noteKind = safeText(note?.kind) || 'text';
      node.textContent = safeText(note?.content);
      notes.appendChild(node);
    });
  }
  bubble.appendChild(notes);

  const textarea = document.createElement('textarea');
  textarea.id = 'annotation-note-input';
  textarea.rows = 3;
  textarea.placeholder = 'Add note';
  bubble.appendChild(textarea);

  const controls = document.createElement('div');
  controls.className = 'annotation-bubble-controls';

  const addButton = document.createElement('button');
  addButton.id = 'annotation-note-save';
  addButton.type = 'button';
  addButton.textContent = 'Add note';
  addButton.addEventListener('click', () => {
    const content = safeText(textarea.value);
    if (!content) return;
    updateActiveAnnotation(annotation.id, (entry) => ({
      ...entry,
      notes: [...(Array.isArray(entry.notes) ? entry.notes : []), { id: createNoteID(), kind: 'text', content }],
    }));
    textarea.value = '';
    renderActiveAnnotations();
    renderAnnotationBubble();
  });
  controls.appendChild(addButton);

  const voiceButton = document.createElement('button');
  voiceButton.id = 'annotation-voice-note';
  voiceButton.type = 'button';
  const recording = activeVoiceNote && activeVoiceNote.annotationID === annotation.id;
  voiceButton.textContent = recording ? 'Stop voice' : 'Voice note';
  voiceButton.addEventListener('click', () => {
    if (recording) {
      void stopAnnotationVoiceNote(false);
      return;
    }
    void startAnnotationVoiceNote(annotation.id);
  });
  controls.appendChild(voiceButton);

  const deleteButton = document.createElement('button');
  deleteButton.id = 'annotation-delete';
  deleteButton.type = 'button';
  deleteButton.textContent = 'Delete';
  deleteButton.addEventListener('click', () => {
    removeActiveAnnotation(annotation.id);
    closeAnnotationBubble();
    renderActiveAnnotations();
  });
  controls.appendChild(deleteButton);

  bubble.appendChild(controls);
  bubble.hidden = false;
  bubble.style.left = `${Math.max(12, Math.min(window.innerWidth - 300, anchor.left))}px`;
  bubble.style.top = `${Math.max(12, Math.min(window.innerHeight - 220, anchor.top + anchor.height + 10))}px`;
}

function renderAnnotationBadge(root, annotation, width, height) {
  const rect = normalizeRects(annotation.rects)[0];
  if (!(root instanceof HTMLElement) || !rect) return;
  const notes = Array.isArray(annotation.notes) ? annotation.notes : [];
  if (notes.length === 0) return;
  const badge = document.createElement('button');
  badge.type = 'button';
  badge.className = 'canvas-annotation-badge';
  badge.dataset.annotationId = annotation.id;
  badge.textContent = String(notes.length);
  badge.style.left = `${(rect.x * width) + (rect.width * width) - 10}px`;
  badge.style.top = `${(rect.y * height) - 10}px`;
  badge.addEventListener('click', (event) => {
    event.preventDefault();
    event.stopPropagation();
    openAnnotationBubble(annotation.id);
  });
  root.appendChild(badge);
}

function renderTextAnnotations(annotations) {
  const pane = document.getElementById('canvas-text');
  const layer = ensureTextAnnotationLayer();
  if (!(pane instanceof HTMLElement) || !(layer instanceof HTMLElement)) return;
  const width = Math.max(pane.scrollWidth, pane.clientWidth, 1);
  const height = Math.max(pane.scrollHeight, pane.clientHeight, 1);
  annotations
    .filter((annotation) => annotation?.target === 'text')
    .forEach((annotation) => {
      normalizeRects(annotation.rects).forEach((rect) => {
        const node = document.createElement('button');
        node.type = 'button';
        node.className = 'canvas-user-highlight is-persistent';
        node.dataset.annotationId = annotation.id;
        node.style.left = `${rect.x * width}px`;
        node.style.top = `${rect.y * height}px`;
        node.style.width = `${rect.width * width}px`;
        node.style.height = `${rect.height * height}px`;
        node.style.background = safeText(annotation.color) || HIGHLIGHT_COLOR;
        node.addEventListener('click', (event) => {
          event.preventDefault();
          event.stopPropagation();
          openAnnotationBubble(annotation.id);
        });
        layer.appendChild(node);
      });
      renderAnnotationBadge(pane, annotation, width, height);
    });
}

function renderPdfAnnotations(annotations) {
  annotations
    .filter((annotation) => annotation?.target === 'pdf')
    .forEach((annotation) => {
      const layer = ensurePdfAnnotationLayer(annotation.page);
      const root = document.querySelector(`.canvas-pdf-page[data-page="${safeText(annotation.page)}"] .canvas-pdf-page-inner`);
      if (!(layer instanceof HTMLElement) || !(root instanceof HTMLElement)) return;
      const width = Math.max(root.clientWidth, 1);
      const height = Math.max(root.clientHeight, 1);
      normalizeRects(annotation.rects).forEach((rect) => {
        const node = document.createElement('button');
        node.type = 'button';
        node.className = 'canvas-user-highlight is-persistent';
        node.dataset.annotationId = annotation.id;
        node.style.left = `${rect.x * width}px`;
        node.style.top = `${rect.y * height}px`;
        node.style.width = `${rect.width * width}px`;
        node.style.height = `${rect.height * height}px`;
        node.style.background = safeText(annotation.color) || HIGHLIGHT_COLOR;
        node.addEventListener('click', (event) => {
          event.preventDefault();
          event.stopPropagation();
          openAnnotationBubble(annotation.id);
        });
        layer.appendChild(node);
      });
      renderAnnotationBadge(root, annotation, width, height);
    });
}

export function renderActiveAnnotations() {
  clearRenderedAnnotations();
  const annotations = listActiveAnnotations();
  if (annotations.length === 0) {
    renderAnnotationBubble();
    return;
  }
  renderTextAnnotations(annotations);
  renderPdfAnnotations(annotations);
  renderAnnotationBubble();
}

function buildTextAnnotation(range) {
  const pane = document.getElementById('canvas-text');
  if (!(pane instanceof HTMLElement) || !pane.classList.contains('is-active')) return null;
  const text = safeText(window.getSelection()?.toString());
  const rects = collectNormalizedClientRects(range, pane, { scrollable: true });
  if (!text || rects.length === 0) return null;
  return {
    id: createAnnotationID(),
    type: 'highlight',
    target: 'text',
    text,
    color: HIGHLIGHT_COLOR,
    rects,
    notes: [],
  };
}

function buildPDFAnnotation(range) {
  const start = range.commonAncestorContainer instanceof Element
    ? range.commonAncestorContainer
    : range.commonAncestorContainer?.parentElement;
  const page = start?.closest('.canvas-pdf-page');
  const pageInner = page?.querySelector('.canvas-pdf-page-inner');
  const text = safeText(window.getSelection()?.toString());
  const pageNumber = Number.parseInt(safeText(page?.dataset?.page), 10);
  if (!(pageInner instanceof HTMLElement) || !Number.isFinite(pageNumber) || pageNumber <= 0) return null;
  const rects = collectNormalizedClientRects(range, pageInner, { scrollable: false });
  if (!text || rects.length === 0) return null;
  return {
    id: createAnnotationID(),
    type: 'highlight',
    target: 'pdf',
    page: pageNumber,
    text,
    color: HIGHLIGHT_COLOR,
    rects,
    notes: [],
  };
}

export function createSelectionAnnotation() {
  const selection = window.getSelection();
  if (!selection || selection.rangeCount === 0 || selection.isCollapsed) return false;
  const range = selection.getRangeAt(0);
  const annotation = buildPDFAnnotation(range) || buildTextAnnotation(range);
  if (!annotation) return false;
  const annotations = listActiveAnnotations();
  annotations.push(annotation);
  saveActiveAnnotations(annotations);
  selection.removeAllRanges();
  renderActiveAnnotations();
  openAnnotationBubble(annotation.id);
  return true;
}

async function startAnnotationVoiceNote(annotationID) {
  if (activeVoiceNote) return;
  const stream = await acquireMicStream();
  const recorder = newMediaRecorder(stream);
  const mimeType = safeText(recorder?.mimeType) || 'audio/webm';
  await sttStart(mimeType);
  const recording = { annotationID, recorder, stream, mimeType, cancelled: false };
  activeVoiceNote = recording;
  renderAnnotationBubble();
  recorder.addEventListener('dataavailable', (event) => {
    if (event.data instanceof Blob && event.data.size > 0) {
      void sttSendBlob(event.data);
    }
  });
  recorder.addEventListener('stop', async () => {
    if (activeVoiceNote === recording) {
      activeVoiceNote = null;
    }
    if (recording.cancelled) {
      renderAnnotationBubble();
      return;
    }
    try {
      const result = await sttStop();
      const transcript = safeText(result?.text);
      if (transcript) {
        updateActiveAnnotation(annotationID, (entry) => ({
          ...entry,
          notes: [...(Array.isArray(entry.notes) ? entry.notes : []), { id: createNoteID(), kind: 'voice', content: transcript }],
        }));
        showStatus('voice note added');
      } else {
        showStatus('voice note empty');
      }
    } catch (err) {
      showStatus(`voice note failed: ${safeText(err?.message || err) || 'unknown error'}`);
    } finally {
      renderActiveAnnotations();
      renderAnnotationBubble();
    }
  }, { once: true });
  recorder.start(250);
  showStatus('voice note recording');
}

async function stopAnnotationVoiceNote(cancel) {
  if (!activeVoiceNote) return;
  const current = activeVoiceNote;
  activeVoiceNote = null;
  if (cancel) {
    current.cancelled = true;
    sttCancel();
  }
  try {
    if (current.recorder && current.recorder.state !== 'inactive') {
      current.recorder.stop();
    }
  } catch (_) {}
  if (current.stream?.getTracks) {
    current.stream.getTracks().forEach((track) => {
      try { track.stop(); } catch (_) {}
    });
  }
  renderAnnotationBubble();
}

export function initAnnotationUi() {
  if (annotationsReady) return;
  annotationsReady = true;
  document.addEventListener('tabura:canvas-rendered', (event) => {
    activeDescriptor = normalizeDescriptor(event?.detail);
    renderActiveAnnotations();
  });
  document.addEventListener('tabura:canvas-cleared', () => {
    activeDescriptor = null;
    clearRenderedAnnotations();
    closeAnnotationBubble();
  });
  document.addEventListener('pointerdown', (event) => {
    const bubble = document.getElementById('annotation-bubble');
    if (!(bubble instanceof HTMLElement) || bubble.hidden) return;
    if (event.target instanceof Element && (bubble.contains(event.target) || event.target.closest('.canvas-user-highlight') || event.target.closest('.canvas-annotation-badge'))) {
      return;
    }
    closeAnnotationBubble();
  }, true);
  document.addEventListener('keydown', (event) => {
    if (event.key === 'Escape') {
      closeAnnotationBubble();
    }
  }, true);
  document.addEventListener('scroll', () => {
    if (!bubbleState) return;
    renderAnnotationBubble();
  }, true);
  window.addEventListener('resize', () => {
    if (!bubbleState) return;
    renderActiveAnnotations();
  });
}
