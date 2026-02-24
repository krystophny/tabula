import { marked } from './vendor/marked.esm.js';

const SOURCE_LANGUAGE_BY_EXT = {
  c: 'c',
  h: 'c',
  cc: 'cpp',
  cxx: 'cpp',
  cpp: 'cpp',
  hpp: 'cpp',
  hh: 'cpp',
  go: 'go',
  rs: 'rust',
  py: 'python',
  js: 'javascript',
  mjs: 'javascript',
  cjs: 'javascript',
  ts: 'typescript',
  jsx: 'jsx',
  tsx: 'tsx',
  java: 'java',
  kt: 'kotlin',
  scala: 'scala',
  rb: 'ruby',
  php: 'php',
  swift: 'swift',
  cs: 'csharp',
  lua: 'lua',
  r: 'r',
  sql: 'sql',
  sh: 'bash',
  bash: 'bash',
  zsh: 'bash',
  ps1: 'powershell',
  json: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  toml: 'toml',
  ini: 'ini',
  xml: 'xml',
  html: 'xml',
  css: 'css',
  scss: 'scss',
  f: 'fortran',
  for: 'fortran',
  f77: 'fortran',
  f90: 'fortran',
  f95: 'fortran',
  f03: 'fortran',
  f08: 'fortran',
};

const SOURCE_LANGUAGE_BY_BASENAME = {
  makefile: 'makefile',
  dockerfile: 'dockerfile',
  cmakelists: 'cmake',
};

export function escapeHtml(text) {
  return String(text)
    .replaceAll('&', '&amp;')
    .replaceAll('<', '&lt;')
    .replaceAll('>', '&gt;')
    .replaceAll('"', '&quot;')
    .replaceAll("'", '&#39;');
}

function normalizeLanguage(langRaw) {
  const lang = String(langRaw || '').trim().toLowerCase();
  if (!lang) return '';
  const aliases = {
    js: 'javascript',
    ts: 'typescript',
    py: 'python',
    rs: 'rust',
    golang: 'go',
    shell: 'bash',
    sh: 'bash',
    zsh: 'bash',
    ps: 'powershell',
    f90: 'fortran',
    f95: 'fortran',
    f03: 'fortran',
    f08: 'fortran',
    for: 'fortran',
  };
  return aliases[lang] || lang;
}

function languageFromArtifactTitle(titleRaw) {
  const title = String(titleRaw || '').trim();
  if (!title) return '';
  const base = title.split(/[\\/]/).pop() || '';
  const lowerBase = base.toLowerCase();
  if (lowerBase === 'cmakelists.txt') return 'cmake';
  if (SOURCE_LANGUAGE_BY_BASENAME[lowerBase]) return SOURCE_LANGUAGE_BY_BASENAME[lowerBase];
  const dot = lowerBase.lastIndexOf('.');
  if (dot < 0 || dot === lowerBase.length - 1) return '';
  const ext = lowerBase.slice(dot + 1);
  return SOURCE_LANGUAGE_BY_EXT[ext] || '';
}

function highlightCode(code, langRaw) {
  const input = String(code || '');
  const lang = normalizeLanguage(langRaw);
  const hljs = window.hljs;
  if (!hljs || typeof hljs.highlight !== 'function') {
    return escapeHtml(input);
  }
  if (lang && typeof hljs.getLanguage === 'function' && hljs.getLanguage(lang)) {
    try {
      return hljs.highlight(input, { language: lang, ignoreIllegals: true }).value;
    } catch (_) {}
  }
  if (typeof hljs.highlightAuto === 'function') {
    try {
      return hljs.highlightAuto(input).value;
    } catch (_) {}
  }
  return escapeHtml(input);
}

function classifyDiffLine(line) {
  if (line.startsWith('diff --git') || line.startsWith('index ') || line.startsWith('+++ ') || line.startsWith('--- ')) {
    return 'meta';
  }
  if (line.startsWith('@@')) {
    return 'hunk';
  }
  if (line.startsWith('+') && !line.startsWith('+++')) {
    return 'add';
  }
  if (line.startsWith('-') && !line.startsWith('---')) {
    return 'del';
  }
  return 'ctx';
}

