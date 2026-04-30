package store

import (
	"database/sql"
	"errors"
	"strings"
)

func scanItem(
	row interface {
		Scan(dest ...any) error
	},
) (Item, error) {
	var (
		out                                Item
		workspaceID, artifactID, actorID   sql.NullInt64
		visibleAfter, followUpAt, dueAt    sql.NullString
		sphere                             string
		source, sourceRef                  sql.NullString
		reviewTarget, reviewer, reviewedAt sql.NullString
	)
	err := row.Scan(
		&out.ID,
		&out.Title,
		&out.Kind,
		&out.State,
		&workspaceID,
		&sphere,
		&artifactID,
		&actorID,
		&visibleAfter,
		&followUpAt,
		&dueAt,
		&source,
		&sourceRef,
		&reviewTarget,
		&reviewer,
		&reviewedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return Item{}, err
	}
	out.Title = strings.TrimSpace(out.Title)
	out.Kind = normalizeItemKind(out.Kind)
	out.State = normalizeItemState(out.State)
	out.WorkspaceID = nullInt64Pointer(workspaceID)
	out.Sphere = normalizeSphere(sphere)
	out.ArtifactID = nullInt64Pointer(artifactID)
	out.ActorID = nullInt64Pointer(actorID)
	out.VisibleAfter = nullStringPointer(visibleAfter)
	out.FollowUpAt = nullStringPointer(followUpAt)
	out.DueAt = nullStringPointer(dueAt)
	out.Source = nullStringPointer(source)
	out.SourceRef = nullStringPointer(sourceRef)
	out.ReviewTarget = nullStringPointer(reviewTarget)
	if out.ReviewTarget != nil {
		*out.ReviewTarget = normalizeItemReviewTarget(*out.ReviewTarget)
		if *out.ReviewTarget == "" {
			out.ReviewTarget = nil
		}
	}
	out.Reviewer = nullStringPointer(reviewer)
	out.ReviewedAt = nullStringPointer(reviewedAt)
	return out, nil
}

func scanItemSummary(
	row interface {
		Scan(dest ...any) error
	},
) (ItemSummary, error) {
	var (
		out                                    ItemSummary
		workspaceID, artifactID, actorID       sql.NullInt64
		visibleAfter, followUpAt, dueAt        sql.NullString
		sphere                                 string
		source, sourceRef                      sql.NullString
		reviewTarget, reviewer, reviewedAt     sql.NullString
		artifactTitle, artifactKind, actorName sql.NullString
	)
	err := row.Scan(
		&out.ID,
		&out.Title,
		&out.Kind,
		&out.State,
		&workspaceID,
		&sphere,
		&artifactID,
		&actorID,
		&visibleAfter,
		&followUpAt,
		&dueAt,
		&source,
		&sourceRef,
		&reviewTarget,
		&reviewer,
		&reviewedAt,
		&out.CreatedAt,
		&out.UpdatedAt,
		&artifactTitle,
		&artifactKind,
		&actorName,
	)
	if err != nil {
		return ItemSummary{}, err
	}
	out.Title = strings.TrimSpace(out.Title)
	out.Kind = normalizeItemKind(out.Kind)
	out.State = normalizeItemState(out.State)
	out.WorkspaceID = nullInt64Pointer(workspaceID)
	out.Sphere = normalizeSphere(sphere)
	out.ArtifactID = nullInt64Pointer(artifactID)
	out.ActorID = nullInt64Pointer(actorID)
	out.VisibleAfter = nullStringPointer(visibleAfter)
	out.FollowUpAt = nullStringPointer(followUpAt)
	out.DueAt = nullStringPointer(dueAt)
	out.Source = nullStringPointer(source)
	out.SourceRef = nullStringPointer(sourceRef)
	out.ReviewTarget = nullStringPointer(reviewTarget)
	if out.ReviewTarget != nil {
		*out.ReviewTarget = normalizeItemReviewTarget(*out.ReviewTarget)
		if *out.ReviewTarget == "" {
			out.ReviewTarget = nil
		}
	}
	out.Reviewer = nullStringPointer(reviewer)
	out.ReviewedAt = nullStringPointer(reviewedAt)
	out.ArtifactTitle = nullStringPointer(artifactTitle)
	if artifactKind.Valid {
		normalized := normalizeArtifactKind(ArtifactKind(artifactKind.String))
		out.ArtifactKind = &normalized
	}
	out.ActorName = nullStringPointer(actorName)
	return out, nil
}

