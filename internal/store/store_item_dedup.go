package store

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
)

func normalizeItemDedupState(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", ItemDedupStateOpen:
		return ItemDedupStateOpen
	case ItemDedupStateReviewLater:
		return ItemDedupStateReviewLater
	case ItemDedupStateKeepSeparate:
		return ItemDedupStateKeepSeparate
	case ItemDedupStateMerged:
		return ItemDedupStateMerged
	default:
		return ""
	}
}

func normalizeItemDedupAction(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case ItemDedupActionMerge:
		return ItemDedupActionMerge
	case ItemDedupActionKeepSeparate:
		return ItemDedupActionKeepSeparate
	case ItemDedupActionReviewLater:
		return ItemDedupActionReviewLater
	default:
		return ""
	}
}

func (s *Store) CreateItemDedupCandidate(opts ItemDedupCandidateOptions) (ItemDedupCandidateGroup, error) {
	kind, items, err := s.validateItemDedupCandidate(opts)
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	defer tx.Rollback()
	res, err := tx.Exec(`INSERT INTO item_dedup_candidates (kind, score, confidence, outcome, reasoning, detector) VALUES (?, ?, ?, ?, ?, ?)`,
		kind, opts.Score, opts.Confidence, strings.TrimSpace(opts.Outcome), strings.TrimSpace(opts.Reasoning), strings.TrimSpace(opts.Detector))
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	if err := insertItemDedupCandidateItems(tx, id, items); err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	return s.GetItemDedupCandidate(id)
}

func (s *Store) FindItemDedupCandidateByItems(kind string, itemIDs []int64) (ItemDedupCandidateGroup, error) {
	cleanKind := normalizeItemKind(kind)
	if cleanKind == "" {
		return ItemDedupCandidateGroup{}, errors.New("dedup candidate kind must be action or project")
	}
	ids := uniqueSortedPositiveIDs(itemIDs)
	if len(ids) < 2 {
		return ItemDedupCandidateGroup{}, errors.New("dedup candidate needs at least two distinct items")
	}
	candidateID, err := s.findItemDedupCandidateIDByItems(cleanKind, ids)
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	return s.GetItemDedupCandidate(candidateID)
}

