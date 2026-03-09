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

const MARKDOWN_EXTENSIONS = new Set(['md', 'markdown', 'mdown', 'mkdn', 'mkd', 'mdx']);
const MATH_SEGMENT_TOKEN_PREFIX = '@@TABURA_MATH_SEGMENT_';

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

function parseDiffHunkHeader(line) {
  const match = /^@@\s*-(\d+)(?:,\d+)?\s+\+(\d+)(?:,\d+)?\s*@@/.exec(line);
  if (!match) return null;
  return {
    oldStart: Number.parseInt(match[1], 10),
    newStart: Number.parseInt(match[2], 10),
  };
}

function parseDiffPathFromHeader(line) {
  const match = /^diff --git a\/(.+?) b\/(.+)$/.exec(line);
  if (!match) return '';
  const right = String(match[2] || '').trim();
  const left = String(match[1] || '').trim();
  if (right && right !== '/dev/null') return right;
  return left;
}

function parseDiffPathFromMarker(line, marker) {
  if (!line.startsWith(marker)) return '';
  const raw = String(line.slice(marker.length)).trim();
  if (!raw || raw === '/dev/null') return '';
  if (raw.startsWith('a/') || raw.startsWith('b/')) {
    return raw.slice(2);
  }
  return raw;
}

function highlightDiffCodeLine(line, langRaw) {
  const lang = normalizeLanguage(langRaw);
  if (!lang) {
    return escapeHtml(line);
  }
  if (line.startsWith('+') && !line.startsWith('+++')) {
    return `${escapeHtml('+')}${highlightCode(line.slice(1), lang)}`;
  }
  if (line.startsWith('-') && !line.startsWith('---')) {
    return `${escapeHtml('-')}${highlightCode(line.slice(1), lang)}`;
  }
  if (line.startsWith(' ')) {
    return `${escapeHtml(' ')}${highlightCode(line.slice(1), lang)}`;
  }
  return escapeHtml(line);
}

function isMarkdownPath(pathRaw) {
  const path = String(pathRaw || '').trim();
  if (!path) return false;
  const base = path.split(/[\\/]/).pop() || '';
  const lowerBase = base.toLowerCase();
  const dot = lowerBase.lastIndexOf('.');
  if (dot < 0 || dot === lowerBase.length - 1) return false;
  return MARKDOWN_EXTENSIONS.has(lowerBase.slice(dot + 1));
}

function inferDiffPath(diffTextRaw) {
  const lines = String(diffTextRaw || '').replaceAll('\r\n', '\n').split('\n');
  let path = '';
  for (const line of lines) {
    if (line.startsWith('diff --git ')) {
      const headerPath = parseDiffPathFromHeader(line);
      if (headerPath) return headerPath;
    } else if (line.startsWith('+++ ')) {
      const plusPath = parseDiffPathFromMarker(line, '+++ ');
      if (plusPath) return plusPath;
    } else if (line.startsWith('--- ')) {
      const minusPath = parseDiffPathFromMarker(line, '--- ');
      if (minusPath) path = minusPath;
    }
  }
  return path;
}

function tokenizeInlineDiffWords(textRaw) {
  return String(textRaw || '').match(/(\s+|[A-Za-z0-9_]+|[^A-Za-z0-9_\s])/g) || [];
}

