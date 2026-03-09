package web

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

const briefingArtifactKind = store.ArtifactKind("briefing")

type briefingRequest struct {
	Date   time.Time
	Sphere string
}

type briefingSnapshot struct {
	Request          briefingRequest
	GeneratedAt      time.Time
	TodayEvents      []calendarEventEntry
	TomorrowEvents   []calendarEventEntry
	TodayDeadlines   []calendarDeadlineEntry
	WeekDeadlines    []calendarDeadlineEntry
	UrgentItems      []briefingOpenItem
	UnreadEmailItems []briefingOpenItem
	InFlightItems    []briefingOpenItem
	ReviewPRItems    []briefingOpenItem
	InboxCounts      map[string]int
	Warnings         []string
}

type briefingOpenItem struct {
	Title     string
	Sphere    string
	Workspace string
	Project   string
}

func parseInlineBriefingIntent(text string, now time.Time) *SystemAction {
	switch normalizeItemCommandText(text) {
	case "show my day", "show me my day", "show briefing", "show my briefing", "update briefing", "refresh briefing", "was steht heute an":
		return &SystemAction{
			Action: "show_briefing",
			Params: map[string]interface{}{
				"date": now.In(time.Local).Format("2006-01-02"),
			},
		}
	default:
		return nil
	}
}

func briefingActionFailurePrefix(string) string {
	return "I couldn't build the daily briefing: "
}

func (a *App) executeBriefingAction(session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	if a == nil || action == nil {
		return "", nil, fmt.Errorf("briefing action is required")
	}
	req, err := a.parseBriefingRequest(action)
	if err != nil {
		return "", nil, err
	}
	targetProject, err := a.systemActionTargetProject(session)
	if err != nil {
		return "", nil, err
	}
	cwd := strings.TrimSpace(targetProject.RootPath)
	if cwd == "" {
		cwd = strings.TrimSpace(a.cwdForProjectKey(targetProject.ProjectKey))
	}
	if cwd == "" {
		return "", nil, fmt.Errorf("briefing cwd is not available")
	}

	snapshot, err := a.collectBriefingSnapshot(req)
	if err != nil {
		return "", nil, err
	}
	content := renderBriefingMarkdown(snapshot)

	relativePath := filepath.ToSlash(filepath.Join(".tabura", "artifacts", "briefing", req.Date.In(time.Local).Format("2006-01-02")+".md"))
	absPath, canvasTitle, err := resolveCanvasFilePath(cwd, relativePath)
	if err != nil {
		return "", nil, err
	}
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		return "", nil, err
	}

	artifactTitle := fmt.Sprintf("Daily Briefing %s", req.Date.In(time.Local).Format("2006-01-02"))
	metaJSON := briefingArtifactMeta(snapshot)
	artifact, err := a.store.CreateArtifact(briefingArtifactKind, &absPath, nil, &artifactTitle, &metaJSON)
	if err != nil {
		return "", nil, err
	}
	if workspace, workspaceErr := a.store.ActiveWorkspace(); workspaceErr == nil {
		_ = a.store.LinkArtifactToWorkspace(workspace.ID, artifact.ID)
	}

	canvasSessionID := strings.TrimSpace(a.canvasSessionIDForProject(targetProject))
	if canvasSessionID == "" {
		return "", nil, fmt.Errorf("canvas session is not available")
	}
	port, ok := a.tunnels.getPort(canvasSessionID)
	if !ok {
		return "", nil, fmt.Errorf("no active MCP tunnel for project %q", targetProject.Name)
	}
	if _, err := a.mcpToolsCall(port, "canvas_artifact_show", map[string]interface{}{
		"session_id":       canvasSessionID,
		"kind":             "text",
		"title":            canvasTitle,
		"markdown_or_text": content,
	}); err != nil {
		return "", nil, err
	}
	a.markProjectOutput(targetProject.ProjectKey)

	return fmt.Sprintf("Opened %s on canvas.", artifactTitle), map[string]interface{}{
		"type":                 "show_briefing",
		"artifact_id":          artifact.ID,
		"path":                 canvasTitle,
		"date":                 req.Date.In(time.Local).Format("2006-01-02"),
		"event_count":          len(snapshot.TodayEvents) + len(snapshot.TomorrowEvents),
		"today_deadline_count": len(snapshot.TodayDeadlines),
		"week_deadline_count":  len(snapshot.WeekDeadlines),
		"warning_count":        len(snapshot.Warnings),
	}, nil
}