func uniqueSortedPositiveIDs(ids []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (s *Store) findItemDedupCandidateIDByItems(kind string, itemIDs []int64) (int64, error) {
	rows, err := s.db.Query(`SELECT c.id, ci.item_id
FROM item_dedup_candidates c
JOIN item_dedup_candidate_items ci ON ci.candidate_id = c.id
WHERE c.kind = ?
ORDER BY c.id, ci.item_id`, kind)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if id, ok, err := scanMatchingItemDedupCandidateID(rows, itemIDs); err != nil || ok {
		return id, err
	}
	return 0, sql.ErrNoRows
}

func scanMatchingItemDedupCandidateID(rows *sql.Rows, itemIDs []int64) (int64, bool, error) {
	var currentID int64
	var currentItems []int64
	flush := func() (int64, bool) {
		if currentID > 0 && equalInt64Slices(currentItems, itemIDs) {
			return currentID, true
		}
		return 0, false
	}
	for rows.Next() {
		var candidateID, itemID int64
		if err := rows.Scan(&candidateID, &itemID); err != nil {
			return 0, false, err
		}
		if currentID != 0 && candidateID != currentID {
			if id, ok := flush(); ok {
				return id, true, nil
			}
			currentItems = currentItems[:0]
		}
		currentID = candidateID
		currentItems = append(currentItems, itemID)
	}
	if err := rows.Err(); err != nil {
		return 0, false, err
	}
	id, ok := flush()
	return id, ok, nil
}

func equalInt64Slices(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (s *Store) validateItemDedupCandidate(opts ItemDedupCandidateOptions) (string, []ItemDedupCandidateItemInput, error) {
	if len(opts.Items) < 2 {
		return "", nil, errors.New("dedup candidate needs at least two items")
	}
	kind := normalizeItemKind(opts.Kind)
	if kind == "" {
		return "", nil, errors.New("dedup candidate kind must be action or project")
	}
	items := make([]ItemDedupCandidateItemInput, 0, len(opts.Items))
	seen := map[int64]bool{}
	for _, entry := range opts.Items {
		item, err := s.GetItem(entry.ItemID)
		if err != nil {
			return "", nil, err
		}
		if item.Kind != kind {
			return "", nil, errors.New("dedup candidate cannot mix action and project items")
		}
		if seen[item.ID] {
			continue
		}
		seen[item.ID] = true
		items = append(items, ItemDedupCandidateItemInput{ItemID: item.ID, Outcome: strings.TrimSpace(entry.Outcome)})
	}
	if len(items) < 2 {
		return "", nil, errors.New("dedup candidate needs at least two distinct items")
	}
	return kind, items, nil
}

func insertItemDedupCandidateItems(tx *sql.Tx, candidateID int64, items []ItemDedupCandidateItemInput) error {
	for pos, item := range items {
		if _, err := tx.Exec(`INSERT INTO item_dedup_candidate_items (candidate_id, item_id, position, outcome) VALUES (?, ?, ?, ?)`,
			candidateID, item.ItemID, pos, strings.TrimSpace(item.Outcome)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetItemDedupCandidate(id int64) (ItemDedupCandidateGroup, error) {
	group, err := scanItemDedupCandidate(s.db.QueryRow(itemDedupCandidateSelect()+` WHERE id = ?`, id))
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	group.Items, err = s.listItemDedupCandidateMembers(group.ID)
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	return group, nil
}

func (s *Store) ListItemDedupCandidatesFiltered(kind string, filter ItemListFilter) ([]ItemDedupCandidateGroup, error) {
	query, args, err := s.itemDedupCandidateQuery(kind, filter, false)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	return s.scanItemDedupCandidateRows(rows)
}

func (s *Store) CountItemDedupCandidatesFiltered(filter ItemListFilter) (int, error) {
	query, args, err := s.itemDedupCandidateQuery("", filter, true)
	if err != nil {
		return 0, err
	}
	var count int
	if err := s.db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) itemDedupCandidateQuery(kind string, filter ItemListFilter, count bool) (string, []any, error) {
	cleanKind := ""
	if strings.TrimSpace(kind) != "" {
		cleanKind = normalizeItemKind(kind)
		if cleanKind == "" {
			return "", nil, errors.New("dedup kind must be action or project")
		}
	}
	filter.Section = ""
	existsClause, args, err := s.itemDedupCandidateFilterExists(filter)
	if err != nil {
		return "", nil, err
	}
	parts := []string{"state IN (?, ?)", existsClause}
	args = append([]any{ItemDedupStateOpen, ItemDedupStateReviewLater}, args...)
	if cleanKind != "" {
		parts = append(parts, "kind = ?")
		args = append(args, cleanKind)
	}
	if count {
		return `SELECT COUNT(*) FROM item_dedup_candidates c WHERE ` + stringsJoin(parts, " AND "), args, nil
	}
	query := itemDedupCandidateSelect() + ` c WHERE ` + stringsJoin(parts, " AND ")
	query += ` ORDER BY CASE state WHEN 'open' THEN 0 ELSE 1 END, datetime(detected_at) ASC, id ASC`
	return query, args, nil
}

func (s *Store) itemDedupCandidateFilterExists(filter ItemListFilter) (string, []any, error) {
	normalized, err := s.prepareItemListFilter(filter)
	if err != nil {
		return "", nil, err
	}
	parts := []string{"ci.candidate_id = c.id"}
	args := []any{}
	parts, args = appendItemFilterClauses(parts, args, normalized, "i.")
	return `EXISTS (
SELECT 1 FROM item_dedup_candidate_items ci
JOIN items i ON i.id = ci.item_id
WHERE ` + stringsJoin(parts, " AND ") + `
)`, args, nil
}

func (s *Store) scanItemDedupCandidateRows(rows *sql.Rows) ([]ItemDedupCandidateGroup, error) {
	groups := []ItemDedupCandidateGroup{}
	for rows.Next() {
		group, err := scanItemDedupCandidate(rows)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range groups {
		members, err := s.listItemDedupCandidateMembers(groups[i].ID)
		if err != nil {
			return nil, err
		}
		groups[i].Items = members
	}
	return groups, nil
}

func itemDedupCandidateSelect() string {
	return `SELECT id, kind, state, score, confidence, outcome, reasoning, detector, detected_at, reviewed_at, canonical_item_id FROM item_dedup_candidates`
}

func scanItemDedupCandidate(row interface{ Scan(...any) error }) (ItemDedupCandidateGroup, error) {
	var out ItemDedupCandidateGroup
	var reviewedAt sql.NullString
	var canonicalID sql.NullInt64
	err := row.Scan(&out.ID, &out.Kind, &out.State, &out.Score, &out.Confidence, &out.Outcome, &out.Reasoning, &out.Detector, &out.DetectedAt, &reviewedAt, &canonicalID)
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	out.Kind = normalizeItemKind(out.Kind)
	out.State = normalizeItemDedupState(out.State)
	out.ReviewedAt = nullStringPointer(reviewedAt)
	out.CanonicalItemID = nullInt64Pointer(canonicalID)
	return out, nil
}

func (s *Store) listItemDedupCandidateMembers(candidateID int64) ([]ItemDedupCandidateMember, error) {
	rows, err := s.db.Query(`SELECT item_id, outcome FROM item_dedup_candidate_items WHERE candidate_id = ? ORDER BY position, item_id`, candidateID)
	if err != nil {
		return nil, err
	}
	var inputs []ItemDedupCandidateItemInput
	for rows.Next() {
		var input ItemDedupCandidateItemInput
		if err := rows.Scan(&input.ItemID, &input.Outcome); err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return s.hydrateItemDedupCandidateMembers(inputs)
}

func (s *Store) hydrateItemDedupCandidateMembers(inputs []ItemDedupCandidateItemInput) ([]ItemDedupCandidateMember, error) {
	out := make([]ItemDedupCandidateMember, 0, len(inputs))
	for _, input := range inputs {
		item, err := s.GetItemSummary(input.ItemID)
		if err != nil {
			return nil, err
		}
		bindings, err := s.GetBindingsByItem(input.ItemID)
		if err != nil {
			return nil, err
		}
		out = append(out, ItemDedupCandidateMember{
			Item:             item,
			Outcome:          strings.TrimSpace(input.Outcome),
			SourceBindings:   bindings,
			SourceContainers: sourceContainersForBindings(bindings),
			Dates:            itemDedupMemberDates(item, bindings),
		})
	}
	return out, nil
}

func sourceContainersForBindings(bindings []ExternalBinding) []string {
	seen := map[string]bool{}
	var out []string
	for _, binding := range bindings {
		if binding.ContainerRef == nil {
			continue
		}
		value := strings.TrimSpace(*binding.ContainerRef)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func itemDedupMemberDates(item ItemSummary, bindings []ExternalBinding) []string {
	var dates []string
	add := func(label string, value *string) {
		if value != nil && strings.TrimSpace(*value) != "" {
			dates = append(dates, label+"="+strings.TrimSpace(*value))
		}
	}
	add("visible", item.VisibleAfter)
	add("follow_up", item.FollowUpAt)
	add("due", item.DueAt)
	for _, binding := range bindings {
		add(binding.Provider+":"+binding.RemoteID+":updated", binding.RemoteUpdatedAt)
	}
	return dates
}

func (s *Store) ApplyItemDedupDecision(candidateID int64, action string, canonicalID *int64) (ItemDedupCandidateGroup, error) {
	switch normalizeItemDedupAction(action) {
	case ItemDedupActionKeepSeparate:
		return s.resolveItemDedupCandidate(candidateID, ItemDedupStateKeepSeparate, nil)
	case ItemDedupActionReviewLater:
		return s.resolveItemDedupCandidate(candidateID, ItemDedupStateReviewLater, nil)
	case ItemDedupActionMerge:
		return s.mergeItemDedupCandidate(candidateID, canonicalID)
	default:
		return ItemDedupCandidateGroup{}, errors.New("dedup action must be merge, keep_separate, or review_later")
	}
}

func (s *Store) resolveItemDedupCandidate(candidateID int64, state string, canonicalID *int64) (ItemDedupCandidateGroup, error) {
	res, err := s.db.Exec(`UPDATE item_dedup_candidates SET state = ?, canonical_item_id = ?, reviewed_at = datetime('now') WHERE id = ?`,
		state, nullablePositiveID(valueOrZero(canonicalID)), candidateID)
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	if affected == 0 {
		return ItemDedupCandidateGroup{}, sql.ErrNoRows
	}
	return s.GetItemDedupCandidate(candidateID)
}

func (s *Store) mergeItemDedupCandidate(candidateID int64, canonicalID *int64) (ItemDedupCandidateGroup, error) {
	if canonicalID == nil || *canonicalID <= 0 {
		return ItemDedupCandidateGroup{}, errors.New("canonical_item_id is required for merge")
	}
	group, err := s.GetItemDedupCandidate(candidateID)
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	if !candidateContainsItem(group, *canonicalID) {
		return ItemDedupCandidateGroup{}, fmt.Errorf("canonical item %d is not in dedup candidate %d", *canonicalID, candidateID)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	defer tx.Rollback()
	if err := mergeItemDedupBindings(tx, group, *canonicalID); err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	if _, err := tx.Exec(`UPDATE item_dedup_candidates SET state = ?, canonical_item_id = ?, reviewed_at = datetime('now') WHERE id = ?`,
		ItemDedupStateMerged, *canonicalID, candidateID); err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return ItemDedupCandidateGroup{}, err
	}
	return s.GetItemDedupCandidate(candidateID)
}

func candidateContainsItem(group ItemDedupCandidateGroup, itemID int64) bool {
	for _, member := range group.Items {
		if member.Item.ID == itemID {
			return true
		}
	}
	return false
}

func mergeItemDedupBindings(tx *sql.Tx, group ItemDedupCandidateGroup, canonicalID int64) error {
	for _, member := range group.Items {
		if member.Item.ID == canonicalID {
			continue
		}
		if _, err := tx.Exec(`UPDATE external_bindings SET item_id = ? WHERE item_id = ?`, canonicalID, member.Item.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE items SET source = NULL, source_ref = NULL, state = ?, updated_at = datetime('now') WHERE id = ?`, ItemStateDone, member.Item.ID); err != nil {
			return err
		}
	}
	return nil
}
