package web

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStaticAppSplitFileLineLimits(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Join(filepath.Dir(filename), "static")
	entries, err := filepath.Glob(filepath.Join(root, "*.js"))
	if err != nil {
		t.Fatalf("glob *.js: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("no first-party js files found")
	}
	for _, path := range entries {
		base := filepath.Base(path)
		if strings.HasSuffix(base, ".min.js") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		lines := strings.Count(string(data), "\n")
		if len(data) > 0 && data[len(data)-1] != '\n' {
			lines++
		}
		if lines > 1000 {
			t.Fatalf("%s has %d lines, want <= 1000", filepath.Base(path), lines)
		}
	}
}
