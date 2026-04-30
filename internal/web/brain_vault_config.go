package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/sloppy-org/slopshell/internal/store"
)

type sloptoolsVaultConfig struct {
	Vaults []sloptoolsVault `toml:"vault"`
}

type sloptoolsVault struct {
	Sphere string `toml:"sphere"`
	Root   string `toml:"root"`
	Brain  string `toml:"brain"`
}

func sloptoolsVaultConfigPath() string {
	if path := strings.TrimSpace(os.Getenv("SLOPTOOLS_VAULT_CONFIG")); path != "" {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "sloptools", "vaults.toml")
}

func loadSloptoolsBrainRootsFromFile() map[string]string {
	path := strings.TrimSpace(sloptoolsVaultConfigPath())
	if path == "" {
		return nil
	}
	var cfg sloptoolsVaultConfig
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil
	}
	roots := map[string]string{}
	for _, vault := range cfg.Vaults {
		if sphere, root := normalizeSloptoolsVault(vault.Sphere, vault.Root, vault.Brain); sphere != "" && root != "" {
			roots[sphere] = root
		}
	}
	return roots
}

func normalizeSloptoolsVault(sphere, root, brain string) (string, string) {
	cleanSphere := strings.ToLower(strings.TrimSpace(sphere))
	switch cleanSphere {
	case store.SphereWork, store.SpherePrivate:
	default:
		return "", ""
	}
	cleanRoot := strings.TrimSpace(root)
	if cleanRoot == "" {
		return "", ""
	}
	brainDir := strings.TrimSpace(brain)
	if brainDir == "" {
		brainDir = "brain"
	}
	rootAbs, err := filepath.Abs(cleanRoot)
	if err != nil {
		return "", ""
	}
	rootAbs = filepath.Clean(rootAbs)
	brainSuffix := filepath.Clean(filepath.FromSlash(brainDir))
	cleanRootAbs := filepath.Clean(rootAbs)
	if !strings.HasSuffix(strings.ToLower(filepath.ToSlash(cleanRootAbs)), "/"+strings.ToLower(filepath.ToSlash(brainSuffix))) &&
		!strings.EqualFold(filepath.Base(cleanRootAbs), brainSuffix) {
		rootAbs = filepath.Join(rootAbs, filepath.FromSlash(brainDir))
	}
	return cleanSphere, filepath.Clean(rootAbs)
}

func (a *App) loadSloptoolsBrainRootsFromMCP() map[string]string {
	if a == nil || a.tunnels == nil || !a.tunnels.hasEndpoint(LocalSessionID) {
		return nil
	}
	ep, ok := a.tunnels.getEndpoint(LocalSessionID)
	if !ok {
		return nil
	}
	if !ep.ok() {
		return nil
	}
	payload, err := sloptoolsBrainConfigFromMCP(ep)
	if err != nil {
		return nil
	}
	roots := map[string]string{}
	sloptoolsBrainRootsFromValue(payload, roots)
	if len(roots) == 0 {
		return nil
	}
	return roots
}

func sloptoolsBrainConfigFromMCP(ep mcpEndpoint) (map[string]any, error) {
	if !ep.ok() {
		return nil, errors.New("mcp endpoint not configured")
	}
	request := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "brain.config.get",
			"arguments": map[string]any{},
		},
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.HTTPURL("/mcp"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := ep.HTTPClient(2 * time.Second).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("brain.config.get failed")
	}
	var envelope map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if errMap, ok := envelope["error"].(map[string]any); ok {
		return nil, errors.New(strings.TrimSpace(fmt.Sprint(errMap["message"])))
	}
	result, _ := envelope["result"].(map[string]any)
	if result == nil {
		return nil, errors.New("brain.config.get missing result")
	}
	if isError, _ := result["isError"].(bool); isError {
		return nil, errors.New(mcpResultErrorText(result))
	}
	structured, _ := result["structuredContent"].(map[string]any)
	if structured == nil {
		return nil, errors.New("brain.config.get missing structuredContent")
	}
	return structured, nil
}

