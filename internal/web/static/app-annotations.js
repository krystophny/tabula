import { apiURL } from './app-env.js';
import { refs, state } from './app-context.js';

const acquireMicStream = (...args) => refs.acquireMicStream(...args);
const newMediaRecorder = (...args) => refs.newMediaRecorder(...args);
const showStatus = (...args) => refs.showStatus(...args);
const sttStart = (...args) => refs.sttStart(...args);
const sttSendBlob = (...args) => refs.sttSendBlob(...args);
const sttStop = (...args) => refs.sttStop(...args);
const sttCancel = (...args) => refs.sttCancel(...args);
const submitMessage = (...args) => refs.submitMessage(...args);
const openSidebarArtifactItem = (...args) => refs.openSidebarArtifactItem(...args);

const ANNOTATION_STORAGE_KEY = 'tabura.annotations.v1';
const HIGHLIGHT_COLOR = 'rgba(253, 230, 138, 0.72)';
const STICKY_NOTE_LABEL = 'Sticky note';
const INK_NOTE_LABEL = 'Ink annotation';

let annotationsReady = false;
let activeDescriptor = null;
let bubbleState = null;
let activeVoiceNote = null;
let scanUploadInFlight = false;

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

function clamp01(value) {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(1, value));
}

function annotationPrimaryRect(annotation) {
  return normalizeRects(annotation?.rects)[0] || null;
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

function activeArtifactDescriptor() {
  const current = state.currentCanvasArtifact || {};
  return {
    kind: safeText(activeDescriptor?.kind || current.kind),
    title: safeText(activeDescriptor?.title || current.title),
    path: safeText(activeDescriptor?.path || current.path),
    event_id: safeText(activeDescriptor?.event_id || activeDescriptor?.eventId || current.event_id || current.eventId),
  };
}

function activeArtifactLabel() {
  const descriptor = activeArtifactDescriptor();
  return descriptor.title || descriptor.path || descriptor.kind || 'current artifact';
}

function activeArtifactBundleInstruction() {
  const descriptor = activeArtifactDescriptor();
  const title = descriptor.title.toLowerCase();
  const kind = descriptor.kind.toLowerCase();
  if (state.prReviewMode || title.endsWith('.diff') || kind === 'pr_diff') {
    return 'Draft review feedback for the current diff using these annotations.';
  }
  if (kind === 'email' || title.startsWith('re:') || title.startsWith('fw:')) {
    return 'Draft an email reply using these annotations.';
  }
  if (kind === 'text_artifact' || kind === 'document' || kind === 'markdown') {
    return 'Revise the current artifact using these annotations.';
  }
  return 'Use these annotations as instructions for the current artifact.';
}

function formatAnnotationTarget(annotation) {
  if (annotation?.target === 'pdf') {
    const page = Number.parseInt(safeText(annotation?.page), 10);
    const line = Number.parseInt(safeText(annotation?.line), 10);
    if (annotation?.type === 'sticky_note') {
      return Number.isFinite(page) && page > 0 ? `PDF sticky note on page ${page}` : 'PDF sticky note';
    }
    if (annotation?.type === 'ink') {
      return Number.isFinite(page) && page > 0 ? `PDF ink on page ${page}` : 'PDF ink';
    }
    if (Number.isFinite(line) && line > 0 && Number.isFinite(page) && page > 0) {
      return `PDF page ${page}, line ${line}`;
    }
    return Number.isFinite(page) && page > 0 ? `PDF page ${page}` : 'PDF selection';
  }
  const line = Number.parseInt(safeText(annotation?.line), 10);
  if (Number.isFinite(line) && line > 0) {
    return `Text line ${line}`;
  }
  return 'Text selection';
}

function annotationPreviewText(annotation) {
  const explicit = safeText(annotation?.text);
  if (explicit) return explicit;
  if (annotation?.type === 'sticky_note') return STICKY_NOTE_LABEL;
  if (annotation?.type === 'ink') return INK_NOTE_LABEL;
  return 'Highlight';
}

function formatAnnotationBundleText(annotations, options = {}) {
  if (!Array.isArray(annotations) || annotations.length === 0) return '';
  const immediate = options.immediate === true;
  const lines = [
    immediate
      ? 'Handle this annotation immediately instead of waiting for a larger bundle.'
      : activeArtifactBundleInstruction(),
    `Artifact: ${activeArtifactLabel()}`,
  ];
  const descriptor = activeArtifactDescriptor();
  if (descriptor.kind) {
    lines.push(`Artifact kind: ${descriptor.kind}`);
  }
  annotations.forEach((annotation, index) => {
    lines.push('');
    lines.push(`Annotation ${index + 1}: ${formatAnnotationTarget(annotation)}`);
    lines.push(`Selection: "${safeText(annotation?.text) || 'Untitled annotation'}"`);
    const notes = Array.isArray(annotation?.notes) ? annotation.notes : [];
    if (notes.length === 0) {
      lines.push('Notes: none');
      return;
    }
    lines.push('Notes:');
    notes.forEach((note) => {
      const kind = safeText(note?.kind) || 'text';
      const content = safeText(note?.content) || '(empty)';
      lines.push(`- ${kind}: ${content}`);
    });
  });
  return lines.join('\n').trim();
}

async function submitAnnotationBundle(annotationID = '') {
  const annotations = listActiveAnnotations();
  if (annotations.length === 0) {
    showStatus('no annotations to send');
    return false;
  }
  const targetID = safeText(annotationID);
  const selected = targetID
    ? annotations.filter((annotation) => safeText(annotation?.id) === targetID)
    : annotations;
  if (selected.length === 0) {
    showStatus('annotation missing');
    return false;
  }
  const bundleText = formatAnnotationBundleText(selected, { immediate: Boolean(targetID) });
  if (!bundleText) {
    showStatus('annotation bundle empty');
    return false;
  }
  try {
    await confirmImportedScanAnnotations(selected);
  } catch (err) {
    showStatus(`scan confirm failed: ${safeText(err?.message || err) || 'unknown error'}`);
    return false;
  }
  const ok = await submitMessage(bundleText, {
    kind: targetID ? 'annotation_immediate' : 'annotation_bundle',
  });
  if (!ok) return false;
  if (targetID) {
    saveActiveAnnotations(annotations.filter((annotation) => safeText(annotation?.id) !== targetID));
  } else {
    saveActiveAnnotations([]);
  }
  closeAnnotationBubble();
  renderActiveAnnotations();
  showStatus(targetID ? 'annotation sent' : 'annotation bundle sent');
  return true;
}

function clearAllActiveAnnotations() {
  if (listActiveAnnotations().length === 0) {
    showStatus('no annotations to clear');
    return;
  }
  saveActiveAnnotations([]);
  closeAnnotationBubble();
  renderActiveAnnotations();
  showStatus('annotations cleared');
}

function activeSidebarItemForScan() {
  const activeID = Number(state.itemSidebarActiveItemID || 0);
  if (activeID <= 0) return null;
  const items = Array.isArray(state.itemSidebarItems) ? state.itemSidebarItems : [];
  return items.find((entry) => Number(entry?.id || 0) === activeID) || null;
}

function normalizeScanBounds(bounds) {
  if (!bounds || typeof bounds !== 'object') return null;
  return {
    x: clamp01(Number(bounds.x)),
    y: clamp01(Number(bounds.y)),
    width: clamp01(Number(bounds.width)),
    height: clamp01(Number(bounds.height)),
  };
}

function normalizedRectFromClientRect(rect, rootRect, options = {}) {
  const width = Math.max(Number(options.width || rootRect.width || 1), 1);
  const height = Math.max(Number(options.height || rootRect.height || 1), 1);
  return {
    x: clamp01((rect.left - rootRect.left + Number(options.scrollLeft || 0)) / width),
    y: clamp01((rect.top - rootRect.top + Number(options.scrollTop || 0)) / height),
    width: clamp01(rect.width / width),
    height: clamp01(rect.height / height),
  };
}

function findTextAnchorRects(anchorText) {
  const pane = document.getElementById('canvas-text');
  const query = safeText(anchorText);
  if (!(pane instanceof HTMLElement) || !pane.classList.contains('is-active') || !query) return [];
  const walker = document.createTreeWalker(pane, NodeFilter.SHOW_TEXT);
  let node = walker.nextNode();
  while (node) {
    const text = String(node.textContent || '');
    const index = text.toLowerCase().indexOf(query.toLowerCase());
    if (index >= 0) {
      const range = document.createRange();
      range.setStart(node, index);
      range.setEnd(node, Math.min(text.length, index + query.length));
      const rects = collectNormalizedClientRects(range, pane, { scrollable: true });
      range.detach?.();
      if (rects.length > 0) return rects;
    }
    node = walker.nextNode();
  }
  return [];
}

function approximateTextRectsForLine(line) {
  const pane = document.getElementById('canvas-text');
  const lineNumber = Number.parseInt(String(line || ''), 10);
  if (!(pane instanceof HTMLElement) || !pane.classList.contains('is-active') || !Number.isFinite(lineNumber) || lineNumber <= 0) {
    return [];
  }
  const sourceNode = pane.querySelector(`[data-source-line="${lineNumber}"]`);
  if (sourceNode instanceof HTMLElement) {
    return [normalizedRectFromClientRect(sourceNode.getBoundingClientRect(), pane.getBoundingClientRect(), {
      scrollLeft: pane.scrollLeft,
      scrollTop: pane.scrollTop,
      width: Math.max(pane.scrollWidth, pane.clientWidth, 1),
      height: Math.max(pane.scrollHeight, pane.clientHeight, 1),
    })];
  }
  const text = String(pane.textContent || '');
  const lineCount = Math.max(1, text.split('\n').length);
  const anchor = pane.querySelector('pre, code, p, li, blockquote');
  const lineHeight = Math.max(18, parseFloat(window.getComputedStyle(anchor || pane).lineHeight) || 22);
  const height = Math.max(pane.scrollHeight, lineCount * lineHeight, 1);
  const top = Math.max(0, Math.min(height - lineHeight, (lineNumber - 1) * lineHeight));
  return [{
    x: 0.02,
    y: clamp01(top / height),
    width: 0.96,
    height: clamp01(lineHeight / height),
  }];
}

function buildImportedScanAnnotation(entry, index, payload = {}) {
  const currentKind = safeText(state.currentCanvasArtifact?.kind || '');
  const page = Number.parseInt(safeText(entry?.page), 10);
  const line = Number.parseInt(safeText(entry?.line), 10);
  const anchorText = safeText(entry?.anchor_text);
  const text = safeText(entry?.content || entry?.text || 'Scanned note');
  const bounds = normalizeScanBounds(entry?.bounds);
  const base = {
    id: createAnnotationID(),
    text,
    anchor_text: anchorText,
    line: Number.isFinite(line) && line > 0 ? line : 0,
    color: HIGHLIGHT_COLOR,
    notes: [],
    confidence: clamp01(Number(entry?.confidence)),
    source: 'scan_upload',
    scan_artifact_id: Number(payload?.scan_artifact?.id || payload?.scan_artifact_id || 0),
    scan_item_id: Number(payload?.item_id || 0),
    source_artifact_id: Number(payload?.artifact_id || 0),
    project_id: safeText(payload?.project_id),
  };
  if (currentKind === 'pdf_artifact') {
    return {
      ...base,
      type: bounds ? 'highlight' : 'sticky_note',
      target: 'pdf',
      page: Number.isFinite(page) && page > 0 ? page : 1,
      rects: bounds ? [bounds] : [{ x: 0.08, y: clamp01(0.08 + (index * 0.06)), width: 0, height: 0 }],
    };
  }
  const rects = findTextAnchorRects(anchorText) || [];
  const mappedRects = rects.length > 0
    ? rects
    : (Number.isFinite(line) && line > 0 ? approximateTextRectsForLine(line) : []);
  return {
    ...base,
    type: 'highlight',
    target: 'text',
    rects: mappedRects.length > 0 ? mappedRects : [{ x: 0.02, y: clamp01(0.06 + (index * 0.08)), width: 0.96, height: 0.05 }],
  };
}

export function importScanAnnotations(payload = {}) {
  const incoming = Array.isArray(payload?.annotations) ? payload.annotations : [];
  if (incoming.length === 0) {
    showStatus('scan imported: no annotations found');
    return 0;
  }
  const annotations = listActiveAnnotations();
  incoming.forEach((entry, index) => {
    annotations.push(buildImportedScanAnnotation(entry, index, payload));
  });
  saveActiveAnnotations(annotations);
  renderActiveAnnotations();
  if (annotations.length > 0) {
    openAnnotationBubble(annotations[annotations.length - incoming.length].id);
  }
  showStatus(`scan imported: ${incoming.length} annotation${incoming.length === 1 ? '' : 's'}`);
  return incoming.length;
}

function ensureScanUploadInput() {
  let input = document.getElementById('scan-upload-input');
  if (input instanceof HTMLInputElement) return input;
  input = document.createElement('input');
  input.id = 'scan-upload-input';
  input.type = 'file';
  input.accept = 'image/*';
  input.hidden = true;
  input.addEventListener('change', () => {
    const [file] = Array.from(input.files || []);
    input.value = '';
    if (file) {
      void uploadScanFile(file);
    }
  });
  document.body.appendChild(input);
  return input;
}

export function openScanImportPicker() {
  if (!activeSidebarItemForScan()) {
    showStatus('select an item before importing a scan');
    return false;
  }
  ensureScanUploadInput().click();
  return true;
}

export async function uploadScanFile(file) {
  const item = activeSidebarItemForScan();
  if (!item) {
    showStatus('select an item before importing a scan');
    return false;
  }
  if (!safeText(state.activeProjectId)) {
    showStatus('scan import requires an active project');
    return false;
  }
  if (scanUploadInFlight) {
    showStatus('scan import already running');
    return false;
  }
  scanUploadInFlight = true;
  try {
    await openSidebarArtifactItem(item);
    const form = new FormData();
    form.set('project_id', safeText(state.activeProjectId));
    form.set('item_id', String(Number(item?.id || 0)));
    form.set('artifact_id', String(Number(item?.artifact_id || 0)));
    form.set('file', file, file.name || 'scan.png');
    const resp = await fetch(apiURL('scan/upload'), { method: 'POST', body: form });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    importScanAnnotations(await resp.json());
    return true;
  } catch (err) {
    showStatus(`scan import failed: ${safeText(err?.message || err) || 'unknown error'}`);
    return false;
  } finally {
    scanUploadInFlight = false;
  }
}

async function confirmImportedScanAnnotations(selected) {
  const pending = Array.isArray(selected)
    ? selected.filter((entry) => safeText(entry?.source) === 'scan_upload' && Number(entry?.scan_artifact_id || 0) > 0)
    : [];
  if (pending.length === 0) return true;
  const groups = new Map();
  pending.forEach((entry) => {
    const key = String(Number(entry?.scan_artifact_id || 0));
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(entry);
  });
  for (const group of groups.values()) {
    const first = group[0] || {};
    const resp = await fetch(apiURL('scan/confirm'), {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        project_id: safeText(first?.project_id || state.activeProjectId),
        item_id: Number(first?.scan_item_id || 0),
        artifact_id: Number(first?.source_artifact_id || 0),
        scan_artifact_id: Number(first?.scan_artifact_id || 0),
        annotations: group.map((entry) => ({
          content: safeText(entry?.text),
          anchor_text: safeText(entry?.anchor_text),
          line: Number(entry?.line || 0),
          page: Number(entry?.page || 0),
          bounds: normalizeScanBounds(Array.isArray(entry?.rects) ? entry.rects[0] : null),
          confidence: Number(entry?.confidence || 0),
        })),
      }),
    });
    if (!resp.ok) {
      const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
      throw new Error(detail);
    }
    await resp.json();
  }
  return true;
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
  document.querySelectorAll('.canvas-annotation-layer, .canvas-annotation-badge, .canvas-sticky-note, .canvas-ink-annotation').forEach((node) => node.remove());
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
  preview.textContent = annotationPreviewText(annotation);
  bubble.appendChild(preview);

  const selectionInput = document.createElement('textarea');
  selectionInput.id = 'annotation-selection-input';
  selectionInput.rows = 2;
  selectionInput.placeholder = 'Annotation text';
  selectionInput.value = safeText(annotation?.text);
  bubble.appendChild(selectionInput);

  const selectionControls = document.createElement('div');
  selectionControls.className = 'annotation-bubble-controls';

  const selectionSave = document.createElement('button');
  selectionSave.id = 'annotation-selection-save';
  selectionSave.type = 'button';
  selectionSave.textContent = 'Save text';
  selectionSave.addEventListener('click', () => {
    const content = safeText(selectionInput.value);
    if (!content) return;
    updateActiveAnnotation(annotation.id, (entry) => ({ ...entry, text: content }));
    renderActiveAnnotations();
    renderAnnotationBubble();
  });
  selectionControls.appendChild(selectionSave);

  bubble.appendChild(selectionControls);

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

  const bundleControls = document.createElement('div');
  bundleControls.className = 'annotation-bubble-controls';

  const sendButton = document.createElement('button');
  sendButton.id = 'annotation-bundle-send';
  sendButton.type = 'button';
  sendButton.textContent = listActiveAnnotations().length > 1 ? 'Send bundle' : 'Send annotation';
  sendButton.addEventListener('click', () => {
    void submitAnnotationBundle();
  });
  bundleControls.appendChild(sendButton);

  const clearButton = document.createElement('button');
  clearButton.id = 'annotation-bundle-clear';
  clearButton.type = 'button';
  clearButton.textContent = 'Clear all';
  clearButton.addEventListener('click', () => {
    clearAllActiveAnnotations();
  });
  bundleControls.appendChild(clearButton);

  bubble.appendChild(bundleControls);
  bubble.hidden = false;
  bubble.style.left = `${Math.max(12, Math.min(window.innerWidth - 300, anchor.left))}px`;
  bubble.style.top = `${Math.max(12, Math.min(window.innerHeight - 220, anchor.top + anchor.height + 10))}px`;
}

