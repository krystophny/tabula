import { apiURL } from './paths.js';

const VISUAL_REASONING_SNAPSHOT_MAX_EDGE_PX = 960;
const VISUAL_REASONING_MARKER_RADIUS_PX = 18;
const VISUAL_REASONING_MARKER_STROKE_PX = 4;

function currentCanvasState() {
  return (window._taburaApp || {}).getState ? window._taburaApp.getState() : {};
}

export function currentCanvasSessionID() {
  return String(currentCanvasState().sessionId || '');
}

export function currentCanvasArtifactMeta(activeArtifactTitle) {
  const current = currentCanvasState().currentCanvasArtifact && typeof currentCanvasState().currentCanvasArtifact === 'object'
    ? currentCanvasState().currentCanvasArtifact
    : {};
  return {
    title: String(activeArtifactTitle || current.title || '').trim(),
    path: normalizeCanvasPath(current.path || ''),
  };
}

export function currentPdfArtifact(activePdfEvent) {
  if (activePdfEvent && typeof activePdfEvent === 'object') {
    return activePdfEvent;
  }
  const current = currentCanvasState().currentCanvasArtifact && typeof currentCanvasState().currentCanvasArtifact === 'object'
    ? currentCanvasState().currentCanvasArtifact
    : null;
  if (String(current?.kind || '').trim() === 'pdf_artifact') {
    return current;
  }
  return null;
}

export function normalizeCanvasPath(pathRaw) {
  const parts = String(pathRaw || '')
    .replaceAll('\\', '/')
    .split('/');
  const stack = [];
  for (const part of parts) {
    const segment = String(part || '').trim();
    if (!segment || segment === '.') continue;
    if (segment === '..') {
      if (stack.length > 0) stack.pop();
      continue;
    }
    stack.push(segment);
  }
  return stack.join('/');
}

function dirnameForCanvasPath(pathRaw) {
  const normalized = normalizeCanvasPath(pathRaw);
  if (!normalized) return '';
  const slash = normalized.lastIndexOf('/');
  if (slash < 0) return '';
  return normalized.slice(0, slash);
}

function resolveCanvasImagePath(basePathRaw, srcRaw) {
  const src = String(srcRaw || '').trim();
  if (!src) return '';
  const lower = src.toLowerCase();
  if (
    lower.startsWith('http://')
    || lower.startsWith('https://')
    || lower.startsWith('data:')
    || lower.startsWith('blob:')
    || lower.startsWith('about:')
  ) {
    return '';
  }
  const normalizedSrc = src.replaceAll('\\', '/');
  if (normalizedSrc.startsWith('/')) {
    return normalizeCanvasPath(normalizedSrc);
  }
  const baseDir = dirnameForCanvasPath(basePathRaw);
  return normalizeCanvasPath(baseDir ? `${baseDir}/${normalizedSrc}` : normalizedSrc);
}

function resolveCanvasImageURL(basePathRaw, srcRaw, sessionID) {
  const src = String(srcRaw || '').trim();
  if (!src) return { url: '', path: '' };
  const lower = src.toLowerCase();
  if (
    lower.startsWith('http://')
    || lower.startsWith('https://')
    || lower.startsWith('data:')
    || lower.startsWith('blob:')
    || lower.startsWith('about:')
  ) {
    return { url: src, path: '' };
  }
  const resolvedPath = resolveCanvasImagePath(basePathRaw, src);
  if (!sessionID || !resolvedPath) {
    return { url: src, path: '' };
  }
  return {
    url: apiURL(`files/${encodeURIComponent(sessionID)}/${encodeURIComponent(resolvedPath)}`),
    path: resolvedPath,
  };
}

export function hydrateTextArtifactImages(root, basePath, sessionID) {
  if (!(root instanceof HTMLElement)) return;
  root.querySelectorAll('img').forEach((node) => {
    if (!(node instanceof HTMLImageElement)) return;
    const originalSrc = node.getAttribute('src') || '';
    const resolved = resolveCanvasImageURL(basePath, originalSrc, sessionID);
    if (resolved.url) {
      node.src = resolved.url;
    }
    if (resolved.path) {
      node.dataset.canvasSourcePath = resolved.path;
    }
    node.dataset.canvasImage = 'true';
    if (!node.alt) {
      node.alt = 'Embedded image';
    }
  });
}

export function getTextImageNodeFromPoint(root, clientX, clientY) {
  if (!(root instanceof HTMLElement) || !root.classList.contains('is-active')) return null;
  const hit = document.elementFromPoint(clientX, clientY);
  if (hit instanceof HTMLImageElement && root.contains(hit)) {
    return hit;
  }
  return null;
}

export function getTextImageAnchorFromPoint(root, clientX, clientY, artifactTitle, compactText) {
  const image = getTextImageNodeFromPoint(root, clientX, clientY);
  if (!(image instanceof HTMLImageElement)) return null;
  const rect = image.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return null;
  const relativeX = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
  const relativeY = Math.max(0, Math.min(1, (clientY - rect.top) / rect.height));
  return {
    element: 'image',
    title: String(artifactTitle || '').trim(),
    path: String(image.dataset.canvasSourcePath || '').trim(),
    relativeX,
    relativeY,
    surroundingText: typeof compactText === 'function'
      ? compactText(image.alt || image.title || '')
      : String(image.alt || image.title || '').trim(),
  };
}

