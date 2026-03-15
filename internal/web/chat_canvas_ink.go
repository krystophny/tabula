package web

import (
	"encoding/base64"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const chatCanvasInkMaxEventsPerSecond = 5

type chatCanvasInkBoundingBox struct {
	RelativeX      float64 `json:"relative_x"`
	RelativeY      float64 `json:"relative_y"`
	RelativeWidth  float64 `json:"relative_width"`
	RelativeHeight float64 `json:"relative_height"`
}

type chatCanvasInkLineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type chatCanvasInkEvent struct {
	Cursor           *chatCursorContext
	Gesture          string
	ArtifactKind     string
	StrokeCount      int
	Requested        bool
	BoundingBox      *chatCanvasInkBoundingBox
	OverlappingLines *chatCanvasInkLineRange
	OverlappingText  string
	SnapshotPath     string
	OccurredAt       time.Time
}

type chatCanvasInkTracker struct {
	mu     sync.Mutex
	events map[string][]*chatCanvasInkEvent
	recent map[string][]time.Time
}

func newChatCanvasInkTracker() *chatCanvasInkTracker {
	return &chatCanvasInkTracker{
		events: map[string][]*chatCanvasInkEvent{},
		recent: map[string][]time.Time{},
	}
}

func normalizeChatCanvasInkBoundingBox(raw *chatCanvasInkBoundingBox) *chatCanvasInkBoundingBox {
	if raw == nil {
		return nil
	}
	box := &chatCanvasInkBoundingBox{
		RelativeX:      clampCanvasInk01(raw.RelativeX),
		RelativeY:      clampCanvasInk01(raw.RelativeY),
		RelativeWidth:  clampCanvasInk01(raw.RelativeWidth),
		RelativeHeight: clampCanvasInk01(raw.RelativeHeight),
	}
	if box.RelativeWidth <= 0 && box.RelativeHeight <= 0 {
		return nil
	}
	return box
}

func normalizeChatCanvasInkLineRange(raw *chatCanvasInkLineRange) *chatCanvasInkLineRange {
	if raw == nil {
		return nil
	}
	start := raw.Start
	end := raw.End
	if start <= 0 && end <= 0 {
		return nil
	}
	if start <= 0 {
		start = end
	}
	if end <= 0 {
		end = start
	}
	if end < start {
		start, end = end, start
	}
	if start <= 0 {
		return nil
	}
	return &chatCanvasInkLineRange{Start: start, End: end}
}

func normalizeChatCanvasInkEvent(raw *chatCanvasInkEvent) *chatCanvasInkEvent {
	if raw == nil {
		return nil
	}
	gesture := strings.ToLower(strings.TrimSpace(raw.Gesture))
	if gesture == "" {
		gesture = "freeform"
	}
	artifactKind := strings.ToLower(strings.TrimSpace(raw.ArtifactKind))
	if artifactKind == "" {
		artifactKind = "text"
	}
	occurredAt := raw.OccurredAt.UTC()
	if occurredAt.IsZero() {
		occurredAt = time.Now().UTC()
	}
	event := &chatCanvasInkEvent{
		Cursor:           normalizeChatCursorContext(raw.Cursor),
		Gesture:          gesture,
		ArtifactKind:     artifactKind,
		StrokeCount:      raw.StrokeCount,
		Requested:        raw.Requested,
		BoundingBox:      normalizeChatCanvasInkBoundingBox(raw.BoundingBox),
		OverlappingLines: normalizeChatCanvasInkLineRange(raw.OverlappingLines),
		OverlappingText:  limitPromptLines(raw.OverlappingText, 8, 420),
		SnapshotPath:     strings.TrimSpace(raw.SnapshotPath),
		OccurredAt:       occurredAt,
	}
	if event.StrokeCount <= 0 {
		event.StrokeCount = 1
	}
	if event.Cursor == nil && event.BoundingBox == nil && event.OverlappingLines == nil && event.OverlappingText == "" && event.SnapshotPath == "" {
		return nil
	}
	return event
}

func (t *chatCanvasInkTracker) enqueue(sessionID string, raw *chatCanvasInkEvent) bool {
	if t == nil {
		return false
	}
	cleanSessionID := strings.TrimSpace(sessionID)
	if cleanSessionID == "" {
		return false
	}
	event := normalizeChatCanvasInkEvent(raw)
	if event == nil {
		return false
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := event.OccurredAt.Add(-1 * time.Second)
	recent := t.recent[cleanSessionID][:0]
	for _, ts := range t.recent[cleanSessionID] {
		if ts.After(cutoff) {
			recent = append(recent, ts)
		}
	}
	if len(recent) >= chatCanvasInkMaxEventsPerSecond {
		t.recent[cleanSessionID] = recent
		return false
	}
	recent = append(recent, event.OccurredAt)
	t.recent[cleanSessionID] = recent
	t.events[cleanSessionID] = append(t.events[cleanSessionID], event)
	return true
}

func (t *chatCanvasInkTracker) consume(sessionID string) []*chatCanvasInkEvent {
	if t == nil {
		return nil
	}
	cleanSessionID := strings.TrimSpace(sessionID)
	if cleanSessionID == "" {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	events := t.events[cleanSessionID]
	if len(events) == 0 {
		return nil
	}
	delete(t.events, cleanSessionID)
	out := make([]*chatCanvasInkEvent, 0, len(events))
	for _, event := range events {
		if event != nil {
			out = append(out, event)
		}
	}
	return out
}

func appendCanvasInkPrompt(prompt string, events []*chatCanvasInkEvent) string {
	contextBlock := formatCanvasInkPromptContext(events)
	prompt = strings.TrimSpace(prompt)
	if contextBlock == "" {
		return prompt
	}
	if prompt == "" {
		return contextBlock
	}
	return contextBlock + "\n\n" + prompt
}

func formatCanvasInkPromptContext(events []*chatCanvasInkEvent) string {
	filtered := make([]*chatCanvasInkEvent, 0, len(events))
	requested := false
	for _, event := range events {
		normalized := normalizeChatCanvasInkEvent(event)
		if normalized == nil {
			continue
		}
		filtered = append(filtered, normalized)
		if normalized.Requested {
			requested = true
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	lines := []string{"## Canvas Ink Events"}
	if requested {
		lines = append(lines, "The latest live ink input arrived during dialogue. Continue from the ink instead of asking the user to repeat or point again.")
	} else {
		lines = append(lines, "The user shared live ink during dialogue.")
	}
	lines = append(lines, "If a snapshot path is present, inspect that image when handwriting or freeform sketch meaning matters.")
	for i, event := range filtered {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, describeCanvasInkEvent(event)))
	}
	return strings.Join(lines, "\n")
}

func describeCanvasInkEvent(event *chatCanvasInkEvent) string {
	if event == nil {
		return ""
	}
	target := "active artifact"
	if event.Cursor != nil {
		if resolved := cursorPromptTarget(event.Cursor); resolved != "" {
			target = resolved
		}
	}
	label := event.Gesture
	if label == "" {
		label = "freeform"
	}
	parts := []string{fmt.Sprintf("%s ink over %s", strings.ReplaceAll(label, "_", " "), target)}
	if event.StrokeCount > 0 {
		parts = append(parts, fmt.Sprintf("%d stroke(s)", event.StrokeCount))
	}
	if event.OverlappingLines != nil {
		lineText := fmt.Sprintf("overlapping line %d", event.OverlappingLines.Start)
		if event.OverlappingLines.End > event.OverlappingLines.Start {
			lineText = fmt.Sprintf("overlapping lines %d-%d", event.OverlappingLines.Start, event.OverlappingLines.End)
		}
		parts = append(parts, lineText)
	}
	if text := strings.TrimSpace(event.OverlappingText); text != "" {
		parts = append(parts, "overlapping text "+quotePromptText(text, 220))
	}
	if box := event.BoundingBox; box != nil {
		parts = append(parts, fmt.Sprintf("bounding box %.0f%%, %.0f%% to %.0f%%, %.0f%%",
			box.RelativeX*100,
			box.RelativeY*100,
			(box.RelativeX+box.RelativeWidth)*100,
			(box.RelativeY+box.RelativeHeight)*100,
		))
	}
	if path := strings.TrimSpace(event.SnapshotPath); path != "" {
		parts = append(parts, fmt.Sprintf("snapshot path `%s`", path))
	}
	return strings.Join(parts, "; ")
}

func (a *App) persistChatCanvasInkSnapshot(sessionID, raw string) string {
	cleanSessionID := strings.TrimSpace(sessionID)
	clean := strings.TrimSpace(raw)
	if cleanSessionID == "" || clean == "" {
		return ""
	}
	data, err := decodeChatCanvasInkSnapshot(clean)
	if err != nil || len(data) == 0 {
		return ""
	}
	session, err := a.store.GetChatSession(cleanSessionID)
	if err != nil {
		return ""
	}
	project, err := a.store.GetProjectByWorkspacePath(session.WorkspacePath)
	if err != nil {
		return ""
	}
	dir := filepath.Join(project.RootPath, ".tabura", "artifacts", "tmp", "live-ink")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ""
	}
	name := fmt.Sprintf("%s-%s.png", time.Now().UTC().Format("20060102-150405"), randomToken())
	absPath := filepath.Join(dir, name)
	if err := os.WriteFile(absPath, data, 0o644); err != nil {
		return ""
	}
	relPath, err := filepath.Rel(project.RootPath, absPath)
	if err != nil {
		return ""
	}
	return filepath.ToSlash(relPath)
}

func decodeChatCanvasInkSnapshot(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	if idx := strings.Index(trimmed, ","); idx >= 0 && strings.HasPrefix(strings.ToLower(trimmed[:idx]), "data:image/png;base64") {
		trimmed = trimmed[idx+1:]
	}
	data, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func clampCanvasInk01(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func recognizeChatCanvasInkGesture(strokes []inkSubmitStroke) string {
	filtered := normalizeInkStrokesForGesture(strokes)
	if len(filtered) == 0 {
		return "freeform"
	}
	if isInkCircle(filtered) {
		return "circle"
	}
	if isInkUnderline(filtered) {
		return "underline"
	}
	if isInkCross(filtered) {
		return "cross"
	}
	if isInkQuestionMark(filtered) {
		return "question_mark"
	}
	if isInkArrow(filtered) {
		return "arrow"
	}
	return "freeform"
}

type inkGesturePoint struct {
	X float64
	Y float64
}

type inkGestureStroke struct {
	Points []inkGesturePoint
}

func normalizeInkStrokesForGesture(strokes []inkSubmitStroke) []inkGestureStroke {
	out := make([]inkGestureStroke, 0, len(strokes))
	for _, stroke := range strokes {
		points := make([]inkGesturePoint, 0, len(stroke.Points))
		for _, point := range stroke.Points {
			if math.IsNaN(point.X) || math.IsInf(point.X, 0) || math.IsNaN(point.Y) || math.IsInf(point.Y, 0) {
				continue
			}
			points = append(points, inkGesturePoint{X: point.X, Y: point.Y})
		}
		if len(points) >= 2 {
			out = append(out, inkGestureStroke{Points: points})
		}
	}
	return out
}

func isInkCircle(strokes []inkGestureStroke) bool {
	if len(strokes) != 1 {
		return false
	}
	stroke := strokes[0]
	minX, minY, maxX, maxY := inkGestureBounds(stroke.Points)
	width := maxX - minX
	height := maxY - minY
	if width < 12 || height < 12 {
		return false
	}
	ratio := width / height
	if ratio < 0.55 || ratio > 1.8 {
		return false
	}
	pathLen := inkGestureLength(stroke.Points)
	if pathLen <= 0 {
		return false
	}
	closure := inkGestureDistance(stroke.Points[0], stroke.Points[len(stroke.Points)-1])
	return closure <= math.Max(width, height)*0.35 && pathLen >= 1.6*math.Max(width, height)
}

func isInkUnderline(strokes []inkGestureStroke) bool {
	if len(strokes) != 1 {
		return false
	}
	stroke := strokes[0]
	minX, minY, maxX, maxY := inkGestureBounds(stroke.Points)
	width := maxX - minX
	height := maxY - minY
	if width < 16 || height <= 0 || width < height*4 {
		return false
	}
	start := stroke.Points[0]
	end := stroke.Points[len(stroke.Points)-1]
	dx := math.Abs(end.X - start.X)
	dy := math.Abs(end.Y - start.Y)
	return dx >= dy*2.5 && inkGestureDistance(start, end) >= inkGestureLength(stroke.Points)*0.8
}

func isInkCross(strokes []inkGestureStroke) bool {
	if len(strokes) != 2 {
		return false
	}
	a := strokes[0]
	b := strokes[1]
	if !inkGestureSegmentIntersects(a.Points[0], a.Points[len(a.Points)-1], b.Points[0], b.Points[len(b.Points)-1]) {
		return false
	}
	return inkGestureDiagonal(a) && inkGestureDiagonal(b)
}

func isInkQuestionMark(strokes []inkGestureStroke) bool {
	if len(strokes) != 2 {
		return false
	}
	mainIdx := 0
	dotIdx := 1
	if inkGestureLength(strokes[1].Points) > inkGestureLength(strokes[0].Points) {
		mainIdx = 1
		dotIdx = 0
	}
	main := strokes[mainIdx]
	dot := strokes[dotIdx]
	minX, minY, maxX, maxY := inkGestureBounds(main.Points)
	dotMinX, dotMinY, dotMaxX, dotMaxY := inkGestureBounds(dot.Points)
	mainWidth := maxX - minX
	mainHeight := maxY - minY
	dotWidth := dotMaxX - dotMinX
	dotHeight := dotMaxY - dotMinY
	if mainHeight < 14 || mainWidth < 8 {
		return false
	}
	if dotWidth > mainWidth*0.4 || dotHeight > mainHeight*0.35 {
		return false
	}
	dotCenterX := dotMinX + dotWidth/2
	return dotMinY > maxY && dotCenterX >= minX-mainWidth*0.2 && dotCenterX <= maxX+mainWidth*0.2
}

func isInkArrow(strokes []inkGestureStroke) bool {
	if len(strokes) < 2 || len(strokes) > 3 {
		return false
	}
	mainIdx := 0
	mainLen := 0.0
	for i, stroke := range strokes {
		if length := inkGestureLength(stroke.Points); length > mainLen {
			mainLen = length
			mainIdx = i
		}
	}
	main := strokes[mainIdx]
	mainStart := main.Points[0]
	mainEnd := main.Points[len(main.Points)-1]
	mainVectorX := mainEnd.X - mainStart.X
	mainVectorY := mainEnd.Y - mainStart.Y
	if math.Hypot(mainVectorX, mainVectorY) < 16 {
		return false
	}
	headCount := 0
	for i, stroke := range strokes {
		if i == mainIdx {
			continue
		}
		start := stroke.Points[0]
		end := stroke.Points[len(stroke.Points)-1]
		if inkGestureDistance(start, mainEnd) > mainLen*0.35 && inkGestureDistance(end, mainEnd) > mainLen*0.35 {
			continue
		}
		sx := end.X - start.X
		sy := end.Y - start.Y
		if math.Hypot(sx, sy) < 6 {
			continue
		}
		angle := inkGestureAngleBetween(mainVectorX, mainVectorY, sx, sy)
		if angle >= 20 && angle <= 80 {
			headCount++
		}
	}
	return headCount >= 1
}

func inkGestureDiagonal(stroke inkGestureStroke) bool {
	start := stroke.Points[0]
	end := stroke.Points[len(stroke.Points)-1]
	dx := math.Abs(end.X - start.X)
	dy := math.Abs(end.Y - start.Y)
	return dx >= 6 && dy >= 6 && dx <= dy*3 && dy <= dx*3
}

func inkGestureBounds(points []inkGesturePoint) (float64, float64, float64, float64) {
	minX := points[0].X
	minY := points[0].Y
	maxX := points[0].X
	maxY := points[0].Y
	for _, point := range points[1:] {
		minX = math.Min(minX, point.X)
		minY = math.Min(minY, point.Y)
		maxX = math.Max(maxX, point.X)
		maxY = math.Max(maxY, point.Y)
	}
	return minX, minY, maxX, maxY
}

func inkGestureLength(points []inkGesturePoint) float64 {
	total := 0.0
	for i := 1; i < len(points); i++ {
		total += inkGestureDistance(points[i-1], points[i])
	}
	return total
}

func inkGestureDistance(a, b inkGesturePoint) float64 {
	return math.Hypot(b.X-a.X, b.Y-a.Y)
}

func inkGestureAngleBetween(ax, ay, bx, by float64) float64 {
	aLen := math.Hypot(ax, ay)
	bLen := math.Hypot(bx, by)
	if aLen == 0 || bLen == 0 {
		return 180
	}
	cosTheta := ((ax * bx) + (ay * by)) / (aLen * bLen)
	if cosTheta > 1 {
		cosTheta = 1
	} else if cosTheta < -1 {
		cosTheta = -1
	}
	return math.Abs(math.Acos(cosTheta) * 180 / math.Pi)
}

func inkGestureSegmentIntersects(a1, a2, b1, b2 inkGesturePoint) bool {
	return inkGestureOrientation(a1, a2, b1)*inkGestureOrientation(a1, a2, b2) <= 0 &&
		inkGestureOrientation(b1, b2, a1)*inkGestureOrientation(b1, b2, a2) <= 0
}

func inkGestureOrientation(a, b, c inkGesturePoint) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}
