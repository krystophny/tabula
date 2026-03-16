package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/email"
	"github.com/krystophny/tabura/internal/store"
)

type mailSyncAccountConfig struct {
	Host            string `json:"host"`
	Port            int    `json:"port"`
	Username        string `json:"username"`
	TLS             bool   `json:"tls"`
	StartTLS        bool   `json:"starttls"`
	FromAddress     string `json:"from_address"`
	TokenPath       string `json:"token_path"`
	TokenFile       string `json:"token_file"`
	CredentialsPath string `json:"credentials_path"`
	CredentialsFile string `json:"credentials_file"`
}

func (s *Server) mailAccountList(args map[string]interface{}) (map[string]interface{}, error) {
	st, err := s.requireStore()
	if err != nil {
		return nil, err
	}
	sphere := strings.TrimSpace(strArg(args, "sphere"))
	accounts, err := st.ListExternalAccounts(sphere)
	if err != nil {
		return nil, err
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
	return map[string]interface{}{
		"accounts": out,
		"count":    len(out),
	}, nil
}

func (s *Server) mailLabelList(args map[string]interface{}) (map[string]interface{}, error) {
	account, provider, err := s.mailProviderForTool(args)
	if err != nil {
		return nil, err
	}
	defer provider.Close()
	labels, err := provider.ListLabels(context.Background())
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"account": account,
		"labels":  labels,
		"count":   len(labels),
	}, nil
}

func (s *Server) mailMessageList(args map[string]interface{}) (map[string]interface{}, error) {
	account, provider, err := s.mailProviderForTool(args)
	if err != nil {
		return nil, err
	}
	defer provider.Close()
	opts, pageToken, err := mailSearchOptionsFromArgs(args)
	if err != nil {
		return nil, err
	}
	ids, nextPageToken, err := listMailMessageIDs(context.Background(), provider, opts, pageToken)
	if err != nil {
		return nil, err
	}
	messages, err := provider.GetMessages(context.Background(), ids, "full")
	if err != nil {
		return nil, err
	}
	sort.Slice(messages, func(i, j int) bool {
		return messages[i].Date.After(messages[j].Date)
	})
	return map[string]interface{}{
		"account":         account,
		"messages":        messages,
		"count":           len(messages),
		"page_token":      pageToken,
		"next_page_token": nextPageToken,
	}, nil
}

func (s *Server) mailMessageGet(args map[string]interface{}) (map[string]interface{}, error) {
	account, provider, err := s.mailProviderForTool(args)
	if err != nil {
		return nil, err
	}
	defer provider.Close()
	messageID := strings.TrimSpace(strArg(args, "message_id"))
	if messageID == "" {
		return nil, fmt.Errorf("message_id is required")
	}
	message, err := provider.GetMessage(context.Background(), messageID, "full")
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"account": account,
		"message": message,
	}, nil
}

func (s *Server) mailAction(args map[string]interface{}) (map[string]interface{}, error) {
	account, provider, err := s.mailProviderForTool(args)
	if err != nil {
		return nil, err
	}
	defer provider.Close()
	action := strings.TrimSpace(strings.ToLower(strArg(args, "action")))
	if action == "" {
		return nil, fmt.Errorf("action is required")
	}
	messageIDs := mailMessageIDsArg(args)
	if len(messageIDs) == 0 {
		return nil, fmt.Errorf("message_ids are required")
	}
	folder := strings.TrimSpace(strArg(args, "folder"))
	label := strings.TrimSpace(strArg(args, "label"))
	var archive *bool
	if value, ok := args["archive"].(bool); ok {
		archive = &value
	}
	count, err := applyMailActionGeneric(context.Background(), account, provider, action, messageIDs, folder, label, archive)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"account":     account,
		"action":      action,
		"message_ids": messageIDs,
		"succeeded":   count,
	}, nil
}

func (s *Server) mailServerFilterList(args map[string]interface{}) (map[string]interface{}, error) {
	account, provider, err := s.mailProviderForTool(args)
	if err != nil {
		return nil, err
	}
	defer provider.Close()
	filterProvider, ok := provider.(email.ServerFilterProvider)
	if !ok {
		return nil, fmt.Errorf("server filters are not supported for this account")
	}
	filters, err := filterProvider.ListServerFilters(context.Background())
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"account":      account,
		"capabilities": filterProvider.ServerFilterCapabilities(),
		"filters":      filters,
		"count":        len(filters),
	}, nil
}

