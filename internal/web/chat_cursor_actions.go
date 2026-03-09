package web

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/krystophny/tabura/internal/store"
)

var titledItemInboxCommandPattern = regexp.MustCompile(`(?i)^(?:move|bring|put)\s+(?:the\s+)?(?:item|mail|message)\s+(?:at\s+line\s+\d+\s+of\s+)?["“]?(.+?)["”]?\s+back\s+to\s+(?:the\s+)?inbox(?:\s*[.!?])?$`)

func parseInlineCursorIntent(text string, cursor *chatCursorContext) *SystemAction {
	cursor = normalizeChatCursorContext(cursor)
	if cursor == nil {
		return nil
	}
	normalized := normalizeItemCommandText(text)
	if normalized == "" {
		return nil
	}

	if cursor.hasPointedItem() {
		view := firstNonEmptyCursorText(cursor.View, cursor.ItemState)
		if match := itemDelegatePattern.FindStringSubmatch(strings.TrimSpace(text)); len(match) == 2 {
			if actor := cleanActorReference(match[1]); actor != "" {
				return &SystemAction{
					Action: "cursor_triage_item",
					Params: map[string]interface{}{
						"item_id":       cursor.ItemID,
						"triage_action": "delegate",
						"actor":         actor,
						"view":          view,
					},
				}
			}
		}
		switch normalized {
		case "open this", "open this item", "show this", "show this item":
			return &SystemAction{
				Action: "cursor_open_item",
				Params: map[string]interface{}{
					"item_id": cursor.ItemID,
				},
			}
		case "delete this", "delete this item", "remove this", "remove this item":
			return &SystemAction{
				Action: "cursor_triage_item",
				Params: map[string]interface{}{
					"item_id":       cursor.ItemID,
					"triage_action": "delete",
					"view":          view,
				},
			}
		case "mark this done", "mark this item done", "complete this", "finish this", "done this":
			return &SystemAction{
				Action: "cursor_triage_item",
				Params: map[string]interface{}{
					"item_id":       cursor.ItemID,
					"triage_action": "done",
					"view":          view,
				},
			}
		case "move this to waiting", "move this item to waiting", "put this in waiting", "wait on this":
			return &SystemAction{
				Action: "cursor_triage_item",
				Params: map[string]interface{}{
					"item_id":       cursor.ItemID,
					"triage_action": "waiting",
					"view":          view,
				},
			}
		case "move this to someday", "put this in someday", "someday this", "not now":
			return &SystemAction{
				Action: "cursor_triage_item",
				Params: map[string]interface{}{
					"item_id":       cursor.ItemID,
					"triage_action": "someday",
					"view":          view,
				},
			}
		case "move this to inbox", "move it to inbox", "move this back to inbox", "move this back to the inbox", "move this mail back to the inbox", "bring this back", "make this active":
			return &SystemAction{
				Action: "cursor_triage_item",
				Params: map[string]interface{}{
					"item_id":       cursor.ItemID,
					"triage_action": "inbox",
					"view":          view,
				},
			}
		}
	}

	if cursor.hasPointedPath() {
		switch normalized {
		case "open this", "open this file", "open this folder", "show this", "show this file", "show this folder":
			return &SystemAction{
				Action: "cursor_open_path",
				Params: map[string]interface{}{
					"path":   cursor.Path,
					"is_dir": cursor.IsDir,
				},
			}
		}
	}

	return nil
}

func parseInlineTitledItemIntent(text string) *SystemAction {
	match := titledItemInboxCommandPattern.FindStringSubmatch(strings.TrimSpace(text))
	if len(match) != 2 {
		return nil
	}
	title := strings.TrimSpace(match[1])
	if title == "" {
		return nil
	}
	return &SystemAction{
		Action: "triage_item_by_title",
		Params: map[string]interface{}{
			"title":         title,
			"triage_action": "inbox",
		},
	}
}

func systemActionCursorTriage(params map[string]interface{}) string {
	for _, key := range []string{"triage_action", "action"} {
		if value := strings.TrimSpace(fmt.Sprint(params[key])); value != "" && value != "<nil>" {
			return strings.ToLower(value)
		}
	}
	return ""
}

func systemActionCursorView(params map[string]interface{}) string {
	return strings.ToLower(strings.TrimSpace(systemActionStringParam(params, "view")))
}

func systemActionPathFlag(params map[string]interface{}) bool {
	switch value := params["is_dir"].(type) {
	case bool:
		return value
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "true")
	default:
		return false
	}
}

func systemActionTitle(params map[string]interface{}) string {
	return strings.TrimSpace(systemActionStringParam(params, "title"))
}

func (a *App) resolveItemByTitle(title string, preferredState string) (store.Item, error) {
	items, err := a.store.ListItemsFiltered(store.ItemListFilter{})
	if err != nil {
		return store.Item{}, err
	}
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return store.Item{}, errors.New("title is required")
	}
	candidates := make([]store.Item, 0, 4)
	for _, item := range items {
		if strings.TrimSpace(item.Title) == cleanTitle {
			candidates = append(candidates, item)
		}
	}
	if len(candidates) == 0 {
		return store.Item{}, fmt.Errorf("could not find item %q", cleanTitle)
	}
	sort.Slice(candidates, func(i, j int) bool {
		if preferredState != "" {
			iPreferred := strings.EqualFold(candidates[i].State, preferredState)
			jPreferred := strings.EqualFold(candidates[j].State, preferredState)
			if iPreferred != jPreferred {
				return iPreferred
			}
		}
		if candidates[i].UpdatedAt == candidates[j].UpdatedAt {
			return candidates[i].ID > candidates[j].ID
		}
		return candidates[i].UpdatedAt > candidates[j].UpdatedAt
	})
	return candidates[0], nil
}