func (a *App) parseBriefingRequest(action *SystemAction) (briefingRequest, error) {
	now := time.Now()
	if a != nil && a.calendarNow != nil {
		now = a.calendarNow()
	}
	req := briefingRequest{
		Date:   now.In(time.Local),
		Sphere: normalizeBriefingSphere(calendarOptionalParam(action.Params, "sphere")),
	}
	if rawDate := calendarOptionalParam(action.Params, "date"); rawDate != "" {
		parsed, err := time.ParseInLocation("2006-01-02", rawDate, time.Local)
		if err != nil {
			return briefingRequest{}, fmt.Errorf("briefing date must be YYYY-MM-DD")
		}
		req.Date = parsed
	}
	return req, nil
}

func normalizeBriefingSphere(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return ""
	case store.SphereWork:
		return store.SphereWork
	case store.SpherePrivate:
		return store.SpherePrivate
	default:
		return ""
	}
}

func (a *App) collectBriefingSnapshot(req briefingRequest) (briefingSnapshot, error) {
	todayReq := calendarActionRequest{View: calendarViewAgenda, Date: req.Date}
	tomorrowReq := calendarActionRequest{View: calendarViewAgenda, Date: req.Date.AddDate(0, 0, 1)}
	weekReq := calendarActionRequest{View: calendarViewWeek, Date: req.Date}

	todayEvents, todayWarnings, err := a.collectCalendarEvents(context.Background(), todayReq, req.Sphere)
	if err != nil {
		return briefingSnapshot{}, err
	}
	tomorrowEvents, tomorrowWarnings, err := a.collectCalendarEvents(context.Background(), tomorrowReq, req.Sphere)
	if err != nil {
		return briefingSnapshot{}, err
	}
	todayDeadlines, err := a.collectCalendarDeadlines(todayReq)
	if err != nil {
		return briefingSnapshot{}, err
	}
	weekDeadlines, err := a.collectCalendarDeadlines(weekReq)
	if err != nil {
		return briefingSnapshot{}, err
	}
	items, err := a.store.ListItems()
	if err != nil {
		return briefingSnapshot{}, err
	}
	generatedAt := time.Now().UTC()
	if a != nil && a.calendarNow != nil {
		generatedAt = a.calendarNow().UTC()
	}
	inboxCounts := map[string]int{
		store.SphereWork:    0,
		store.SpherePrivate: 0,
	}
	workspaceNames := map[int64]string{}
	projectNames := map[string]string{}
	var (
		urgentItems      []briefingOpenItem
		unreadEmailItems []briefingOpenItem
		inFlightItems    []briefingOpenItem
		reviewPRItems    []briefingOpenItem
	)
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.State), store.ItemStateDone) {
			continue
		}
		sphere := normalizeBriefingItemSphere(item.Sphere)
		if req.Sphere != "" && !strings.EqualFold(req.Sphere, sphere) {
			continue
		}
		openItem := briefingOpenItem{
			Title:     strings.TrimSpace(item.Title),
			Sphere:    sphere,
			Workspace: calendarWorkspaceName(a, item.WorkspaceID, workspaceNames),
			Project:   calendarProjectName(a, item.ProjectID, projectNames),
		}
		if item.State == store.ItemStateInbox {
			inboxCounts[sphere]++
		}
		if briefingItemIsUrgent(item) {
			urgentItems = append(urgentItems, openItem)
		}
		if briefingItemIsUnreadEmail(item) {
			unreadEmailItems = append(unreadEmailItems, openItem)
		}
		if item.State == store.ItemStateWaiting {
			inFlightItems = append(inFlightItems, openItem)
		}
		if briefingItemIsReviewPR(item) {
			reviewPRItems = append(reviewPRItems, openItem)
		}
	}
	sortBriefingItems(urgentItems)
	sortBriefingItems(unreadEmailItems)
	sortBriefingItems(inFlightItems)
	sortBriefingItems(reviewPRItems)

	return briefingSnapshot{
		Request:          req,
		GeneratedAt:      generatedAt,
		TodayEvents:      todayEvents,
		TomorrowEvents:   tomorrowEvents,
		TodayDeadlines:   filterBriefingDeadlines(todayDeadlines, req),
		WeekDeadlines:    filterBriefingUpcomingDeadlines(weekDeadlines, req),
		UrgentItems:      urgentItems,
		UnreadEmailItems: unreadEmailItems,
		InFlightItems:    inFlightItems,
		ReviewPRItems:    reviewPRItems,
		InboxCounts:      inboxCounts,
		Warnings:         dedupeBriefingWarnings(todayWarnings, tomorrowWarnings),
	}, nil
}