func (s *Server) mailServerFilterUpsert(args map[string]interface{}) (map[string]interface{}, error) {
	account, provider, err := s.mailProviderForTool(args)
	if err != nil {
		return nil, err
	}
	defer provider.Close()
	filterProvider, ok := provider.(email.ServerFilterProvider)
	if !ok {
		return nil, fmt.Errorf("server filters are not supported for this account")
	}
	raw, ok := args["filter"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("filter is required")
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var filter email.ServerFilter
	if err := json.Unmarshal(data, &filter); err != nil {
		return nil, fmt.Errorf("invalid filter: %w", err)
	}
	if overrideID := strings.TrimSpace(strArg(args, "filter_id")); overrideID != "" {
		filter.ID = overrideID
	}
	if strings.TrimSpace(filter.Name) == "" {
		return nil, fmt.Errorf("filter name is required")
	}
	saved, err := filterProvider.UpsertServerFilter(context.Background(), filter)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"account": account,
		"filter":  saved,
	}, nil
}

func (s *Server) mailServerFilterDelete(args map[string]interface{}) (map[string]interface{}, error) {
	account, provider, err := s.mailProviderForTool(args)
	if err != nil {
		return nil, err
	}
	defer provider.Close()
	filterProvider, ok := provider.(email.ServerFilterProvider)
	if !ok {
		return nil, fmt.Errorf("server filters are not supported for this account")
	}
	filterID := strings.TrimSpace(strArg(args, "filter_id"))
	if filterID == "" {
		return nil, fmt.Errorf("filter_id is required")
	}
	if err := filterProvider.DeleteServerFilter(context.Background(), filterID); err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"account":   account,
		"filter_id": filterID,
		"deleted":   true,
	}, nil
}

func (s *Server) mailProviderForTool(args map[string]interface{}) (store.ExternalAccount, email.EmailProvider, error) {
	st, err := s.requireStore()
	if err != nil {
		return store.ExternalAccount{}, nil, err
	}
	accountID, err := int64Arg(args, "account_id")
	if err != nil {
		return store.ExternalAccount{}, nil, err
	}
	account, err := st.GetExternalAccount(accountID)
	if err != nil {
		return store.ExternalAccount{}, nil, err
	}
	provider, err := s.emailProviderForAccount(context.Background(), account)
	if err != nil {
		return store.ExternalAccount{}, nil, err
	}
	return account, provider, nil
}

func (s *Server) emailProviderForAccount(ctx context.Context, account store.ExternalAccount) (email.EmailProvider, error) {
	if s.newEmailProvider != nil {
		return s.newEmailProvider(ctx, account)
	}
	cfg, err := decodeMailSyncAccountConfig(account)
	if err != nil {
		return nil, err
	}
	switch account.Provider {
	case store.ExternalProviderGmail:
		return email.NewGmailWithFiles(gmailCredentialsPathForAccount(cfg), gmailTokenPathForAccount(account, cfg))
	case store.ExternalProviderIMAP:
		if cfg.Host == "" {
			return nil, fmt.Errorf("imap host is required")
		}
		if cfg.Username == "" {
			return nil, fmt.Errorf("imap username is required")
		}
		password, _, err := s.store.ResolveExternalAccountPasswordForAccount(ctx, account)
		if err != nil {
			return nil, err
		}
		useTLS := cfg.TLS || cfg.Port == 993
		return email.NewIMAPClient(account.AccountName, cfg.Host, cfg.Port, cfg.Username, password, useTLS, cfg.StartTLS), nil
	case store.ExternalProviderExchange:
		exchangeCfg, err := decodeExchangeAccountConfig(account)
		if err != nil {
			return nil, err
		}
		return email.NewExchangeMailProvider(exchangeCfg)
	case store.ExternalProviderExchangeEWS:
		ewsCfg, err := decodeExchangeEWSAccountConfig(account)
		if err != nil {
			return nil, err
		}
		password, _, err := s.store.ResolveExternalAccountPasswordForAccount(ctx, account)
		if err != nil {
			return nil, err
		}
		ewsCfg.Password = password
		return email.NewExchangeEWSMailProvider(ewsCfg)
	default:
		return nil, fmt.Errorf("email provider %s is not supported", account.Provider)
	}
}

