package aggregateitem

import (
	"sort"
	"strings"
)

const (
	SourceKindEmail    = "email"
	SourceKindGitHub   = "github"
	SourceKindGitLab   = "gitlab"
	SourceKindLocal    = "local"
	SourceKindMarkdown = "markdown"
	SourceKindTodoist  = "todoist"
)

type ScanResult struct {
	Aggregates []Aggregate `json:"aggregates"`
	Candidates []Candidate `json:"candidates,omitempty"`
	Changed    bool        `json:"changed"`
}

type Aggregate struct {
	ID             string          `json:"id"`
	Paths          []string        `json:"paths"`
	Title          string          `json:"title,omitempty"`
	Outcome        string          `json:"outcome,omitempty"`
	SourceBindings []SourceBinding `json:"bindings,omitempty"`
	BindingIDs     []string        `json:"binding_ids,omitempty"`
	ReviewState    string          `json:"review_state,omitempty"`
}

type Candidate struct {
	ID          string   `json:"id"`
	Paths       []string `json:"paths"`
	Score       float64  `json:"score"`
	Confidence  float64  `json:"confidence"`
	Reasoning   string   `json:"reasoning"`
	Detector    string   `json:"detector"`
	ReviewState string   `json:"review_state"`
}

type Commitment struct {
	Title          string          `json:"title,omitempty"`
	Kind           string          `json:"kind,omitempty"`
	Sphere         string          `json:"sphere,omitempty"`
	Status         string          `json:"status,omitempty"`
	Outcome        string          `json:"outcome,omitempty"`
	NextAction     string          `json:"next_action,omitempty"`
	Context        string          `json:"context,omitempty"`
	FollowUp       string          `json:"follow_up,omitempty"`
	Due            string          `json:"due,omitempty"`
	Actor          string          `json:"actor,omitempty"`
	WaitingFor     string          `json:"waiting_for,omitempty"`
	Project        string          `json:"project,omitempty"`
	LastEvidenceAt string          `json:"last_evidence_at,omitempty"`
	ReviewState    string          `json:"review_state,omitempty"`
	People         []string        `json:"people,omitempty"`
	Labels         []string        `json:"labels,omitempty"`
	SourceBindings []SourceBinding `json:"source_bindings,omitempty"`
	LocalOverlay   LocalOverlay    `json:"local_overlay,omitempty"`
}

type SourceBinding struct {
	Provider         string          `json:"provider"`
	Ref              string          `json:"ref"`
	Location         BindingLocation `json:"location,omitempty"`
	URL              string          `json:"url,omitempty"`
	Writeable        bool            `json:"writeable"`
	AuthoritativeFor []string        `json:"authoritative_for,omitempty"`
	Summary          string          `json:"summary,omitempty"`
	CreatedAt        string          `json:"created_at,omitempty"`
	UpdatedAt        string          `json:"updated_at,omitempty"`
}

type BindingLocation struct {
	Path   string `json:"path,omitempty"`
	Anchor string `json:"anchor,omitempty"`
}

type LocalOverlay struct {
	Status    string `json:"status,omitempty"`
	FollowUp  string `json:"follow_up,omitempty"`
	Due       string `json:"due,omitempty"`
	Actor     string `json:"actor,omitempty"`
	ClosedAt  string `json:"closed_at,omitempty"`
	ClosedVia string `json:"closed_via,omitempty"`
}

type Projection struct {
	ID             string   `json:"id"`
	Paths          []string `json:"paths,omitempty"`
	Title          string   `json:"title"`
	Status         string   `json:"status,omitempty"`
	Sphere         string   `json:"sphere,omitempty"`
	Outcome        string   `json:"outcome,omitempty"`
	NextAction     string   `json:"next_action,omitempty"`
	Context        string   `json:"context,omitempty"`
	FollowUp       string   `json:"follow_up,omitempty"`
	Due            string   `json:"due,omitempty"`
	Actor          string   `json:"actor,omitempty"`
	WaitingFor     string   `json:"waiting_for,omitempty"`
	Project        string   `json:"project,omitempty"`
	LastEvidenceAt string   `json:"last_evidence_at,omitempty"`
	ReviewState    string   `json:"review_state,omitempty"`
	People         []string `json:"people,omitempty"`
	Labels         []string `json:"labels,omitempty"`
	SourceKinds    []string `json:"source_kinds,omitempty"`
	Providers      []string `json:"providers,omitempty"`
	BindingIDs     []string `json:"binding_ids,omitempty"`
	Writeable      bool     `json:"writeable,omitempty"`
	ClosedAt       string   `json:"closed_at,omitempty"`
	ClosedVia      string   `json:"closed_via,omitempty"`
}

