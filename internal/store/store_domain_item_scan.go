package store

import (
	"database/sql"
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
