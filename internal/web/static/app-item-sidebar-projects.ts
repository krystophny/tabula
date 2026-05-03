function projectChildCount(item, field) {
  const children = item && typeof item === 'object' && item.children && typeof item.children === 'object'
    ? item.children
    : {};
  const value = Number(children[field] || 0);
  return Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0;
}

export function filterProjectItemsForSidebarView(items, view) {
  const normalizedView = String(view || '').trim().toLowerCase() || 'inbox';
  return (Array.isArray(items) ? items : []).filter((item) => {
    const next = projectChildCount(item, 'next');
    const waiting = projectChildCount(item, 'waiting');
    const deferred = projectChildCount(item, 'deferred');
    const notStarted = projectChildCount(item, 'not_started');
    const someday = projectChildCount(item, 'someday');
    const stalled = Boolean(item?.health?.stalled);
    const deadline = item && typeof item === 'object' && item.deadline && typeof item.deadline === 'object'
      ? item.deadline
      : {};
    const duePressure = Number(deadline.overdue || 0) > 0
      || Number(deadline.due_today || 0) > 0
      || Number(deadline.due_this_week || 0) > 0;
    if (normalizedView === 'next' || normalizedView === 'inbox') return next > 0 || duePressure;
    if (normalizedView === 'waiting') return next === 0 && waiting > 0;
    if (normalizedView === 'deferred') return next === 0 && waiting === 0 && (deferred > 0 || notStarted > 0);
    if (normalizedView === 'someday') return next === 0 && waiting === 0 && deferred === 0 && notStarted === 0 && someday > 0;
    if (normalizedView === 'review') return stalled;
    return true;
  });
}

function parseTimestamp(value) {
  const text = String(value || '').trim();
  if (!text) return null;
  const parsed = Date.parse(/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(text) ? `${text.replace(' ', 'T')}Z` : text);
  return Number.isFinite(parsed) ? parsed : null;
}

export function formatDateBadge(label, value) {
  const parsed = parseTimestamp(value);
  if (parsed === null) return '';
  return `${label} ${new Date(parsed).toISOString().slice(0, 10)}`;
}

function daysUntilTimestamp(value) {
  const parsed = parseTimestamp(value);
  return parsed === null ? null : Math.ceil((parsed - Date.now()) / 86400000);
}

export function deadlineBadge(value) {
  const days = daysUntilTimestamp(value);
  if (days === null) return '';
  if (days < 0) return `overdue ${Math.abs(days)}d`;
  if (days === 0) return 'due today';
  if (days <= 7) return `due ${days}d`;
  return formatDateBadge('due', value);
}

export function projectItemChildBadges(item) {
  const next = projectChildCount(item, 'next');
  const waiting = projectChildCount(item, 'waiting');
  const deferred = projectChildCount(item, 'deferred');
  const notStarted = projectChildCount(item, 'not_started');
  const someday = projectChildCount(item, 'someday');
  const active = [
    `next ${next}`,
    `waiting ${waiting}`,
    `deferred ${deferred}`,
  ].filter((badge) => !badge.endsWith(' 0'));
  if (active.length > 0) return active;
  return [
    `scheduled ${notStarted}`,
    `someday ${someday}`,
  ].filter((badge) => !badge.endsWith(' 0'));
}

export function projectDeadlineBadges(item) {
  const deadline = item && typeof item === 'object' && item.deadline && typeof item.deadline === 'object'
    ? item.deadline
    : {};
  const fields = [
    ['overdue', 'overdue'],
    ['due_today', 'due today'],
    ['due_this_week', 'due 7d'],
  ];
  return fields.flatMap(([field, label]) => {
    const value = Number(deadline[field] || 0);
    return Number.isFinite(value) && value > 0 ? [`${label} ${Math.trunc(value)}`] : [];
  });
}

export function deadlineLevelForItem(item) {
  if (String(item?.kind || '').trim().toLowerCase() === 'project') {
    const deadline = item && typeof item === 'object' && item.deadline && typeof item.deadline === 'object'
      ? item.deadline
      : {};
    if (Number(deadline.overdue || 0) > 0) return 'overdue';
    if (Number(deadline.due_today || 0) > 0 || Number(deadline.due_this_week || 0) > 0) return 'soon';
    return String(deadline.next_due_at || '').trim() ? 'dated' : '';
  }
  const days = daysUntilTimestamp(item?.due_at);
  if (days === null) return '';
  if (days < 0) return 'overdue';
  return days <= 7 ? 'soon' : 'dated';
}