func sloptoolsBrainRootsFromValue(value any, roots map[string]string) {
	if roots == nil {
		return
	}
	switch v := value.(type) {
	case map[string]any:
		if nested, ok := v["config"]; ok {
			sloptoolsBrainRootsFromValue(nested, roots)
		}
		if nested, ok := v["brain"]; ok {
			sloptoolsBrainRootsFromValue(nested, roots)
		}
		if nested, ok := v["roots"]; ok {
			sloptoolsBrainRootsFromValue(nested, roots)
		}
		if nested, ok := v["vaults"]; ok {
			sloptoolsBrainRootsFromValue(nested, roots)
		}
		if nested, ok := v["vault"]; ok {
			sloptoolsBrainRootsFromValue(nested, roots)
		}
		if root := sloptoolsBrainPathFromAny(v["work_root"]); root != "" {
			roots[store.SphereWork] = root
		}
		if root := sloptoolsBrainPathFromAny(v["private_root"]); root != "" {
			roots[store.SpherePrivate] = root
		}
		if root := sloptoolsBrainPathFromAny(v["work"]); root != "" {
			roots[store.SphereWork] = root
		}
		if root := sloptoolsBrainPathFromAny(v["private"]); root != "" {
			roots[store.SpherePrivate] = root
		}
		if sphere, root := normalizeSloptoolsVaultValue(v); sphere != "" && root != "" {
			roots[sphere] = root
		}
	case []any:
		for _, item := range v {
			sloptoolsBrainRootsFromValue(item, roots)
		}
	}
}

func normalizeSloptoolsVaultValue(value any) (string, string) {
	m, ok := value.(map[string]any)
	if !ok {
		return "", ""
	}
	sphere := strings.ToLower(strings.TrimSpace(fmt.Sprint(m["sphere"])))
	switch sphere {
	case store.SphereWork, store.SpherePrivate:
	default:
		return "", ""
	}
	root := normalizeSloptoolsBrainRootValue(m, sphere)
	return sphere, root
}

func normalizeSloptoolsBrainRootValue(value any, sphere string) string {
	m, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	root := strings.TrimSpace(fmt.Sprint(m["root"]))
	if root == "" {
		root = strings.TrimSpace(fmt.Sprint(m["root_path"]))
	}
	if root == "" {
		root = strings.TrimSpace(fmt.Sprint(m["path"]))
	}
	if root == "" {
		return ""
	}
	brainDir := strings.TrimSpace(fmt.Sprint(m["brain"]))
	if brainDir == "" {
		brainDir = "brain"
	}
	if filepath.IsAbs(root) {
		root = filepath.Clean(root)
	} else {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return ""
		}
		root = filepath.Clean(absRoot)
	}
	if strings.EqualFold(filepath.Base(root), brainDir) || strings.HasSuffix(strings.ToLower(filepath.ToSlash(root)), "/"+strings.ToLower(filepath.ToSlash(brainDir))) {
		return root
	}
	if sphere != "" && strings.EqualFold(strings.TrimSpace(fmt.Sprint(m["sphere"])), sphere) && strings.TrimSpace(brainDir) != "" {
		return filepath.Clean(filepath.Join(root, filepath.FromSlash(brainDir)))
	}
	if brainDir != "" {
		return filepath.Clean(filepath.Join(root, filepath.FromSlash(brainDir)))
	}
	return root
}

func sloptoolsBrainPathFromAny(value any) string {
	raw := strings.TrimSpace(fmt.Sprint(value))
	if raw == "" || raw == "<nil>" {
		return ""
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	absPath, err := filepath.Abs(raw)
	if err != nil {
		return ""
	}
	return filepath.Clean(absPath)
}

func (a *App) brainPresetRoots() map[string]string {
	roots := map[string]string{
		store.SphereWork:    a.brainPresetRootEnv(brainPresetIDWork),
		store.SpherePrivate: a.brainPresetRootEnv(brainPresetIDPrivate),
	}
	for sphere, root := range loadSloptoolsBrainRootsFromFile() {
		roots[sphere] = root
	}
	for sphere, root := range a.loadSloptoolsBrainRootsFromMCP() {
		roots[sphere] = root
	}
	return roots
}

func presetRootAvailable(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func configuredBrainPresetSphereOrder(activeSphere string) []string {
	switch strings.ToLower(strings.TrimSpace(activeSphere)) {
	case store.SphereWork:
		return []string{store.SphereWork, store.SpherePrivate}
	case store.SpherePrivate:
		return []string{store.SpherePrivate, store.SphereWork}
	default:
		return []string{store.SphereWork, store.SpherePrivate}
	}
}