function highlightDiff(code) {
  const lines = code.split('\n');
  return lines.map((line) => {
    const kind = classifyDiffLine(line);
    if (kind === 'meta' || kind === 'hunk') {
      return `<span class="hl-diff-line hl-diff-${kind}">${escapeHtml(line)}</span>`;
    }
    if (!line) {
      return '<span class="hl-diff-line hl-diff-ctx"></span>';
    }
    return `<span class="hl-diff-line hl-diff-${kind}">${escapeHtml(line)}</span>`;
  }).join('');
}

function renderHighlightedCodeBlock(code, langRaw) {
  const normalized = normalizeLanguage(langRaw) || 'plaintext';
  const highlighted = highlightCode(code, normalized);
  return `<pre><code class="hljs language-${escapeHtml(normalized)}">${highlighted}</code></pre>\n`;
}

function renderCodeBlock(code, langRaw) {
  const lang = normalizeLanguage(langRaw);
  if (lang === 'fortran') {
    return renderHighlightedCodeBlock(code, 'fortran');
  }
  if (lang === 'diff' || lang === 'patch' || lang === 'git') {
    return `<pre><code class="hljs language-${escapeHtml(lang)}">${highlightDiff(code)}</code></pre>\n`;
  }
  return renderHighlightedCodeBlock(code, lang || 'plaintext');
}

const renderer = new marked.Renderer();
renderer.code = ({ text, lang }) => renderCodeBlock(text || '', lang || '');

marked.setOptions({
  breaks: true,
  renderer,
});

const els = {};
let activeTextEventId = null;
let activeArtifactTitle = '';
let activePdfEvent = null;
let previousArtifactText = '';
let previousBlockTexts = [];
let previousArtifactTitle = '';

const MATH_SEGMENT_TOKEN_PREFIX = '@@TABURA_MATH_SEGMENT_';

export function getEls() {
  if (!els.text) {
    els.text = document.getElementById('canvas-text');
    els.image = document.getElementById('canvas-image');
    els.img = document.getElementById('canvas-img');
    els.pdf = document.getElementById('canvas-pdf');
  }
  return els;
}

export function sanitizeHtml(html) {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const dangerous = doc.querySelectorAll('script,iframe,object,embed,link[rel="import"],form,svg,base,style');
  dangerous.forEach(el => el.remove());
  doc.querySelectorAll('*').forEach(el => {
    for (const attr of [...el.attributes]) {
      const val = attr.value.trim().toLowerCase();
      const isDangerous = attr.name.startsWith('on')
        || val.startsWith('javascript:')
        || val.startsWith('vbscript:')
        || (val.startsWith('data:') && !val.startsWith('data:image/'));
      if (isDangerous) {
        el.removeAttribute(attr.name);
      }
    }
  });
  return doc.body.innerHTML;
}

function extractMathSegments(markdownSource) {
  const source = String(markdownSource || '');
  const stash = [];
  let text = source;

  const normalizeMathSegment = (segment) => {
    const raw = String(segment || '');
    const trimmed = raw.trim();
    if (!trimmed.startsWith('$$') || !trimmed.endsWith('$$')) {
      return raw;
    }
    const inner = trimmed.slice(2, -2).trim();
    if (!inner) return raw;
    const hasTagOrLabel = /\\(?:tag|label)\{[^}]+\}/.test(inner);
    const hasDisplayEnv = /\\begin\{(?:equation|equation\*|align|align\*|aligned|gather|gather\*|multline|multline\*|split|eqnarray)\}/.test(inner);
    if (!hasTagOrLabel || hasDisplayEnv) {
      return raw;
    }
    return `\\begin{equation}\n${inner}\n\\end{equation}`;
  };

  const patterns = [
    /\$\$[\s\S]+?\$\$/g,
    /\\\[[\s\S]+?\\\]/g,
    /\\\([\s\S]+?\\\)/g,
  ];

  for (const pattern of patterns) {
    text = text.replace(pattern, (segment) => {
      const token = `${MATH_SEGMENT_TOKEN_PREFIX}${stash.length}@@`;
      stash.push(normalizeMathSegment(segment));
      return token;
    });
  }

  return { text, stash };
}

