package aggregateitem

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCommitmentProjectionCoversAcceptedSourceKinds(t *testing.T) {
	commitment := Commitment{
		Title:  "Review GTD boundary",
		Status: "open",
		SourceBindings: []SourceBinding{
			binding("markdown", "brain/commitments/task.md", false),
			binding("todoist", "task-1", true),
			binding("github", "sloppy-org/slopshell#725", true),
			binding("gitlab", "plasma/slopshell#11", true),
			binding("mail", "AAMk-msg", true),
			binding("manual", "local-note", false),
		},
	}

	got := commitment.Projection("brain/commitments/task.md", "brain/commitments/task.md")

	wantKinds := []string{
		SourceKindEmail,
		SourceKindGitHub,
		SourceKindGitLab,
		SourceKindLocal,
		SourceKindMarkdown,
		SourceKindTodoist,
	}
	if !reflect.DeepEqual(got.SourceKinds, wantKinds) {
		t.Fatalf("SourceKinds = %#v, want %#v", got.SourceKinds, wantKinds)
	}
	if got.Writeable != true {
		t.Fatalf("Writeable = false, want true")
	}
	if len(got.BindingIDs) != 6 {
		t.Fatalf("BindingIDs = %#v, want all bindings", got.BindingIDs)
	}
	if !containsString(got.BindingIDs, "mail:AAMk-msg") {
		t.Fatalf("BindingIDs = %#v, want case-preserved mail ref", got.BindingIDs)
	}
}

func TestCommitmentProjectionSeparatesSourceFieldsAndLocalOverlay(t *testing.T) {
	commitment := Commitment{
		Title:      "Remote source title",
		Status:     "open",
		Due:        "2026-05-04",
		FollowUp:   "2026-05-01",
		Actor:      "remote-owner",
		Outcome:    "Ship review fix",
		NextAction: "Update PR",
		Sphere:     "work",
		Labels:     []string{"review", "gtd"},
		SourceBindings: []SourceBinding{
			binding("todoist", "task-1", true),
		},
		LocalOverlay: LocalOverlay{
			Status:    "closed",
			Due:       "2026-05-07",
			FollowUp:  "2026-05-02",
			Actor:     "chris",
			ClosedAt:  "2026-04-29T18:00:00Z",
			ClosedVia: "slopshell",
		},
	}

	got := commitment.Projection("brain/commitments/review.md")

	if got.Title != "Remote source title" {
		t.Fatalf("Title = %q, want source title", got.Title)
	}
	if got.Status != "closed" || got.Due != "2026-05-07" || got.FollowUp != "2026-05-02" || got.Actor != "chris" {
		t.Fatalf("overlay fields not projected: %#v", got)
	}
	if got.Outcome != "Ship review fix" || got.NextAction != "Update PR" || got.Sphere != "work" {
		t.Fatalf("source fields not projected: %#v", got)
	}
	if got.ClosedAt != "2026-04-29T18:00:00Z" || got.ClosedVia != "slopshell" {
		t.Fatalf("closure overlay not projected: %#v", got)
	}
}

func TestCommitmentSerializationPreservesOverlayAndBindings(t *testing.T) {
	commitment := Commitment{
		Title:  "Round trip",
		Status: "open",
		SourceBindings: []SourceBinding{{
			Provider:         "github",
			Ref:              "sloppy-org/slopshell#725",
			Location:         BindingLocation{Path: "brain/commitments/round-trip.md", Anchor: "L7"},
			URL:              "https://github.com/sloppy-org/slopshell/issues/725",
			Writeable:        true,
			AuthoritativeFor: []string{"title", "status"},
			Summary:          "review issue",
		}},
		LocalOverlay: LocalOverlay{
			Status:    "waiting",
			FollowUp:  "2026-05-02",
			ClosedVia: "slopshell",
		},
	}

	data, err := json.Marshal(commitment)
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	var got Commitment
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal() error: %v", err)
	}

	if got.SourceBindings[0].Location.Path != "brain/commitments/round-trip.md" {
		t.Fatalf("binding location = %#v", got.SourceBindings[0].Location)
	}
	if !got.SourceBindings[0].Writeable || !reflect.DeepEqual(got.SourceBindings[0].AuthoritativeFor, []string{"title", "status"}) {
		t.Fatalf("extended binding schema lost: %#v", got.SourceBindings[0])
	}
	if got.LocalOverlay.Status != "waiting" || got.LocalOverlay.FollowUp != "2026-05-02" {
		t.Fatalf("local_overlay lost: %#v", got.LocalOverlay)
	}
}

func TestAggregateProjectionUsesSloptoolsScanAggregateShape(t *testing.T) {
	aggregate := Aggregate{
		ID:          "gtd-aggregate-1",
		Paths:       []string{"brain/commitments/a.md", "brain/commitments/b.md"},
		Title:       "Send alpha budget",
		Outcome:     "Budget sent",
		ReviewState: "open",
		SourceBindings: []SourceBinding{
			binding("github", "sloppy-org/slopshell#725", true),
			binding("todoist", "task-1", false),
		},
	}

	got := aggregate.Projection()

	if got.Title != "Send alpha budget" || got.Outcome != "Budget sent" || got.ReviewState != "open" {
		t.Fatalf("projection source fields = %#v", got)
	}
	if !reflect.DeepEqual(got.SourceKinds, []string{SourceKindGitHub, SourceKindTodoist}) {
		t.Fatalf("SourceKinds = %#v", got.SourceKinds)
	}
	if !got.Writeable {
		t.Fatalf("Writeable = false, want true")
	}
}

func binding(provider, ref string, writeable bool) SourceBinding {
	return SourceBinding{
		Provider:         provider,
		Ref:              ref,
		Location:         BindingLocation{Path: "brain/commitments/item.md"},
		Writeable:        writeable,
		AuthoritativeFor: []string{"title", "status"},
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