func decodeMailSyncAccountConfig(account store.ExternalAccount) (mailSyncAccountConfig, error) {
	var cfg mailSyncAccountConfig
	raw := strings.TrimSpace(account.ConfigJSON)
	if raw == "" || raw == "{}" {
		return cfg, nil
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return mailSyncAccountConfig{}, fmt.Errorf("decode %s mail config: %w", account.Provider, err)
	}
	cfg.Host = strings.TrimSpace(cfg.Host)
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.FromAddress = strings.TrimSpace(cfg.FromAddress)
	cfg.TokenPath = strings.TrimSpace(cfg.TokenPath)
	cfg.TokenFile = strings.TrimSpace(cfg.TokenFile)
	cfg.CredentialsPath = strings.TrimSpace(cfg.CredentialsPath)
	cfg.CredentialsFile = strings.TrimSpace(cfg.CredentialsFile)
	return cfg, nil
}

func decodeExchangeAccountConfig(account store.ExternalAccount) (email.ExchangeConfig, error) {
	config := map[string]interface{}{}
	raw := strings.TrimSpace(account.ConfigJSON)
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			return email.ExchangeConfig{}, fmt.Errorf("decode exchange account config: %w", err)
		}
	}
	return email.ExchangeConfigFromMap(account.AccountName, config)
}

func decodeExchangeEWSAccountConfig(account store.ExternalAccount) (email.ExchangeEWSConfig, error) {
	config := map[string]interface{}{}
	raw := strings.TrimSpace(account.ConfigJSON)
	if raw != "" && raw != "{}" {
		if err := json.Unmarshal([]byte(raw), &config); err != nil {
			return email.ExchangeEWSConfig{}, fmt.Errorf("decode exchange ews account config: %w", err)
		}
	}
	return email.ExchangeEWSConfigFromMap(account.AccountName, config)
}

func mailSyncConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".tabura"
	}
	return filepath.Join(home, ".config", "tabura")
}

func emailConfigPath(configDir, explicitPath, fileName string) string {
	switch {
	case strings.TrimSpace(explicitPath) != "":
		clean := filepath.Clean(strings.TrimSpace(explicitPath))
		if filepath.IsAbs(clean) {
			return clean
		}
		return filepath.Join(configDir, clean)
	case strings.TrimSpace(fileName) != "":
		return filepath.Join(configDir, strings.TrimSpace(fileName))
	default:
		return ""
	}
}

func gmailTokenPathForAccount(account store.ExternalAccount, cfg mailSyncAccountConfig) string {
	configDir := mailSyncConfigDir()
	if path := emailConfigPath(configDir, cfg.TokenPath, ""); path != "" {
		return path
	}
	if strings.TrimSpace(cfg.TokenFile) != "" {
		return filepath.Join(configDir, "tokens", strings.TrimSpace(cfg.TokenFile))
	}
	return store.ExternalAccountTokenPath(configDir, account.Provider, account.AccountName)
}

func gmailCredentialsPathForAccount(cfg mailSyncAccountConfig) string {
	configDir := mailSyncConfigDir()
	if path := emailConfigPath(configDir, cfg.CredentialsPath, cfg.CredentialsFile); path != "" {
		return path
	}
	return filepath.Join(configDir, "gmail_credentials.json")
}

func mailSearchOptionsFromArgs(args map[string]interface{}) (email.SearchOptions, string, error) {
	opts := email.DefaultSearchOptions()
	opts.Folder = strings.TrimSpace(strArg(args, "folder"))
	opts.Text = strings.TrimSpace(strArg(args, "text"))
	opts.Subject = strings.TrimSpace(strArg(args, "subject"))
	opts.From = strings.TrimSpace(strArg(args, "from"))
	opts.To = strings.TrimSpace(strArg(args, "to"))
	if raw, ok := optionalStringArg(args, "limit"); ok && raw != nil {
		value, err := strconv.Atoi(*raw)
		if err != nil || value <= 0 {
			return email.SearchOptions{}, "", fmt.Errorf("limit must be a positive integer")
		}
		opts.MaxResults = int64(value)
	}
	if raw, ok := args["limit"].(float64); ok && raw > 0 {
		opts.MaxResults = int64(raw)
	}
	if raw, ok := args["days"].(float64); ok && raw > 0 {
		opts = opts.WithLastDays(int(raw))
	}
	if raw, ok := optionalStringArg(args, "after"); ok && raw != nil && *raw != "" {
		value, err := time.Parse(time.RFC3339, *raw)
		if err != nil {
			return email.SearchOptions{}, "", fmt.Errorf("after must be RFC3339")
		}
		opts.After = value
	}
	if raw, ok := optionalStringArg(args, "before"); ok && raw != nil && *raw != "" {
		value, err := time.Parse(time.RFC3339, *raw)
		if err != nil {
			return email.SearchOptions{}, "", fmt.Errorf("before must be RFC3339")
		}
		opts.Before = value
	}
	if value, ok := args["include_spam_trash"].(bool); ok {
		opts.IncludeSpamTrash = value
	}
	if value, ok := args["has_attachment"].(bool); ok {
		opts.HasAttachment = &value
	}
	if value, ok := args["is_read"].(bool); ok {
		opts.IsRead = &value
	}
	if value, ok := args["is_flagged"].(bool); ok {
		opts.IsFlagged = &value
	}
	return opts, strings.TrimSpace(strArg(args, "page_token")), nil
}

