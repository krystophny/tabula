package web

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sloppy-org/slopshell/internal/store"
	tabsync "github.com/sloppy-org/slopshell/internal/sync"
)

const (
	brainGTDReviewListTool = "brain.gtd.review_list"
	brainGTDListTool       = "brain.gtd.list"
)

type brainGTDReviewList struct {
	Items []brainGTDReviewItem `json:"items"`
	Count int                  `json:"count"`
}

type brainGTDReviewItem struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	SourceRef string `json:"source_ref"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Queue     string `json:"queue"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Project   string `json:"project"`
	Due       string `json:"due"`
	FollowUp  string `json:"follow_up"`
}

type brainGTDCommitmentList struct {
	Items []brainGTDCommitmentItem `json:"items"`
	Count int                      `json:"count"`
}

type brainGTDCommitmentItem struct {
	Path     string   `json:"path"`
	Title    string   `json:"title"`
	Status   string   `json:"status"`
	Project  string   `json:"project"`
	Due      string   `json:"due"`
	FollowUp string   `json:"follow_up"`
	Bindings []string `json:"bindings"`
}

type brainGTDSyncResult struct {
	Imported int
	Migrated int
	Merged   int
}

var fetchBrainGTDReviewList = defaultFetchBrainGTDReviewList
var fetchBrainGTDCommitmentList = defaultFetchBrainGTDCommitmentList

func (a *App) syncBrainGTDReviewLists(ctx context.Context) (brainGTDSyncResult, error) {
	if a == nil || a.store == nil || !brainGTDSyncEnabled() {
		return brainGTDSyncResult{}, nil
	}
	total := brainGTDSyncResult{}
	for _, sphere := range []string{store.SphereWork, store.SpherePrivate} {
		list, err := fetchBrainGTDCommitmentList(a, ctx, sphere)
		if err != nil {
			return total, fmt.Errorf("sync GTD %s commitments: %w", sphere, err)
		}
		result, err := a.syncBrainGTDCanonicalBindings(ctx, sphere, list)
		if err != nil {
			return total, err
		}
		total.Migrated += result.Migrated
		total.Merged += result.Merged
		review, err := fetchBrainGTDReviewList(a, ctx, sphere)
		if err != nil {
			return total, fmt.Errorf("sync GTD %s review list: %w", sphere, err)
		}
		imported, err := a.importBrainGTDReviewItems(ctx, sphere, review.Items)
		if err != nil {
			return total, err
		}
		total.Imported += imported
	}
	if total.Imported > 0 {
		a.broadcastItemsIngested(total.Imported, store.ExternalProviderMarkdown)
	}
	return total, nil
}

func brainGTDSyncEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("SLOPSHELL_BRAIN_GTD_SYNC"))) {
	case "1", "on", "true", "yes":
		return true
	default:
		return false
	}
}

func defaultFetchBrainGTDReviewList(_ *App, ctx context.Context, sphere string) (brainGTDReviewList, error) {
	result, err := sloptoolsBrainGTDCall(ctx, brainGTDReviewListTool, map[string]interface{}{
		"sphere":  sphere,
		"limit":   10000,
		"sources": []string{"markdown", "tasks"},
	})
	if err != nil {
		return brainGTDReviewList{}, err
	}
	var out brainGTDReviewList
	return out, decodeBrainGTDToolResult(result, &out)
}

func defaultFetchBrainGTDCommitmentList(_ *App, ctx context.Context, sphere string) (brainGTDCommitmentList, error) {
	result, err := sloptoolsBrainGTDCall(ctx, brainGTDListTool, map[string]interface{}{
		"sphere": sphere,
	})
	if err != nil {
		return brainGTDCommitmentList{}, err
	}
	var out brainGTDCommitmentList
	return out, decodeBrainGTDToolResult(result, &out)
}