function restoreMathSegments(renderedHtml, mathSegments) {
  let output = String(renderedHtml || '');
  if (!Array.isArray(mathSegments) || mathSegments.length === 0) {
    return output;
  }
  for (let i = 0; i < mathSegments.length; i += 1) {
    const token = `${MATH_SEGMENT_TOKEN_PREFIX}${i}@@`;
    const safeSegment = escapeHtml(String(mathSegments[i] || ''));
    output = output.replaceAll(token, safeSegment);
  }
  return output;
}

function typesetMarkdownMath(root, attempt = 0) {
  if (!(root instanceof Element) || !root.isConnected) return;
  const mj = window.MathJax;
  if (!mj || typeof mj.typesetPromise !== 'function') {
    if (attempt >= 40) return;
    window.setTimeout(() => typesetMarkdownMath(root, attempt + 1), 75);
    return;
  }
  const startupReady = mj.startup && mj.startup.promise && typeof mj.startup.promise.then === 'function'
    ? mj.startup.promise
    : Promise.resolve();
  const originalMathText = root.textContent || '';
  const needsRefPass = /\\(?:eq)?ref\{[^}]+\}/.test(originalMathText) || /\\label\{[^}]+\}/.test(originalMathText);
  void startupReady
    .then(() => {
      if (!root.isConnected) return;
      if (typeof mj.texReset === 'function') {
        mj.texReset();
      }
      if (typeof mj.typesetClear === 'function') {
        mj.typesetClear([root]);
      }
      return mj.typesetPromise([root]).then(() => {
        if (!needsRefPass || !root.isConnected) return;
        return mj.typesetPromise([root]);
      });
    })
    .catch((err) => {
      console.warn('MathJax typeset failed:', err);
    });
}

export function hideAll() {
  const e = getEls();
  if (e.text) e.text.style.display = 'none';
  if (e.image) e.image.style.display = 'none';
  if (e.pdf) e.pdf.style.display = 'none';
  if (e.text) e.text.classList.remove('is-active');
  if (e.image) e.image.classList.remove('is-active');
  if (e.pdf) e.pdf.classList.remove('is-active');
}

function isSelectionInside(root, selection) {
  if (!selection || selection.rangeCount === 0) return false;
  const range = selection.getRangeAt(0);
  return root.contains(range.commonAncestorContainer);
}

function getSelectionOffsets(root, range) {
  const startProbe = range.cloneRange();
  startProbe.selectNodeContents(root);
  startProbe.setEnd(range.startContainer, range.startOffset);
  const startOffset = startProbe.toString().length;

  const endProbe = range.cloneRange();
  endProbe.selectNodeContents(root);
  endProbe.setEnd(range.endContainer, range.endOffset);
  const endOffset = endProbe.toString().length;

  return { startOffset, endOffset };
}

function lineFromOffset(lines, charOffset) {
  let charCount = 0;
  for (let i = 0; i < lines.length; i++) {
    if (charCount + lines[i].length >= charOffset) {
      return i + 1;
    }
    charCount += lines[i].length + 1;
  }
  return Math.max(1, lines.length);
}

function textRangeFromClientPoint(clientX, clientY) {
  if (typeof document.caretRangeFromPoint === 'function') {
    return document.caretRangeFromPoint(clientX, clientY);
  }
  if (typeof document.caretPositionFromPoint === 'function') {
    const caret = document.caretPositionFromPoint(clientX, clientY);
    if (!caret) return null;
    const range = document.createRange();
    range.setStart(caret.offsetNode, caret.offset);
    range.collapse(true);
    return range;
  }
  return null;
}

