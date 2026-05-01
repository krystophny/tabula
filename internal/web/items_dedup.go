package web

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/sloppy-org/slopshell/internal/aggregateitem"
	"github.com/sloppy-org/slopshell/internal/store"
)

type itemDedupActionRequest struct {
	CanonicalItemID *int64 `json:"canonical_item_id,omitempty"`
}

func (a *App) handleItemDedupReview(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if !a.resurfaceDueItemsForRead(w) {
		return
	}
	filter, err := parseItemListFilterQuery(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.refreshSloptoolsItemDedupCandidates(r.Context(), filter); err != nil {
		writeItemStoreError(w, err)
		return
	}
	groups, err := a.store.ListItemDedupCandidatesFiltered(r.URL.Query().Get("kind"), filter)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"groups": groups,
		"total":  len(groups),
	})
}

func (a *App) handleItemDedupAction(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	candidateID, err := parseURLInt64Param(r, "candidate_id")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req itemDedupActionRequest
	if r.Body != nil && strings.EqualFold(r.Method, http.MethodPost) {
		if err := decodeJSON(r, &req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid JSON")
			return
		}
	}
	action := strings.TrimSpace(chi.URLParam(r, "action"))
	group, err := a.store.ApplyItemDedupDecision(candidateID, action, req.CanonicalItemID)
	if err != nil {
		writeItemStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"group":  group,
		"action": action,
	})
}

func (a *App) refreshSloptoolsItemDedupCandidates(ctx context.Context, filter store.ItemListFilter) error {
	if a == nil || !a.sloptoolsDedupEndpointReady() {
		return nil
	}
	client, err := aggregateitem.NewClient(
		a.localMCPEndpointURL(),
		a.localMCPEndpoint.HTTPClient(20*time.Second),
	)
	if err != nil {
		return err
	}
	result, err := client.Scan(ctx, aggregateitem.ScanRequest{
		Sphere: sloptoolsDedupScanSphere(filter.Sphere, a.runtimeActiveSphere()),
	})
	if err != nil {
		if sloptoolsDedupUnavailable(err) {
			return nil
		}
		return err
	}
	return a.storeSloptoolsDedupCandidates(result.Candidates)
}

func sloptoolsDedupUnavailable(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such file")
}

func (a *App) sloptoolsDedupEndpointReady() bool {
	if !a.localMCPEndpoint.ok() {
		return false
	}
	if a.localMCPEndpoint.socket == "" {
		return strings.TrimSpace(a.localMCPEndpoint.httpURL) != ""
	}
	if _, err := os.Stat(a.localMCPEndpoint.socket); err != nil {
		return false
	}
	return true
}

func sloptoolsDedupScanSphere(raw, active string) string {
	if sphere := normalizeRuntimeActiveSphere(raw); sphere != "" {
		return sphere
	}
	if sphere := normalizeRuntimeActiveSphere(active); sphere != "" {
		return sphere
	}
	return store.SpherePrivate
}

func (a *App) storeSloptoolsDedupCandidates(candidates []aggregateitem.Candidate) error {
	for _, candidate := range candidates {
		if !sloptoolsCandidateNeedsReview(candidate) {
			continue
		}
		kind, items, err := a.resolveSloptoolsCandidateItems(candidate.Paths)
		if err != nil {
			return err
		}
		if len(items) < 2 {
			continue
		}
		if err := a.createSloptoolsDedupCandidate(kind, candidate, items); err != nil {
			return err
		}
	}
	return nil
}

func sloptoolsCandidateNeedsReview(candidate aggregateitem.Candidate) bool {
	switch strings.ToLower(strings.TrimSpace(candidate.ReviewState)) {
	case "", store.ItemDedupStateOpen, store.ItemDedupStateReviewLater:
		return true
	default:
		return false
	}
}

func (a *App) resolveSloptoolsCandidateItems(paths []string) (string, []store.ItemDedupCandidateItemInput, error) {
	var kind string
	seen := map[int64]bool{}
	items := make([]store.ItemDedupCandidateItemInput, 0, len(paths))
	for _, path := range cleanSloptoolsCandidatePaths(paths) {
		item, err := a.findSloptoolsCandidateItem(path)
		if err != nil {
			return "", nil, err
		}
		if item.ID == 0 {
			return "", nil, nil
		}
		if kind == "" {
			kind = item.Kind
		}
		if item.Kind != kind {
			return "", nil, nil
		}
		if !seen[item.ID] {
			seen[item.ID] = true
			items = append(items, store.ItemDedupCandidateItemInput{ItemID: item.ID})
		}
	}
	return kind, items, nil
}

func cleanSloptoolsCandidatePaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if clean := strings.TrimSpace(path); clean != "" {
			out = append(out, clean)
		}
	}
	return out
}

func (a *App) findSloptoolsCandidateItem(path string) (store.Item, error) {
	for _, alias := range sloptoolsPathAliases(path) {
		item, err := a.findSloptoolsCandidateItemAlias(alias)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return item, err
		}
	}
	return store.Item{}, nil
}

func (a *App) findSloptoolsCandidateItemAlias(path string) (store.Item, error) {
	for _, source := range []string{"brain.gtd", "brain", "markdown", "meetings"} {
		item, err := a.store.GetItemBySource(source, path)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return item, err
		}
	}
	return a.store.FindItemByArtifactRefPath(path)
}

func sloptoolsPathAliases(path string) []string {
	clean := filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if clean == "." || clean == "" {
		return nil
	}
	aliases := []string{clean}
	if strings.HasPrefix(clean, "brain/") {
		aliases = append(aliases, strings.TrimPrefix(clean, "brain/"))
	} else {
		aliases = append(aliases, "brain/"+clean)
	}
	return uniqueStrings(aliases)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		key := strings.ToLower(strings.TrimSpace(value))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, strings.TrimSpace(value))
	}
	return out
}

func (a *App) createSloptoolsDedupCandidate(kind string, candidate aggregateitem.Candidate, items []store.ItemDedupCandidateItemInput) error {
	itemIDs := make([]int64, 0, len(items))
	for _, item := range items {
		itemIDs = append(itemIDs, item.ItemID)
	}
	if _, err := a.store.FindItemDedupCandidateByItems(kind, itemIDs); err == nil {
		return nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	detector := strings.TrimSpace(candidate.Detector)
	if detector == "" {
		detector = "sloptools"
	}
	_, err := a.store.CreateItemDedupCandidate(store.ItemDedupCandidateOptions{
		Kind:       kind,
		Score:      candidate.Score,
		Confidence: candidate.Confidence,
		Reasoning:  candidate.Reasoning,
		Detector:   detector,
		Items:      items,
	})
	return err
}