func (a Aggregate) Projection() Projection {
	bindings := normalizeSourceBindings(a.SourceBindings)
	return Projection{
		ID:          strings.TrimSpace(a.ID),
		Paths:       cleanStrings(a.Paths),
		Title:       strings.TrimSpace(a.Title),
		Outcome:     strings.TrimSpace(a.Outcome),
		ReviewState: strings.TrimSpace(a.ReviewState),
		SourceKinds: sourceKinds(bindings),
		Providers:   providers(bindings),
		BindingIDs:  bindingIDs(bindings, a.BindingIDs),
		Writeable:   hasWriteableBinding(bindings),
	}
}

func (c Commitment) Projection(id string, paths ...string) Projection {
	bindings := normalizeSourceBindings(c.SourceBindings)
	overlay := c.LocalOverlay
	return Projection{
		ID:             strings.TrimSpace(id),
		Paths:          cleanStrings(paths),
		Title:          strings.TrimSpace(c.Title),
		Status:         firstNonEmpty(overlay.Status, c.Status),
		Sphere:         strings.TrimSpace(c.Sphere),
		Outcome:        strings.TrimSpace(c.Outcome),
		NextAction:     strings.TrimSpace(c.NextAction),
		Context:        strings.TrimSpace(c.Context),
		FollowUp:       firstNonEmpty(overlay.FollowUp, c.FollowUp),
		Due:            firstNonEmpty(overlay.Due, c.Due),
		Actor:          firstNonEmpty(overlay.Actor, c.Actor),
		WaitingFor:     strings.TrimSpace(c.WaitingFor),
		Project:        strings.TrimSpace(c.Project),
		LastEvidenceAt: strings.TrimSpace(c.LastEvidenceAt),
		ReviewState:    strings.TrimSpace(c.ReviewState),
		People:         cleanStrings(c.People),
		Labels:         cleanStrings(c.Labels),
		SourceKinds:    sourceKinds(bindings),
		Providers:      providers(bindings),
		BindingIDs:     bindingIDs(bindings, nil),
		Writeable:      hasWriteableBinding(bindings),
		ClosedAt:       strings.TrimSpace(overlay.ClosedAt),
		ClosedVia:      strings.TrimSpace(overlay.ClosedVia),
	}
}

func normalizeSourceBindings(bindings []SourceBinding) []SourceBinding {
	out := make([]SourceBinding, 0, len(bindings))
	for _, binding := range bindings {
		binding.Provider = strings.ToLower(strings.TrimSpace(binding.Provider))
		binding.Ref = strings.TrimSpace(binding.Ref)
		binding.URL = strings.TrimSpace(binding.URL)
		binding.Summary = strings.TrimSpace(binding.Summary)
		binding.CreatedAt = strings.TrimSpace(binding.CreatedAt)
		binding.UpdatedAt = strings.TrimSpace(binding.UpdatedAt)
		binding.Location.Path = strings.TrimSpace(binding.Location.Path)
		binding.Location.Anchor = strings.TrimSpace(binding.Location.Anchor)
		binding.AuthoritativeFor = cleanStrings(binding.AuthoritativeFor)
		if binding.Provider != "" && binding.Ref != "" {
			out = append(out, binding)
		}
	}
	return out
}

func sourceKinds(bindings []SourceBinding) []string {
	return uniqueSorted(bindingValues(bindings, bindingKind))
}

func providers(bindings []SourceBinding) []string {
	return uniqueSorted(bindingValues(bindings, func(binding SourceBinding) string {
		return binding.Provider
	}))
}

func bindingIDs(bindings []SourceBinding, existing []string) []string {
	ids := cleanStrings(existing)
	for _, binding := range bindings {
		ids = append(ids, binding.Provider+":"+binding.Ref)
	}
	return uniqueSortedIDs(ids)
}

func hasWriteableBinding(bindings []SourceBinding) bool {
	for _, binding := range bindings {
		if binding.Writeable {
			return true
		}
	}
	return false
}

func bindingValues(bindings []SourceBinding, value func(SourceBinding) string) []string {
	out := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if got := strings.TrimSpace(value(binding)); got != "" {
			out = append(out, got)
		}
	}
	return out
}

func bindingKind(binding SourceBinding) string {
	switch binding.Provider {
	case "github":
		return SourceKindGitHub
	case "gitlab":
		return SourceKindGitLab
	case "todoist":
		return SourceKindTodoist
	case "markdown", "meetings":
		return SourceKindMarkdown
	case "gmail", "imap", "exchange", "exchange_ews", "mail":
		return SourceKindEmail
	case "local", "manual":
		return SourceKindLocal
	default:
		return binding.Provider
	}
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueSortedIDs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		key := strings.ToLower(value)
		if value == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i]) < strings.ToLower(out[j])
	})
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
