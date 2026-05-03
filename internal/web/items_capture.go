package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/sloppy-org/slopshell/internal/store"
)

// itemCaptureRequest is the body for POST /api/items/capture, the always-on
// quick capture surface used by the web UI and `sls gtd capture`.
//
// Capture intentionally diverges from /api/items in three ways: it always
// lands the new item in the inbox, it accepts kind=project to create a
// composite outcome rather than an action, and it accepts a single
// project_item_id to link the captured action under an existing outcome in
// one round-trip. The active workspace is contextual metadata, not the
// captured object: capturing a project/outcome creates Item(kind=project),
// never a workspace.
type itemCaptureRequest struct {
	Title           string  `json:"title"`
	Kind            string  `json:"kind"`
	Track           string  `json:"track"`
	Sphere          *string `json:"sphere"`
	WorkspaceID     *int64  `json:"workspace_id"`
	ArtifactID      *int64  `json:"artifact_id"`
	ActorID         *int64  `json:"actor_id"`
	LabelID         *int64  `json:"label_id"`
	Label           string  `json:"label"`
	ProjectItemID   *int64  `json:"project_item_id"`
	ProjectItemRole string  `json:"project_item_role"`
	Source          *string `json:"source"`
	SourceRef       *string `json:"source_ref"`
}

type itemCaptureProjectLink struct {
	ProjectItemID int64                 `json:"project_item_id"`
	Role          string                `json:"role"`
	Links         []store.ItemChildLink `json:"links"`
}