function getActiveArtifactTitle() {
  return activeArtifactTitle;
}

export function getActiveTextEventId() {
  return activeTextEventId;
}

export function getLocationFromPoint(clientX, clientY) {
  const e = getEls();
  if (!e.text || !activeTextEventId) return null;
  const range = textRangeFromClientPoint(clientX, clientY);
  if (!range || !e.text.contains(range.startContainer)) return null;
  try {
    const startProbe = range.cloneRange();
    startProbe.selectNodeContents(e.text);
    startProbe.setEnd(range.startContainer, range.startOffset);
    const offset = startProbe.toString().length;
    const lines = (e.text.textContent || '').split('\n');
    const line = lineFromOffset(lines, offset);
    const title = getActiveArtifactTitle();
    return { line, title };
  } catch (_) {
    return null;
  }
}

export function getLocationFromSelection() {
  const e = getEls();
  if (!e.text || !activeTextEventId) return null;
  const selection = window.getSelection();
  if (!selection || selection.isCollapsed || !isSelectionInside(e.text, selection)) return null;
  const selectedText = selection.toString().trim();
  if (!selectedText) return null;
  const range = selection.getRangeAt(0);
  const { startOffset } = getSelectionOffsets(e.text, range);
  const lines = (e.text.textContent || '').split('\n');
  const line = lineFromOffset(lines, startOffset);
  const title = getActiveArtifactTitle();
  return { line, selectedText, title };
}

export function showLineHighlight(clientX, clientY) {
  clearLineHighlight();
  const e = getEls();
  if (!e.text) return;
  const range = textRangeFromClientPoint(clientX, clientY);
  if (!range || !e.text.contains(range.startContainer)) return;
  const rangeRect = range.getBoundingClientRect();
  const rootRect = e.text.getBoundingClientRect();
  const top = rangeRect.top - rootRect.top + e.text.scrollTop;
  const lineHeight = rangeRect.height || parseFloat(window.getComputedStyle(e.text).lineHeight) || 22;
  if (window.getComputedStyle(e.text).position === 'static') {
    e.text.style.position = 'relative';
  }
  const highlight = document.createElement('div');
  highlight.className = 'review-line-highlight';
  highlight.style.top = `${top}px`;
  highlight.style.height = `${lineHeight}px`;
  e.text.appendChild(highlight);
}

export function clearLineHighlight() {
  const e = getEls();
  if (!e.text) return;
  e.text.querySelectorAll('.review-line-highlight').forEach(el => el.remove());
}

function clearTextInteractionHandlers() {
}

function renderPdfSurface(event) {
  const e = getEls();
  const pdfState = (window._taburaApp || {}).getState ? window._taburaApp.getState() : {};
  const pdfSid = pdfState.sessionId || '';
  const pdfURL = `/api/files/${encodeURIComponent(pdfSid)}/${encodeURIComponent(event.path)}`;
  e.pdf.innerHTML = '';

  const surface = document.createElement('div');
  surface.className = 'canvas-pdf-surface';
  const objectEl = document.createElement('object');
  objectEl.className = 'canvas-pdf-object';
  objectEl.type = 'application/pdf';
  objectEl.data = pdfURL;
  const fallback = document.createElement('div');
  fallback.className = 'canvas-pdf-fallback';
  fallback.innerHTML = `PDF preview unavailable. <a href="${pdfURL}" target="_blank" rel="noopener noreferrer">Open PDF</a>`;
  const hitLayer = document.createElement('div');
  hitLayer.className = 'canvas-pdf-hit-layer';
  objectEl.appendChild(fallback);
  surface.appendChild(objectEl);
  surface.appendChild(hitLayer);
  e.pdf.appendChild(surface);
  e.pdf._pdfHitLayer = hitLayer;
}

