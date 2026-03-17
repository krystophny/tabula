package web

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/krystophny/tabura/internal/email"
	"github.com/krystophny/tabura/internal/store"
)

type mailActionRequest struct {
	Action     string   `json:"action"`
	MessageID  string   `json:"message_id,omitempty"`
	MessageIDs []string `json:"message_ids,omitempty"`
	Folder     string   `json:"folder,omitempty"`
	Label      string   `json:"label,omitempty"`
	Archive    *bool    `json:"archive,omitempty"`
}

func (a *App) handleMailAccountList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	accounts, err := a.store.ListExternalAccounts("")
	if err != nil {
		writeDomainStoreError(w, err)
		return
	}
	out := make([]store.ExternalAccount, 0, len(accounts))
	for _, account := range accounts {
		if account.Enabled && store.IsEmailProvider(account.Provider) {
			out = append(out, account)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Sphere == out[j].Sphere {
			return strings.ToLower(out[i].AccountName) < strings.ToLower(out[j].AccountName)
		}
		return out[i].Sphere < out[j].Sphere
	})
	writeAPIData(w, http.StatusOK, map[string]any{
		"accounts": out,
		"count":    len(out),
	})
}

func (a *App) handleMailLabelList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	account, provider, err := a.emailProviderForRoute(r.Context(), r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer provider.Close()
	if err := a.guardMailAccountBackoff(account); err != nil {
		writeAPIError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	labels, err := provider.ListLabels(r.Context())
	if err != nil {
		a.writeMailProviderError(w, account, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"account": account,
		"labels":  labels,
		"count":   len(labels),
	})
}

func (a *App) handleMailMessageList(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	account, provider, err := a.emailProviderForRoute(r.Context(), r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer provider.Close()
	if err := a.guardMailAccountBackoff(account); err != nil {
		writeAPIError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	opts, pageToken, err := mailSearchOptionsFromRequest(r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	format, err := mailMessageFormatFromRequest(r, "full")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	ids, nextPageToken, err := a.mailMessageIDsForRequest(r.Context(), provider, opts, pageToken)
	if err != nil {
		if isMailAPIRequestError(err) {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		a.writeMailProviderError(w, account, err)
		return
	}
	messages, err := provider.GetMessages(r.Context(), ids, format)
	if err != nil {
		a.writeMailProviderError(w, account, err)
		return
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Date.After(messages[j].Date)
	})
	writeAPIData(w, http.StatusOK, map[string]any{
		"account":         account,
		"messages":        messages,
		"count":           len(messages),
		"format":          format,
		"next_page_token": nextPageToken,
		"page_token":      pageToken,
	})
}

func (a *App) handleMailMessageGet(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	account, provider, err := a.emailProviderForRoute(r.Context(), r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer provider.Close()
	if err := a.guardMailAccountBackoff(account); err != nil {
		writeAPIError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	messageID := strings.TrimSpace(chi.URLParam(r, "message_id"))
	if messageID == "" {
		writeAPIError(w, http.StatusBadRequest, "message_id is required")
		return
	}
	format, err := mailMessageFormatFromRequest(r, "full")
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	message, err := provider.GetMessage(r.Context(), messageID, format)
	if err != nil {
		a.writeMailProviderError(w, account, err)
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"account": account,
		"message": message,
		"format":  format,
	})
}

func (a *App) handleMailAction(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	account, provider, err := a.emailProviderForRoute(r.Context(), r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	defer provider.Close()
	if err := a.guardMailAccountBackoff(account); err != nil {
		writeAPIError(w, http.StatusTooManyRequests, err.Error())
		return
	}
	var req mailActionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	messageIDs := compactStringList(append(req.MessageIDs, req.MessageID))
	if len(messageIDs) == 0 {
		writeAPIError(w, http.StatusBadRequest, "message_ids are required")
		return
	}
	action := strings.TrimSpace(strings.ToLower(req.Action))
	if action == "" {
		writeAPIError(w, http.StatusBadRequest, "action is required")
		return
	}
	result, err := a.executeMailAction(r.Context(), account, provider, mailActionCommand{
		Action:     action,
		MessageIDs: messageIDs,
		Folder:     strings.TrimSpace(req.Folder),
		Label:      strings.TrimSpace(req.Label),
		Archive:    req.Archive,
	})
	if err != nil {
		status := http.StatusBadRequest
		if !isMailAPIRequestError(err) {
			a.writeMailProviderError(w, account, err)
			return
		}
		writeAPIError(w, status, err.Error())
		return
	}
	writeAPIData(w, http.StatusOK, map[string]any{
		"account":     account,
		"action":      action,
		"message_ids": messageIDs,
		"succeeded":   result.Succeeded,
		"logs":        result.Logs,
	})
}

func isMailAPIRequestError(err error) bool {
	var reqErr *requestError
	return errors.As(err, &reqErr)
}

func (a *App) mailMessageIDsForRequest(ctx context.Context, provider email.EmailProvider, opts email.SearchOptions, pageToken string) ([]string, string, error) {
	if pager, ok := provider.(email.MessagePageProvider); ok {
		page, err := pager.ListMessagesPage(ctx, opts, pageToken)
		if err != nil {
			return nil, "", err
		}
		return page.IDs, strings.TrimSpace(page.NextPageToken), nil
	}
	if strings.TrimSpace(pageToken) != "" {
		return nil, "", errBadRequest("page_token is not supported for this provider")
	}
	ids, err := provider.ListMessages(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	return ids, "", nil
}

func mailSearchOptionsFromRequest(r *http.Request) (email.SearchOptions, string, error) {
	query := r.URL.Query()
	opts := email.DefaultSearchOptions()
	opts.Folder = strings.TrimSpace(query.Get("folder"))
	opts.Text = strings.TrimSpace(query.Get("text"))
	opts.Subject = strings.TrimSpace(query.Get("subject"))
	opts.From = strings.TrimSpace(query.Get("from"))
	opts.To = strings.TrimSpace(query.Get("to"))
	opts.IncludeSpamTrash = parseBoolString(query.Get("include_spam_trash"), false)
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return email.SearchOptions{}, "", errBadRequest("limit must be a positive integer")
		}
		opts.MaxResults = int64(value)
	}
	if raw := strings.TrimSpace(query.Get("days")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return email.SearchOptions{}, "", errBadRequest("days must be a positive integer")
		}
		opts = opts.WithLastDays(value)
	}
	if raw := strings.TrimSpace(query.Get("after")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return email.SearchOptions{}, "", errBadRequest("after must be RFC3339")
		}
		opts.After = value
	}
	if raw := strings.TrimSpace(query.Get("before")); raw != "" {
		value, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			return email.SearchOptions{}, "", errBadRequest("before must be RFC3339")
		}
		opts.Before = value
	}
	if raw := strings.TrimSpace(query.Get("has_attachment")); raw != "" {
		value := parseBoolString(raw, false)
		opts.HasAttachment = &value
	}
	if raw := strings.TrimSpace(query.Get("is_read")); raw != "" {
		value := parseBoolString(raw, false)
		opts.IsRead = &value
	}
	if raw := strings.TrimSpace(query.Get("is_flagged")); raw != "" {
		value := parseBoolString(raw, false)
		opts.IsFlagged = &value
	}
	return opts, strings.TrimSpace(query.Get("page_token")), nil
}

func mailMessageFormatFromRequest(r *http.Request, defaultFormat string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = strings.ToLower(strings.TrimSpace(defaultFormat))
	}
	switch format {
	case "", "full":
		return "full", nil
	case "metadata":
		return "metadata", nil
	default:
		return "", errBadRequest("format must be full or metadata")
	}
}

type mailActionApplyResult struct {
	Count       int
	Resolutions []email.ActionResolution
}

func applyMailAction(ctx context.Context, account store.ExternalAccount, provider email.EmailProvider, action string, messageIDs []string, folder, label string, archive *bool) (mailActionApplyResult, error) {
	switch action {
	case "mark_read":
		count, err := provider.MarkRead(ctx, messageIDs)
		return mailActionApplyResult{Count: count}, err
	case "mark_unread":
		count, err := provider.MarkUnread(ctx, messageIDs)
		return mailActionApplyResult{Count: count}, err
	case "archive":
		if resolvedProvider, ok := provider.(email.ResolvedArchiveProvider); ok {
			resolutions, err := resolvedProvider.ArchiveResolved(ctx, messageIDs)
			return mailActionApplyResult{Count: len(resolutions), Resolutions: resolutions}, err
		}
		count, err := provider.Archive(ctx, messageIDs)
		return mailActionApplyResult{Count: count}, err
	case "move_to_inbox":
		if resolvedProvider, ok := provider.(email.ResolvedMoveToInboxProvider); ok {
			resolutions, err := resolvedProvider.MoveToInboxResolved(ctx, messageIDs)
			return mailActionApplyResult{Count: len(resolutions), Resolutions: resolutions}, err
		}
		count, err := provider.MoveToInbox(ctx, messageIDs)
		return mailActionApplyResult{Count: count}, err
	case "trash":
		if resolvedProvider, ok := provider.(email.ResolvedTrashProvider); ok {
			resolutions, err := resolvedProvider.TrashResolved(ctx, messageIDs)
			return mailActionApplyResult{Count: len(resolutions), Resolutions: resolutions}, err
		}
		count, err := provider.Trash(ctx, messageIDs)
		return mailActionApplyResult{Count: count}, err
	case "delete":
		count, err := provider.Delete(ctx, messageIDs)
		return mailActionApplyResult{Count: count}, err
	case "move_to_folder":
		folderProvider, ok := provider.(email.NamedFolderProvider)
		if !ok {
			return mailActionApplyResult{}, errBadRequest("move_to_folder is not supported for this account")
		}
		if strings.TrimSpace(folder) == "" {
			return mailActionApplyResult{}, errBadRequest("folder is required")
		}
		if resolvedProvider, ok := provider.(email.ResolvedNamedFolderProvider); ok {
			resolutions, err := resolvedProvider.MoveToFolderResolved(ctx, messageIDs, folder)
			return mailActionApplyResult{Count: len(resolutions), Resolutions: resolutions}, err
		}
		count, err := folderProvider.MoveToFolder(ctx, messageIDs, folder)
		return mailActionApplyResult{Count: count}, err
	case "apply_label":
		labelProvider, ok := provider.(email.NamedLabelProvider)
		if !ok {
			return mailActionApplyResult{}, errBadRequest("apply_label is not supported for this account")
		}
		if strings.TrimSpace(label) == "" {
			return mailActionApplyResult{}, errBadRequest("label is required")
		}
		archiveValue := false
		if archive != nil {
			archiveValue = *archive
		}
		count, err := labelProvider.ApplyNamedLabel(ctx, messageIDs, label, archiveValue)
		return mailActionApplyResult{Count: count}, err
	case "archive_label":
		if strings.TrimSpace(label) == "" {
			return mailActionApplyResult{}, errBadRequest("label is required")
		}
		if folderProvider, ok := provider.(email.NamedFolderProvider); ok {
			target := label
			if account.Provider == store.ExternalProviderExchangeEWS {
				target = "Archive/" + label
			}
			if resolvedProvider, ok := provider.(email.ResolvedNamedFolderProvider); ok {
				resolutions, err := resolvedProvider.MoveToFolderResolved(ctx, messageIDs, target)
				return mailActionApplyResult{Count: len(resolutions), Resolutions: resolutions}, err
			}
			count, err := folderProvider.MoveToFolder(ctx, messageIDs, target)
			return mailActionApplyResult{Count: count}, err
		}
		if labelProvider, ok := provider.(email.NamedLabelProvider); ok {
			count, err := labelProvider.ApplyNamedLabel(ctx, messageIDs, label, true)
			return mailActionApplyResult{Count: count}, err
		}
		if resolvedProvider, ok := provider.(email.ResolvedArchiveProvider); ok {
			resolutions, err := resolvedProvider.ArchiveResolved(ctx, messageIDs)
			return mailActionApplyResult{Count: len(resolutions), Resolutions: resolutions}, err
		}
		count, err := provider.Archive(ctx, messageIDs)
		return mailActionApplyResult{Count: count}, err
	default:
		return mailActionApplyResult{}, errBadRequest("unsupported action")
	}
}
