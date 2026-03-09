package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/krystophny/tabura/internal/store"
)

type ideaNoteMeta struct {
	Title            string                `json:"title,omitempty"`
	Transcript       string                `json:"transcript,omitempty"`
	CaptureMode      string                `json:"capture_mode,omitempty"`
	CapturedAt       string                `json:"captured_at,omitempty"`
	Workspace        string                `json:"workspace,omitempty"`
	Notes            []string              `json:"notes,omitempty"`
	Refinements      []ideaNoteRefinement  `json:"refinements,omitempty"`
	PromotionPreview *ideaPromotionPreview `json:"promotion_preview,omitempty"`
	Promotions       []ideaPromotionRecord `json:"promotions,omitempty"`
}

type ideaNoteRefinement struct {
	Kind      string `json:"kind,omitempty"`
	Heading   string `json:"heading,omitempty"`
	Prompt    string `json:"prompt,omitempty"`
	Body      string `json:"body,omitempty"`
	RefinedAt string `json:"refined_at,omitempty"`
}

type ideaPromotionPreview struct {
	Target     string                   `json:"target,omitempty"`
	CreatedAt  string                   `json:"created_at,omitempty"`
	Candidates []ideaPromotionCandidate `json:"candidates,omitempty"`
	Issue      *ideaPromotionIssueDraft `json:"issue,omitempty"`
}

type ideaPromotionCandidate struct {
	Index   int    `json:"index,omitempty"`
	Title   string `json:"title,omitempty"`
	Details string `json:"details,omitempty"`
}

type ideaPromotionIssueDraft struct {
	Title string `json:"title,omitempty"`
	Body  string `json:"body,omitempty"`
}

type ideaPromotionRecord struct {
	Target    string   `json:"target,omitempty"`
	CreatedAt string   `json:"created_at,omitempty"`
	Count     int      `json:"count,omitempty"`
	Refs      []string `json:"refs,omitempty"`
}

func ideaNoteString(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func ideaArtifactMeta(title, transcript, captureMode, workspaceName string, capturedAt time.Time) (*string, error) {
	meta := ideaNoteMeta{
		Title:       strings.TrimSpace(title),
		Transcript:  normalizeIdeaText(transcript),
		CaptureMode: normalizeChatCaptureMode(captureMode),
		CapturedAt:  capturedAt.UTC().Format(time.RFC3339),
		Workspace:   strings.TrimSpace(workspaceName),
	}
	if meta.Transcript != "" {
		meta.Notes = []string{meta.Transcript}
	}
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	return &text, nil
}

func parseIdeaNoteMeta(metaJSON *string, fallbackTitle string) ideaNoteMeta {
	meta := ideaNoteMeta{
		Title: strings.TrimSpace(fallbackTitle),
	}
	if metaJSON != nil {
		if raw := strings.TrimSpace(*metaJSON); raw != "" {
			var parsed ideaNoteMeta
			if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
				meta = parsed
				if strings.TrimSpace(meta.Title) == "" {
					meta.Title = strings.TrimSpace(fallbackTitle)
				}
			}
		}
	}
	meta.Title = strings.TrimSpace(meta.Title)
	meta.Transcript = normalizeIdeaText(meta.Transcript)
	meta.CaptureMode = normalizeChatCaptureMode(meta.CaptureMode)
	meta.Workspace = strings.TrimSpace(meta.Workspace)
	meta.CapturedAt = strings.TrimSpace(meta.CapturedAt)
	meta.Notes = normalizeIdeaNoteLines(meta.Notes)
	if len(meta.Notes) == 0 && meta.Transcript != "" {
		meta.Notes = []string{meta.Transcript}
	}
	out := make([]ideaNoteRefinement, 0, len(meta.Refinements))
	for _, refinement := range meta.Refinements {
		refinement.Kind = strings.TrimSpace(refinement.Kind)
		refinement.Heading = strings.TrimSpace(refinement.Heading)
		refinement.Prompt = strings.TrimSpace(refinement.Prompt)
		refinement.Body = strings.TrimSpace(refinement.Body)
		refinement.RefinedAt = strings.TrimSpace(refinement.RefinedAt)
		if refinement.Body == "" {
			continue
		}
		if refinement.Heading == "" {
			refinement.Heading = ideaRefinementHeading(refinement.Kind)
		}
		out = append(out, refinement)
	}
	meta.Refinements = out
	meta.PromotionPreview = normalizeIdeaPromotionPreview(meta.PromotionPreview)
	meta.Promotions = normalizeIdeaPromotionRecords(meta.Promotions)
	return meta
}

