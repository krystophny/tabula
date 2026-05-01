package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type gtdQueue struct {
	Name            string
	Aliases         []string
	Path            string
	Label           string
	IsProjectReview bool
}

var gtdQueueList = []gtdQueue{
	{Name: "inbox", Path: "/api/items/inbox", Label: "Inbox"},
	{Name: "next", Path: "/api/items/next", Label: "Next actions"},
	{Name: "waiting", Path: "/api/items/waiting", Label: "Waiting for"},
	{Name: "later", Aliases: []string{"deferred", "defer"}, Path: "/api/items/deferred", Label: "Later / Deferred"},
	{Name: "someday", Aliases: []string{"maybe"}, Path: "/api/items/someday", Label: "Someday / Maybe"},
	{Name: "review", Path: "/api/items/review", Label: "Review"},
	{Name: "projects", Path: "/api/items/projects", Label: "Project items", IsProjectReview: true},
}

func findGtdQueue(name string) *gtdQueue {
	target := strings.ToLower(strings.TrimSpace(name))
	if target == "" {
		return nil
	}
	for i := range gtdQueueList {
		q := &gtdQueueList[i]
		if q.Name == target {
			return q
		}
		for _, alias := range q.Aliases {
			if alias == target {
				return q
			}
		}
	}
	return nil
}

func isGtdSubcommand(args []string) bool {
	i := firstPositionalArg(args)
	if i >= len(args) {
		return false
	}
	if !strings.EqualFold(args[i], "gtd") {
		return false
	}
	if i+1 >= len(args) {
		return true
	}
	return findGtdQueue(args[i+1]) != nil
}

type gtdFilters struct {
	sphere          string
	source          string
	sourceContainer string
	label           string
	labelID         string
	actorID         string
	workspace       string
	projectItemID   string
	dueBefore       string
	dueAfter        string
	followUpBefore  string
	followUpAfter   string
	jsonOut         bool
}