function renderInlineMarkdownDiff(oldLineRaw, newLineRaw) {
  const oldLine = String(oldLineRaw || '');
  const newLine = String(newLineRaw || '');
  if (!oldLine || !newLine || oldLine === newLine) {
    return escapeHtml(newLine);
  }
  if (oldLine.length > 2000 || newLine.length > 2000) {
    return escapeHtml(newLine);
  }

  const oldTokens = tokenizeInlineDiffWords(oldLine);
  const newTokens = tokenizeInlineDiffWords(newLine);
  if (oldTokens.length === 0 || newTokens.length === 0) {
    return escapeHtml(newLine);
  }
  if (oldTokens.length > 320 || newTokens.length > 320) {
    return escapeHtml(newLine);
  }

  const lcs = Array.from({ length: oldTokens.length + 1 }, () => new Uint16Array(newTokens.length + 1));
  for (let i = 1; i <= oldTokens.length; i += 1) {
    for (let j = 1; j <= newTokens.length; j += 1) {
      if (oldTokens[i - 1] === newTokens[j - 1]) {
        lcs[i][j] = lcs[i - 1][j - 1] + 1;
      } else {
        lcs[i][j] = Math.max(lcs[i - 1][j], lcs[i][j - 1]);
      }
    }
  }

  const opsReversed = [];
  let i = oldTokens.length;
  let j = newTokens.length;
  while (i > 0 || j > 0) {
    if (i > 0 && j > 0 && oldTokens[i - 1] === newTokens[j - 1]) {
      opsReversed.push({ type: 'equal', text: oldTokens[i - 1] });
      i -= 1;
      j -= 1;
      continue;
    }
    if (j > 0 && (i === 0 || lcs[i][j - 1] >= lcs[i - 1][j])) {
      opsReversed.push({ type: 'add', text: newTokens[j - 1] });
      j -= 1;
      continue;
    }
    if (i > 0) {
      opsReversed.push({ type: 'del', text: oldTokens[i - 1] });
      i -= 1;
      continue;
    }
  }

  const ops = opsReversed.reverse();
  const merged = [];
  for (const op of ops) {
    if (merged.length > 0 && merged[merged.length - 1].type === op.type) {
      merged[merged.length - 1].text += op.text;
    } else {
      merged.push({ type: op.type, text: op.text });
    }
  }
  if (!merged.some((op) => op.type !== 'equal')) {
    return escapeHtml(newLine);
  }

  return merged.map((op) => {
    const safe = escapeHtml(op.text);
    if (op.type === 'add') {
      return `<ins class="md-diff-ins">${safe}</ins>`;
    }
    if (op.type === 'del') {
      return `<del class="md-diff-del-inline">${safe}</del>`;
    }
    return safe;
  }).join('');
}

function renderRemovedMarkdownLine(lineRaw) {
  const text = String(lineRaw || '');
  const safe = text ? escapeHtml(text) : '&nbsp;';
  return `<div class="md-diff-del-line"><del>${safe}</del></div>`;
}

function buildMarkdownDiffPreview(diffTextRaw, artifactTitleRaw) {
  const diffText = String(diffTextRaw || '').replaceAll('\r\n', '\n');
  if (!diffText.trim()) return null;
  const artifactTitle = String(artifactTitleRaw || '').trim();
  const inferredPath = inferDiffPath(diffText);
  const markdownPath = isMarkdownPath(artifactTitle)
    ? artifactTitle
    : (isMarkdownPath(inferredPath) ? inferredPath : '');
  if (!markdownPath) {
    return null;
  }
  if (!(diffText.startsWith('diff --git ') || diffText.includes('\ndiff --git '))) {
    return null;
  }

  const lines = diffText.split('\n');
  const previewLines = [];
  const lineMap = [];
  const changedMap = [];
  let sawHunk = false;
  let inHunk = false;
  let pendingDeletionLines = [];
  let newLine = null;
  let lastMappedLine = null;

  const appendLine = (text, fileLine, changed) => {
    previewLines.push(String(text || ''));
    if (Number.isFinite(fileLine) && fileLine > 0) {
      const resolved = Math.trunc(fileLine);
      lineMap.push(resolved);
      lastMappedLine = resolved;
    } else {
      lineMap.push(null);
    }
    changedMap.push(Boolean(changed));
  };

  appendLine(`\`${markdownPath}\``, null, false);
  appendLine('', null, false);

  const flushPendingDeletions = () => {
    if (pendingDeletionLines.length === 0) return;
    for (const deletedLine of pendingDeletionLines) {
      appendLine(renderRemovedMarkdownLine(deletedLine), null, true);
    }
    pendingDeletionLines = [];
  };

  for (const line of lines) {
    if (line.startsWith('diff --git ')) {
      flushPendingDeletions();
      inHunk = false;
      continue;
    }

    const hunk = parseDiffHunkHeader(line);
    if (hunk) {
      sawHunk = true;
      inHunk = true;
      flushPendingDeletions();
      const hunkStart = Number.isFinite(hunk.newStart) ? hunk.newStart : null;
      if (
        Number.isFinite(lastMappedLine)
        && Number.isFinite(hunkStart)
        && hunkStart > lastMappedLine + 1
        && previewLines.length > 0
      ) {
        appendLine('', null, false);
        appendLine('[...]', null, false);
        appendLine('', null, false);
      }
      newLine = Number.isFinite(hunkStart) ? hunkStart : null;
      continue;
    }

    if (!inHunk || line.startsWith('\\ No newline at end of file')) continue;

    if (line.startsWith('+') && !line.startsWith('+++')) {
      const nextText = line.slice(1);
      if (pendingDeletionLines.length > 0) {
        const deletedText = pendingDeletionLines.shift() || '';
        appendLine(renderInlineMarkdownDiff(deletedText, nextText), newLine, true);
      } else {
        appendLine(nextText, newLine, true);
      }
      if (Number.isFinite(newLine)) newLine += 1;
      continue;
    }

    if (line.startsWith('-') && !line.startsWith('---')) {
      pendingDeletionLines.push(line.slice(1));
      continue;
    }

    if (line.startsWith(' ')) {
      flushPendingDeletions();
      appendLine(line.slice(1), newLine, false);
      if (Number.isFinite(newLine)) newLine += 1;
    }
  }
  flushPendingDeletions();

  if (!sawHunk || previewLines.length === 0) return null;
  if (!lineMap.some((line) => Number.isFinite(line) && line > 0)) return null;

  return {
    markdown: previewLines.join('\n'),
    lineMap,
    changedMap,
  };
}

