package store

import (
	"errors"
	"strings"
	"time"
)

func normalizeOptionalSidebarSectionFilter(raw string) (string, error) {
	clean := strings.ToLower(strings.TrimSpace(raw))
	if clean == "" {
		return "", nil
	}
	switch clean {
	case ItemSidebarSectionProject,
		ItemSidebarSectionPeople,
		ItemSidebarSectionDrift,
		ItemSidebarSectionDedup,
		ItemSidebarSectionRecentMeetings:
		return clean, nil
	}
	return "", errors.New("section must be one of project_items, people, drift, dedup, recent_meetings")
}

func normalizeItemListFilter(filter ItemListFilter) (ItemListFilter, error) {
	normalized := ItemListFilter{
		Source:              normalizeOptionalSourceFilter(filter.Source),
		SourceContainer:     strings.TrimSpace(filter.SourceContainer),
		Track:               strings.TrimSpace(filter.Track),
		WorkspaceUnassigned: filter.WorkspaceUnassigned,
	}
	sphere, err := normalizeOptionalSphereFilter(filter.Sphere)
	if err != nil {
		return ItemListFilter{}, err
	}
	normalized.Sphere = sphere
	section, err := normalizeOptionalSidebarSectionFilter(filter.Section)
	if err != nil {
		return ItemListFilter{}, err
	}
	normalized.Section = section
	if section == ItemSidebarSectionRecentMeetings {
		normalized.recentMeetingsCutoff = time.Now().UTC().Add(-RecentMeetingsLookbackHours * time.Hour).Format(time.RFC3339Nano)
	}
	if filter.WorkspaceID != nil {
		if *filter.WorkspaceID <= 0 {
			return ItemListFilter{}, errors.New("workspace_id must be a positive integer")
		}
		value := *filter.WorkspaceID
		normalized.WorkspaceID = &value
	}
	if normalized.WorkspaceID != nil && normalized.WorkspaceUnassigned {
		return ItemListFilter{}, errors.New("workspace_id cannot be combined with workspace_id=null")
	}
	if filter.ProjectItemID != nil {
		if *filter.ProjectItemID <= 0 {
			return ItemListFilter{}, errors.New("project_item_id must be a positive integer")
		}
		value := *filter.ProjectItemID
		normalized.ProjectItemID = &value
	}
	if filter.ActorID != nil {
		if *filter.ActorID <= 0 {
			return ItemListFilter{}, errors.New("actor_id must be a positive integer")
		}
		value := *filter.ActorID
		normalized.ActorID = &value
	}
	if normalized.DueBefore, err = normalizeOptionalRFC3339Filter(filter.DueBefore, "due_before"); err != nil {
		return ItemListFilter{}, err
	}
	if normalized.DueAfter, err = normalizeOptionalRFC3339Filter(filter.DueAfter, "due_after"); err != nil {
		return ItemListFilter{}, err
	}
	if normalized.FollowUpBefore, err = normalizeOptionalRFC3339Filter(filter.FollowUpBefore, "follow_up_before"); err != nil {
		return ItemListFilter{}, err
	}
	if normalized.FollowUpAfter, err = normalizeOptionalRFC3339Filter(filter.FollowUpAfter, "follow_up_after"); err != nil {
		return ItemListFilter{}, err
	}
	normalized.IncludeProjectItems = filter.IncludeProjectItems
	normalized.Label = normalizeOptionalContextQuery(filter.Label)
	if filter.LabelID != nil {
		if *filter.LabelID <= 0 {
			return ItemListFilter{}, errors.New("label_id must be a positive integer")
		}
		value := *filter.LabelID
		normalized.LabelID = &value
	}
	if normalized.Label != "" && normalized.LabelID != nil {
		return ItemListFilter{}, errors.New("label cannot be combined with label_id")
	}
	return normalized, nil
}

func normalizeOptionalRFC3339Filter(raw, field string) (string, error) {
	clean := strings.TrimSpace(raw)
	if clean == "" {
		return "", nil
	}
	normalized, err := normalizeRFC3339String(clean)
	if err != nil {
		return "", errors.New(field + " must be a valid RFC3339 timestamp")
	}
	return normalized, nil
}