func normalizeBriefingItemSphere(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), store.SphereWork) {
		return store.SphereWork
	}
	return store.SpherePrivate
}

func briefingItemIsUrgent(item store.Item) bool {
	haystack := strings.ToLower(strings.Join([]string{
		item.Title,
		stringFromPointer(item.Source),
		stringFromPointer(item.SourceRef),
	}, " "))
	for _, token := range []string{"[p0]", " p0 ", "p0:", "urgent", "asap"} {
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return strings.HasPrefix(haystack, "p0 ")
}

func briefingItemIsUnreadEmail(item store.Item) bool {
	source := strings.ToLower(strings.TrimSpace(stringFromPointer(item.Source)))
	switch source {
	case store.ExternalProviderGmail, store.ExternalProviderIMAP, store.ExternalProviderExchange:
		return true
	default:
		return false
	}
}

func briefingItemIsReviewPR(item store.Item) bool {
	if !strings.EqualFold(strings.TrimSpace(stringFromPointer(item.Source)), "github") {
		return false
	}
	return strings.Contains(strings.ToUpper(strings.TrimSpace(stringFromPointer(item.SourceRef))), "#PR-")
}

func sortBriefingItems(items []briefingOpenItem) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Sphere == items[j].Sphere {
			return strings.ToLower(items[i].Title) < strings.ToLower(items[j].Title)
		}
		return items[i].Sphere < items[j].Sphere
	})
}