func listMailMessageIDs(ctx context.Context, provider email.EmailProvider, opts email.SearchOptions, pageToken string) ([]string, string, error) {
	if pageToken != "" {
		pager, ok := provider.(email.MessagePageProvider)
		if !ok {
			return nil, "", fmt.Errorf("page_token is not supported for this provider")
		}
		page, err := pager.ListMessagesPage(ctx, opts, pageToken)
		if err != nil {
			return nil, "", err
		}
		return page.IDs, strings.TrimSpace(page.NextPageToken), nil
	}
	ids, err := provider.ListMessages(ctx, opts)
	if err != nil {
		return nil, "", err
	}
	return ids, "", nil
}

func mailMessageIDsArg(args map[string]interface{}) []string {
	values := []string{}
	if raw, ok := args["message_ids"].([]interface{}); ok {
		for _, value := range raw {
			text, ok := value.(string)
			if ok {
				values = append(values, text)
			}
		}
	}
	if raw, ok := args["message_ids"].([]string); ok {
		values = append(values, raw...)
	}
	if raw := strings.TrimSpace(strArg(args, "message_id")); raw != "" {
		values = append(values, raw)
	}
	return compactStringList(values)
}

func applyMailActionGeneric(ctx context.Context, account store.ExternalAccount, provider email.EmailProvider, action string, messageIDs []string, folder, label string, archive *bool) (int, error) {
	switch action {
	case "mark_read":
		return provider.MarkRead(ctx, messageIDs)
	case "mark_unread":
		return provider.MarkUnread(ctx, messageIDs)
	case "archive":
		return provider.Archive(ctx, messageIDs)
	case "move_to_inbox":
		return provider.MoveToInbox(ctx, messageIDs)
	case "trash":
		return provider.Trash(ctx, messageIDs)
	case "delete":
		return provider.Delete(ctx, messageIDs)
	case "move_to_folder":
		folderProvider, ok := provider.(email.NamedFolderProvider)
		if !ok {
			return 0, fmt.Errorf("move_to_folder is not supported for this account")
		}
		if folder == "" {
			return 0, fmt.Errorf("folder is required")
		}
		return folderProvider.MoveToFolder(ctx, messageIDs, folder)
	case "apply_label":
		labelProvider, ok := provider.(email.NamedLabelProvider)
		if !ok {
			return 0, fmt.Errorf("apply_label is not supported for this account")
		}
		if label == "" {
			return 0, fmt.Errorf("label is required")
		}
		archiveValue := false
		if archive != nil {
			archiveValue = *archive
		}
		return labelProvider.ApplyNamedLabel(ctx, messageIDs, label, archiveValue)
	case "archive_label":
		if label == "" {
			return 0, fmt.Errorf("label is required")
		}
		if folderProvider, ok := provider.(email.NamedFolderProvider); ok {
			target := label
			if account.Provider == store.ExternalProviderExchangeEWS {
				target = "Archive/" + label
			}
			return folderProvider.MoveToFolder(ctx, messageIDs, target)
		}
		if labelProvider, ok := provider.(email.NamedLabelProvider); ok {
			return labelProvider.ApplyNamedLabel(ctx, messageIDs, label, true)
		}
		return provider.Archive(ctx, messageIDs)
	default:
		return 0, fmt.Errorf("unsupported action")
	}
}

func compactStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		clean := strings.TrimSpace(value)
		if clean == "" {
			continue
		}
		key := strings.ToLower(clean)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, clean)
	}
	return out
}
