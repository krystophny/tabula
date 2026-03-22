package web

import "testing"

func TestLatestCanvasPositionVisualAttachment(t *testing.T) {
	attachment := latestCanvasPositionVisualAttachment([]*chatCanvasPositionEvent{
		{
			Cursor:          &chatCursorContext{Title: "doc.pdf", Page: 1},
			SnapshotDataURL: "data:image/png;base64,Zm9v",
		},
	})
	if attachment == nil {
		t.Fatal("expected visual attachment")
	}
	if attachment.DataURL != "data:image/png;base64,Zm9v" {
		t.Fatalf("DataURL = %q, want original snapshot", attachment.DataURL)
	}
}

func TestBuildAppServerTurnInput_IncludesImageURL(t *testing.T) {
	input := buildAppServerTurnInput("inspect this", &chatVisualAttachment{
		DataURL: "data:image/png;base64,Zm9v",
	})
	if len(input) != 2 {
		t.Fatalf("len(input) = %d, want 2", len(input))
	}
	if got := input[1]["type"]; got != "image_url" {
		t.Fatalf("input[1].type = %v, want image_url", got)
	}
	if got := input[1]["image_url"]; got != "data:image/png;base64,Zm9v" {
		t.Fatalf("input[1].image_url = %v, want data URL", got)
	}
}

func TestBuildLocalAssistantUserContent_IncludesImageURLObject(t *testing.T) {
	content := buildLocalAssistantUserContent("inspect this", &chatVisualAttachment{
		DataURL: "data:image/png;base64,Zm9v",
	})
	if len(content) != 2 {
		t.Fatalf("len(content) = %d, want 2", len(content))
	}
	if got := content[1]["type"]; got != "image_url" {
		t.Fatalf("content[1].type = %v, want image_url", got)
	}
	imageURL, _ := content[1]["image_url"].(map[string]any)
	if got := imageURL["url"]; got != "data:image/png;base64,Zm9v" {
		t.Fatalf("content[1].image_url.url = %v, want data URL", got)
	}
}
