package surface

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRootFromCaller(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller() failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func TestGestureTruthTableDocIsIndexedAndStructured(t *testing.T) {
	root := repoRootFromCaller(t)

	truthTablePath := filepath.Join(root, "docs", "gesture-truth-table.md")
	truthTable, err := os.ReadFile(truthTablePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", truthTablePath, err)
	}
	content := string(truthTable)

	requiredSnippets := []string{
		"| Input | Blank surface | Artifact visible | Annotation mode | Dialogue live | Meeting live |",
		"Canonical tap-to-voice rule",
		"`start local capture bound to the current context`",
		"Live session state wins over ordinary prompt/annotation routing.",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("%s missing snippet %q", truthTablePath, snippet)
		}
	}

	specIndexPath := filepath.Join(root, "docs", "spec-index.md")
	specIndex, err := os.ReadFile(specIndexPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", specIndexPath, err)
	}
	if !strings.Contains(string(specIndex), "`gesture-truth-table.md`") {
		t.Fatalf("%s does not reference gesture-truth-table.md", specIndexPath)
	}
}

func TestInteractionGrammarDocIsIndexedAndLinked(t *testing.T) {
	root := repoRootFromCaller(t)

	grammarPath := filepath.Join(root, "docs", "interaction-grammar.md")
	grammarDoc, err := os.ReadFile(grammarPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", grammarPath, err)
	}
	content := string(grammarDoc)

	requiredSnippets := []string{
		"## Authoritative Ontology",
		"## Authoritative Live Model",
		"## Canonical Action Semantics",
		"## Allowed Tool Modalities",
		"## Rules for Auxiliary Surfaces",
		"## Rules for New Artifact Kinds",
		"Tabura has exactly five primary product nouns:",
		"Tabura exposes exactly two live runtime modes:",
		"Project is not a product concept.",
		"- **Workspace**",
		"- **Artifact**",
		"- **Item**",
		"- **Actor**",
		"- **Label**",
		"- Dialogue",
		"- Meeting",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("%s missing snippet %q", grammarPath, snippet)
		}
	}

	specIndexPath := filepath.Join(root, "docs", "spec-index.md")
	specIndex, err := os.ReadFile(specIndexPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", specIndexPath, err)
	}
	if !strings.Contains(string(specIndex), "`interaction-grammar.md`") {
		t.Fatalf("%s does not reference interaction-grammar.md", specIndexPath)
	}

	claudePath := filepath.Join(root, "CLAUDE.md")
	claudeDoc, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", claudePath, err)
	}
	if !strings.Contains(string(claudeDoc), "`docs/interaction-grammar.md`") {
		t.Fatalf("%s does not reference docs/interaction-grammar.md", claudePath)
	}
}

func TestNativeClientsPlanDocIsIndexedAndAnchored(t *testing.T) {
	root := repoRootFromCaller(t)

	planPath := filepath.Join(root, "docs", "native-clients-plan.md")
	planDoc, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", planPath, err)
	}
	content := string(planDoc)

	requiredSnippets := []string{
		"## Architecture Decision",
		"server-driven thin native clients",
		"**Capture**: audio PCM, ink strokes, taps, and gestures.",
		"**Render**: structured chat/canvas output rendered with native surfaces.",
		"`internal/web/mdns.go`",
		"`internal/web/push.go`",
		"`platforms/ios/TaburaIOS/TaburaInkCaptureView.swift` uses `PencilKit`",
		"`platforms/android/app/src/main/kotlin/com/tabura/android/TaburaInkSurfaceView.kt`",
		"`platforms/android/app/src/main/kotlin/com/tabura/android/TaburaBooxInkSurfaceView.kt`",
		"`internal/web/static/app-runtime-ui.ts` toggles `black-screen` dialogue mode.",
		"| iOS + Apple Pencil | ~9ms | `PencilKit` with native prediction |",
		"- `#638` mDNS advertisement and push relay",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("%s missing snippet %q", planPath, snippet)
		}
	}

	specIndexPath := filepath.Join(root, "docs", "spec-index.md")
	specIndex, err := os.ReadFile(specIndexPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", specIndexPath, err)
	}
	if !strings.Contains(string(specIndex), "`native-clients-plan.md`") {
		t.Fatalf("%s does not reference native-clients-plan.md", specIndexPath)
	}
}

func TestNativeClientsGuideIsIndexedAndHonest(t *testing.T) {
	root := repoRootFromCaller(t)

	guidePath := filepath.Join(root, "docs", "native-clients.md")
	guideDoc, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", guidePath, err)
	}
	content := string(guideDoc)

	requiredSnippets := []string{
		"release/run/verification guide for the shipped native thin-client slice",
		"swift test --package-path platforms/ios",
		"ANDROID_HOME=/home/ert/android-sdk gradle -p platforms/android app:testDebugUnitTest",
		"gradle -p platforms/android/flow-contracts test",
		"./scripts/playwright.sh",
		"The structural tests in `platforms/ios/project_files_test.go` and `platforms/android/project_files_test.go` are regression guards",
		"Boox raw drawing and e-ink refresh",
		"Do not describe the native clients as a broader completed product unless the automated checks above pass",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(content, snippet) {
			t.Fatalf("%s missing snippet %q", guidePath, snippet)
		}
	}

	specIndexPath := filepath.Join(root, "docs", "spec-index.md")
	specIndex, err := os.ReadFile(specIndexPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", specIndexPath, err)
	}
	if !strings.Contains(string(specIndex), "`native-clients.md`") {
		t.Fatalf("%s does not reference native-clients.md", specIndexPath)
	}

	planPath := filepath.Join(root, "docs", "native-clients-plan.md")
	planDoc, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatalf("ReadFile(%q) error: %v", planPath, err)
	}
	if !strings.Contains(string(planDoc), "[`native-clients.md`](native-clients.md)") {
		t.Fatalf("%s does not reference native-clients.md", planPath)
	}
}
