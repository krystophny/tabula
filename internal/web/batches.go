package web

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

const batchProgressEventType = "batch_progress"

func parseBatchIDParam(r *http.Request) (int64, error) {
	return parseURLInt64Param(r, "batch_id")
}

func parseBatchWorkspaceIDQuery(r *http.Request) (*int64, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	if raw == "" {
		return nil, nil
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return nil, errors.New("workspace_id must be a positive integer")
	}
	return &value, nil
}

func (a *App) handleBatchList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspaceID, err := parseBatchWorkspaceIDQuery(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	runs, err := a.store.ListBatchRuns(workspaceID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"batch_runs": runs,
	})
}

func (a *App) handleBatchGet(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	batchID, err := parseBatchIDParam(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	run, err := a.store.GetBatchRun(batchID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	items, err := a.store.ListBatchRunItems(batchID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"batch_run": run,
		"items":     items,
	})
}

func (a *App) handleBatchArtifact(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	batchID, err := parseBatchIDParam(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	run, err := a.store.GetBatchRun(batchID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	items, err := a.store.ListBatchRunItems(batchID)
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"batch_run": run,
		"markdown":  renderBatchArtifactMarkdown(run, items),
	})
}

func renderBatchArtifactMarkdown(run store.BatchRun, items []store.BatchRunItem) string {
	var b strings.Builder
	b.WriteString("# Batch Progress\n\n")
	fmt.Fprintf(&b, "- Workspace ID: %d\n", run.WorkspaceID)
	fmt.Fprintf(&b, "- Status: %s\n", strings.TrimSpace(run.Status))
	fmt.Fprintf(&b, "- Started: %s\n", strings.TrimSpace(run.StartedAt))
	if run.FinishedAt != nil && strings.TrimSpace(*run.FinishedAt) != "" {
		fmt.Fprintf(&b, "- Finished: %s\n", strings.TrimSpace(*run.FinishedAt))
	}
	b.WriteString("\n")
	if len(items) == 0 {
		b.WriteString("_No items recorded for this batch._\n")
		return b.String()
	}
	b.WriteString("| Issue | Status | PR | Notes |\n")
	b.WriteString("| --- | --- | --- | --- |\n")
	for _, item := range items {
		fmt.Fprintf(
			&b,
			"| %s | %s | %s | %s |\n",
			batchItemLabel(item),
			markdownTableCell(item.Status),
			batchItemPRCell(item),
			batchItemNotesCell(item),
		)
	}
	return b.String()
}

func batchItemLabel(item store.BatchRunItem) string {
	if item.ItemTitle != nil && strings.TrimSpace(*item.ItemTitle) != "" {
		return markdownTableCell(*item.ItemTitle)
	}
	return fmt.Sprintf("Item %d", item.ItemID)
}

func batchItemPRCell(item store.BatchRunItem) string {
	if item.PRNumber == nil {
		return ""
	}
	label := fmt.Sprintf("#%d", *item.PRNumber)
	if item.PRURL != nil && strings.TrimSpace(*item.PRURL) != "" {
		return fmt.Sprintf("[%s](%s)", label, strings.TrimSpace(*item.PRURL))
	}
	return label
}

func batchItemNotesCell(item store.BatchRunItem) string {
	if item.ErrorMsg == nil {
		return ""
	}
	return markdownTableCell(*item.ErrorMsg)
}

func markdownTableCell(value string) string {
	clean := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	clean = strings.ReplaceAll(clean, "|", "\\|")
	return clean
}

func (a *App) recordBatchRunStatus(batchID int64, status string, finishedAt *string) (store.BatchRun, error) {
	run, err := a.store.SetBatchRunStatus(batchID, status, finishedAt)
	if err != nil {
		return store.BatchRun{}, err
	}
	a.broadcastBatchProgress(run, nil)
	return run, nil
}

func (a *App) recordBatchRunItemUpdate(batchID, itemID int64, update store.BatchRunItemUpdate) (store.BatchRunItem, error) {
	item, err := a.store.UpsertBatchRunItem(batchID, itemID, update)
	if err != nil {
		return store.BatchRunItem{}, err
	}
	run, err := a.store.GetBatchRun(batchID)
	if err != nil {
		return store.BatchRunItem{}, err
	}
	a.broadcastBatchProgress(run, &item)
	return item, nil
}

func (a *App) broadcastBatchProgress(run store.BatchRun, item *store.BatchRunItem) {
	payload := map[string]any{
		"type":         batchProgressEventType,
		"batch":        run,
		"batch_id":     run.ID,
		"workspace_id": run.WorkspaceID,
	}
	if item != nil {
		payload["item"] = item
		payload["item_id"] = item.ItemID
	}
	a.hub.forEachChatConn(func(conn *chatWSConn) {
		_ = conn.writeJSON(payload)
	})
}
