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
	query := `SELECT id, title, kind, state, track, workspace_id, ` + scopedContextSelect("context_items", "item_id", "items.id") + ` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
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
	i.track,
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
 actors.name,
 (
   SELECT links.parent_item_id
   FROM item_children links
   JOIN items parent ON parent.id = links.parent_item_id
   WHERE links.child_item_id = i.id
   ORDER BY CASE links.role WHEN 'next_action' THEN 0 WHEN 'support' THEN 1 ELSE 2 END,
            datetime(links.created_at) ASC,
            links.parent_item_id ASC
   LIMIT 1
 ),
 (
   SELECT parent.title
   FROM item_children links
   JOIN items parent ON parent.id = links.parent_item_id
   WHERE links.child_item_id = i.id
   ORDER BY CASE links.role WHEN 'next_action' THEN 0 WHEN 'support' THEN 1 ELSE 2 END,
            datetime(links.created_at) ASC,
            links.parent_item_id ASC
   LIMIT 1
 )
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

func appendVisibleNowClauses(parts []string, args []any, column itemFilterColumnFunc, now time.Time) ([]string, []any) {
	cutoff := now.UTC().Format(time.RFC3339Nano)
	parts = append(parts,
		visibleNowClause(column("visible_after")),
		visibleNowClause(column("follow_up_at")),
	)
	return parts, append(args, cutoff, cutoff)
}

func visibleNowClause(column string) string {
	return `(
	     ` + column + ` IS NULL
	     OR trim(` + column + `) = ''
	     OR datetime(` + column + `) <= datetime(?)
	   )`
}

func dueAtBeforeClause(column string) string {
	return `(
	     ` + column + ` IS NOT NULL
	     AND trim(` + column + `) <> ''
	     AND datetime(` + column + `) <= datetime(?)
	   )`
}

func activeNowOrOverdueClause(column itemFilterColumnFunc) string {
	return `(
	     ` + dueAtBeforeClause(column("due_at")) + `
	     OR (
	       ` + visibleNowClause(column("visible_after")) + `
	       AND ` + visibleNowClause(column("follow_up_at")) + `
	     )
	   )`
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
	cutoff := time.Now().UTC().Format(time.RFC3339Nano)
	column := func(name string) string { return "i." + name }
	parts := []string{
		`(
		  (i.state = ? AND ` + activeNowOrOverdueClause(column) + `)
		  OR (i.state = ? AND ` + dueAtBeforeClause("i.due_at") + `)
		)`,
	}
	args := []any{ItemStateNext, cutoff, cutoff, cutoff, ItemStateDeferred, cutoff}
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
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	column := func(name string) string { return "i." + name }
	reviewClause, reviewArgs := reviewQueueClause(time.Now().UTC(), column, column)
	parts := []string{reviewClause}
	args := append([]any{}, reviewArgs...)
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "i.")
	query := itemSummarySelect + ` WHERE ` + stringsJoin(parts, ` AND `) + ` ORDER BY i.updated_at DESC, i.id ASC`
	return s.listItemSummaries(query, args...)
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
	column := func(name string) string { return name }
	outerColumn := func(name string) string { return "items." + name }
	reviewClause, reviewArgs := reviewQueueClause(now, column, outerColumn)
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
  COALESCE(SUM(CASE
    WHEN (
      (state = ? AND ` + activeNowOrOverdueClause(column) + `)
      OR (state = ? AND ` + dueAtBeforeClause("due_at") + `)
    )
    THEN 1 ELSE 0 END), 0) AS next_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS waiting_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS deferred_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS someday_count,
  COALESCE(SUM(CASE WHEN ` + reviewClause + ` THEN 1 ELSE 0 END), 0) AS review_count,
  COALESCE(SUM(CASE WHEN state = ? THEN 1 ELSE 0 END), 0) AS done_count
FROM items
`
	args := []any{ItemStateInbox, cutoff, ItemStateNext, cutoff, cutoff, cutoff, ItemStateDeferred, cutoff, ItemStateWaiting, ItemStateDeferred, ItemStateSomeday}
	args = append(args, reviewArgs...)
	args = append(args, ItemStateDone)
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