function countNewlines(textRaw) {
  const text = String(textRaw || '');
  let count = 0;
  for (let i = 0; i < text.length; i += 1) {
    if (text.charCodeAt(i) === 10) count += 1;
  }
  return count;
}

function resolveTokenSourceMeta(lineMap, changedMap, startLine, endLine) {
  const start = Math.max(1, Math.trunc(startLine || 1));
  const end = Math.max(start, Math.trunc(endLine || start));
  let sourceLine = null;
  let changed = false;
  for (let line = start; line <= end; line += 1) {
    const mapped = lineMap[line - 1];
    if (sourceLine === null && Number.isFinite(mapped) && mapped > 0) {
      sourceLine = mapped;
    }
    if (changedMap[line - 1]) {
      changed = true;
    }
    if (sourceLine !== null && changed) break;
  }
  return { sourceLine, changed };
}

function annotateToken(token, startLine, endLine, lineMap, changedMap) {
  if (!token || typeof token !== 'object') return;
  const meta = resolveTokenSourceMeta(lineMap, changedMap, startLine, endLine);
  if (Number.isFinite(meta.sourceLine) && meta.sourceLine > 0) {
    token.taburaSourceLine = Math.trunc(meta.sourceLine);
  } else {
    delete token.taburaSourceLine;
  }
  token.taburaDiffChanged = Boolean(meta.changed);
}

function annotateListItems(token, listStartLine, lineMap, changedMap) {
  if (!token || token.type !== 'list' || !Array.isArray(token.items) || typeof token.raw !== 'string') return;
  let localCursor = 0;
  for (const item of token.items) {
    const raw = typeof item?.raw === 'string' ? item.raw : '';
    if (!raw) continue;
    const found = token.raw.indexOf(raw, localCursor);
    const index = found >= 0 ? found : localCursor;
    const start = listStartLine + countNewlines(token.raw.slice(0, index));
    const end = start + countNewlines(raw);
    annotateToken(item, start, end, lineMap, changedMap);
    localCursor = index + raw.length;
  }
}

function annotateMarkdownTokens(tokens, sourceTextRaw, lineMap, changedMap) {
  if (!Array.isArray(tokens) || tokens.length === 0) return;
  const sourceText = String(sourceTextRaw || '');
  let cursor = 0;
  for (const token of tokens) {
    const raw = typeof token?.raw === 'string' ? token.raw : '';
    if (!raw) continue;
    const found = sourceText.indexOf(raw, cursor);
    const index = found >= 0 ? found : cursor;
    const start = 1 + countNewlines(sourceText.slice(0, index));
    const end = start + countNewlines(raw);
    annotateToken(token, start, end, lineMap, changedMap);
    annotateListItems(token, start, lineMap, changedMap);
    cursor = index + raw.length;
  }
}

function markdownTokenAttrs(token) {
  if (!token || typeof token !== 'object') return '';
  const attrs = [];
  if (Number.isFinite(token.taburaSourceLine) && token.taburaSourceLine > 0) {
    attrs.push(`data-source-line="${Math.trunc(token.taburaSourceLine)}"`);
  }
  if (token.taburaDiffChanged) {
    attrs.push('class="md-diff-changed"');
  }
  if (attrs.length === 0) return '';
  return ` ${attrs.join(' ')}`;
}

function injectAttrsIntoOpeningTag(htmlRaw, tagNameRaw, attrs) {
  const html = String(htmlRaw || '');
  const tagName = String(tagNameRaw || '').trim();
  if (!attrs || !tagName || !html) return html;
  const open = `<${tagName}`;
  const index = html.indexOf(open);
  if (index < 0) return html;
  const insertAt = index + open.length;
  return `${html.slice(0, insertAt)}${attrs}${html.slice(insertAt)}`;
}

