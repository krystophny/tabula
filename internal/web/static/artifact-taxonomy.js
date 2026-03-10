const IMAGE_EXTENSIONS = new Set([
  '.apng',
  '.avif',
  '.bmp',
  '.gif',
  '.ico',
  '.jpeg',
  '.jpg',
  '.png',
  '.svg',
  '.tif',
  '.tiff',
  '.webp',
]);

export const CANONICAL_ACTION_SEMANTICS = Object.freeze([
  'open_show',
  'annotate_capture',
  'compose',
  'bundle_review',
  'dispatch_execute',
  'track_item',
  'delegate_actor',
]);

const DEFAULT_CANVAS_SURFACE = 'text_artifact';
const DEFAULT_SPEC = Object.freeze({
  family: 'artifact',
  canvas_surface: DEFAULT_CANVAS_SURFACE,
  interaction_model: 'canonical_canvas',
  actions: Object.freeze([
    'open_show',
    'annotate_capture',
    'compose',
    'bundle_review',
    'track_item',
  ]),
  mail_actions: false,
});

export const ARTIFACT_KIND_TAXONOMY = Object.freeze({
  annotation: Object.freeze({
    family: 'review_bundle',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'bundle_review', 'dispatch_execute', 'track_item']),
    mail_actions: false,
  }),
  document: Object.freeze({
    family: 'reference',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'bundle_review', 'track_item']),
    mail_actions: false,
  }),
  email: Object.freeze({
    family: 'message',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'compose', 'dispatch_execute', 'track_item']),
    mail_actions: true,
  }),
  email_thread: Object.freeze({
    family: 'message',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'compose', 'bundle_review', 'dispatch_execute', 'track_item']),
    mail_actions: true,
  }),
  external_note: Object.freeze({
    family: 'captured_note',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'compose', 'track_item']),
    mail_actions: false,
  }),
  external_task: Object.freeze({
    family: 'action_card',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'compose', 'dispatch_execute', 'track_item', 'delegate_actor']),
    mail_actions: false,
  }),
  github_issue: Object.freeze({
    family: 'proposal',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'compose', 'bundle_review', 'dispatch_execute', 'track_item']),
    mail_actions: false,
  }),
  github_pr: Object.freeze({
    family: 'proposal',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'bundle_review', 'dispatch_execute', 'track_item', 'delegate_actor']),
    mail_actions: false,
  }),
  idea_note: Object.freeze({
    family: 'planning_note',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'compose', 'bundle_review', 'track_item']),
    mail_actions: false,
  }),
  image: Object.freeze({
    family: 'reference',
    canvas_surface: 'image_artifact',
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'bundle_review', 'track_item']),
    mail_actions: false,
  }),
  markdown: Object.freeze({
    family: 'reference',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'bundle_review', 'track_item']),
    mail_actions: false,
  }),
  pdf: Object.freeze({
    family: 'reference',
    canvas_surface: 'pdf_artifact',
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'bundle_review', 'track_item']),
    mail_actions: false,
  }),
  plan_note: Object.freeze({
    family: 'planning_note',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'compose', 'bundle_review', 'track_item']),
    mail_actions: false,
  }),
  reference: Object.freeze({
    family: 'reference',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'bundle_review', 'track_item']),
    mail_actions: false,
  }),
  transcript: Object.freeze({
    family: 'transcript',
    canvas_surface: DEFAULT_CANVAS_SURFACE,
    interaction_model: 'canonical_canvas',
    actions: Object.freeze(['open_show', 'annotate_capture', 'bundle_review', 'track_item']),
    mail_actions: false,
  }),
});

export function normalizeArtifactKind(kind) {
  return String(kind || '').trim().toLowerCase();
}

export function artifactKindSpec(kind) {
  const normalized = normalizeArtifactKind(kind);
  const spec = ARTIFACT_KIND_TAXONOMY[normalized];
  if (spec) return spec;
  return DEFAULT_SPEC;
}

function imageExtensionFromPath(refPath) {
  const normalized = String(refPath || '').trim().toLowerCase();
  if (!normalized) return '';
  const dot = normalized.lastIndexOf('.');
  if (dot < 0) return '';
  return normalized.slice(dot);
}

export function artifactCanvasEventKind(kind, refPath = '') {
  const normalized = normalizeArtifactKind(kind);
  if (normalized === 'email_draft') return DEFAULT_CANVAS_SURFACE;
  if (normalized === 'pdf') return 'pdf_artifact';
  if (normalized === 'image') return 'image_artifact';
  if (String(refPath || '').trim().toLowerCase().endsWith('.pdf')) return 'pdf_artifact';
  if (IMAGE_EXTENSIONS.has(imageExtensionFromPath(refPath))) return 'image_artifact';
  return artifactKindSpec(normalized).canvas_surface || DEFAULT_CANVAS_SURFACE;
}

export function artifactSupportsMailActions(kind) {
  return artifactKindSpec(kind).mail_actions === true;
}

export function artifactUsesThreadHTML(kind) {
  return normalizeArtifactKind(kind) === 'email_thread';
}
