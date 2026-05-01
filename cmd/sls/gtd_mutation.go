package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type gtdItemResponse struct {
	Item    gtdItem        `json:"item"`
	Links   []gtdChildLink `json:"links"`
	Warning string         `json:"warning"`
}

type gtdChildLink struct {
	ParentItemID int64  `json:"parent_item_id"`
	ChildItemID  int64  `json:"child_item_id"`
	Role         string `json:"role"`
}

type gtdMutationClient struct {
	base   string
	client *http.Client
}

func isGtdMutationName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "close", "drop", "defer", "delegate", "route", "link-project", "unlink-project":
		return true
	default:
		return false
	}
}

func handleGtdMutationCommand(args []string, opts cliOptions, stdout, stderr io.Writer) int {
	mutator, err := newGtdMutationClient(opts)
	if err != nil {
		fmt.Fprintf(stderr, "sls gtd %s: %v\n", args[0], err)
		return 1
	}
	result, err := mutator.run(args)
	if err != nil {
		fmt.Fprintf(stderr, "sls gtd %s: %v\n", args[0], err)
		if errors.Is(err, errGtdUsage) {
			return 2
		}
		return 1
	}
	fmt.Fprintln(stdout, result)
	return 0
}

var errGtdUsage = errors.New("usage error")

func newGtdHTTPClient(opts cliOptions) (*http.Client, string, error) {
	base := opts.resolveBaseURL()
	client, err := newClientForBrain(base, opts.effectiveTokenFile())
	if err != nil {
		return nil, "", err
	}
	return client, base, nil
}

func newGtdMutationClient(opts cliOptions) (*gtdMutationClient, error) {
	client, base, err := newGtdHTTPClient(opts)
	if err != nil {
		return nil, err
	}
	return &gtdMutationClient{base: strings.TrimRight(base, "/"), client: client}, nil
}

func (c *gtdMutationClient) run(args []string) (string, error) {
	if len(args) == 0 {
		return "", errGtdUsage
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "close":
		return c.close(args[1:])
	case "drop":
		return c.drop(args[1:])
	case "defer":
		return c.deferItem(args[1:])
	case "delegate":
		return c.delegate(args[1:])
	case "route":
		return c.route(args[1:])
	case "link-project":
		return c.linkProject(args[1:])
	case "unlink-project":
		return c.unlinkProject(args[1:])
	default:
		return "", errGtdUsage
	}
}

func (c *gtdMutationClient) close(args []string) (string, error) {
	itemID, err := singleItemID("close", args)
	if err != nil {
		return "", err
	}
	var resp gtdItemResponse
	if err := c.request(http.MethodPut, fmt.Sprintf("/api/items/%d/gtd-status", itemID), map[string]any{"state": "done"}, &resp); err != nil {
		return "", err
	}
	return fmt.Sprintf("closed #%d state=%s", resp.Item.ID, resp.Item.State), nil
}

func (c *gtdMutationClient) drop(args []string) (string, error) {
	itemArgs, yes, err := parseGtdDropArgs(args)
	if err != nil {
		return "", err
	}
	itemID, err := singleItemID("drop", itemArgs)
	if err != nil {
		return "", err
	}
	item, err := c.fetchItem(itemID)
	if err != nil {
		return "", err
	}
	if optionalStringPtr(item.Source) != "" && !yes {
		return "", fmt.Errorf("source-backed item #%d requires --yes before drop", itemID)
	}
	if err := c.request(http.MethodDelete, fmt.Sprintf("/api/items/%d", itemID), nil, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("dropped #%d", itemID), nil
}

func (c *gtdMutationClient) deferItem(args []string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("%w: usage: sls gtd defer <item> <rfc3339>", errGtdUsage)
	}
	itemID, err := parseGtdItemID(args[0])
	if err != nil {
		return "", err
	}
	followUp := strings.TrimSpace(args[1])
	var resp gtdItemResponse
	body := map[string]any{"state": "deferred", "follow_up_at": followUp}
	if err := c.request(http.MethodPut, fmt.Sprintf("/api/items/%d", itemID), body, &resp); err != nil {
		return "", err
	}
	return fmt.Sprintf("deferred #%d follow_up=%s", resp.Item.ID, optionalStringPtr(resp.Item.FollowUpAt)), nil
}

func (c *gtdMutationClient) delegate(args []string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("%w: usage: sls gtd delegate <item> <actor-id>", errGtdUsage)
	}
	itemID, err := parseGtdItemID(args[0])
	if err != nil {
		return "", err
	}
	actorID, err := parsePositiveInt64(args[1], "actor")
	if err != nil {
		return "", err
	}
	var resp gtdItemResponse
	if err := c.request(http.MethodPut, fmt.Sprintf("/api/items/%d/assign", itemID), map[string]any{"actor_id": actorID}, &resp); err != nil {
		return "", err
	}
	return fmt.Sprintf("delegated #%d actor_id=%d state=%s", resp.Item.ID, actorID, resp.Item.State), nil
}