function highlightDiff(code) {
  const lines = code.split('\n');
  let oldLine = null;
  let newLine = null;
  let filePath = '';
  let fileLang = '';
  return lines.map((line, index) => {
    const kind = classifyDiffLine(line);
    const hunk = parseDiffHunkHeader(line);
    if (hunk) {
      oldLine = Number.isFinite(hunk.oldStart) ? hunk.oldStart : null;
      newLine = Number.isFinite(hunk.newStart) ? hunk.newStart : null;
    }

    if (line.startsWith('diff --git ')) {
      const nextPath = parseDiffPathFromHeader(line);
      if (nextPath) {
        filePath = nextPath;
        fileLang = languageFromArtifactTitle(filePath);
      }
      oldLine = null;
      newLine = null;
    } else if (line.startsWith('+++ ')) {
      const plusPath = parseDiffPathFromMarker(line, '+++ ');
      if (plusPath) {
        filePath = plusPath;
        fileLang = languageFromArtifactTitle(filePath);
      }
    } else if (line.startsWith('--- ') && !filePath) {
      const minusPath = parseDiffPathFromMarker(line, '--- ');
      if (minusPath) {
        filePath = minusPath;
        fileLang = languageFromArtifactTitle(filePath);
      }
    }

    let oldAtLine = null;
    let newAtLine = null;
    if (!hunk && (Number.isFinite(oldLine) || Number.isFinite(newLine))) {
      if (line.startsWith('+') && !line.startsWith('+++')) {
        if (Number.isFinite(newLine)) {
          newAtLine = newLine;
          newLine += 1;
        }
      } else if (line.startsWith('-') && !line.startsWith('---')) {
        if (Number.isFinite(oldLine)) {
          oldAtLine = oldLine;
          oldLine += 1;
        }
      } else if (line.startsWith(' ')) {
        if (Number.isFinite(oldLine)) {
          oldAtLine = oldLine;
          oldLine += 1;
        }
        if (Number.isFinite(newLine)) {
          newAtLine = newLine;
          newLine += 1;
        }
      }
    }

    const fileLine = Number.isFinite(newAtLine)
      ? newAtLine
      : (Number.isFinite(oldAtLine) ? oldAtLine : null);
    const attrs = [`class="hl-diff-line hl-diff-${kind}"`, `data-diff-line="${index + 1}"`];
    if (filePath) {
      attrs.push(`data-file-path="${escapeHtml(filePath)}"`);
    }
    if (Number.isFinite(fileLine)) {
      attrs.push(`data-file-line="${fileLine}"`);
    }
    if (Number.isFinite(oldAtLine)) {
      attrs.push(`data-old-line="${oldAtLine}"`);
    }
    if (Number.isFinite(newAtLine)) {
      attrs.push(`data-new-line="${newAtLine}"`);
    }
    if (!line) {
      return `<span ${attrs.join(' ')}></span>`;
    }
    return `<span ${attrs.join(' ')}>${highlightDiffCodeLine(line, fileLang)}</span>`;
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

const baseRenderer = new marked.Renderer();
const renderer = new marked.Renderer();
renderer.code = function codeRenderer(token) {
  const rendered = renderCodeBlock(token?.text || '', token?.lang || '');
  return injectAttrsIntoOpeningTag(rendered, 'pre', markdownTokenAttrs(token));
};
renderer.heading = function headingRenderer(token) {
  const html = baseRenderer.heading.call(this, token);
  return injectAttrsIntoOpeningTag(html, `h${token?.depth || 1}`, markdownTokenAttrs(token));
};
renderer.paragraph = function paragraphRenderer(token) {
  const html = baseRenderer.paragraph.call(this, token);
  return injectAttrsIntoOpeningTag(html, 'p', markdownTokenAttrs(token));
};
renderer.blockquote = function blockquoteRenderer(token) {
  const html = baseRenderer.blockquote.call(this, token);
  return injectAttrsIntoOpeningTag(html, 'blockquote', markdownTokenAttrs(token));
};
renderer.list = function listRenderer(token) {
  const html = baseRenderer.list.call(this, token);
  const tag = token?.ordered ? 'ol' : 'ul';
  return injectAttrsIntoOpeningTag(html, tag, markdownTokenAttrs(token));
};
renderer.listitem = function listItemRenderer(token) {
  const html = baseRenderer.listitem.call(this, token);
  return injectAttrsIntoOpeningTag(html, 'li', markdownTokenAttrs(token));
};
renderer.table = function tableRenderer(token) {
  const html = baseRenderer.table.call(this, token);
  return injectAttrsIntoOpeningTag(html, 'table', markdownTokenAttrs(token));
};
renderer.hr = function hrRenderer(token) {
  const html = baseRenderer.hr.call(this, token);
  return injectAttrsIntoOpeningTag(html, 'hr', markdownTokenAttrs(token));
};

marked.setOptions({
  breaks: true,
  renderer,
});

export function sanitizeHtml(html) {
  const doc = new DOMParser().parseFromString(html, 'text/html');
  const dangerous = doc.querySelectorAll('script,iframe,object,embed,link[rel="import"],form,svg,base,style');
  dangerous.forEach((el) => el.remove());
  doc.querySelectorAll('*').forEach((el) => {
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

function getBlockSelector() {
  return 'p, h1, h2, h3, h4, h5, h6, pre, ul, ol, table, blockquote, hr';
}

function captureBlockTexts(root) {
  const blocks = root.querySelectorAll(getBlockSelector());
  const texts = [];
  for (let i = 0; i < blocks.length; i += 1) {
    texts.push(blocks[i].textContent || '');
  }
  return texts;
}

function applyDiffHighlight(root, oldBlockTexts) {
  if (!oldBlockTexts || oldBlockTexts.length === 0 || !root) return;
  const blocks = root.querySelectorAll(getBlockSelector());
  const changedBlocks = [];
  const maxLen = Math.max(oldBlockTexts.length, blocks.length);
  for (let i = 0; i < maxLen; i += 1) {
    if (i >= blocks.length) break;
    const oldContent = i < oldBlockTexts.length ? oldBlockTexts[i] : '';
    const newContent = blocks[i].textContent || '';
    if (oldContent !== newContent) {
      blocks[i].classList.add('diff-highlight');
      changedBlocks.push(blocks[i]);
    }
  }
  for (let i = oldBlockTexts.length; i < blocks.length; i += 1) {
    blocks[i].classList.add('diff-highlight');
    changedBlocks.push(blocks[i]);
  }
  if (changedBlocks[0] && changedBlocks[0].isConnected) {
    changedBlocks[0].scrollIntoView({ behavior: 'smooth', block: 'center' });
  }
}

export function renderTextArtifact(root, event, previousState) {
  const nextArtifactTitle = String(event.title || '');
  const priorBlocks = Array.isArray(previousState?.previousBlockTexts) ? previousState.previousBlockTexts : [];
  const shouldHighlightChanges = previousState?.previousArtifactTitle === nextArtifactTitle && priorBlocks.length > 0;
  const oldBlockTexts = shouldHighlightChanges ? priorBlocks.slice() : [];
  const textBody = String(event.text || '');
  const isUnifiedDiff = textBody.startsWith('diff --git ') || textBody.includes('\ndiff --git ');
  const diffPreview = isUnifiedDiff ? buildMarkdownDiffPreview(textBody, nextArtifactTitle) : null;
  const sourceLang = languageFromArtifactTitle(nextArtifactTitle);

  if (diffPreview) {
    const markdownSource = diffPreview.markdown;
    const { text: markdownText, stash: mathSegments } = extractMathSegments(markdownSource);
    const tokens = marked.lexer(markdownText);
    annotateMarkdownTokens(tokens, markdownText, diffPreview.lineMap, diffPreview.changedMap);
    const renderedMarkdownHtml = marked.parser(tokens);
    root.innerHTML = restoreMathSegments(sanitizeHtml(renderedMarkdownHtml), mathSegments);
    typesetMarkdownMath(root);
  } else if (isUnifiedDiff) {
    root.innerHTML = sanitizeHtml(renderCodeBlock(textBody, 'diff'));
  } else if (sourceLang) {
    root.innerHTML = sanitizeHtml(renderHighlightedCodeBlock(textBody, sourceLang));
  } else {
    const { text: markdownText, stash: mathSegments } = extractMathSegments(textBody);
    const renderedMarkdownHtml = marked.parse(markdownText);
    root.innerHTML = restoreMathSegments(sanitizeHtml(renderedMarkdownHtml), mathSegments);
    typesetMarkdownMath(root);
  }

  const nextBlocks = captureBlockTexts(root);
  applyDiffHighlight(root, oldBlockTexts);
  return {
    activeArtifactTitle: nextArtifactTitle,
    previousArtifactText: event.text || '',
    previousArtifactTitle: nextArtifactTitle,
    previousBlockTexts: nextBlocks,
  };
}