func (s *Store) prepareItemListFilter(filter ItemListFilter) (ItemListFilter, error) {
	normalized, err := normalizeItemListFilter(filter)
	if err != nil {
		return ItemListFilter{}, err
	}
	if normalized.Label == "" {
		return normalized, nil
	}
	for _, term := range splitContextQueryTerms(normalized.Label) {
		labelIDs, err := s.resolveContextQueryIDs(term)
		if err != nil {
			return ItemListFilter{}, err
		}
		normalized.resolvedLabelGroups = append(normalized.resolvedLabelGroups, labelIDs)
	}
	normalized.labelResolved = true
	return normalized, nil
}

func appendItemSectionFilterClauses(parts []string, args []any, filter ItemListFilter, column, outerColumn func(string) string) ([]string, []any) {
	switch filter.Section {
	case ItemSidebarSectionProject:
		parts = append(parts, column("kind")+" = ?")
		args = append(args, ItemKindProject)
	case ItemSidebarSectionPeople:
		parts = append(parts, column("actor_id")+" IS NOT NULL")
	case ItemSidebarSectionDrift:
		parts = append(parts, `EXISTS (
SELECT 1 FROM external_binding_drifts drift
WHERE drift.item_id = `+outerColumn("id")+`
  AND drift.resolved_at IS NULL
)`)
	case ItemSidebarSectionDedup:
		parts = append(parts, `EXISTS (
SELECT 1 FROM item_dedup_candidate_items dci
JOIN item_dedup_candidates dc ON dc.id = dci.candidate_id
WHERE dci.item_id = `+outerColumn("id")+`
  AND dc.state IN ('open', 'review_later')
)`)
	case ItemSidebarSectionRecentMeetings:
		cutoff := filter.recentMeetingsCutoff
		if cutoff == "" {
			cutoff = time.Now().UTC().Add(-RecentMeetingsLookbackHours * time.Hour).Format(time.RFC3339Nano)
		}
		parts = append(parts, column("artifact_id")+" IS NOT NULL", `EXISTS (
SELECT 1 FROM artifacts mart
WHERE mart.id = `+column("artifact_id")+`
  AND datetime(mart.created_at) >= datetime(?)
  AND (
    lower(trim(mart.kind)) = 'transcript'
    OR (mart.meta_json IS NOT NULL AND mart.meta_json LIKE '%"source":"meeting_summary"%')
    OR (mart.meta_json IS NOT NULL AND mart.meta_json LIKE '%"source":"meeting_notes"%')
  )
)`)
		args = append(args, cutoff)
	}
	return parts, args
}

type itemFilterColumnFunc func(string) string

func appendItemFilterClauses(parts []string, args []any, filter ItemListFilter, alias string) ([]string, []any) {
	column := func(name string) string {
		return alias + name
	}
	outerColumn := func(name string) string {
		if alias == "" {
			return "items." + name
		}
		return alias + name
	}
	parts, args = appendItemScopeFilterClauses(parts, args, filter, column, outerColumn)
	parts, args = appendItemDateFilterClauses(parts, args, filter, column)
	parts, args = appendItemOwnerFilterClauses(parts, args, filter, column, outerColumn)
	parts, args = appendItemSectionFilterClauses(parts, args, filter, column, outerColumn)
	return appendItemLabelFilterClauses(parts, args, filter, outerColumn)
}

func appendItemScopeFilterClauses(parts []string, args []any, filter ItemListFilter, column, outerColumn itemFilterColumnFunc) ([]string, []any) {
	if filter.Sphere != "" {
		parts = append(parts, scopedContextFilter("context_items", "item_id", outerColumn("id")))
		args = append(args, filter.Sphere)
	}
	if filter.Source != "" {
		parts = append(parts, "lower(trim("+column("source")+")) = ?")
		args = append(args, filter.Source)
	}
	if filter.Track != "" {
		parts = append(parts, "lower(trim("+column("track")+")) = lower(trim(?))")
		args = append(args, filter.Track)
	}
	if filter.SourceContainer != "" {
		parts = append(parts, `EXISTS (
SELECT 1 FROM external_bindings eb
WHERE eb.item_id = `+outerColumn("id")+`
  AND eb.container_ref IS NOT NULL
  AND lower(trim(eb.container_ref)) = lower(trim(?))
)`)
		args = append(args, filter.SourceContainer)
	}
	return parts, args
}