func parseGtdFilters(args []string) (gtdFilters, error) {
	fs := flag.NewFlagSet("sls gtd", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var f gtdFilters
	fs.StringVar(&f.sphere, "vault", "", "vault sphere: work|private")
	fs.StringVar(&f.source, "source", "", "filter by source backend (todoist, github, imap, ...)")
	fs.StringVar(&f.sourceContainer, "source-container", "", "filter by upstream container (Todoist project, mail folder, GitHub Project)")
	fs.StringVar(&f.label, "label", "", "filter by label name")
	fs.StringVar(&f.labelID, "label-id", "", "filter by label id")
	fs.StringVar(&f.actorID, "actor-id", "", "filter by actor id (delegated to)")
	fs.StringVar(&f.workspace, "workspace", "", "filter by Slopshell workspace id, or 'null' for unassigned")
	fs.StringVar(&f.projectItemID, "project-item-id", "", "filter by GTD project item id")
	fs.StringVar(&f.projectItemID, "project-item", "", "alias for --project-item-id")
	fs.StringVar(&f.projectItemID, "project", "", "alias for --project-item-id")
	fs.StringVar(&f.dueBefore, "due-before", "", "filter items with due_at before this RFC3339 timestamp")
	fs.StringVar(&f.dueAfter, "due-after", "", "filter items with due_at after this RFC3339 timestamp")
	fs.StringVar(&f.followUpBefore, "follow-up-before", "", "filter items with follow_up_at before this timestamp")
	fs.StringVar(&f.followUpAfter, "follow-up-after", "", "filter items with follow_up_at after this timestamp")
	fs.BoolVar(&f.jsonOut, "json", false, "emit raw JSON instead of plain text")
	if err := fs.Parse(args); err != nil {
		return gtdFilters{}, err
	}
	if extra := fs.Args(); len(extra) > 0 {
		return gtdFilters{}, fmt.Errorf("unexpected positional argument: %s", extra[0])
	}
	return f, nil
}

func (f gtdFilters) query() url.Values {
	q := url.Values{}
	setIf := func(key, value string) {
		if v := strings.TrimSpace(value); v != "" {
			q.Set(key, v)
		}
	}
	setIf("sphere", f.sphere)
	setIf("source", f.source)
	setIf("source_container", f.sourceContainer)
	setIf("label", f.label)
	setIf("label_id", f.labelID)
	setIf("actor_id", f.actorID)
	setIf("workspace_id", f.workspace)
	setIf("project_item_id", f.projectItemID)
	setIf("due_before", f.dueBefore)
	setIf("due_after", f.dueAfter)
	setIf("follow_up_before", f.followUpBefore)
	setIf("follow_up_after", f.followUpAfter)
	return q
}

type gtdItem struct {
	ID           int64   `json:"id"`
	Title        string  `json:"title"`
	Kind         string  `json:"kind"`
	State        string  `json:"state"`
	Sphere       string  `json:"sphere"`
	Source       *string `json:"source"`
	SourceRef    *string `json:"source_ref"`
	WorkspaceID  *int64  `json:"workspace_id"`
	ActorID      *int64  `json:"actor_id"`
	ActorName    *string `json:"actor_name"`
	DueAt        *string `json:"due_at"`
	FollowUpAt   *string `json:"follow_up_at"`
	VisibleAfter *string `json:"visible_after"`
}

type gtdItemsResponse struct {
	Items   []gtdItem `json:"items"`
	Overdue []int64   `json:"overdue"`
}

type gtdProjectHealth struct {
	HasNextAction bool `json:"has_next_action"`
	HasWaiting    bool `json:"has_waiting"`
	HasDeferred   bool `json:"has_deferred"`
	HasSomeday    bool `json:"has_someday"`
	Stalled       bool `json:"stalled"`
}

type gtdProjectCounts struct {
	Inbox    int `json:"inbox"`
	Next     int `json:"next"`
	Waiting  int `json:"waiting"`
	Deferred int `json:"deferred"`
	Someday  int `json:"someday"`
	Review   int `json:"review"`
	Done     int `json:"done"`
	Total    int `json:"total"`
}

type gtdProjectReview struct {
	Item     gtdItem          `json:"item"`
	Health   gtdProjectHealth `json:"health"`
	Children gtdProjectCounts `json:"children"`
}

type gtdProjectsResponse struct {
	ProjectItems []gtdProjectReview `json:"project_items"`
	Total        int                `json:"total"`
	Stalled      int                `json:"stalled"`
}

func handleGtdCommand(args []string, opts cliOptions, stdout, stderr io.Writer) int {
	if len(args) == 0 || strings.EqualFold(args[0], "gtd") {
		printGtdUsage(stderr)
		return 2
	}
	queue := findGtdQueue(args[0])
	if queue == nil {
		fmt.Fprintf(stderr, "unknown gtd subcommand %q (want inbox|next|waiting|later|someday|review|projects)\n", args[0])
		return 2
	}
	filters, err := parseGtdFilters(args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "sls gtd %s: %v\n", queue.Name, err)
		return 2
	}
	body, err := fetchGtd(opts, *queue, filters)
	if err != nil {
		fmt.Fprintf(stderr, "sls gtd %s: %v\n", queue.Name, err)
		return 1
	}
	if err := renderGtdResponse(stdout, *queue, filters, body); err != nil {
		fmt.Fprintf(stderr, "sls gtd %s: %v\n", queue.Name, err)
		return 1
	}
	return 0
}

func printGtdUsage(out io.Writer) {
	fmt.Fprintln(out, "usage: sls gtd <queue> [filters...]")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "queues:")
	fmt.Fprintln(out, "  inbox                    unprocessed captures and imported candidates")
	fmt.Fprintln(out, "  next                     actionable next actions across all sources")
	fmt.Fprintln(out, "  waiting                  delegated/awaited items")
	fmt.Fprintln(out, "  later (deferred)         deferred/tickler items hidden until follow_up")
	fmt.Fprintln(out, "  someday (maybe)          parked actions and project items")
	fmt.Fprintln(out, "  review                   stalled, drift, or needs-clarification items")
	fmt.Fprintln(out, "  projects                 Item(kind=project) outcomes only")
	fmt.Fprintln(out, "")
	fmt.Fprintln(out, "filters:")
	fmt.Fprintln(out, "  --vault work|private       sphere filter")
	fmt.Fprintln(out, "  --source NAME              source backend (todoist|github|imap|gmail|...)")
	fmt.Fprintln(out, "  --source-container NAME    upstream container (Todoist project, mail folder)")
	fmt.Fprintln(out, "  --label NAME               filter by label name")
	fmt.Fprintln(out, "  --label-id N               filter by label id")
	fmt.Fprintln(out, "  --actor-id N               filter by actor id (delegate)")
	fmt.Fprintln(out, "  --workspace ID|null        filesystem-backed Slopshell workspace id")
	fmt.Fprintln(out, "  --project-item-id N        GTD project-item id (alias: --project, --project-item)")
	fmt.Fprintln(out, "  --due-before TS            ISO timestamp")
	fmt.Fprintln(out, "  --due-after TS             ISO timestamp")
	fmt.Fprintln(out, "  --follow-up-before TS      ISO timestamp")
	fmt.Fprintln(out, "  --follow-up-after TS       ISO timestamp")
	fmt.Fprintln(out, "  --json                     emit raw JSON")
}