func encodeIdeaNoteMeta(meta ideaNoteMeta) (*string, error) {
	meta = parseIdeaNoteMetaPtr(meta)
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil, err
	}
	text := string(raw)
	return &text, nil
}

func parseIdeaNoteMetaPtr(meta ideaNoteMeta) ideaNoteMeta {
	normalized := parseIdeaNoteMeta(nil, meta.Title)
	normalized.Transcript = meta.Transcript
	normalized.CaptureMode = meta.CaptureMode
	normalized.CapturedAt = meta.CapturedAt
	normalized.Workspace = meta.Workspace
	normalized.Notes = meta.Notes
	normalized.Refinements = meta.Refinements
	normalized.PromotionPreview = meta.PromotionPreview
	normalized.Promotions = meta.Promotions
	return parseIdeaNoteMeta(mustJSONString(normalized), normalized.Title)
}

func mustJSONString(meta ideaNoteMeta) *string {
	raw, err := json.Marshal(meta)
	if err != nil {
		text := "{}"
		return &text
	}
	text := string(raw)
	return &text
}

func normalizeIdeaNoteLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	seen := map[string]struct{}{}
	for _, line := range lines {
		clean := normalizeIdeaText(line)
		if clean == "" {
			continue
		}
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
}

func normalizeIdeaPromotionPreview(preview *ideaPromotionPreview) *ideaPromotionPreview {
	if preview == nil {
		return nil
	}
	normalized := &ideaPromotionPreview{
		Target:    normalizeIdeaPromotionTarget(preview.Target),
		CreatedAt: strings.TrimSpace(preview.CreatedAt),
	}
	for _, candidate := range preview.Candidates {
		title := strings.TrimSpace(candidate.Title)
		details := strings.TrimSpace(candidate.Details)
		if title == "" {
			continue
		}
		index := candidate.Index
		if index <= 0 {
			index = len(normalized.Candidates) + 1
		}
		normalized.Candidates = append(normalized.Candidates, ideaPromotionCandidate{
			Index:   index,
			Title:   title,
			Details: details,
		})
	}
	if preview.Issue != nil {
		title := strings.TrimSpace(preview.Issue.Title)
		body := strings.TrimSpace(preview.Issue.Body)
		if title != "" || body != "" {
			normalized.Issue = &ideaPromotionIssueDraft{
				Title: title,
				Body:  body,
			}
		}
	}
	if normalized.Target == "" {
		return nil
	}
	if normalized.Target == ideaPromotionTargetGitHub && normalized.Issue == nil {
		return nil
	}
	if normalized.Target != ideaPromotionTargetGitHub && len(normalized.Candidates) == 0 {
		return nil
	}
	return normalized
}