func appendItemDateFilterClauses(parts []string, args []any, filter ItemListFilter, column itemFilterColumnFunc) ([]string, []any) {
	parts, args = appendItemDateWindow(parts, args, column("due_at"), "<=", filter.DueBefore)
	parts, args = appendItemDateWindow(parts, args, column("due_at"), ">=", filter.DueAfter)
	parts, args = appendItemDateWindow(parts, args, column("follow_up_at"), "<=", filter.FollowUpBefore)
	return appendItemDateWindow(parts, args, column("follow_up_at"), ">=", filter.FollowUpAfter)
}

func appendItemDateWindow(parts []string, args []any, column, operator, value string) ([]string, []any) {
	if value == "" {
		return parts, args
	}
	parts = append(parts,
		column+" IS NOT NULL",
		"trim("+column+") <> ''",
		"datetime("+column+") "+operator+" datetime(?)",
	)
	return parts, append(args, value)
}

func appendItemOwnerFilterClauses(parts []string, args []any, filter ItemListFilter, column, outerColumn itemFilterColumnFunc) ([]string, []any) {
	if filter.WorkspaceID != nil {
		parts = append(parts, column("workspace_id")+" = ?")
		args = append(args, *filter.WorkspaceID)
	}
	if filter.WorkspaceUnassigned {
		parts = append(parts, column("workspace_id")+" IS NULL")
	}
	if filter.ActorID != nil {
		parts = append(parts, column("actor_id")+" = ?")
		args = append(args, *filter.ActorID)
	}
	if filter.ProjectItemID != nil {
		parts = append(parts, `EXISTS (
SELECT 1 FROM item_children link
WHERE link.parent_item_id = ?
  AND link.child_item_id = `+outerColumn("id")+`
)`)
		args = append(args, *filter.ProjectItemID)
	}
	return parts, args
}

func appendItemLabelFilterClauses(parts []string, args []any, filter ItemListFilter, outerColumn itemFilterColumnFunc) ([]string, []any) {
	if filter.labelResolved {
		if len(filter.resolvedLabelGroups) == 0 {
			parts = append(parts, "0=1")
			return parts, args
		}
		for _, labelIDs := range filter.resolvedLabelGroups {
			if len(labelIDs) == 0 {
				parts = append(parts, "0=1")
				return parts, args
			}
			labelItemMatch, labelItemArgs := contextLinkExistsClause("context_items", "item_id", outerColumn("id"), labelIDs)
			labelWorkspaceMatch, labelWorkspaceArgs := contextLinkExistsClause("context_workspaces", "workspace_id", outerColumn("workspace_id"), labelIDs)
			parts = append(parts, `(`+labelItemMatch+` OR `+labelWorkspaceMatch+`)`)
			args = append(args, labelItemArgs...)
			args = append(args, labelWorkspaceArgs...)
		}
		return parts, args
	}
	if filter.LabelID != nil {
		contextItemMatch := `EXISTS (
WITH RECURSIVE context_tree(id) AS (
  SELECT id FROM contexts WHERE id = ?
  UNION ALL
  SELECT c.id
  FROM contexts c
  JOIN context_tree tree ON c.parent_id = tree.id
)
SELECT 1
FROM context_items ci
JOIN context_tree tree ON tree.id = ci.context_id
WHERE ci.item_id = ` + outerColumn("id") + `
)`
		contextWorkspaceMatch := `EXISTS (
WITH RECURSIVE context_tree(id) AS (
  SELECT id FROM contexts WHERE id = ?
  UNION ALL
  SELECT c.id
  FROM contexts c
  JOIN context_tree tree ON c.parent_id = tree.id
)
SELECT 1
FROM context_workspaces cw
JOIN context_tree tree ON tree.id = cw.context_id
WHERE cw.workspace_id = ` + outerColumn("workspace_id") + `
)`
		parts = append(parts, `(`+contextItemMatch+` OR `+contextWorkspaceMatch+`)`)
		args = append(args, *filter.LabelID, *filter.LabelID)
	}
	return parts, args
}
