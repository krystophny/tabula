package surface

import (
	"strings"
	"testing"
)

func TestInterfacesMarkdownIncludesKnownRoutesAndTools(t *testing.T) {
	doc := InterfacesMarkdown()
	if strings.Contains(doc, "POST /mcp") {
		t.Fatalf("InterfacesMarkdown should not publish the private control RPC route")
	}
	if !strings.Contains(doc, "GET /api/runtime") {
		t.Fatalf("InterfacesMarkdown missing runtime route")
	}
	if strings.Contains(doc, "cancel-delegates") {
		t.Fatalf("InterfacesMarkdown should not list removed cancel-delegates route")
	}
}

func TestRuntimeControlToolNamesCSVIncludesBacktickedNames(t *testing.T) {
	csv := RuntimeControlToolNamesCSV()
	if !strings.Contains(csv, "`canvas_session_open`") {
		t.Fatalf("RuntimeControlToolNamesCSV missing canvas_session_open")
	}
	if strings.Contains(csv, "cancel-delegates") {
		t.Fatalf("RuntimeControlToolNamesCSV should not include removed route names")
	}
}