export function renderCanvas(event) {
  const e = getEls();

  if (event.kind === 'text_artifact') {
    hideAll();
    e.text.style.display = '';
    e.text.classList.add('is-active');
    clearTextInteractionHandlers();
    activeTextEventId = event.event_id;
    const nextArtifactTitle = String(event.title || '');
    const shouldHighlightChanges = previousArtifactTitle === nextArtifactTitle && previousBlockTexts.length > 0;
    activeArtifactTitle = nextArtifactTitle;
    activePdfEvent = null;
    const oldBlockTexts = shouldHighlightChanges ? previousBlockTexts.slice() : [];
    const textBody = String(event.text || '');
    const sourceLang = languageFromArtifactTitle(activeArtifactTitle);
    if (sourceLang) {
      e.text.innerHTML = sanitizeHtml(renderHighlightedCodeBlock(textBody, sourceLang));
    } else {
      const { text: markdownText, stash: mathSegments } = extractMathSegments(textBody);
      const renderedMarkdownHtml = marked.parse(markdownText);
      e.text.innerHTML = restoreMathSegments(sanitizeHtml(renderedMarkdownHtml), mathSegments);
      typesetMarkdownMath(e.text);
    }
    previousArtifactText = event.text || '';
    previousBlockTexts = captureBlockTexts(e.text);
    previousArtifactTitle = nextArtifactTitle;
    applyDiffHighlight(e.text, oldBlockTexts);
  } else if (event.kind === 'image_artifact') {
    clearTextInteractionHandlers();
    hideAll();
    e.image.style.display = '';
    e.image.classList.add('is-active');
    const state = (window._taburaApp || {}).getState ? window._taburaApp.getState() : {};
    const sid = state.sessionId || '';
    e.img.src = `/api/files/${encodeURIComponent(sid)}/${encodeURIComponent(event.path)}`;
    e.img.alt = event.title || 'Image';
    activeTextEventId = null;
    activeArtifactTitle = event.title || '';
    activePdfEvent = null;
  } else if (event.kind === 'pdf_artifact') {
    clearTextInteractionHandlers();
    hideAll();
    e.pdf.style.display = '';
    e.pdf.classList.add('is-active');
    void renderPdfSurface(event);
    activeTextEventId = null;
    activeArtifactTitle = event.title || '';
    activePdfEvent = event;
  } else if (event.kind === 'clear_canvas') {
    clearTextInteractionHandlers();
    clearCanvas();
  }
}

export function clearCanvas() {
  clearTextInteractionHandlers();
  hideAll();
  activeTextEventId = null;
  activeArtifactTitle = '';
  activePdfEvent = null;
  previousArtifactText = '';
  previousBlockTexts = [];
  previousArtifactTitle = '';
}

function getBlockSelector() {
  return 'p, h1, h2, h3, h4, h5, h6, pre, ul, ol, table, blockquote, hr';
}

function captureBlockTexts(root) {
  const blocks = root.querySelectorAll(getBlockSelector());
  const texts = [];
  for (let i = 0; i < blocks.length; i++) {
    texts.push(blocks[i].textContent || '');
  }
  return texts;
}

function applyDiffHighlight(root, oldBlockTexts) {
  if (!oldBlockTexts || oldBlockTexts.length === 0 || !root) return;
  const blocks = root.querySelectorAll(getBlockSelector());
  const changedBlocks = [];
  const maxLen = Math.max(oldBlockTexts.length, blocks.length);
  for (let i = 0; i < maxLen; i++) {
    if (i >= blocks.length) break;
    const oldContent = i < oldBlockTexts.length ? oldBlockTexts[i] : '';
    const newContent = blocks[i].textContent || '';
    if (oldContent !== newContent) {
      blocks[i].classList.add('diff-highlight');
      changedBlocks.push(blocks[i]);
    }
  }
  // New blocks beyond old length
  for (let i = oldBlockTexts.length; i < blocks.length; i++) {
    blocks[i].classList.add('diff-highlight');
    changedBlocks.push(blocks[i]);
  }
  if (changedBlocks.length === 0) return;
  if (changedBlocks[0] && changedBlocks[0].isConnected) {
    changedBlocks[0].scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
}

export function getPreviousArtifactText() {
  return previousArtifactText;
}