func fetchGtd(opts cliOptions, queue gtdQueue, filters gtdFilters) ([]byte, error) {
	base := opts.resolveBaseURL()
	client, err := newClientForBrain(base, opts.effectiveTokenFile())
	if err != nil {
		return nil, err
	}
	target := strings.TrimRight(base, "/") + queue.Path
	if q := filters.query(); len(q) > 0 {
		target += "?" + q.Encode()
	}
	resp, err := client.Get(target)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", queue.Name, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", queue.Name, err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func renderGtdResponse(out io.Writer, queue gtdQueue, filters gtdFilters, body []byte) error {
	if filters.jsonOut {
		_, err := out.Write(append(append([]byte{}, body...), '\n'))
		return err
	}
	if queue.IsProjectReview {
		return renderGtdProjects(out, queue, body)
	}
	return renderGtdItems(out, queue, body)
}

func renderGtdItems(out io.Writer, queue gtdQueue, body []byte) error {
	var resp gtdItemsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode items: %w", err)
	}
	overdue := overdueIDSet(resp.Overdue)
	if len(resp.Items) == 0 {
		fmt.Fprintf(out, "%s: (empty)\n", queue.Label)
		return nil
	}
	suffix := ""
	if len(overdue) > 0 {
		suffix = fmt.Sprintf(", %d overdue", len(overdue))
	}
	fmt.Fprintf(out, "%s (%d items%s)\n", queue.Label, len(resp.Items), suffix)
	for _, item := range resp.Items {
		fmt.Fprintln(out, formatGtdItemLine(item, overdue[item.ID]))
	}
	return nil
}

func renderGtdProjects(out io.Writer, queue gtdQueue, body []byte) error {
	var resp gtdProjectsResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("decode project items: %w", err)
	}
	if resp.Total == 0 {
		fmt.Fprintf(out, "%s: (empty)\n", queue.Label)
		return nil
	}
	fmt.Fprintf(out, "%s (%d total, %d stalled)\n", queue.Label, resp.Total, resp.Stalled)
	for _, review := range resp.ProjectItems {
		fmt.Fprintln(out, formatGtdProjectLine(review))
	}
	return nil
}

func overdueIDSet(ids []int64) map[int64]bool {
	if len(ids) == 0 {
		return nil
	}
	out := make(map[int64]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func formatGtdItemLine(item gtdItem, overdue bool) string {
	parts := []string{fmt.Sprintf("  #%d", item.ID)}
	if overdue {
		parts = append(parts, "[overdue]")
	}
	if src := optionalStringPtr(item.Source); src != "" {
		parts = append(parts, "["+src+"]")
	} else {
		parts = append(parts, "[local]")
	}
	parts = append(parts, item.Title)
	tail := gtdItemTail(item)
	line := strings.Join(parts, " ")
	if tail != "" {
		line += "  " + tail
	}
	return line
}

func gtdItemTail(item gtdItem) string {
	var bits []string
	if item.Sphere != "" {
		bits = append(bits, "sphere="+item.Sphere)
	}
	if name := optionalStringPtr(item.ActorName); name != "" {
		bits = append(bits, "actor="+name)
	} else if item.ActorID != nil {
		bits = append(bits, fmt.Sprintf("actor_id=%d", *item.ActorID))
	}
	if due := optionalStringPtr(item.DueAt); due != "" {
		bits = append(bits, "due="+due)
	}
	if follow := optionalStringPtr(item.FollowUpAt); follow != "" {
		bits = append(bits, "follow_up="+follow)
	}
	if visible := optionalStringPtr(item.VisibleAfter); visible != "" {
		bits = append(bits, "visible_after="+visible)
	}
	if item.WorkspaceID != nil {
		bits = append(bits, fmt.Sprintf("workspace=%d", *item.WorkspaceID))
	}
	return strings.Join(bits, "  ")
}

func formatGtdProjectLine(review gtdProjectReview) string {
	flag := "       "
	if review.Health.Stalled {
		flag = "[stall]"
	}
	line := fmt.Sprintf("  #%d %s %s", review.Item.ID, flag, review.Item.Title)
	counts := fmt.Sprintf("next=%d waiting=%d deferred=%d someday=%d review=%d done=%d",
		review.Children.Next,
		review.Children.Waiting,
		review.Children.Deferred,
		review.Children.Someday,
		review.Children.Review,
		review.Children.Done,
	)
	return line + "  " + counts
}

func optionalStringPtr(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}