func (a *App) handleItemCapture(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	var req itemCaptureRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	plan, err := a.planItemCapture(req)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	item, err := a.createCapturedItem(req, plan)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	label, err := a.attachCaptureLabel(item.ID, plan.labelID, plan.labelName)
	if err != nil {
		_ = a.store.DeleteItem(item.ID)
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	link, err := a.attachCaptureProjectLink(item.ID, plan.projectItemID, plan.projectItemRole)
	if err != nil {
		_ = a.store.DeleteItem(item.ID)
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	final, err := a.store.GetItem(item.ID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	payload := map[string]any{"item": final}
	if label != nil {
		payload["label"] = label
	}
	if link != nil {
		payload["project_item"] = link
	}
	writeAPIData(w, http.StatusCreated, payload)
}

// captureValidationPlan is the validated, normalized projection of a capture
// request. The handler runs validation up-front so that we never insert an
// item we'd have to roll back because of an invalid label or project link.
type captureValidationPlan struct {
	kind            string
	labelID         int64
	labelName       string
	projectItemID   int64
	projectItemRole string
}

func (a *App) planItemCapture(req itemCaptureRequest) (captureValidationPlan, error) {
	plan := captureValidationPlan{}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return plan, errors.New("title is required")
	}
	plan.kind = normalizedCaptureKind(req.Kind)
	if plan.kind == "" {
		return plan, errors.New("kind must be action or project")
	}
	if err := validateCaptureProjectLinkInputs(plan.kind, req); err != nil {
		return plan, err
	}
	if req.Sphere != nil {
		clean := strings.ToLower(strings.TrimSpace(*req.Sphere))
		if clean != "" && clean != store.SphereWork && clean != store.SpherePrivate {
			return plan, errors.New("sphere must be work or private")
		}
	}
	if req.LabelID != nil && *req.LabelID > 0 && strings.TrimSpace(req.Label) != "" {
		return plan, errors.New("label_id and label cannot be combined")
	}
	if req.LabelID != nil {
		if *req.LabelID <= 0 {
			return plan, errors.New("label_id must be a positive integer")
		}
		plan.labelID = *req.LabelID
	}
	plan.labelName = strings.TrimSpace(req.Label)
	if req.ProjectItemID != nil {
		if *req.ProjectItemID <= 0 {
			return plan, errors.New("project_item_id must be a positive integer")
		}
		plan.projectItemID = *req.ProjectItemID
		role := strings.TrimSpace(req.ProjectItemRole)
		if role == "" {
			role = store.ItemLinkRoleNextAction
		}
		if normalizedRole := normalizeItemCaptureLinkRole(role); normalizedRole != "" {
			plan.projectItemRole = normalizedRole
		} else {
			return plan, errors.New("project_item_role must be next_action, support, or blocked_by")
		}
	}
	return plan, nil
}

// validateCaptureProjectLinkInputs rejects link payloads that would only
// confuse the user — supplying a project link role without an id, or trying
// to link a kind=project capture as a child of another project (the link
// would succeed but the result is rarely what someone wants from quick
// capture and is best done explicitly via the existing project-item-link
// endpoint).
func validateCaptureProjectLinkInputs(kind string, req itemCaptureRequest) error {
	if req.ProjectItemID == nil && strings.TrimSpace(req.ProjectItemRole) != "" {
		return errors.New("project_item_role requires project_item_id")
	}
	if kind == store.ItemKindProject && req.ProjectItemID != nil {
		return errors.New("project_item_id cannot be set when capturing a project")
	}
	return nil
}

func (a *App) createCapturedItem(req itemCaptureRequest, plan captureValidationPlan) (store.Item, error) {
	if req.Source != nil && strings.EqualFold(strings.TrimSpace(*req.Source), store.ExternalProviderTodoist) && strings.TrimSpace(optionalStringValue(req.SourceRef)) == "" {
		if plan.kind != store.ItemKindAction {
			return store.Item{}, errors.New("source=todoist quick capture is only supported for kind=action")
		}
		return a.createTodoistBackedItem(itemCreateRequest{
			Title:        strings.TrimSpace(req.Title),
			State:        store.ItemStateInbox,
			Track:        req.Track,
			WorkspaceID:  req.WorkspaceID,
			Sphere:       req.Sphere,
			ArtifactID:   req.ArtifactID,
			ActorID:      req.ActorID,
			VisibleAfter: nil,
			FollowUpAt:   nil,
			DueAt:        nil,
			Source:       req.Source,
			SourceRef:    req.SourceRef,
		})
	}
	return a.store.CreateItem(strings.TrimSpace(req.Title), store.ItemOptions{
		Kind:        plan.kind,
		State:       store.ItemStateInbox,
		Track:       req.Track,
		WorkspaceID: req.WorkspaceID,
		Sphere:      req.Sphere,
		ArtifactID:  req.ArtifactID,
		ActorID:     req.ActorID,
		Source:      req.Source,
		SourceRef:   req.SourceRef,
	})
}

func (a *App) attachCaptureLabel(itemID, labelID int64, labelName string) (*store.Label, error) {
	if labelID == 0 && labelName == "" {
		return nil, nil
	}
	resolved, err := a.resolveCaptureLabel(labelID, labelName)
	if err != nil {
		return nil, err
	}
	if err := a.store.LinkLabelToItem(resolved.ID, itemID); err != nil {
		return nil, err
	}
	return &resolved, nil
}

func (a *App) resolveCaptureLabel(labelID int64, labelName string) (store.Label, error) {
	if labelID > 0 {
		label, err := a.store.GetLabel(labelID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return store.Label{}, fmt.Errorf("label_id %d not found", labelID)
			}
			return store.Label{}, err
		}
		return label, nil
	}
	return a.store.CreateLabel(labelName, nil)
}

func (a *App) attachCaptureProjectLink(itemID, projectItemID int64, role string) (*itemCaptureProjectLink, error) {
	if projectItemID == 0 {
		return nil, nil
	}
	parent, err := a.store.GetItem(projectItemID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("project_item_id %d not found", projectItemID)
		}
		return nil, err
	}
	if parent.Kind != store.ItemKindProject {
		return nil, fmt.Errorf("project_item_id %d is not a project", projectItemID)
	}
	if err := a.store.LinkItemChild(projectItemID, itemID, role); err != nil {
		return nil, err
	}
	links, err := a.store.ListItemChildLinks(projectItemID)
	if err != nil {
		return nil, err
	}
	return &itemCaptureProjectLink{
		ProjectItemID: projectItemID,
		Role:          role,
		Links:         links,
	}, nil
}

func normalizedCaptureKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", store.ItemKindAction:
		return store.ItemKindAction
	case store.ItemKindProject, "outcome":
		return store.ItemKindProject
	default:
		return ""
	}
}

func normalizeItemCaptureLinkRole(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case store.ItemLinkRoleNextAction:
		return store.ItemLinkRoleNextAction
	case store.ItemLinkRoleSupport:
		return store.ItemLinkRoleSupport
	case store.ItemLinkRoleBlockedBy:
		return store.ItemLinkRoleBlockedBy
	default:
		return ""
	}
}