func filterBriefingDeadlines(deadlines []calendarDeadlineEntry, req briefingRequest) []calendarDeadlineEntry {
	out := make([]calendarDeadlineEntry, 0, len(deadlines))
	for _, entry := range deadlines {
		if req.Sphere != "" && !strings.EqualFold(req.Sphere, normalizeBriefingItemSphere(entry.Sphere)) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func filterBriefingUpcomingDeadlines(deadlines []calendarDeadlineEntry, req briefingRequest) []calendarDeadlineEntry {
	cutoff := req.Date.In(time.Local).Format("2006-01-02")
	out := make([]calendarDeadlineEntry, 0, len(deadlines))
	for _, entry := range deadlines {
		if req.Sphere != "" && !strings.EqualFold(req.Sphere, normalizeBriefingItemSphere(entry.Sphere)) {
			continue
		}
		if entry.When.In(time.Local).Format("2006-01-02") <= cutoff {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func dedupeBriefingWarnings(groups ...[]string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, group := range groups {
		for _, warning := range group {
			clean := strings.TrimSpace(warning)
			if clean == "" {
				continue
			}
			if _, ok := seen[clean]; ok {
				continue
			}
			seen[clean] = struct{}{}
			out = append(out, clean)
		}
	}
	sort.Strings(out)
	return out
}

func renderBriefingMarkdown(snapshot briefingSnapshot) string {
	var b strings.Builder
	date := snapshot.Request.Date.In(time.Local)
	fmt.Fprintf(&b, "# Daily Briefing\n\n")
	fmt.Fprintf(&b, "- Date: %s\n", date.Format("Monday, January 2, 2006"))
	fmt.Fprintf(&b, "- Generated: %s\n", snapshot.GeneratedAt.In(time.Local).Format(time.RFC3339))
	if snapshot.Request.Sphere != "" {
		fmt.Fprintf(&b, "- Sphere filter: `%s`\n", snapshot.Request.Sphere)
	}
	if len(snapshot.Warnings) > 0 {
		fmt.Fprintf(&b, "- Source warnings: %d\n", len(snapshot.Warnings))
	}
	b.WriteString("\n## Schedule\n\n")
	for _, sphere := range briefingSphereOrder(snapshot.Request.Sphere) {
		fmt.Fprintf(&b, "### %s\n\n", briefingSphereTitle(sphere))
		events := briefingEventsForSphere(snapshot.TodayEvents, sphere)
		if len(events) == 0 {
			b.WriteString("_No scheduled events._\n\n")
			continue
		}
		for _, event := range events {
			fmt.Fprintf(&b, "- %s\n", renderBriefingEventLine(event))
		}
		b.WriteString("\n")
	}

	dueToday, resurfaceToday := splitBriefingDeadlines(snapshot.TodayDeadlines)
	fmt.Fprintf(&b, "## Attention Needed\n\n")
	fmt.Fprintf(&b, "- Inbox items: work %d, private %d\n", snapshot.InboxCounts[store.SphereWork], snapshot.InboxCounts[store.SpherePrivate])
	fmt.Fprintf(&b, "- P0/urgent items: %d\n", len(snapshot.UrgentItems))
	fmt.Fprintf(&b, "- Items due today: %d\n", len(dueToday))
	fmt.Fprintf(&b, "- Items resurfacing today: %d\n", len(resurfaceToday))
	fmt.Fprintf(&b, "- Unread emails requiring action: %d\n\n", len(snapshot.UnreadEmailItems))
	renderBriefingItemSection(&b, "Urgent items", snapshot.UrgentItems)
	renderBriefingDeadlineSection(&b, "Due today", dueToday, false)
	renderBriefingDeadlineSection(&b, "Resurfacing today", resurfaceToday, false)
	renderBriefingItemSection(&b, "Unread email follow-ups", snapshot.UnreadEmailItems)

	fmt.Fprintf(&b, "## Active Work\n\n")
	fmt.Fprintf(&b, "- Watched workspaces: 0\n")
	fmt.Fprintf(&b, "- Orchestrator items in flight: %d\n", len(snapshot.InFlightItems))
	fmt.Fprintf(&b, "- PRs awaiting review: %d\n\n", len(snapshot.ReviewPRItems))
	b.WriteString("### Watched workspaces\n\n")
	b.WriteString("_No watched workspaces configured._\n\n")
	renderBriefingItemSection(&b, "Items in flight", snapshot.InFlightItems)
	renderBriefingItemSection(&b, "PRs awaiting review", snapshot.ReviewPRItems)

	fmt.Fprintf(&b, "## Upcoming\n\n")
	b.WriteString("### Tomorrow's key events\n\n")
	if len(snapshot.TomorrowEvents) == 0 {
		b.WriteString("_No events tomorrow._\n\n")
	} else {
		for _, event := range snapshot.TomorrowEvents {
			fmt.Fprintf(&b, "- %s: %s\n", briefingSphereTitle(normalizeBriefingItemSphere(event.Sphere)), renderBriefingEventLine(event))
		}
		b.WriteString("\n")
	}
	renderBriefingDeadlineSection(&b, "Items due this week", snapshot.WeekDeadlines, true)
	renderBriefingDeadlineSection(&b, "Deadlines approaching", snapshot.WeekDeadlines, true)

	if len(snapshot.Warnings) > 0 {
		b.WriteString("## Source Warnings\n\n")
		for _, warning := range snapshot.Warnings {
			fmt.Fprintf(&b, "- %s\n", warning)
		}
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String()) + "\n"
}

func briefingSphereOrder(filter string) []string {
	if filter != "" {
		return []string{filter}
	}
	return []string{store.SphereWork, store.SpherePrivate}
}

func briefingSphereTitle(sphere string) string {
	if strings.EqualFold(strings.TrimSpace(sphere), store.SphereWork) {
		return "Work"
	}
	return "Private"
}

func briefingEventsForSphere(events []calendarEventEntry, sphere string) []calendarEventEntry {
	out := make([]calendarEventEntry, 0, len(events))
	for _, event := range events {
		if strings.EqualFold(normalizeBriefingItemSphere(event.Sphere), normalizeBriefingItemSphere(sphere)) {
			out = append(out, event)
		}
	}
	return out
}

func renderBriefingEventLine(event calendarEventEntry) string {
	label := strings.TrimSpace(event.Summary)
	if label == "" {
		label = "(Untitled event)"
	}
	parts := []string{calendarTimeLabel(event.Start, event.End, event.AllDay), label}
	if strings.TrimSpace(event.Location) != "" {
		parts = append(parts, "@ "+event.Location)
	}
	if len(event.Attendees) > 0 {
		parts = append(parts, "with "+strings.Join(event.Attendees, ", "))
	}
	parts = append(parts, "["+firstNonEmptyCalendarValue(event.Source, event.Provider, "calendar")+"]")
	return strings.Join(parts, " ")
}

func splitBriefingDeadlines(deadlines []calendarDeadlineEntry) ([]calendarDeadlineEntry, []calendarDeadlineEntry) {
	var due []calendarDeadlineEntry
	var resurface []calendarDeadlineEntry
	for _, entry := range deadlines {
		switch strings.ToLower(strings.TrimSpace(entry.Kind)) {
		case "due":
			due = append(due, entry)
		case "resurface":
			resurface = append(resurface, entry)
		}
	}
	return due, resurface
}

func renderBriefingItemSection(b *strings.Builder, title string, items []briefingOpenItem) {
	fmt.Fprintf(b, "### %s\n\n", title)
	if len(items) == 0 {
		b.WriteString("_None._\n\n")
		return
	}
	for _, item := range items {
		fmt.Fprintf(b, "- %s\n", renderBriefingItemLine(item))
	}
	b.WriteString("\n")
}

func renderBriefingItemLine(item briefingOpenItem) string {
	parts := []string{strings.TrimSpace(item.Title)}
	location := firstNonEmpty(item.Workspace, item.Project)
	if location != "" {
		parts = append(parts, "["+location+"]")
	}
	parts = append(parts, "("+briefingSphereTitle(item.Sphere)+")")
	return strings.Join(parts, " ")
}

func renderBriefingDeadlineSection(b *strings.Builder, title string, deadlines []calendarDeadlineEntry, includeDate bool) {
	fmt.Fprintf(b, "### %s\n\n", title)
	if len(deadlines) == 0 {
		b.WriteString("_None._\n\n")
		return
	}
	for _, entry := range deadlines {
		fmt.Fprintf(b, "- %s\n", renderBriefingDeadlineLine(entry, includeDate))
	}
	b.WriteString("\n")
}

func renderBriefingDeadlineLine(entry calendarDeadlineEntry, includeDate bool) string {
	title := strings.TrimSpace(entry.Title)
	if title == "" {
		title = "(Untitled item)"
	}
	timeLabel := entry.When.In(time.Local).Format("15:04")
	if includeDate {
		timeLabel = entry.When.In(time.Local).Format("Mon 15:04")
	}
	parts := []string{timeLabel, title}
	location := firstNonEmpty(entry.Workspace, entry.Project)
	if location != "" {
		parts = append(parts, "["+location+"]")
	}
	parts = append(parts, "("+briefingSphereTitle(entry.Sphere)+")")
	return strings.Join(parts, " ")
}

func briefingArtifactMeta(snapshot briefingSnapshot) string {
	payload := map[string]interface{}{
		"date":                 snapshot.Request.Date.In(time.Local).Format("2006-01-02"),
		"sphere":               snapshot.Request.Sphere,
		"today_event_count":    len(snapshot.TodayEvents),
		"tomorrow_event_count": len(snapshot.TomorrowEvents),
		"today_deadline_count": len(snapshot.TodayDeadlines),
		"week_deadline_count":  len(snapshot.WeekDeadlines),
		"urgent_count":         len(snapshot.UrgentItems),
		"unread_email_count":   len(snapshot.UnreadEmailItems),
		"in_flight_count":      len(snapshot.InFlightItems),
		"review_pending_count": len(snapshot.ReviewPRItems),
		"inbox_work_count":     snapshot.InboxCounts[store.SphereWork],
		"inbox_private_count":  snapshot.InboxCounts[store.SpherePrivate],
		"warning_count":        len(snapshot.Warnings),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(raw)
}
