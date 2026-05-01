import * as context from './app-context.js';

const { ITEM_SIDEBAR_VIEWS } = context;

export function defaultItemSidebarCounts() {
  return {
    inbox: 0,
    next: 0,
    waiting: 0,
    deferred: 0,
    someday: 0,
    review: 0,
    done: 0,
  };
}

export function normalizeItemSidebarCounts(rawCounts) {
  const counts = defaultItemSidebarCounts();
  if (!rawCounts || typeof rawCounts !== 'object') return counts;
  ITEM_SIDEBAR_VIEWS.forEach((view) => {
    const value = Number(rawCounts[view] ?? 0);
    counts[view] = Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0;
  });
  return counts;
}

export function setInboxTriggerCount(count) {
  const edgeLeftTap = document.getElementById('edge-left-tap');
  if (!(edgeLeftTap instanceof HTMLElement)) return;
  const normalizedCount = Number.isFinite(Number(count)) && Number(count) > 0
    ? Math.trunc(Number(count))
    : 0;
  if (normalizedCount > 0) {
    edgeLeftTap.dataset.inboxCount = String(normalizedCount);
    edgeLeftTap.classList.add('has-inbox-count');
    return;
  }
  edgeLeftTap.dataset.inboxCount = '';
  edgeLeftTap.classList.remove('has-inbox-count');
}

export function defaultItemSidebarSectionCounts() {
  return {
    project_items_open: 0,
    people_open: 0,
    drift_review: 0,
    dedup_review: 0,
    recent_meetings: 0,
  };
}

export function normalizeItemSidebarSectionCounts(rawSections) {
  const out = defaultItemSidebarSectionCounts();
  if (!rawSections || typeof rawSections !== 'object') return out;
  const fields = ['project_items_open', 'people_open', 'drift_review', 'dedup_review', 'recent_meetings'];
  for (const field of fields) {
    const raw = Number(rawSections[field] ?? 0);
    if (Number.isFinite(raw) && raw > 0) {
      out[field] = Math.trunc(raw);
    }
  }
  return out;
}
