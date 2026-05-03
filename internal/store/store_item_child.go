package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

const itemChildrenTableSchema = `CREATE TABLE IF NOT EXISTS item_children (
  parent_item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  child_item_id INTEGER NOT NULL REFERENCES items(id) ON DELETE CASCADE,
  role TEXT NOT NULL DEFAULT 'next_action' CHECK (role IN ('next_action', 'support', 'blocked_by')),
  created_at TEXT NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (parent_item_id, child_item_id)
);
CREATE INDEX IF NOT EXISTS idx_item_children_child_item_id
  ON item_children(child_item_id);`

func (s *Store) migrateItemChildLinkSupport() error {
	_, err := s.db.Exec(itemChildrenTableSchema)
	return err
}

func (s *Store) LinkItemChild(parentItemID, childItemID int64, role string) error {
	cleanRole := normalizeItemLinkRole(role)
	if cleanRole == "" {
		return errors.New("item child role must be next_action, support, or blocked_by")
	}
	if parentItemID <= 0 || childItemID <= 0 {
		return errors.New("parent_item_id and child_item_id must be positive integers")
	}
	if parentItemID == childItemID {
		return errors.New("parent_item_id and child_item_id must differ")
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	parent, err := scanItem(tx.QueryRow(
		`SELECT id, title, kind, state, workspace_id, `+scopedContextSelect("context_items", "item_id", "items.id")+` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items
		 WHERE id = ?`,
		parentItemID,
	))
	if err != nil {
		return err
	}
	if parent.Kind != ItemKindProject {
		return errors.New("parent item must be a project")
	}
	if _, err := scanItem(tx.QueryRow(
		`SELECT id, title, kind, state, workspace_id, `+scopedContextSelect("context_items", "item_id", "items.id")+` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items
		 WHERE id = ?`,
		childItemID,
	)); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`INSERT INTO item_children (parent_item_id, child_item_id, role)
		 VALUES (?, ?, ?)
		 ON CONFLICT(parent_item_id, child_item_id) DO UPDATE SET role = excluded.role`,
		parentItemID,
		childItemID,
		cleanRole,
	); err != nil {
		return err
	}
	if err := s.touchItemTx(tx, parentItemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UnlinkItemChild(parentItemID, childItemID int64) error {
	if parentItemID <= 0 || childItemID <= 0 {
		return errors.New("parent_item_id and child_item_id must be positive integers")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	item, err := scanItem(tx.QueryRow(
		`SELECT id, title, kind, state, workspace_id, `+scopedContextSelect("context_items", "item_id", "items.id")+` AS sphere, artifact_id, actor_id, visible_after, follow_up_at, due_at, source, source_ref, review_target, reviewer, reviewed_at, created_at, updated_at
		 FROM items
		 WHERE id = ?`,
		parentItemID,
	))
	if err != nil {
		return err
	}
	if item.Kind != ItemKindProject {
		return errors.New("parent item must be a project")
	}
	res, err := tx.Exec(
		`DELETE FROM item_children
		 WHERE parent_item_id = ? AND child_item_id = ?`,
		parentItemID,
		childItemID,
	)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	if err := s.touchItemTx(tx, parentItemID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListItemChildLinks(parentItemID int64) ([]ItemChildLink, error) {
	item, err := s.GetItem(parentItemID)
	if err != nil {
		return nil, err
	}
	if item.Kind != ItemKindProject {
		return nil, errors.New("item is not a project")
	}
	rows, err := s.db.Query(
		`SELECT parent_item_id, child_item_id, role, created_at
		 FROM item_children
		 WHERE parent_item_id = ?
		 ORDER BY CASE role WHEN 'next_action' THEN 0 WHEN 'support' THEN 1 ELSE 2 END,
		          datetime(created_at) ASC,
		          child_item_id ASC`,
		parentItemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []ItemChildLink{}
	for rows.Next() {
		var link ItemChildLink
		if err := rows.Scan(&link.ParentItemID, &link.ChildItemID, &link.Role, &link.CreatedAt); err != nil {
			return nil, err
		}
		link.Role = normalizeItemLinkRole(link.Role)
		out = append(out, link)
	}
	return out, rows.Err()
}

// ListProjectItemReviewsFiltered returns the active GTD project-item review
// surface: every Item(kind=project) that is not done, paired with its current
// health and per-state child counts. The list backs the weekly outcome review
// and surfaces stalled outcomes without inventing tasks.
//
// The filter respects sphere/workspace/source/source-container/label/actor
// scoping just like the other GTD list endpoints. Source containers (Todoist
// projects, GitHub Projects) match through the existing `source_container`
// filter — they are never promoted into the review as project items
// themselves. Workspace filtering scopes the project items to a single
// workspace; project items are never converted into workspaces by this query.
//
// Stalled project items sort first; healthy items follow in updated_at desc
// order, so weekly review walks the riskiest outcomes before the rest.
func (s *Store) ListProjectItemReviewsFiltered(filter ItemListFilter) ([]ProjectItemReview, error) {
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	parts := []string{"i.kind = ?", "i.state <> ?"}
	args := []any{ItemKindProject, ItemStateDone}
	parts, args = appendItemFilterClauses(parts, args, normalizedFilter, "i.")
	query := itemSummarySelect + ` WHERE ` + stringsJoin(parts, ` AND `) + ` ORDER BY i.updated_at DESC, i.id ASC`
	items, err := s.listItemSummaries(query, args...)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return []ProjectItemReview{}, nil
	}
	statsByParent, err := s.collectProjectChildStats(items)
	if err != nil {
		return nil, err
	}
	reviews := make([]ProjectItemReview, 0, len(items))
	for _, item := range items {
		stats := statsByParent[item.ID]
		reviews = append(reviews, ProjectItemReview{
			Item:       item,
			Children:   stats.Counts,
			Health:     projectHealthFromCounts(stats.Counts),
			NextAction: stats.NextAction,
			Deadline:   stats.Deadline,
		})
	}
	sortProjectItemReviewsForWeeklyReview(reviews)
	return reviews, nil
}

type projectChildStats struct {
	Counts     ProjectChildCounts
	NextAction *ProjectNextAction
	Deadline   ProjectDeadlinePressure
}

// collectProjectChildStats loads child-state tallies, the first active next
// action, and hard-deadline pressure for every project item in one round-trip.
func (s *Store) collectProjectChildStats(parents []ItemSummary) (map[int64]projectChildStats, error) {
	out := make(map[int64]projectChildStats, len(parents))
	if len(parents) == 0 {
		return out, nil
	}
	placeholders := make([]string, 0, len(parents))
	args := make([]any, 0, len(parents))
	for _, parent := range parents {
		placeholders = append(placeholders, "?")
		args = append(args, parent.ID)
		out[parent.ID] = projectChildStats{}
	}
	now := time.Now().UTC()
	rows, err := s.db.Query(
		`SELECT links.parent_item_id,
		        child.id,
		        child.title,
		        child.state,
		        child.visible_after,
		        child.follow_up_at,
		        child.due_at
		 FROM item_children links
		 JOIN items child ON child.id = links.child_item_id
		 WHERE links.parent_item_id IN (`+stringsJoin(placeholders, ",")+`)
		 ORDER BY links.parent_item_id ASC,
		          CASE WHEN child.due_at IS NULL OR trim(child.due_at) = '' THEN 1 ELSE 0 END ASC,
		          datetime(child.due_at) ASC,
		          datetime(links.created_at) ASC,
		          child.id ASC`,
		args...,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			parentID, childID               int64
			title, state                    string
			visibleAfter, followUpAt, dueAt sql.NullString
		)
		if err := rows.Scan(&parentID, &childID, &title, &state, &visibleAfter, &followUpAt, &dueAt); err != nil {
			return nil, err
		}
		stats := out[parentID]
		started := !itemStartsInFuture(now, visibleAfter, followUpAt, dueAt)
		stats.Counts = applyChildItemCount(stats.Counts, state, started)
		if normalizeItemState(state) != ItemStateDone {
			stats.Deadline = applyProjectDeadlinePressure(stats.Deadline, dueAt, now)
		}
		if normalizeItemState(state) == ItemStateNext && started && stats.NextAction == nil {
			stats.NextAction = &ProjectNextAction{
				ID:    childID,
				Title: strings.TrimSpace(title),
				DueAt: nullStringPointer(dueAt),
			}
		}
		out[parentID] = stats
	}
	return out, rows.Err()
}

func applyChildItemCount(counts ProjectChildCounts, state string, started bool) ProjectChildCounts {
	switch normalizeItemState(state) {
	case ItemStateInbox:
		if started {
			counts.Inbox++
		} else {
			counts.NotStarted++
		}
	case ItemStateNext:
		if started {
			counts.Next++
		} else {
			counts.NotStarted++
		}
	case ItemStateWaiting:
		counts.Waiting++
	case ItemStateDeferred:
		counts.Deferred++
	case ItemStateSomeday:
		counts.Someday++
	case ItemStateReview:
		counts.Review++
	case ItemStateDone:
		counts.Done++
	}
	counts.Total++
	return counts
}

func itemStartsInFuture(now time.Time, visibleAfter, followUpAt, dueAt sql.NullString) bool {
	if timestampIsBeforeOrEqual(dueAt, now) {
		return false
	}
	return timestampIsAfter(visibleAfter, now) || timestampIsAfter(followUpAt, now)
}

func timestampIsAfter(raw sql.NullString, now time.Time) bool {
	ts, ok := parseOptionalItemTime(raw)
	return ok && ts.After(now)
}

func timestampIsBeforeOrEqual(raw sql.NullString, now time.Time) bool {
	ts, ok := parseOptionalItemTime(raw)
	return ok && !ts.After(now)
}

func parseOptionalItemTime(raw sql.NullString) (time.Time, bool) {
	if !raw.Valid {
		return time.Time{}, false
	}
	value := strings.TrimSpace(raw.String)
	if value == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts.UTC(), true
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts.UTC(), true
	}
	return time.Time{}, false
}

func applyProjectDeadlinePressure(pressure ProjectDeadlinePressure, raw sql.NullString, now time.Time) ProjectDeadlinePressure {
	dueAt, ok := parseOptionalItemTime(raw)
	if !ok {
		return pressure
	}
	dueText := strings.TrimSpace(raw.String)
	if pressure.NextDueAt == nil || dueAt.Before(mustParseProjectDue(*pressure.NextDueAt)) {
		pressure.NextDueAt = &dueText
	}
	if dueAt.Before(now) {
		pressure.Overdue++
		return pressure
	}
	if sameUTCDate(dueAt, now) {
		pressure.DueToday++
		return pressure
	}
	if !dueAt.After(now.AddDate(0, 0, 7)) {
		pressure.DueThisWeek++
	}
	return pressure
}

func mustParseProjectDue(raw string) time.Time {
	ts, ok := parseOptionalItemTime(sql.NullString{String: raw, Valid: true})
	if !ok {
		return time.Time{}
	}
	return ts
}

func sameUTCDate(left, right time.Time) bool {
	ly, lm, ld := left.UTC().Date()
	ry, rm, rd := right.UTC().Date()
	return ly == ry && lm == rm && ld == rd
}

func projectHealthFromCounts(counts ProjectChildCounts) ProjectItemHealth {
	health := ProjectItemHealth{
		HasNextAction: counts.Next > 0,
		HasWaiting:    counts.Waiting > 0,
		HasDeferred:   counts.Deferred > 0,
		HasSomeday:    counts.Someday > 0,
	}
	health.Stalled = !health.HasNextAction && !health.HasWaiting && !health.HasDeferred && !health.HasSomeday
	return health
}

func sortProjectItemReviewsForWeeklyReview(reviews []ProjectItemReview) {
	sort.SliceStable(reviews, func(i, j int) bool {
		if deadlineRank(reviews[i].Deadline) != deadlineRank(reviews[j].Deadline) {
			return deadlineRank(reviews[i].Deadline) < deadlineRank(reviews[j].Deadline)
		}
		if reviews[i].Health.Stalled != reviews[j].Health.Stalled {
			return reviews[i].Health.Stalled
		}
		if reviews[i].Item.UpdatedAt != reviews[j].Item.UpdatedAt {
			return reviews[i].Item.UpdatedAt > reviews[j].Item.UpdatedAt
		}
		return reviews[i].Item.ID < reviews[j].Item.ID
	})
}

func deadlineRank(deadline ProjectDeadlinePressure) int {
	switch {
	case deadline.Overdue > 0:
		return 0
	case deadline.DueToday > 0:
		return 1
	case deadline.DueThisWeek > 0:
		return 2
	default:
		return 3
	}
}

func (s *Store) ListPersonOpenLoopDashboardsFiltered(filter ItemListFilter) ([]PersonOpenLoopDashboard, error) {
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return nil, err
	}
	items, err := s.listPersonOpenLoopItems(normalizedFilter, nil)
	if err != nil {
		return nil, err
	}
	actors, err := s.actorsByID()
	if err != nil {
		return nil, err
	}
	out := dashboardsFromPersonItems(items, actors)
	sortPersonOpenLoopDashboards(out)
	return out, nil
}

func (s *Store) GetPersonOpenLoopDashboardFiltered(actorID int64, filter ItemListFilter) (PersonOpenLoopDashboard, error) {
	if actorID <= 0 {
		return PersonOpenLoopDashboard{}, errors.New("actor_id must be a positive integer")
	}
	normalizedFilter, err := s.prepareItemListFilter(filter)
	if err != nil {
		return PersonOpenLoopDashboard{}, err
	}
	items, err := s.listPersonOpenLoopItems(normalizedFilter, &actorID)
	if err != nil {
		return PersonOpenLoopDashboard{}, err
	}
	actor, err := s.GetActor(actorID)
	if err != nil {
		return PersonOpenLoopDashboard{}, err
	}
	dashboard := newPersonOpenLoopDashboard(actor)
	addPersonOpenLoopItems(&dashboard, items)
	dashboard.ProjectItems, err = s.listLinkedProjectItemsForPerson(items, normalizedFilter)
	if err != nil {
		return PersonOpenLoopDashboard{}, err
	}
	return dashboard, nil
}

func (s *Store) listPersonOpenLoopItems(filter ItemListFilter, actorID *int64) ([]ItemSummary, error) {
	scoped := filter
	scoped.Section = ""
	scoped.ActorID = nil
	parts := []string{"i.actor_id IS NOT NULL", "i.kind = ?", "i.state IN (?, ?, ?, ?, ?)"}
	args := []any{ItemKindAction, ItemStateWaiting, ItemStateDeferred, ItemStateNext, ItemStateInbox, ItemStateDone}
	if actorID != nil {
		parts = append(parts, "i.actor_id = ?")
		args = append(args, *actorID)
	}
	parts, args = appendItemFilterClauses(parts, args, scoped, "i.")
	query := itemSummarySelect + ` WHERE ` + stringsJoin(parts, ` AND `) + ` ORDER BY i.updated_at DESC, i.id ASC`
	return s.listItemSummaries(query, args...)
}

func (s *Store) actorsByID() (map[int64]Actor, error) {
	actors, err := s.ListActors()
	if err != nil {
		return nil, err
	}
	out := make(map[int64]Actor, len(actors))
	for _, actor := range actors {
		out[actor.ID] = actor
	}
	return out, nil
}

func dashboardsFromPersonItems(items []ItemSummary, actors map[int64]Actor) []PersonOpenLoopDashboard {
	byActorID := map[int64]*PersonOpenLoopDashboard{}
	for _, item := range items {
		if item.ActorID == nil {
			continue
		}
		actor, ok := actors[*item.ActorID]
		if !ok || actor.Kind != ActorKindHuman {
			continue
		}
		dashboard := byActorID[actor.ID]
		if dashboard == nil {
			next := newPersonOpenLoopDashboard(actor)
			dashboard = &next
			byActorID[actor.ID] = dashboard
		}
		addPersonOpenLoopItem(dashboard, item)
	}
	out := make([]PersonOpenLoopDashboard, 0, len(byActorID))
	for _, dashboard := range byActorID {
		if dashboard.Counts.Open > 0 {
			out = append(out, *dashboard)
		}
	}
	return out
}

func newPersonOpenLoopDashboard(actor Actor) PersonOpenLoopDashboard {
	personPath, diagnostics := personDashboardMeta(actor)
	return PersonOpenLoopDashboard{
		Actor:       actor,
		Person:      actor.Name,
		PersonPath:  personPath,
		Diagnostics: diagnostics,
	}
}

func addPersonOpenLoopItems(dashboard *PersonOpenLoopDashboard, items []ItemSummary) {
	for _, item := range items {
		addPersonOpenLoopItem(dashboard, item)
	}
}

func addPersonOpenLoopItem(dashboard *PersonOpenLoopDashboard, item ItemSummary) {
	switch item.State {
	case ItemStateWaiting, ItemStateDeferred:
		dashboard.WaitingOnThem = append(dashboard.WaitingOnThem, item)
		dashboard.Counts.WaitingOnThem++
		dashboard.Counts.Open++
	case ItemStateNext, ItemStateInbox:
		dashboard.IOweThem = append(dashboard.IOweThem, item)
		dashboard.Counts.IOweThem++
		dashboard.Counts.Open++
	case ItemStateDone:
		dashboard.RecentlyClosed = append(dashboard.RecentlyClosed, item)
		dashboard.Counts.RecentlyClosed++
	}
}

func personDashboardMeta(actor Actor) (*string, []string) {
	meta := map[string]any{}
	if actor.MetaJSON != nil && strings.TrimSpace(*actor.MetaJSON) != "" {
		_ = json.Unmarshal([]byte(*actor.MetaJSON), &meta)
	}
	personPath := firstMetaString(meta, "person_path", "brain_person_path")
	diagnostics := metaDiagnostics(meta, actor.Name)
	if personPath == nil && actor.Kind == ActorKindHuman && !hasNeedsPersonNoteDiagnostic(diagnostics) {
		diagnostics = append(diagnostics, "needs_person_note: "+actor.Name)
	}
	return personPath, diagnostics
}

func firstMetaString(meta map[string]any, keys ...string) *string {
	for _, key := range keys {
		value := strings.TrimSpace(stringFromMeta(meta[key]))
		if value != "" {
			return &value
		}
	}
	return nil
}

func metaDiagnostics(meta map[string]any, fallbackName string) []string {
	out := []string{}
	if raw, ok := meta["diagnostics"].([]any); ok {
		for _, entry := range raw {
			if text := strings.TrimSpace(stringFromMeta(entry)); text != "" {
				out = append(out, text)
			}
		}
	}
	if text := strings.TrimSpace(stringFromMeta(meta["diagnostic"])); text != "" {
		out = append(out, text)
	}
	if needs, ok := meta["needs_person_note"].(bool); ok && needs && !hasNeedsPersonNoteDiagnostic(out) {
		name := strings.TrimSpace(stringFromMeta(meta["name"]))
		if name == "" {
			name = strings.TrimSpace(stringFromMeta(meta["person"]))
		}
		if name == "" {
			name = strings.TrimSpace(fallbackName)
		}
		out = append(out, "needs_person_note: "+name)
	}
	return out
}

func stringFromMeta(value any) string {
	text, _ := value.(string)
	return text
}

func hasNeedsPersonNoteDiagnostic(diagnostics []string) bool {
	for _, diagnostic := range diagnostics {
		if strings.HasPrefix(strings.TrimSpace(diagnostic), "needs_person_note:") {
			return true
		}
	}
	return false
}

func sortPersonOpenLoopDashboards(dashboards []PersonOpenLoopDashboard) {
	sort.Slice(dashboards, func(i, j int) bool {
		left, right := dashboards[i], dashboards[j]
		if left.Counts.Open != right.Counts.Open {
			return left.Counts.Open > right.Counts.Open
		}
		return strings.ToLower(left.Person) < strings.ToLower(right.Person)
	})
}

func (s *Store) listLinkedProjectItemsForPerson(items []ItemSummary, filter ItemListFilter) ([]ItemSummary, error) {
	childIDs := itemSummaryIDs(items)
	if len(childIDs) == 0 {
		return []ItemSummary{}, nil
	}
	scoped := filter
	scoped.Section = ""
	scoped.ActorID = nil
	scoped.ProjectItemID = nil
	placeholders := make([]string, 0, len(childIDs))
	args := []any{ItemKindProject}
	for _, id := range childIDs {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	parts := []string{"i.kind = ?", `EXISTS (
SELECT 1 FROM item_children link
WHERE link.parent_item_id = i.id
  AND link.child_item_id IN (` + stringsJoin(placeholders, ",") + `)
)`}
	parts, args = appendItemFilterClauses(parts, args, scoped, "i.")
	query := itemSummarySelect + ` WHERE ` + stringsJoin(parts, ` AND `)
	return s.listItemSummaries(query, args...)
}

func itemSummaryIDs(items []ItemSummary) []int64 {
	out := make([]int64, 0, len(items))
	for _, item := range items {
		if item.ID > 0 {
			out = append(out, item.ID)
		}
	}
	return out
}

// ListItemParentLinks returns every project-item link that names this item as
// child. Used by the gesture endpoint so completing a child action surfaces
// refreshed project-item health for every owning outcome without forcing the
// frontend to query each parent separately.
func (s *Store) ListItemParentLinks(childItemID int64) ([]ItemChildLink, error) {
	if childItemID <= 0 {
		return nil, errors.New("child_item_id must be a positive integer")
	}
	rows, err := s.db.Query(
		`SELECT parent_item_id, child_item_id, role, created_at
		 FROM item_children
		 WHERE child_item_id = ?
		 ORDER BY parent_item_id ASC`,
		childItemID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ItemChildLink{}
	for rows.Next() {
		var link ItemChildLink
		if err := rows.Scan(&link.ParentItemID, &link.ChildItemID, &link.Role, &link.CreatedAt); err != nil {
			return nil, err
		}
		link.Role = normalizeItemLinkRole(link.Role)
		out = append(out, link)
	}
	return out, rows.Err()
}

func (s *Store) GetProjectItemHealth(itemID int64) (ProjectItemHealth, error) {
	review, err := s.GetProjectItemReview(itemID)
	if err != nil {
		return ProjectItemHealth{}, err
	}
	return review.Health, nil
}

// GetProjectItemReview returns the per-state child tallies and derived health
// for a single project item in one query. Callers that render counts and
// health together (gesture responses, UI rows) avoid the second round-trip
// they would otherwise pay if they called health and counts separately.
func (s *Store) GetProjectItemReview(itemID int64) (ProjectItemReview, error) {
	item, err := s.GetItem(itemID)
	if err != nil {
		return ProjectItemReview{}, err
	}
	if item.Kind != ItemKindProject {
		return ProjectItemReview{}, errors.New("item is not a project")
	}
	summary := ItemSummary{Item: item}
	statsByParent, err := s.collectProjectChildStats([]ItemSummary{summary})
	if err != nil {
		return ProjectItemReview{}, err
	}
	stats := statsByParent[itemID]
	return ProjectItemReview{
		Item:       summary,
		Children:   stats.Counts,
		Health:     projectHealthFromCounts(stats.Counts),
		NextAction: stats.NextAction,
		Deadline:   stats.Deadline,
	}, nil
}

func (s *Store) touchItem(id int64) error {
	res, err := s.db.Exec(`UPDATE items SET updated_at = datetime('now') WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}