func (c *gtdMutationClient) route(args []string) (string, error) {
	if len(args) != 2 {
		return "", fmt.Errorf("%w: usage: sls gtd route <item> <workspace-id|null>", errGtdUsage)
	}
	itemID, err := parseGtdItemID(args[0])
	if err != nil {
		return "", err
	}
	workspaceID, err := parseOptionalGtdID(args[1], "workspace")
	if err != nil {
		return "", err
	}
	var resp gtdItemResponse
	if err := c.request(http.MethodPut, fmt.Sprintf("/api/items/%d/workspace", itemID), map[string]any{"workspace_id": workspaceID}, &resp); err != nil {
		return "", err
	}
	return formatGtdRouteResult(resp.Item, resp.Warning), nil
}

func (c *gtdMutationClient) linkProject(args []string) (string, error) {
	itemArgs, role, err := parseGtdLinkProjectArgs(args)
	if err != nil {
		return "", err
	}
	ids, err := twoItemIDs("link-project", itemArgs)
	if err != nil {
		return "", err
	}
	var resp gtdItemResponse
	body := map[string]any{"project_item_id": ids[1], "role": role}
	if err := c.request(http.MethodPost, fmt.Sprintf("/api/items/%d/project-item-link", ids[0]), body, &resp); err != nil {
		return "", err
	}
	return fmt.Sprintf("linked #%d project_item=%d role=%s", ids[0], ids[1], linkRole(resp.Links, ids[0])), nil
}

func (c *gtdMutationClient) unlinkProject(args []string) (string, error) {
	ids, err := twoItemIDs("unlink-project", args)
	if err != nil {
		return "", err
	}
	var resp gtdItemResponse
	body := map[string]any{"project_item_id": ids[1]}
	if err := c.request(http.MethodDelete, fmt.Sprintf("/api/items/%d/project-item-link", ids[0]), body, &resp); err != nil {
		return "", err
	}
	return fmt.Sprintf("unlinked #%d project_item=%d", ids[0], ids[1]), nil
}

func (c *gtdMutationClient) fetchItem(itemID int64) (gtdItem, error) {
	var resp gtdItemResponse
	if err := c.request(http.MethodGet, fmt.Sprintf("/api/items/%d", itemID), nil, &resp); err != nil {
		return gtdItem{}, err
	}
	return resp.Item, nil
}

func (c *gtdMutationClient) request(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.base+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out == nil || len(strings.TrimSpace(string(data))) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return err
	}
	return nil
}

func singleItemID(action string, args []string) (int64, error) {
	if len(args) != 1 {
		return 0, fmt.Errorf("%w: usage: sls gtd %s <item>", errGtdUsage, action)
	}
	return parseGtdItemID(args[0])
}

func twoItemIDs(action string, args []string) ([2]int64, error) {
	if len(args) != 2 {
		return [2]int64{}, fmt.Errorf("%w: usage: sls gtd %s <item> <project-item>", errGtdUsage, action)
	}
	first, err := parseGtdItemID(args[0])
	if err != nil {
		return [2]int64{}, err
	}
	second, err := parseGtdItemID(args[1])
	if err != nil {
		return [2]int64{}, err
	}
	return [2]int64{first, second}, nil
}

func parseGtdItemID(raw string) (int64, error) {
	return parsePositiveInt64(strings.TrimPrefix(strings.TrimSpace(raw), "#"), "item")
}

func parsePositiveInt64(raw, name string) (int64, error) {
	id, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("%w: %s must be a positive integer", errGtdUsage, name)
	}
	return id, nil
}

func parseOptionalGtdID(raw, name string) (*int64, error) {
	if strings.EqualFold(strings.TrimSpace(raw), "null") {
		return nil, nil
	}
	value, err := parsePositiveInt64(raw, name)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func parseGtdDropArgs(args []string) ([]string, bool, error) {
	out := make([]string, 0, len(args))
	yes := false
	for _, arg := range args {
		if strings.EqualFold(strings.TrimSpace(arg), "--yes") {
			yes = true
			continue
		}
		out = append(out, arg)
	}
	return out, yes, nil
}

func parseGtdLinkProjectArgs(args []string) ([]string, string, error) {
	out := make([]string, 0, len(args))
	role := "next_action"
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--role":
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("%w: --role requires a value", errGtdUsage)
			}
			role = strings.TrimSpace(args[i+1])
			i++
		case strings.HasPrefix(arg, "--role="):
			role = strings.TrimSpace(strings.TrimPrefix(arg, "--role="))
		default:
			out = append(out, args[i])
		}
	}
	return out, role, nil
}

func formatGtdRouteResult(item gtdItem, warning string) string {
	target := "null"
	if item.WorkspaceID != nil {
		target = strconv.FormatInt(*item.WorkspaceID, 10)
	}
	out := fmt.Sprintf("routed #%d workspace=%s", item.ID, target)
	if strings.TrimSpace(warning) != "" {
		out += " warning=" + warning
	}
	return out
}

func linkRole(links []gtdChildLink, childID int64) string {
	for _, link := range links {
		if link.ChildItemID == childID && strings.TrimSpace(link.Role) != "" {
			return strings.TrimSpace(link.Role)
		}
	}
	return ""
}

func gtdURL(base, path string, q url.Values) string {
	target := strings.TrimRight(base, "/") + path
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	return target
}