type itemUpdatePlan struct {
	item              Item
	parts             []string
	args              []any
	artifactUpdated   bool
	scopeUpdated      bool
	artifactID        *int64
	targetWorkspaceID *int64
	clearDeferredTime bool
}

func (s *Store) buildItemUpdatePlan(id int64, item Item, updates ItemUpdate) (itemUpdatePlan, error) {
	plan := itemUpdatePlan{item: item, targetWorkspaceID: item.WorkspaceID}
	if err := appendItemCoreUpdateClauses(&plan, updates); err != nil {
		return itemUpdatePlan{}, err
	}
	if err := appendItemStateUpdateClauses(&plan, item, updates); err != nil {
		return itemUpdatePlan{}, err
	}
	if err := appendItemTimeUpdateClauses(&plan, updates); err != nil {
		return itemUpdatePlan{}, err
	}
	if err := s.appendItemSourceUpdateClauses(id, &plan, updates); err != nil {
		return itemUpdatePlan{}, err
	}
	if err := appendItemReviewUpdateClauses(&plan, updates); err != nil {
		return itemUpdatePlan{}, err
	}
	if err := s.resolveItemUpdateScope(&plan, updates); err != nil {
		return itemUpdatePlan{}, err
	}
	appendDeferredTimeClears(&plan, updates)
	return plan, nil
}

func appendItemCoreUpdateClauses(plan *itemUpdatePlan, updates ItemUpdate) error {
	if updates.Title != nil {
		title := strings.TrimSpace(*updates.Title)
		if title == "" {
			return errors.New("item title is required")
		}
		plan.parts = append(plan.parts, "title = ?")
		plan.args = append(plan.args, title)
	}
	if updates.Kind != nil {
		kind := normalizeItemKind(*updates.Kind)
		if kind == "" {
			return errors.New("invalid item kind")
		}
		plan.parts = append(plan.parts, "kind = ?")
		plan.args = append(plan.args, kind)
	}
	if updates.ArtifactID != nil {
		plan.artifactUpdated = true
		if *updates.ArtifactID > 0 {
			value := *updates.ArtifactID
			plan.artifactID = &value
		}
	}
	if updates.ActorID != nil {
		plan.parts = append(plan.parts, "actor_id = ?")
		plan.args = append(plan.args, nullablePositiveID(*updates.ActorID))
	}
	return nil
}

func appendItemStateUpdateClauses(plan *itemUpdatePlan, item Item, updates ItemUpdate) error {
	if updates.State != nil {
		next := normalizeItemState(*updates.State)
		if err := validateItemTransition(item.State, next); err != nil {
			return err
		}
		plan.clearDeferredTime = (next == ItemStateInbox && item.State != ItemStateInbox) || next == ItemStateNext
		plan.parts = append(plan.parts, "state = ?")
		plan.args = append(plan.args, next)
	}
	if updates.WorkspaceID != nil {
		plan.parts = append(plan.parts, "workspace_id = ?")
		plan.args = append(plan.args, nullablePositiveID(*updates.WorkspaceID))
		plan.targetWorkspaceID = nil
		if *updates.WorkspaceID > 0 {
			value := *updates.WorkspaceID
			plan.targetWorkspaceID = &value
		}
	}
	return nil
}

