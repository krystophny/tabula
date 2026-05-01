import * as env from './app-env.js';
import * as context from './app-context.js';

const { apiURL } = env;
const { refs, state } = context;

const itemSidebarCountsEndpoint = (...args) => refs.itemSidebarCountsEndpoint(...args);
const normalizeItemSidebarFilters = (...args) => refs.normalizeItemSidebarFilters(...args);
const loadItemSidebarView = (...args) => refs.loadItemSidebarView(...args);
const showStatus = (...args) => refs.showStatus(...args);

function queryFromCountsEndpoint(filters) {
  const endpoint = String(itemSidebarCountsEndpoint(filters) || '');
  const index = endpoint.indexOf('?');
  return index >= 0 ? endpoint.slice(index) : '';
}

function peopleEndpoint(filters) {
  return `items/people${queryFromCountsEndpoint(filters)}`;
}

function personEndpoint(actorID, filters) {
  return `items/people/${encodeURIComponent(String(actorID))}${queryFromCountsEndpoint(filters)}`;
}

function countValue(counts, field) {
  const value = Number(counts?.[field] || 0);
  return Number.isFinite(value) && value > 0 ? Math.trunc(value) : 0;
}

function normalizeDiagnostics(raw) {
  return Array.isArray(raw)
    ? raw.map((entry) => String(entry || '').trim()).filter(Boolean)
    : [];
}

function normalizePersonRow(row) {
  const actor = row && typeof row === 'object' ? row.actor : null;
  const actorID = Number(actor?.id || row?.actor_id || 0);
  const person = String(row?.person || actor?.name || '').trim();
  const counts = row?.counts && typeof row.counts === 'object' ? row.counts : {};
  return {
    id: actorID,
    actor_id: actorID,
    title: person,
    actor_name: person,
    kind: 'person_dashboard',
    person,
    person_path: String(row?.person_path || '').trim(),
    diagnostics: normalizeDiagnostics(row?.diagnostics),
    counts: {
      waiting_on_them: countValue(counts, 'waiting_on_them'),
      i_owe_them: countValue(counts, 'i_owe_them'),
      recently_closed: countValue(counts, 'recently_closed'),
      open: countValue(counts, 'open'),
    },
  };
}

export async function fetchItemSidebarPeopleDashboard(filters = state.itemSidebarFilters) {
  const normalized = normalizeItemSidebarFilters(filters);
  const resp = await fetch(apiURL(peopleEndpoint(normalized)), { cache: 'no-store' });
  if (!resp.ok) {
    const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
    throw new Error(detail);
  }
  const payload = await resp.json();
  const rows = Array.isArray(payload?.people) ? payload.people : [];
  return rows.map(normalizePersonRow).filter((row) => row.id > 0 && row.person);
}

export async function fetchItemSidebarPersonDashboard(actorID, filters = state.itemSidebarFilters) {
  const cleanActorID = Number(actorID || 0);
  if (!Number.isFinite(cleanActorID) || cleanActorID <= 0) return null;
  const normalized = normalizeItemSidebarFilters(filters);
  const resp = await fetch(apiURL(personEndpoint(Math.trunc(cleanActorID), normalized)), { cache: 'no-store' });
  if (!resp.ok) {
    const detail = (await resp.text()).trim() || `HTTP ${resp.status}`;
    throw new Error(detail);
  }
  const payload = await resp.json();
  const row = payload?.person && typeof payload.person === 'object' ? payload.person : null;
  if (!row) return null;
  return {
    ...normalizePersonRow(row),
    waiting_on_them: Array.isArray(row.waiting_on_them) ? row.waiting_on_them : [],
    i_owe_them: Array.isArray(row.i_owe_them) ? row.i_owe_them : [],
    recently_closed: Array.isArray(row.recently_closed) ? row.recently_closed : [],
    project_items: Array.isArray(row.project_items) ? row.project_items : [],
  };
}

export async function openPersonOpenLoops(person) {
  const actorID = Number(person?.actor_id || person?.id || 0);
  if (actorID <= 0) return false;
  state.itemSidebarActiveItemID = actorID;
  const nextFilters = {
    ...state.itemSidebarFilters,
    actor_id: actorID,
    section: 'people',
  };
  await loadItemSidebarView('waiting', nextFilters);
  showStatus(`person: ${String(person?.person || person?.title || actorID).trim()}`);
  return true;
}