func sloptoolsBrainGTDCall(ctx context.Context, tool string, args map[string]interface{}) (map[string]interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	callCtx, cancel := context.WithTimeout(ctx, sourceSyncCommandTimeout)
	defer cancel()
	body, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(callCtx, sloptoolsBinary(), "tools", "call", tool, "--args", string(body))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if callCtx.Err() != nil {
			return nil, callCtx.Err()
		}
		return nil, fmt.Errorf("%s failed: %w: %s", tool, err, strings.TrimSpace(stderr.String()))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("decode %s: %w", tool, err)
	}
	return result, nil
}

func sloptoolsBinary() string {
	if bin := strings.TrimSpace(os.Getenv("SLOPSHELL_SLOPTOOLS_BIN")); bin != "" {
		return bin
	}
	if bin, err := exec.LookPath("sloptools"); err == nil {
		return bin
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".local", "bin", "sloptools")
	}
	return "sloptools"
}

func decodeBrainGTDToolResult(result map[string]interface{}, target any) error {
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func (a *App) importBrainGTDReviewItems(ctx context.Context, sphere string, items []brainGTDReviewItem) (int, error) {
	imported := 0
	for _, item := range items {
		changed, err := a.upsertBrainGTDReviewItem(ctx, sphere, item)
		if err != nil {
			return imported, err
		}
		if changed {
			imported++
		}
	}
	return imported, nil
}

func (a *App) upsertBrainGTDReviewItem(ctx context.Context, sphere string, item brainGTDReviewItem) (bool, error) {
	provider := brainGTDReviewSource(item)
	sourceRef := brainGTDReviewSourceRef(item)
	if provider == "" || sourceRef == "" {
		return false, nil
	}
	account, err := a.ensureBrainGTDAccount(sphere, provider)
	if err != nil {
		return false, err
	}
	artifact, err := a.upsertBrainGTDArtifact(ctx, account, item, sourceRef)
	if err != nil {
		return false, err
	}
	incoming := brainGTDStoreItem(sphere, provider, sourceRef, item, artifact.ID)
	binding := store.ExternalBinding{
		AccountID:    account.ID,
		Provider:     account.Provider,
		ObjectType:   brainGTDObjectType(item),
		RemoteID:     sourceRef,
		ContainerRef: optionalString(strings.TrimSpace(item.Project)),
	}
	before, err := a.store.GetItemBySource(provider, sourceRef)
	existed := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, err
	}
	if existed {
		if err := a.updateBrainGTDReviewItem(before.ID, incoming); err != nil {
			return false, err
		}
		binding.ItemID = &before.ID
		binding.ArtifactID = &artifact.ID
		if _, err := a.store.UpsertExternalBinding(binding); err != nil {
			return false, err
		}
		return brainGTDReviewChanged(before, incoming), nil
	}
	_, err = tabsync.NewStoreSink(a.store).UpsertItemFromSource(ctx, incoming, binding)
	if err != nil {
		return false, err
	}
	return !existed || brainGTDReviewChanged(before, incoming), nil
}

func (a *App) upsertBrainGTDArtifact(ctx context.Context, account store.ExternalAccount, item brainGTDReviewItem, sourceRef string) (store.Artifact, error) {
	kind := store.ArtifactKindExternalTask
	refPath := (*string)(nil)
	if brainGTDReviewSource(item) == store.ExternalProviderMarkdown {
		kind = store.ArtifactKindMarkdown
		refPath = optionalString(sourceRef)
	}
	title := optionalString(brainGTDReviewTitle(item))
	meta := brainGTDMetaJSON(item)
	artifact := store.Artifact{Kind: kind, RefPath: refPath, Title: title, MetaJSON: &meta}
	return tabsync.NewStoreSink(a.store).UpsertArtifact(ctx, artifact, store.ExternalBinding{
		AccountID:    account.ID,
		Provider:     account.Provider,
		ObjectType:   brainGTDObjectType(item),
		RemoteID:     sourceRef,
		ContainerRef: optionalString(strings.TrimSpace(item.Project)),
	})
}