func appendItemTimeUpdateClauses(plan *itemUpdatePlan, updates ItemUpdate) error {
	fields := []struct {
		column string
		value  *string
	}{
		{column: "visible_after", value: updates.VisibleAfter},
		{column: "follow_up_at", value: updates.FollowUpAt},
		{column: "due_at", value: updates.DueAt},
	}
	for _, field := range fields {
		if field.value == nil {
			continue
		}
		value, err := normalizeOptionalRFC3339String(field.value)
		if err != nil {
			return err
		}
		plan.parts = append(plan.parts, field.column+" = ?")
		plan.args = append(plan.args, value)
	}
	return nil
}

func (s *Store) appendItemSourceUpdateClauses(id int64, plan *itemUpdatePlan, updates ItemUpdate) error {
	if updates.Source == nil {
		return nil
	}
	sourceValue := strings.TrimSpace(*updates.Source)
	sourceRefValue := strings.TrimSpace(nullStringValue(updates.SourceRef))
	switch {
	case sourceValue == "" && sourceRefValue != "":
		return errors.New("item source and source_ref are required")
	case sourceValue != "" && sourceRefValue == "":
		return errors.New("item source and source_ref are required")
	case sourceValue != "" && sourceRefValue != "":
		return s.UpdateItemSource(id, sourceValue, sourceRefValue)
	default:
		plan.parts = append(plan.parts, "source = ?", "source_ref = ?")
		plan.args = append(plan.args, nil, nil)
		return nil
	}
}

func appendItemReviewUpdateClauses(plan *itemUpdatePlan, updates ItemUpdate) error {
	if updates.ReviewTarget == nil && updates.Reviewer == nil {
		return nil
	}
	if err := validateReviewTargetPointer(updates.ReviewTarget); err != nil {
		return err
	}
	cleanTarget := normalizedReviewTargetPointer(updates.ReviewTarget)
	cleanReviewer := normalizedReviewerPointer(updates.Reviewer)
	if cleanTarget == nil && cleanReviewer != nil {
		return errors.New("review target is required when reviewer is set")
	}
	plan.parts = append(plan.parts, "review_target = ?", "reviewer = ?", "reviewed_at = ?")
	plan.args = append(plan.args,
		normalizeOptionalString(cleanTarget),
		normalizeOptionalString(cleanReviewer),
		normalizeOptionalString(reviewTimestampPointer(updates.ReviewTarget, updates.Reviewer)),
	)
	return nil
}

func (s *Store) resolveItemUpdateScope(plan *itemUpdatePlan, updates ItemUpdate) error {
	if updates.Sphere != nil {
		if plan.targetWorkspaceID != nil {
			return errors.New("item sphere is derived from workspace")
		}
		nextSphere := normalizeRequiredSphere(*updates.Sphere)
		if nextSphere == "" {
			return errors.New("item sphere must be work or private")
		}
		plan.item.Sphere = nextSphere
		plan.scopeUpdated = true
	}
	if plan.targetWorkspaceID == nil {
		return nil
	}
	workspaceSphere, err := s.workspaceSphere(*plan.targetWorkspaceID)
	if err != nil {
		return err
	}
	plan.item.Sphere = workspaceSphere
	plan.scopeUpdated = true
	return nil
}

func appendDeferredTimeClears(plan *itemUpdatePlan, updates ItemUpdate) {
	if !plan.clearDeferredTime {
		return
	}
	if updates.VisibleAfter == nil {
		plan.parts = append(plan.parts, "visible_after = NULL")
	}
	if updates.FollowUpAt == nil {
		plan.parts = append(plan.parts, "follow_up_at = NULL")
	}
}

func (s *Store) applyItemUpdateParts(id int64, parts []string, args []any) error {
	if len(parts) == 0 {
		return nil
	}
	parts = append(parts, "updated_at = datetime('now')")
	args = append(args, id)
	res, err := s.db.Exec(`UPDATE items SET `+stringsJoin(parts, ", ")+` WHERE id = ?`, args...)
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