func reviewQueueClause(now time.Time, column, outerColumn itemFilterColumnFunc) (string, []any) {
	cutoff := now.UTC().Format(time.RFC3339Nano)
	clause := `(` + column("state") + ` = ?
OR (` + column("state") + ` <> ?
  AND ` + dueAtBeforeClause(column("due_at")) + `)
OR (` + column("state") + ` = ?
  AND ` + column("follow_up_at") + ` IS NOT NULL AND trim(` + column("follow_up_at") + `) <> ''
  AND datetime(` + column("follow_up_at") + `) <= datetime(?))
OR (` + column("kind") + ` = ?
  AND ` + column("state") + ` <> ?
  AND NOT EXISTS (
    SELECT 1 FROM item_children links JOIN items child ON child.id = links.child_item_id
    WHERE links.parent_item_id = ` + outerColumn("id") + `
      AND child.state IN (?, ?, ?, ?)
  )))`
	args := []any{ItemStateReview, ItemStateDone, cutoff, ItemStateWaiting, cutoff, ItemKindProject, ItemStateDone}
	args = append(args, ItemStateNext, ItemStateWaiting, ItemStateDeferred, ItemStateSomeday)
	return clause, args
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
// item (people we owe or await), unresolved external-binding drift, open dedup
// candidate groups awaiting a user decision (dedup review backlog), and
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

	cutoff := now.UTC().Format(time.RFC3339Nano)
	soon := now.UTC().AddDate(0, 0, 7).Format(time.RFC3339Nano)
	projectParts := []string{
		"items.kind = ?",
		"items.state <> ?",
		`(
		  ` + dueAtBeforeClause("items.due_at") + `
		  OR EXISTS (
		    SELECT 1
		    FROM item_children links
		    JOIN items child ON child.id = links.child_item_id
		    WHERE links.parent_item_id = items.id
		      AND child.state <> ?
		      AND (
		        ` + dueAtBeforeClause("child.due_at") + `
		        OR (child.state IN (?, ?) AND ` + activeNowOrOverdueClause(func(name string) string { return "child." + name }) + `)
		      )
		  )
		)`,
	}
	projectArgs := []any{
		ItemKindProject,
		ItemStateDone,
		soon,
		ItemStateDone,
		soon,
		ItemStateInbox,
		ItemStateNext,
		cutoff,
		cutoff,
		cutoff,
	}
	projectParts, projectArgs = appendItemFilterClauses(projectParts, projectArgs, normalizedFilter, "")
	projectQuery := `SELECT COUNT(*) FROM items WHERE ` + stringsJoin(projectParts, ` AND `)
	if err := s.db.QueryRow(projectQuery, projectArgs...).Scan(&out.ProjectItemsOpen); err != nil {
		return out, err
	}

	peopleParts := []string{"items.actor_id IS NOT NULL", "items.kind = ?", "items.state IN (?, ?, ?, ?)", "actors.kind = ?"}
	peopleArgs := []any{ItemKindAction, ItemStateWaiting, ItemStateDeferred, ItemStateNext, ItemStateInbox, ActorKindHuman}
	peopleParts, peopleArgs = appendItemFilterClauses(peopleParts, peopleArgs, normalizedFilter, "")
	peopleQuery := `SELECT COUNT(DISTINCT items.actor_id) FROM items JOIN actors ON actors.id = items.actor_id WHERE ` + stringsJoin(peopleParts, ` AND `)
	if err := s.db.QueryRow(peopleQuery, peopleArgs...).Scan(&out.PeopleOpen); err != nil {
		return out, err
	}

	driftFilter := normalizedFilter
	driftFilter.Section = ""
	if out.DriftReview, err = s.CountUnresolvedExternalBindingDrifts(driftFilter); err != nil {
		return out, err
	}

	dedupFilter := normalizedFilter
	dedupFilter.Section = ""
	if out.DedupReview, err = s.CountItemDedupCandidatesFiltered(dedupFilter); err != nil {
		return out, err
	}

	meetingCutoff := now.UTC().Add(-7 * 24 * time.Hour).Format(time.RFC3339Nano)
	meetingQuery := `SELECT COUNT(DISTINCT a.id)
FROM artifacts a
WHERE datetime(a.created_at) >= datetime(?)
  AND (
    lower(trim(a.kind)) = 'transcript'
    OR (a.meta_json IS NOT NULL AND a.meta_json LIKE '%"source":"meeting_summary"%')
    OR (a.meta_json IS NOT NULL AND a.meta_json LIKE '%"source":"meeting_notes"%')
  )`
	if err := s.db.QueryRow(meetingQuery, meetingCutoff).Scan(&out.RecentMeetings); err != nil {
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
	query := `SELECT id, title, kind, state, track, workspace_id, ` + scopedContextSelect("context_items", "item_id", "items.id") + ` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
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