func brainGTDStoreItem(sphere, provider, sourceRef string, item brainGTDReviewItem, artifactID int64) store.Item {
	followUp := brainGTDDateTime(item.FollowUp, false)
	return store.Item{
		Title:        brainGTDReviewTitle(item),
		Kind:         store.ItemKindAction,
		State:        brainGTDState(item.Status, item.Queue),
		Sphere:       sphere,
		ArtifactID:   &artifactID,
		VisibleAfter: followUp,
		FollowUpAt:   followUp,
		DueAt:        brainGTDDateTime(item.Due, true),
		Source:       &provider,
		SourceRef:    &sourceRef,
	}
}

func brainGTDReviewChanged(existing store.Item, incoming store.Item) bool {
	return existing.Title != incoming.Title ||
		existing.State != incoming.State ||
		optionalStoreString(existing.VisibleAfter) != optionalStoreString(incoming.VisibleAfter) ||
		optionalStoreString(existing.FollowUpAt) != optionalStoreString(incoming.FollowUpAt) ||
		optionalStoreString(existing.DueAt) != optionalStoreString(incoming.DueAt) ||
		optionalStoreString(existing.Source) != optionalStoreString(incoming.Source) ||
		optionalStoreString(existing.SourceRef) != optionalStoreString(incoming.SourceRef)
}

func (a *App) updateBrainGTDReviewItem(id int64, incoming store.Item) error {
	return a.store.UpdateItem(id, store.ItemUpdate{
		Title:        &incoming.Title,
		State:        &incoming.State,
		ArtifactID:   incoming.ArtifactID,
		VisibleAfter: incoming.VisibleAfter,
		FollowUpAt:   incoming.FollowUpAt,
		DueAt:        incoming.DueAt,
	})
}

func (a *App) syncBrainGTDCanonicalBindings(ctx context.Context, sphere string, list brainGTDCommitmentList) (brainGTDSyncResult, error) {
	result := brainGTDSyncResult{}
	bySource := brainGTDBindingIndex(list.Items)
	items, err := a.store.ListItemsFiltered(store.ItemListFilter{Sphere: sphere})
	if err != nil {
		return result, err
	}
	for _, item := range items {
		source := strings.ToLower(strings.TrimSpace(optionalStoreString(item.Source)))
		sourceRef := strings.TrimSpace(optionalStoreString(item.SourceRef))
		if source == "" || source == store.ExternalProviderMarkdown || sourceRef == "" {
			continue
		}
		canonical, ok := bySource[brainGTDSourceKey(source, sourceRef)]
		if !ok {
			continue
		}
		merged, err := a.repointItemToBrainGTD(ctx, item, canonical)
		if err != nil {
			return result, err
		}
		if merged {
			result.Merged++
		} else {
			result.Migrated++
		}
	}
	return result, nil
}

func (a *App) repointItemToBrainGTD(ctx context.Context, item store.Item, canonical brainGTDCommitmentItem) (bool, error) {
	path := strings.TrimSpace(canonical.Path)
	if path == "" {
		return false, nil
	}
	artifact, err := a.upsertCanonicalMarkdownArtifact(ctx, item.Sphere, canonical)
	if err != nil {
		return false, err
	}
	winner, err := a.store.GetItemBySource(store.ExternalProviderMarkdown, path)
	if errors.Is(err, sql.ErrNoRows) {
		return false, a.updateItemToCanonicalMarkdown(item.ID, canonical, artifact.ID, path)
	}
	if err != nil {
		return false, err
	}
	if winner.ID == item.ID {
		return false, nil
	}
	if err := a.moveExternalBindings(item.ID, winner.ID, artifact.ID); err != nil {
		return false, err
	}
	duplicateRef := fmt.Sprintf("%s#merged-%d", path, item.ID)
	return true, a.updateItemAsMergedDuplicate(item.ID, canonical, artifact.ID, duplicateRef)
}

