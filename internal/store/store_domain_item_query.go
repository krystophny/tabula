package store

import (
	"database/sql"
	"errors"
	"sort"
	"time"
)

func (s *Store) ListItemsByState(state string) ([]Item, error) {
	return s.ListItemsByStateFiltered(state, ItemListFilter{})
}

func (s *Store) ListItemsByStateForSphere(state, sphere string) ([]Item, error) {
	return s.ListItemsByStateFiltered(state, ItemListFilter{Sphere: sphere})
}

func (s *Store) ListItemsByStateFiltered(state string, filter ItemListFilter) ([]Item, error) {
	cleanState := normalizeItemState(state)
	if cleanState == "" {
		return nil, errors.New("invalid item state")
	}
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	parts := []string{"state = ?"}
	args := []any{cleanState}
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "")
	query := `SELECT id, title, kind, state, workspace_id, ` + scopedContextSelect("context_items", "item_id", "items.id") + ` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items
		 WHERE ` + stringsJoin(parts, " AND ")
	rows, err := s.db.Query(
		query,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt == out[j].UpdatedAt {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, nil
}

var itemSummarySelect = `SELECT
	i.id,
	i.title,
	i.kind,
	i.state,
	i.workspace_id,
	` + scopedContextSelect("context_items", "item_id", "i.id") + `,
 i.artifact_id,
 i.actor_id,
 i.visible_after,
 i.follow_up_at,
 i.due_at,
 i.source,
 i.source_ref,
 i.review_target,
 i.reviewer,
 i.reviewed_at,
 i.created_at,
 i.updated_at,
 a.title,
 a.kind,
 actors.name
FROM items i
LEFT JOIN artifacts a ON a.id = i.artifact_id
LEFT JOIN actors ON actors.id = i.actor_id`

func sortItemSummaries(items []ItemSummary) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt == items[j].UpdatedAt {
			return items[i].ID < items[j].ID
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
}

func (s *Store) GetItemSummary(id int64) (ItemSummary, error) {
	query := itemSummarySelect + `
 WHERE i.id = ?`
	items, err := s.listItemSummaries(query, id)
	if err != nil {
		return ItemSummary{}, err
	}
	if len(items) == 0 {
		return ItemSummary{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (s *Store) listItemSummaries(query string, args ...any) ([]ItemSummary, error) {
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ItemSummary
	for rows.Next() {
		item, err := scanItemSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sortItemSummaries(out)
	return out, nil
}

func (s *Store) ListInboxItems(now time.Time) ([]ItemSummary, error) {
	return s.ListInboxItemsFiltered(now, ItemListFilter{})
}

func (s *Store) ListInboxItemsForSphere(now time.Time, sphere string) ([]ItemSummary, error) {
	return s.ListInboxItemsFiltered(now, ItemListFilter{Sphere: sphere})
}

func (s *Store) ListInboxItemsFiltered(now time.Time, filter ItemListFilter) ([]ItemSummary, error) {
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	cutoff := now.UTC().Format(time.RFC3339Nano)
	parts := []string{
		`i.state = ?`,
		`(
	     i.visible_after IS NULL
	     OR trim(i.visible_after) = ''
	     OR datetime(i.visible_after) <= datetime(?)
	   )`,
	}
	args := []any{ItemStateInbox, cutoff}
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "i.")
	query := itemSummarySelect + `
 WHERE ` + stringsJoin(parts, `
   AND `) + `
 ORDER BY i.updated_at DESC, i.id ASC`
	return s.listItemSummaries(query, args...)
}

func (s *Store) ListWaitingItems() ([]ItemSummary, error) {
	return s.ListWaitingItemsFiltered(ItemListFilter{})
}

func (s *Store) ListWaitingItemsForSphere(sphere string) ([]ItemSummary, error) {
	return s.ListWaitingItemsFiltered(ItemListFilter{Sphere: sphere})
}

func (s *Store) ListNextItems() ([]ItemSummary, error) {
	return s.ListNextItemsFiltered(ItemListFilter{})
}

func (s *Store) ListDeferredItems() ([]ItemSummary, error) {
	return s.ListDeferredItemsFiltered(ItemListFilter{})
}

func (s *Store) ListReviewItems() ([]ItemSummary, error) {
	return s.ListReviewItemsFiltered(ItemListFilter{})
}

func (s *Store) ListWaitingItemsFiltered(filter ItemListFilter) ([]ItemSummary, error) {
	return s.listItemSummariesByState(ItemStateWaiting, filter)
}

// ListNextItemsFiltered returns items in the next state that are currently
// actionable. Project items (kind=project) are excluded by default so they do
// not masquerade as executable next actions; callers that want the project
// drill-down opt in via Section=ItemSidebarSectionProject, and callers that
// genuinely need both kinds opt in via IncludeProjectItems.
func (s *Store) ListNextItemsFiltered(filter ItemListFilter) ([]ItemSummary, error) {
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	parts := []string{"i.state = ?"}
	args := []any{ItemStateNext}
	if !normalizedFilter.IncludeProjectItems && normalizedFilter.Section != ItemSidebarSectionProject {
		parts = append(parts, "i.kind = ?")
		args = append(args, ItemKindAction)
	}
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "i.")
	query := itemSummarySelect + ` WHERE ` + stringsJoin(parts, ` AND `) + `
 ORDER BY i.updated_at DESC, i.id ASC`
	return s.listItemSummaries(query, args...)
}

func (s *Store) ListDeferredItemsFiltered(filter ItemListFilter) ([]ItemSummary, error) {
	return s.listItemSummariesByState(ItemStateDeferred, filter)
}

func (s *Store) ListReviewItemsFiltered(filter ItemListFilter) ([]ItemSummary, error) {
	return s.listItemSummariesByState(ItemStateReview, filter)
}

func (s *Store) ListSomedayItems() ([]ItemSummary, error) {
	return s.ListSomedayItemsFiltered(ItemListFilter{})
}

func (s *Store) ListSomedayItemsForSphere(sphere string) ([]ItemSummary, error) {
	return s.ListSomedayItemsFiltered(ItemListFilter{Sphere: sphere})
}

func (s *Store) ListSomedayItemsFiltered(filter ItemListFilter) ([]ItemSummary, error) {
	return s.listItemSummariesByState(ItemStateSomeday, filter)
}

func (s *Store) listItemSummariesByState(state string, filter ItemListFilter) ([]ItemSummary, error) {
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	parts := []string{"i.state = ?"}
	args := []any{state}
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "i.")
	query := itemSummarySelect + ` WHERE ` + stringsJoin(parts, ` AND `)
	query += ` ORDER BY i.updated_at DESC, i.id ASC`
	return s.listItemSummaries(query, args...)
}

func (s *Store) ListDoneItems(limit int) ([]ItemSummary, error) {
	return s.ListDoneItemsFiltered(limit, ItemListFilter{})
}

func (s *Store) ListDoneItemsForSphere(limit int, sphere string) ([]ItemSummary, error) {
	return s.ListDoneItemsFiltered(limit, ItemListFilter{Sphere: sphere})
}

func (s *Store) ListDoneItemsFiltered(limit int, filter ItemListFilter) ([]ItemSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	parts := []string{"i.state = ?"}
	args := []any{ItemStateDone}
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "i.")
	query := itemSummarySelect + ` WHERE ` + stringsJoin(parts, ` AND `)
	query += ` ORDER BY i.updated_at DESC, i.id ASC LIMIT ?`
	args = append(args, limit)
	return s.listItemSummaries(query, args...)
}

func (s *Store) CountItemsByState(now time.Time) (map[string]int, error) {
	return s.CountItemsByStateFiltered(now, ItemListFilter{})
}

func (s *Store) CountItemsByStateForSphere(now time.Time, sphere string) (map[string]int, error) {
	return s.CountItemsByStateFiltered(now, ItemListFilter{Sphere: sphere})
}

func (s *Store) CountItemsByStateFiltered(now time.Time, filter ItemListFilter) (map[string]int, error) {
	counts := map[string]int{
		ItemStateInbox:    0,
		ItemStateNext:     0,
		ItemStateWaiting:  0,
		ItemStateDeferred: 0,
		ItemStateSomeday:  0,
		ItemStateReview:   0,
		ItemStateDone:     0,
	}
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	cutoff := now.UTC().Format(time.RFC3339Nano)
	var inbox, next, waiting, deferred, someday, review, done int
	query := `
SELECT
  COALESCE(SUM(CASE
    WHEN state = ?
      AND (
        visible_after IS NULL
        OR trim(visible_after) = ''
        OR datetime(visible_after) <= datetime(?)
      )
    THEN 1 ELSE 0 END), 0) AS inbox_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS next_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS waiting_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS deferred_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS someday_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS review_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS done_count
FROM items
`
	args := []any{
		ItemStateInbox,
		cutoff,
		ItemStateNext,
		ItemStateWaiting,
		ItemStateDeferred,
		ItemStateSomeday,
		ItemStateReview,
		ItemStateDone,
	}
	parts := []string{}
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "")
	if len(parts) > 0 {
		query += ` WHERE ` + stringsJoin(parts, ` AND `)
	}
	if err := s.db.QueryRow(query, args...).Scan(&inbox, &next, &waiting, &deferred, &someday, &review, &done); err != nil {
		return nil, err
	}
	counts[ItemStateInbox] = inbox
	counts[ItemStateNext] = next
	counts[ItemStateWaiting] = waiting
	counts[ItemStateDeferred] = deferred
	counts[ItemStateSomeday] = someday
	counts[ItemStateReview] = review
	counts[ItemStateDone] = done
	return counts, nil
}