export function pdfPageAnchorAtPoint(clientX, clientY) {
  const hits = typeof document.elementsFromPoint === 'function'
    ? document.elementsFromPoint(clientX, clientY)
    : [document.elementFromPoint(clientX, clientY)];
  for (const hit of hits) {
    if (!(hit instanceof Element)) continue;
    const page = hit.closest('.canvas-pdf-page');
    const pageInner = page?.querySelector('.canvas-pdf-page-inner');
    const pageNumber = Number.parseInt(safeText(page?.dataset?.page), 10);
    if (!(pageInner instanceof HTMLElement) || !Number.isFinite(pageNumber) || pageNumber <= 0) {
      continue;
    }
    const bounds = pageInner.getBoundingClientRect();
    const width = Math.max(bounds.width, 1);
    const height = Math.max(bounds.height, 1);
    return {
      page,
      pageInner,
      pageNumber,
      width,
      height,
      xNorm: clamp01((clientX - bounds.left) / width),
      yNorm: clamp01((clientY - bounds.top) / height),
      xPx: clamp01((clientX - bounds.left) / width) * width,
      yPx: clamp01((clientY - bounds.top) / height) * height,
    };
  }
  return null;
}

function renderAnnotationBadge(root, annotation, width, height) {
  const rect = annotationPrimaryRect(annotation);
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
  badge.addEventListener('dblclick', (event) => {
    event.preventDefault();
    event.stopPropagation();
    void submitAnnotationBundle(annotation.id);
  });
  root.appendChild(badge);
}