func (a *App) upsertCanonicalMarkdownArtifact(ctx context.Context, sphere string, item brainGTDCommitmentItem) (store.Artifact, error) {
	review := brainGTDReviewItem{
		Source:   store.ExternalProviderMarkdown,
		Title:    item.Title,
		Status:   item.Status,
		Queue:    item.Status,
		Path:     item.Path,
		Project:  item.Project,
		Due:      item.Due,
		FollowUp: item.FollowUp,
	}
	account, err := a.ensureBrainGTDAccount(sphere, store.ExternalProviderMarkdown)
	if err != nil {
		return store.Artifact{}, err
	}
	return a.upsertBrainGTDArtifact(ctx, account, review, item.Path)
}

func (a *App) updateItemToCanonicalMarkdown(id int64, canonical brainGTDCommitmentItem, artifactID int64, path string) error {
	source := store.ExternalProviderMarkdown
	return a.store.UpdateItem(id, store.ItemUpdate{
		Title:        optionalString(brainGTDCommitmentTitle(canonical)),
		State:        optionalString(brainGTDState(canonical.Status, canonical.Status)),
		ArtifactID:   &artifactID,
		VisibleAfter: brainGTDDateTime(canonical.FollowUp, false),
		FollowUpAt:   brainGTDDateTime(canonical.FollowUp, false),
		DueAt:        brainGTDDateTime(canonical.Due, true),
		Source:       &source,
		SourceRef:    &path,
	})
}

func (a *App) updateItemAsMergedDuplicate(id int64, canonical brainGTDCommitmentItem, artifactID int64, sourceRef string) error {
	source := store.ExternalProviderMarkdown
	done := store.ItemStateDone
	title := brainGTDCommitmentTitle(canonical)
	return a.store.UpdateItem(id, store.ItemUpdate{
		Title:      &title,
		State:      &done,
		ArtifactID: &artifactID,
		Source:     &source,
		SourceRef:  &sourceRef,
	})
}