// SidebarSectionCounts captures counts for the compact sidebar's secondary
// expandable sections. Project items are open Items with kind=project; they
// stay surfaced as filters and never as Workspaces.
type SidebarSectionCounts struct {
	ProjectItemsOpen int `json:"project_items_open"`
	PeopleOpen       int `json:"people_open"`
	DriftReview      int `json:"drift_review"`
	DedupReview      int `json:"dedup_review"`
	RecentMeetings   int `json:"recent_meetings"`
}

// CountSidebarSectionsFiltered counts open project items (Item.kind=project,
// state != done), distinct actors with at least one open delegated/awaiting
// item (people we owe or await), items currently in the GTD review queue with
// a review_target set (drift dispatch backlog), open items whose
// (source, source_ref) pair has duplicates (dedup review backlog), and
// meeting-note artifacts created within the last seven days. The filter
// respects sphere/workspace/label scoping so the sidebar matches the active
// queue context.
func (s *Store) CountSidebarSectionsFiltered(now time.Time, filter ItemListFilter) (SidebarSectionCounts, error) {
	out := SidebarSectionCounts{}
	scoped := filter
	scoped.Section = ""
	normalizedFilter, err := s.prepareItemListFilter(scoped)
	if err != nil {
		return out, err
	}

	projectParts := []string{"items.kind = ?", "items.state <> ?"}
	projectArgs := []any{ItemKindProject, ItemStateDone}
	projectParts, projectArgs = appendItemFilterClauses(projectParts, projectArgs, normalizedFilter, "")
	projectQuery := `SELECT COUNT(*) FROM items WHERE ` + stringsJoin(projectParts, ` AND `)
	if err := s.db.QueryRow(projectQuery, projectArgs...).Scan(&out.ProjectItemsOpen); err != nil {
		return out, err
	}

	peopleParts := []string{"items.actor_id IS NOT NULL", "items.state <> ?"}
	peopleArgs := []any{ItemStateDone}
	peopleParts, peopleArgs = appendItemFilterClauses(peopleParts, peopleArgs, normalizedFilter, "")
	peopleQuery := `SELECT COUNT(DISTINCT items.actor_id) FROM items WHERE ` + stringsJoin(peopleParts, ` AND `)
	if err := s.db.QueryRow(peopleQuery, peopleArgs...).Scan(&out.PeopleOpen); err != nil {
		return out, err
	}

	driftParts := []string{
		"items.state = ?",
		"items.review_target IS NOT NULL",
		"trim(items.review_target) <> ''",
	}
	driftArgs := []any{ItemStateReview}
	driftParts, driftArgs = appendItemFilterClauses(driftParts, driftArgs, normalizedFilter, "")
	driftQuery := `SELECT COUNT(*) FROM items WHERE ` + stringsJoin(driftParts, ` AND `)
	if err := s.db.QueryRow(driftQuery, driftArgs...).Scan(&out.DriftReview); err != nil {
		return out, err
	}

	dedupParts := []string{
		"items.state <> ?",
		"items.source IS NOT NULL",
		"trim(items.source) <> ''",
		"items.source_ref IS NOT NULL",
		"trim(items.source_ref) <> ''",
		`EXISTS (
SELECT 1 FROM items dup
WHERE dup.id <> items.id
  AND lower(trim(dup.source)) = lower(trim(items.source))
  AND lower(trim(dup.source_ref)) = lower(trim(items.source_ref))
)`,
	}
	dedupArgs := []any{ItemStateDone}
	dedupParts, dedupArgs = appendItemFilterClauses(dedupParts, dedupArgs, normalizedFilter, "")
	dedupQuery := `SELECT COUNT(*) FROM items WHERE ` + stringsJoin(dedupParts, ` AND `)
	if err := s.db.QueryRow(dedupQuery, dedupArgs...).Scan(&out.DedupReview); err != nil {
		return out, err
	}

	cutoff := now.UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)
	meetingQuery := `SELECT COUNT(DISTINCT a.id)
FROM artifacts a
WHERE datetime(a.created_at) >= datetime(?)
  AND (
    lower(trim(a.kind)) = 'transcript'
    OR (a.meta_json IS NOT NULL AND a.meta_json LIKE '%"source":"meeting_summary"%')
    OR (a.meta_json IS NOT NULL AND a.meta_json LIKE '%"source":"meeting_notes"%')
  )`
	if err := s.db.QueryRow(meetingQuery, cutoff).Scan(&out.RecentMeetings); err != nil {
		return out, err
	}
	return out, nil
}

func (s *Store) ListItems() ([]Item, error) {
	return s.ListItemsFiltered(ItemListFilter{})
}

func (s *Store) ListItemsFiltered(filter ItemListFilter) ([]Item, error) {
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	query := `SELECT id, title, kind, state, workspace_id, ` + scopedContextSelect("context_items", "item_id", "items.id") + ` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items`
	args := []any{}
	parts := []string{}
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "")
	if len(parts) > 0 {
		query += ` WHERE ` + stringsJoin(parts, ` AND `)
	}
	rows, err := s.db.Query(
		query,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		item, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt == out[j].UpdatedAt {
			return out[i].ID < out[j].ID
		}
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, nil
}