function drawVisualMarker(ctx, xPx, yPx, scale) {
  if (!ctx) return;
  const radius = Math.max(10, VISUAL_REASONING_MARKER_RADIUS_PX * scale);
  const stroke = Math.max(2, VISUAL_REASONING_MARKER_STROKE_PX * scale);
  ctx.save();
  ctx.lineWidth = stroke;
  ctx.strokeStyle = '#ff3b30';
  ctx.fillStyle = 'rgba(255, 59, 48, 0.18)';
  ctx.beginPath();
  ctx.arc(xPx, yPx, radius, 0, Math.PI * 2);
  ctx.fill();
  ctx.stroke();
  ctx.beginPath();
  ctx.moveTo(xPx - radius * 1.35, yPx);
  ctx.lineTo(xPx + radius * 1.35, yPx);
  ctx.moveTo(xPx, yPx - radius * 1.35);
  ctx.lineTo(xPx, yPx + radius * 1.35);
  ctx.stroke();
  ctx.restore();
}

function buildMarkedVisualSnapshot(width, height, drawSource, relativeX, relativeY) {
  const safeWidth = Math.max(1, Math.floor(Number(width) || 0));
  const safeHeight = Math.max(1, Math.floor(Number(height) || 0));
  if (safeWidth <= 0 || safeHeight <= 0) return '';
  const maxEdge = Math.max(safeWidth, safeHeight);
  const scale = maxEdge > VISUAL_REASONING_SNAPSHOT_MAX_EDGE_PX
    ? VISUAL_REASONING_SNAPSHOT_MAX_EDGE_PX / maxEdge
    : 1;
  const outWidth = Math.max(1, Math.round(safeWidth * scale));
  const outHeight = Math.max(1, Math.round(safeHeight * scale));
  const canvas = document.createElement('canvas');
  canvas.width = outWidth;
  canvas.height = outHeight;
  const ctx = canvas.getContext('2d');
  if (!ctx) return '';
  try {
    drawSource(ctx, outWidth, outHeight);
    drawVisualMarker(
      ctx,
      Math.max(0, Math.min(outWidth, relativeX * outWidth)),
      Math.max(0, Math.min(outHeight, relativeY * outHeight)),
      scale,
    );
    return canvas.toDataURL('image/png');
  } catch (_) {
    return '';
  }
}

export function captureImageElementSnapshot(image, relativeX, relativeY) {
  if (!(image instanceof HTMLImageElement)) return '';
  const width = image.naturalWidth || image.width || 0;
  const height = image.naturalHeight || image.height || 0;
  if (width <= 0 || height <= 0) return '';
  return buildMarkedVisualSnapshot(width, height, (ctx, outWidth, outHeight) => {
    ctx.drawImage(image, 0, 0, outWidth, outHeight);
  }, relativeX, relativeY);
}

export function captureCanvasSnapshot(sourceCanvas, relativeX, relativeY) {
  if (!(sourceCanvas instanceof HTMLCanvasElement)) return '';
  const width = sourceCanvas.width || 0;
  const height = sourceCanvas.height || 0;
  if (width <= 0 || height <= 0) return '';
  return buildMarkedVisualSnapshot(width, height, (ctx, outWidth, outHeight) => {
    ctx.drawImage(sourceCanvas, 0, 0, outWidth, outHeight);
  }, relativeX, relativeY);
}

export function captureVisualReasoningContext({
  textRoot,
  imagePane,
  imageEl,
  pdfRoot,
  clientX,
  clientY,
  getPdfPageNodeFromPoint,
}) {
  if (!Number.isFinite(clientX) || !Number.isFinite(clientY)) {
    return null;
  }
  const textImage = getTextImageNodeFromPoint(textRoot, clientX, clientY);
  if (textImage instanceof HTMLImageElement) {
    const rect = textImage.getBoundingClientRect();
    if (rect.width > 0 && rect.height > 0) {
      const relativeX = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
      const relativeY = Math.max(0, Math.min(1, (clientY - rect.top) / rect.height));
      return {
        kind: 'markdown_image',
        snapshotDataURL: captureImageElementSnapshot(textImage, relativeX, relativeY),
      };
    }
  }
  if (imagePane instanceof HTMLElement && imagePane.classList.contains('is-active') && imageEl instanceof HTMLImageElement) {
    const rect = imageEl.getBoundingClientRect();
    if (rect.width > 0 && rect.height > 0 && clientX >= rect.left && clientX <= rect.right && clientY >= rect.top && clientY <= rect.bottom) {
      const relativeX = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
      const relativeY = Math.max(0, Math.min(1, (clientY - rect.top) / rect.height));
      return {
        kind: 'image_artifact',
        snapshotDataURL: captureImageElementSnapshot(imageEl, relativeX, relativeY),
      };
    }
  }
  const pageNode = typeof getPdfPageNodeFromPoint === 'function'
    ? getPdfPageNodeFromPoint(pdfRoot, clientX, clientY)
    : null;
  if (pageNode instanceof HTMLElement) {
    const pageInner = pageNode.querySelector('.canvas-pdf-page-inner');
    const pageCanvas = pageNode.querySelector('.canvas-pdf-canvas');
    if (pageInner instanceof HTMLElement && pageCanvas instanceof HTMLCanvasElement) {
      const rect = pageInner.getBoundingClientRect();
      if (rect.width > 0 && rect.height > 0) {
        const relativeX = Math.max(0, Math.min(1, (clientX - rect.left) / rect.width));
        const relativeY = Math.max(0, Math.min(1, (clientY - rect.top) / rect.height));
        return {
          kind: 'pdf_page',
          snapshotDataURL: captureCanvasSnapshot(pageCanvas, relativeX, relativeY),
        };
      }
    }
  }
  return null;
}