func normalizeIdeaPromotionRecords(records []ideaPromotionRecord) []ideaPromotionRecord {
	if len(records) == 0 {
		return nil
	}
	out := make([]ideaPromotionRecord, 0, len(records))
	for _, record := range records {
		target := normalizeIdeaPromotionTarget(record.Target)
		if target == "" {
			continue
		}
		refs := make([]string, 0, len(record.Refs))
		for _, ref := range record.Refs {
			clean := strings.TrimSpace(ref)
			if clean != "" {
				refs = append(refs, clean)
			}
		}
		out = append(out, ideaPromotionRecord{
			Target:    target,
			CreatedAt: strings.TrimSpace(record.CreatedAt),
			Count:     record.Count,
			Refs:      refs,
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func renderIdeaNoteMarkdown(meta ideaNoteMeta) string {
	meta = parseIdeaNoteMetaPtr(meta)
	title := strings.TrimSpace(meta.Title)
	if title == "" {
		title = "Idea"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", title)
	b.WriteString("## Notes\n")
	if len(meta.Notes) == 0 {
		b.WriteString("- No notes yet.\n")
	} else {
		for _, note := range meta.Notes {
			fmt.Fprintf(&b, "- %s\n", note)
		}
	}
	b.WriteString("\n## Context\n")
	contextLines := 0
	if meta.CaptureMode != "" {
		fmt.Fprintf(&b, "- Captured: %s\n", meta.CaptureMode)
		contextLines++
	}
	if meta.Workspace != "" {
		fmt.Fprintf(&b, "- Workspace: %s\n", meta.Workspace)
		contextLines++
	}
	if meta.CapturedAt != "" {
		fmt.Fprintf(&b, "- Date: %s\n", meta.CapturedAt)
		contextLines++
	}
	if contextLines == 0 {
		b.WriteString("- Date: unavailable\n")
	}
	for _, refinement := range meta.Refinements {
		heading := strings.TrimSpace(refinement.Heading)
		if heading == "" {
			heading = ideaRefinementHeading(refinement.Kind)
		}
		body := strings.TrimSpace(refinement.Body)
		if body == "" {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", heading, body)
	}
	appendIdeaPromotionPreviewMarkdown(&b, meta.PromotionPreview)
	appendIdeaPromotionHistoryMarkdown(&b, meta.Promotions)
	return strings.TrimSpace(b.String())
}

func appendIdeaPromotionPreviewMarkdown(b *strings.Builder, preview *ideaPromotionPreview) {
	preview = normalizeIdeaPromotionPreview(preview)
	if preview == nil {
		return
	}
	b.WriteString("\n\n## Promotion Review\n\n")
	switch preview.Target {
	case ideaPromotionTargetTask:
		b.WriteString("- Pending: task draft\n")
		b.WriteString("- Confirm with: `create this idea task`\n")
	case ideaPromotionTargetItems:
		b.WriteString("- Pending: item proposals\n")
		b.WriteString("- Confirm with: `create these idea items` or `create selected idea items 1,2`\n")
	case ideaPromotionTargetGitHub:
		b.WriteString("- Pending: GitHub issue draft\n")
		b.WriteString("- Confirm with: `create this idea GitHub issue`\n")
	}
	b.WriteString("- Optional: add `and mark this idea done` or `and keep this idea`\n")
	switch preview.Target {
	case ideaPromotionTargetTask, ideaPromotionTargetItems:
		for _, candidate := range preview.Candidates {
			fmt.Fprintf(b, "\n### %d. %s\n", candidate.Index, candidate.Title)
			if candidate.Details != "" {
				fmt.Fprintf(b, "\n%s\n", candidate.Details)
			}
		}
	case ideaPromotionTargetGitHub:
		if preview.Issue != nil {
			fmt.Fprintf(b, "\n### %s\n", preview.Issue.Title)
			if preview.Issue.Body != "" {
				fmt.Fprintf(b, "\n%s\n", preview.Issue.Body)
			}
		}
	}
}

func appendIdeaPromotionHistoryMarkdown(b *strings.Builder, records []ideaPromotionRecord) {
	records = normalizeIdeaPromotionRecords(records)
	if len(records) == 0 {
		return
	}
	b.WriteString("\n\n## Promotions\n")
	for _, record := range records {
		label := record.Target
		switch record.Target {
		case ideaPromotionTargetTask:
			label = "task"
		case ideaPromotionTargetItems:
			label = "items"
		case ideaPromotionTargetGitHub:
			label = "GitHub issue"
		}
		line := fmt.Sprintf("- %s", label)
		if record.Count > 0 {
			line += fmt.Sprintf(" x%d", record.Count)
		}
		if record.CreatedAt != "" {
			line += fmt.Sprintf(" on %s", record.CreatedAt)
		}
		if len(record.Refs) > 0 {
			line += fmt.Sprintf(" [%s]", strings.Join(record.Refs, ", "))
		}
		b.WriteString(line + "\n")
	}
}

func parseInlineIdeaRefinementIntent(text string) *SystemAction {
	normalized := normalizeItemCommandText(text)
	if normalized == "" {
		return nil
	}
	kind := ""
	switch {
	case normalized == "expand this idea" || normalized == "expand this" || normalized == "expand on this idea" || normalized == "refine this idea" || normalized == "baue diese idee aus" || normalized == "baue das aus" || normalized == "verfeinere diese idee":
		kind = "expand"
	case strings.Contains(normalized, "pros and cons") || strings.Contains(normalized, "vor und nachteile") || strings.Contains(normalized, "vor- und nachteile") || strings.Contains(normalized, "pro und contra"):
		kind = "pros_cons"
	case normalized == "compare alternatives" || normalized == "compare options" || normalized == "show alternatives" || normalized == "vergleiche alternativen" || normalized == "vergleiche optionen" || normalized == "zeige alternativen":
		kind = "alternatives"
	case normalized == "outline an implementation" || normalized == "outline implementation" || normalized == "draft implementation" || normalized == "skizziere eine umsetzung" || normalized == "umsetzung skizzieren" || normalized == "entwirf eine umsetzung":
		kind = "implementation"
	}
	if kind == "" {
		return nil
	}
	return &SystemAction{
		Action: "refine_idea_note",
		Params: map[string]interface{}{
			"kind": kind,
			"text": strings.TrimSpace(text),
		},
	}
}

func ideaRefinementHeading(kind string) string {
	switch strings.TrimSpace(kind) {
	case "expand":
		return "Expansion"
	case "pros_cons":
		return "Pros and Cons"
	case "alternatives":
		return "Alternatives"
	case "implementation":
		return "Implementation Outline"
	default:
		return "Idea Notes"
	}
}

func generateIdeaNoteRefinement(meta ideaNoteMeta, kind, prompt string, refinedAt time.Time) ideaNoteRefinement {
	subject := strings.TrimSpace(meta.Title)
	if subject == "" {
		subject = "This idea"
	}
	summary := strings.TrimSpace(meta.Transcript)
	if summary == "" && len(meta.Notes) > 0 {
		summary = strings.TrimSpace(meta.Notes[0])
	}
	if summary == "" {
		summary = subject
	}
	body := buildIdeaNoteRefinementBody(kind, subject, summary)
	return ideaNoteRefinement{
		Kind:      strings.TrimSpace(kind),
		Heading:   ideaRefinementHeading(kind),
		Prompt:    strings.TrimSpace(prompt),
		Body:      body,
		RefinedAt: refinedAt.UTC().Format(time.RFC3339),
	}
}

func buildIdeaNoteRefinementBody(kind, subject, summary string) string {
	switch strings.TrimSpace(kind) {
	case "expand":
		return strings.Join([]string{
			fmt.Sprintf("%s can be turned into a focused workflow instead of a one-off capture.", subject),
			"",
			fmt.Sprintf("- Clarify the target outcome behind: %s", summary),
			"- Start with the smallest slice that proves the workflow is useful.",
			"- Decide which parts should stay manual and which parts should become repeatable automation.",
		}, "\n")
	case "pros_cons":
		return strings.Join([]string{
			"### Pros",
			fmt.Sprintf("- Keeps the work centered on a concrete outcome: %s", subject),
			"- Creates a reusable structure for follow-up, review, and delegation.",
			"",
			"### Cons",
			"- Adds implementation scope that needs clear boundaries to avoid drift.",
			"- Will need a lightweight review loop so captured notes stay accurate over time.",
		}, "\n")
	case "alternatives":
		return strings.Join([]string{
			"1. Lightweight path: keep the idea as a single note and only add minimal metadata for quick retrieval.",
			"2. Structured path: split the idea into explicit capture, review, and execution stages with clearer ownership.",
			"3. Deferred path: park the idea until there is a stronger trigger or a specific user workflow to anchor it.",
		}, "\n")
	case "implementation":
		return strings.Join([]string{
			"1. Capture the current workflow and identify the single user outcome that matters most.",
			fmt.Sprintf("2. Implement the narrowest end-to-end slice for %s.", subject),
			"3. Add regression coverage for capture, rendering, and follow-up refinement so the note stays editable.",
		}, "\n")
	default:
		return fmt.Sprintf("- %s", summary)
	}
}

func (a *App) resolveActiveIdeaNoteArtifact(projectKey string) (*store.Artifact, error) {
	canvas := a.resolveCanvasContext(projectKey)
	if canvas == nil || strings.TrimSpace(canvas.ArtifactTitle) == "" {
		return nil, errors.New("open the idea note on canvas first")
	}
	title := strings.TrimSpace(canvas.ArtifactTitle)
	artifacts, err := a.store.ListArtifactsByKind(store.ArtifactKindIdeaNote)
	if err != nil {
		return nil, err
	}
	for _, artifact := range artifacts {
		if ideaNoteString(artifact.Title) == title {
			candidate := artifact
			return &candidate, nil
		}
	}
	return nil, errors.New("active canvas artifact is not an idea note")
}

func (a *App) renderIdeaNoteOnCanvas(projectKey, title string, meta ideaNoteMeta) error {
	canvasSessionID := strings.TrimSpace(a.resolveCanvasSessionID(projectKey))
	if canvasSessionID == "" {
		return errors.New("canvas session is not available")
	}
	port, ok := a.tunnels.getPort(canvasSessionID)
	if !ok {
		return errors.New("canvas tunnel is not available")
	}
	_, err := a.mcpToolsCall(port, "canvas_artifact_show", map[string]interface{}{
		"session_id":       canvasSessionID,
		"kind":             "text",
		"title":            strings.TrimSpace(title),
		"markdown_or_text": renderIdeaNoteMarkdown(meta),
	})
	return err
}

func (a *App) refineConversationIdea(session store.ChatSession, action *SystemAction) (string, map[string]interface{}, error) {
	if action == nil {
		return "", nil, errors.New("idea action is required")
	}
	artifact, err := a.resolveActiveIdeaNoteArtifact(session.ProjectKey)
	if err != nil {
		return "", nil, err
	}
	meta := parseIdeaNoteMeta(artifact.MetaJSON, ideaNoteString(artifact.Title))
	refinement := generateIdeaNoteRefinement(
		meta,
		systemActionStringParam(action.Params, "kind"),
		systemActionStringParam(action.Params, "text"),
		time.Now().UTC(),
	)
	meta.Refinements = append(meta.Refinements, refinement)
	metaJSON, err := encodeIdeaNoteMeta(meta)
	if err != nil {
		return "", nil, err
	}
	if err := a.store.UpdateArtifact(artifact.ID, store.ArtifactUpdate{MetaJSON: metaJSON}); err != nil {
		return "", nil, err
	}
	if err := a.renderIdeaNoteOnCanvas(session.ProjectKey, meta.Title, meta); err != nil {
		return "", nil, err
	}
	return fmt.Sprintf("Updated idea note with %s.", refinement.Heading), map[string]interface{}{
		"type":        "artifact_updated",
		"artifact_id": artifact.ID,
		"artifact":    meta.Title,
		"heading":     refinement.Heading,
	}, nil
}