function renderStickyNoteMarker(root, annotation, width, height) {
  const rect = annotationPrimaryRect(annotation);
  if (!(root instanceof HTMLElement) || !rect) return;
  const marker = document.createElement('button');
  marker.type = 'button';
  marker.className = 'canvas-sticky-note';
  marker.dataset.annotationId = annotation.id;
  marker.textContent = 'Note';
  marker.style.left = `${rect.x * width}px`;
  marker.style.top = `${rect.y * height}px`;
  marker.addEventListener('click', (event) => {
    event.preventDefault();
    event.stopPropagation();
    openAnnotationBubble(annotation.id);
  });
  marker.addEventListener('dblclick', (event) => {
    event.preventDefault();
    event.stopPropagation();
    void submitAnnotationBundle(annotation.id);
  });
  root.appendChild(marker);
  renderAnnotationBadge(root, annotation, width, height);
}

function renderInkAnnotation(root, annotation, width, height) {
  const rect = annotationPrimaryRect(annotation);
  if (!(root instanceof HTMLElement) || !rect) return;
  const strokes = Array.isArray(annotation?.strokes) ? annotation.strokes : [];
  if (strokes.length === 0) return;
  const minWidth = 12;
  const minHeight = 12;
  const baseLeft = rect.x * width;
  const baseTop = rect.y * height;
  const baseWidth = Math.max(rect.width * width, minWidth);
  const baseHeight = Math.max(rect.height * height, minHeight);
  const hitPadding = 8;

  const button = document.createElement('button');
  button.type = 'button';
  button.className = 'canvas-ink-annotation';
  button.dataset.annotationId = annotation.id;
  button.style.left = `${Math.max(0, baseLeft - hitPadding)}px`;
  button.style.top = `${Math.max(0, baseTop - hitPadding)}px`;
  button.style.width = `${baseWidth + (hitPadding * 2)}px`;
  button.style.height = `${baseHeight + (hitPadding * 2)}px`;
  button.addEventListener('click', (event) => {
    event.preventDefault();
    event.stopPropagation();
    openAnnotationBubble(annotation.id);
  });
  button.addEventListener('dblclick', (event) => {
    event.preventDefault();
    event.stopPropagation();
    void submitAnnotationBundle(annotation.id);
  });

  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  svg.setAttribute('viewBox', `0 0 ${baseWidth + (hitPadding * 2)} ${baseHeight + (hitPadding * 2)}`);
  svg.setAttribute('aria-hidden', 'true');
  for (const stroke of strokes) {
    const points = Array.isArray(stroke?.points) ? stroke.points : [];
    if (points.length === 0) continue;
    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    const d = points.map((point, index) => {
      const x = ((clamp01(Number(point?.x)) - rect.x) * width) + hitPadding;
      const y = ((clamp01(Number(point?.y)) - rect.y) * height) + hitPadding;
      return `${index === 0 ? 'M' : 'L'} ${x.toFixed(2)} ${y.toFixed(2)}`;
    }).join(' ');
    path.setAttribute('d', d);
    path.setAttribute('stroke-width', `${Math.max(1.5, clamp01(Number(stroke?.width)) * width)}`);
    path.setAttribute('stroke-linecap', 'round');
    path.setAttribute('stroke-linejoin', 'round');
    path.setAttribute('fill', 'none');
    svg.appendChild(path);
  }
  button.appendChild(svg);
  root.appendChild(button);
  renderAnnotationBadge(root, annotation, width, height);
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
        node.addEventListener('dblclick', (event) => {
          event.preventDefault();
          event.stopPropagation();
          void submitAnnotationBundle(annotation.id);
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
      if (annotation?.type === 'sticky_note') {
        renderStickyNoteMarker(root, annotation, width, height);
        return;
      }
      if (annotation?.type === 'ink') {
        renderInkAnnotation(root, annotation, width, height);
        return;
      }
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
        node.addEventListener('dblclick', (event) => {
          event.preventDefault();
          event.stopPropagation();
          void submitAnnotationBundle(annotation.id);
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

export function createPdfStickyNoteAt(clientX, clientY) {
  const anchor = pdfPageAnchorAtPoint(clientX, clientY);
  if (!anchor) return false;
  const annotation = {
    id: createAnnotationID(),
    type: 'sticky_note',
    target: 'pdf',
    page: anchor.pageNumber,
    text: STICKY_NOTE_LABEL,
    color: HIGHLIGHT_COLOR,
    rects: [{ x: anchor.xNorm, y: anchor.yNorm, width: 0, height: 0 }],
    notes: [],
  };
  const annotations = listActiveAnnotations();
  annotations.push(annotation);
  saveActiveAnnotations(annotations);
  renderActiveAnnotations();
  openAnnotationBubble(annotation.id);
  return true;
}

export function persistPdfInkAnnotation(pageNumber, pageWidth, pageHeight, stroke) {
  const points = Array.isArray(stroke?.points) ? stroke.points : [];
  if (!Number.isFinite(pageNumber) || pageNumber <= 0 || !Number.isFinite(pageWidth) || pageWidth <= 0 || !Number.isFinite(pageHeight) || pageHeight <= 0 || points.length === 0) {
    return false;
  }
  let minX = Number.POSITIVE_INFINITY;
  let minY = Number.POSITIVE_INFINITY;
  let maxX = 0;
  let maxY = 0;
  const normalizedPoints = points.map((point) => {
    const x = clamp01(Number(point?.x) / pageWidth);
    const y = clamp01(Number(point?.y) / pageHeight);
    minX = Math.min(minX, x);
    minY = Math.min(minY, y);
    maxX = Math.max(maxX, x);
    maxY = Math.max(maxY, y);
    return { x, y };
  });
  if (!Number.isFinite(minX) || !Number.isFinite(minY)) return false;
  const annotation = {
    id: createAnnotationID(),
    type: 'ink',
    target: 'pdf',
    page: pageNumber,
    text: INK_NOTE_LABEL,
    rects: [{
      x: minX,
      y: minY,
      width: Math.max(0, maxX - minX),
      height: Math.max(0, maxY - minY),
    }],
    strokes: [{
      width: clamp01(Number(stroke?.width) / pageWidth),
      points: normalizedPoints,
    }],
    notes: [],
  };
  const annotations = listActiveAnnotations();
  annotations.push(annotation);
  saveActiveAnnotations(annotations);
  renderActiveAnnotations();
  return true;
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