func (a *App) executeCursorAction(ctx context.Context, session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(action.Action)) {
	case "cursor_open_item":
		itemID := systemActionItemID(action.Params)
		if itemID <= 0 {
			return "", nil, errors.New("item_id is required")
		}
		item, err := a.store.GetItem(itemID)
		if err != nil {
			return "", nil, err
		}
		return fmt.Sprintf("Opened item %q.", item.Title), map[string]interface{}{
			"type":    "open_item_sidebar_item",
			"item_id": item.ID,
		}, nil
	case "cursor_triage_item":
		itemID := systemActionItemID(action.Params)
		if itemID <= 0 {
			return "", nil, errors.New("item_id is required")
		}
		item, err := a.store.GetItem(itemID)
		if err != nil {
			return "", nil, err
		}
		view := systemActionCursorView(action.Params)
		triageAction := systemActionCursorTriage(action.Params)
		payload := map[string]interface{}{
			"type":    "item_state_changed",
			"item_id": item.ID,
		}
		if view != "" {
			payload["view"] = view
		}
		switch triageAction {
		case "done":
			if err := a.syncRemoteEmailItemState(ctx, item, store.ItemStateDone); err != nil {
				return "", nil, err
			}
			if err := a.store.TriageItemDone(item.ID); err != nil {
				return "", nil, err
			}
			return fmt.Sprintf("Marked item %q done.", item.Title), payload, nil
		case "inbox":
			if err := a.syncRemoteEmailItemState(ctx, item, store.ItemStateInbox); err != nil {
				return "", nil, err
			}
			if err := a.store.UpdateItemState(item.ID, store.ItemStateInbox); err != nil {
				return "", nil, err
			}
			payload["view"] = store.ItemStateInbox
			return fmt.Sprintf("Moved item %q back to inbox.", item.Title), payload, nil
		case "delete":
			if err := a.store.DeleteItem(item.ID); err != nil {
				return "", nil, err
			}
			return fmt.Sprintf("Deleted item %q.", item.Title), payload, nil
		case "waiting":
			if err := a.store.UpdateItemState(item.ID, store.ItemStateWaiting); err != nil {
				return "", nil, err
			}
			return fmt.Sprintf("Moved item %q to waiting.", item.Title), payload, nil
		case "someday":
			if err := a.store.TriageItemSomeday(item.ID); err != nil {
				return "", nil, err
			}
			return fmt.Sprintf("Moved item %q to someday.", item.Title), payload, nil
		case "delegate":
			actor, err := a.resolveActorByName(systemActionActorName(action.Params))
			if err != nil {
				return "", nil, err
			}
			if err := a.store.TriageItemDelegate(item.ID, actor.ID); err != nil {
				return "", nil, err
			}
			payload["actor_id"] = actor.ID
			return fmt.Sprintf("Delegated item %q to %s.", item.Title, actor.Name), payload, nil
		default:
			return "", nil, fmt.Errorf("unsupported cursor triage action: %s", triageAction)
		}
	case "cursor_open_path":
		path := strings.TrimSpace(systemActionStringParam(action.Params, "path"))
		if path == "" {
			return "", nil, errors.New("path is required")
		}
		isDir := systemActionPathFlag(action.Params)
		if isDir {
			return fmt.Sprintf("Opened folder %q.", path), map[string]interface{}{
				"type":   "open_workspace_path",
				"path":   path,
				"is_dir": true,
			}, nil
		}
		return fmt.Sprintf("Opened file %q.", path), map[string]interface{}{
			"type":   "open_workspace_path",
			"path":   path,
			"is_dir": false,
		}, nil
	default:
		return "", nil, fmt.Errorf("unsupported cursor action: %s", action.Action)
	}
}

func (a *App) executeTitledItemAction(ctx context.Context, session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	switch strings.ToLower(strings.TrimSpace(action.Action)) {
	case "triage_item_by_title":
		title := systemActionTitle(action.Params)
		if title == "" {
			return "", nil, errors.New("title is required")
		}
		triageAction := systemActionCursorTriage(action.Params)
		item, err := a.resolveItemByTitle(title, store.ItemStateDone)
		if err != nil {
			return "", nil, err
		}
		payload := map[string]interface{}{
			"type":    "item_state_changed",
			"item_id": item.ID,
			"view":    item.State,
		}
		switch triageAction {
		case "inbox":
			if err := a.syncRemoteEmailItemState(ctx, item, store.ItemStateInbox); err != nil {
				return "", nil, err
			}
			if err := a.store.UpdateItemState(item.ID, store.ItemStateInbox); err != nil {
				return "", nil, err
			}
			payload["view"] = store.ItemStateInbox
			return fmt.Sprintf("Moved item %q back to inbox.", item.Title), payload, nil
		default:
			return "", nil, fmt.Errorf("unsupported titled item triage action: %s", triageAction)
		}
	default:
		return "", nil, fmt.Errorf("unsupported titled item action: %s", action.Action)
	}
}