func (a *App) moveExternalBindings(fromItemID, toItemID, artifactID int64) error {
	bindings, err := a.store.GetBindingsByItem(fromItemID)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		if binding.Provider == store.ExternalProviderMarkdown {
			continue
		}
		if _, err := a.store.UpsertExternalBinding(store.ExternalBinding{
			AccountID:       binding.AccountID,
			Provider:        binding.Provider,
			ObjectType:      binding.ObjectType,
			RemoteID:        binding.RemoteID,
			ItemID:          &toItemID,
			ArtifactID:      &artifactID,
			ContainerRef:    binding.ContainerRef,
			RemoteUpdatedAt: binding.RemoteUpdatedAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) ensureBrainGTDAccount(sphere, provider string) (store.ExternalAccount, error) {
	accounts, err := a.store.ListExternalAccountsByProvider(provider)
	if err != nil {
		return store.ExternalAccount{}, err
	}
	for _, account := range accounts {
		if account.Sphere == sphere && account.Enabled {
			return account, nil
		}
	}
	return a.store.CreateExternalAccount(sphere, provider, brainGTDAccountLabel(sphere, provider), nil)
}

func brainGTDBindingIndex(items []brainGTDCommitmentItem) map[string]brainGTDCommitmentItem {
	out := map[string]brainGTDCommitmentItem{}
	for _, item := range items {
		for _, binding := range item.Bindings {
			for _, key := range brainGTDKeysForBinding(binding) {
				out[key] = item
			}
		}
	}
	return out
}

func brainGTDKeysForBinding(binding string) []string {
	provider, ref, ok := strings.Cut(strings.TrimSpace(binding), ":")
	if !ok || provider == "" || ref == "" {
		return nil
	}
	provider = strings.ToLower(provider)
	keys := []string{brainGTDSourceKey(provider, ref)}
	parts := strings.Split(ref, ":")
	last := strings.TrimSpace(parts[len(parts)-1])
	if last != "" {
		keys = append(keys, brainGTDSourceKey(provider, last))
		if provider == store.ExternalProviderTodoist {
			keys = append(keys, brainGTDSourceKey(provider, "task:"+last))
		}
	}
	return keys
}

func brainGTDSourceKey(provider, ref string) string {
	return strings.ToLower(strings.TrimSpace(provider)) + "\x00" + strings.TrimSpace(ref)
}

func brainGTDReviewSource(item brainGTDReviewItem) string {
	source := strings.ToLower(strings.TrimSpace(item.Source))
	switch source {
	case "markdown", "brain", "brain.gtd":
		return store.ExternalProviderMarkdown
	case store.ExternalProviderExchangeEWS, store.ExternalProviderTodoist, store.ExternalProviderGoogleTasks:
		return source
	default:
		return source
	}
}

func brainGTDReviewSourceRef(item brainGTDReviewItem) string {
	for _, value := range []string{item.SourceRef, item.Path} {
		if clean := strings.TrimSpace(value); clean != "" {
			return clean
		}
	}
	source := brainGTDReviewSource(item)
	id := strings.TrimSpace(item.ID)
	if strings.HasPrefix(id, source+":") {
		return strings.TrimPrefix(id, source+":")
	}
	return id
}

func brainGTDReviewTitle(item brainGTDReviewItem) string {
	if title := strings.TrimSpace(item.Title); title != "" {
		return title
	}
	if ref := brainGTDReviewSourceRef(item); ref != "" {
		return ref
	}
	return "Untitled GTD item"
}

func brainGTDCommitmentTitle(item brainGTDCommitmentItem) string {
	if title := strings.TrimSpace(item.Title); title != "" {
		return title
	}
	return strings.TrimSpace(item.Path)
}

func brainGTDObjectType(item brainGTDReviewItem) string {
	if brainGTDReviewSource(item) == store.ExternalProviderMarkdown {
		return "commitment"
	}
	return "task"
}

func brainGTDState(status, queue string) string {
	switch strings.ToLower(strings.TrimSpace(firstNonEmptyString(queue, status))) {
	case "done", "closed":
		return store.ItemStateDone
	case store.ItemStateWaiting:
		return store.ItemStateWaiting
	case store.ItemStateDeferred:
		return store.ItemStateDeferred
	case store.ItemStateSomeday:
		return store.ItemStateSomeday
	case "maybe_stale", store.ItemStateReview:
		return store.ItemStateReview
	case store.ItemStateInbox:
		return store.ItemStateInbox
	default:
		return store.ItemStateNext
	}
}

func brainGTDDateTime(raw string, endOfDay bool) *string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		out := parsed.UTC().Format(time.RFC3339)
		return &out
	}
	if len(value) >= len("2006-01-02") {
		if parsed, err := time.Parse("2006-01-02", value[:10]); err == nil {
			if endOfDay {
				parsed = parsed.Add(24*time.Hour - time.Second)
			}
			out := parsed.UTC().Format(time.RFC3339)
			return &out
		}
	}
	return nil
}

func brainGTDMetaJSON(item brainGTDReviewItem) string {
	payload := map[string]string{
		"source":     strings.TrimSpace(item.Source),
		"source_ref": strings.TrimSpace(item.SourceRef),
		"path":       strings.TrimSpace(item.Path),
		"status":     strings.TrimSpace(item.Status),
		"queue":      strings.TrimSpace(item.Queue),
		"project":    strings.TrimSpace(item.Project),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		log.Printf("brain gtd meta json: %v", err)
		return "{}"
	}
	return string(body)
}

func brainGTDAccountLabel(sphere, provider string) string {
	switch provider {
	case store.ExternalProviderMarkdown:
		return "GTD Markdown " + sphere
	case store.ExternalProviderGoogleTasks:
		return "Google Tasks " + sphere
	default:
		return store.ExternalProviderDisplayName(provider) + " " + sphere
	}
}

func optionalString(value string) *string {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return nil
	}
	return &clean
}
