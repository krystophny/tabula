package web

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/sloppy-org/slopshell/internal/store"
)

const (
	workspaceMarkdownBacklinkScanFileCap = 5000
	workspaceMarkdownBacklinkResultCap   = 50
	workspaceMarkdownBacklinkExcerptCap  = 160
)

type workspaceMarkdownLinkRef struct {
	Target            string `json:"target"`
	Type              string `json:"type"`
	OK                bool   `json:"ok"`
	Blocked           bool   `json:"blocked,omitempty"`
	Reason            string `json:"reason,omitempty"`
	ResolvedPath      string `json:"resolved_path,omitempty"`
	VaultRelativePath string `json:"vault_relative_path,omitempty"`
	FileURL           string `json:"file_url,omitempty"`
	Kind              string `json:"kind,omitempty"`
}

type workspaceMarkdownBacklink struct {
	SourcePath string `json:"source_path"`
	LinkType   string `json:"link_type"`
	LinkTarget string `json:"link_target"`
	Excerpt    string `json:"excerpt,omitempty"`
	FileURL    string `json:"file_url,omitempty"`
}

type workspaceMarkdownLinkPanel struct {
	OK                 bool                        `json:"ok"`
	SourcePath         string                      `json:"source_path"`
	Outgoing           []workspaceMarkdownLinkRef  `json:"outgoing"`
	BrokenCount        int                         `json:"broken_count"`
	Backlinks          []workspaceMarkdownBacklink `json:"backlinks"`
	BacklinksTruncated bool                        `json:"backlinks_truncated,omitempty"`
	ScanLimitReached   bool                        `json:"scan_limit_reached,omitempty"`
	Error              string                      `json:"error,omitempty"`
}

var (
	markdownInlineLinkRE = regexp.MustCompile(`\[(?:[^\]]*)\]\(([^)\s]+)(?:\s+"[^"]*")?\)`)
	markdownWikilinkRE   = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)
	markdownCodeFenceRE  = regexp.MustCompile("(?s)```.*?```|`[^`\n]*`")
)

func stripBrainPrefixForWorkspace(workspace store.Workspace, sourceRel string) string {
	brainRoot := absoluteCleanPath(workspace.DirPath)
	if filepath.Base(brainRoot) != "brain" {
		return sourceRel
	}
	parts := strings.SplitN(strings.TrimSpace(sourceRel), "/", 2)
	if len(parts) == 2 && strings.EqualFold(parts[0], "brain") {
		return parts[1]
	}
	return sourceRel
}

func parseMarkdownLinkRefs(text string) []workspaceMarkdownLinkRef {
	stripped := markdownCodeFenceRE.ReplaceAllString(text, " ")
	seen := map[string]bool{}
	refs := []workspaceMarkdownLinkRef{}
	addRef := func(target, kind string) {
		clean := strings.TrimSpace(target)
		if clean == "" {
			return
		}
		key := kind + "\x00" + clean
		if seen[key] {
			return
		}
		seen[key] = true
		refs = append(refs, workspaceMarkdownLinkRef{Target: clean, Type: kind})
	}
	for _, match := range markdownInlineLinkRE.FindAllStringSubmatch(stripped, -1) {
		if len(match) >= 2 {
			addRef(match[1], "markdown")
		}
	}
	for _, match := range markdownWikilinkRE.FindAllStringSubmatch(stripped, -1) {
		if len(match) >= 2 {
			addRef(match[1], "wikilink")
		}
	}
	return refs
}

func resolveOutgoingMarkdownLinks(workspace store.Workspace, sourceRel string, refs []workspaceMarkdownLinkRef) []workspaceMarkdownLinkRef {
	resolved := make([]workspaceMarkdownLinkRef, 0, len(refs))
	for _, ref := range refs {
		linkType := ""
		if ref.Type == "wikilink" {
			linkType = "wikilink"
		}
		result := resolveWorkspaceMarkdownLink(workspace, sourceRel, ref.Target, linkType)
		merged := workspaceMarkdownLinkRef{
			Target:            ref.Target,
			Type:              ref.Type,
			OK:                result.OK,
			Blocked:           result.Blocked,
			Reason:            result.Reason,
			ResolvedPath:      result.ResolvedPath,
			VaultRelativePath: result.VaultRelativePath,
			FileURL:           result.FileURL,
			Kind:              result.Kind,
		}
		resolved = append(resolved, merged)
	}
	return resolved
}

func collectMarkdownBacklinks(workspace store.Workspace, sourceRel string) ([]workspaceMarkdownBacklink, bool, bool, error) {
	brainRoot, vaultRoot, err := brainWorkspaceRoots(workspace)
	if err != nil {
		return nil, false, false, err
	}
	sourceAbs := filepath.Clean(filepath.Join(brainRoot, filepath.FromSlash(sourceRel)))
	sourceVaultRel, err := filepath.Rel(vaultRoot, sourceAbs)
	if err != nil {
		return nil, false, false, err
	}
	sourceVaultRel = filepath.ToSlash(filepath.Clean(sourceVaultRel))
	sourceBase := strings.TrimSuffix(filepath.Base(sourceAbs), ".md")

	results := []workspaceMarkdownBacklink{}
	scanLimitReached := false
	truncated := false

	files, fileLimitReached, err := listWorkspaceMarkdownNoteFiles(brainRoot, workspaceMarkdownBacklinkScanFileCap)
	if err != nil {
		return nil, false, false, err
	}
	scanLimitReached = fileLimitReached
	for _, path := range files {
		if path == sourceAbs {
			continue
		}
		if pathInWorkPersonalGuardrail(path) {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := string(data)
		refs := parseMarkdownLinkRefs(text)
		if len(refs) == 0 {
			continue
		}
		fileVaultRel, relErr := filepath.Rel(vaultRoot, path)
		if relErr != nil {
			continue
		}
		fileVaultRel = filepath.ToSlash(filepath.Clean(fileVaultRel))
		brainRel, brainRelErr := filepath.Rel(brainRoot, path)
		if brainRelErr != nil {
			continue
		}
		brainRel = filepath.ToSlash(filepath.Clean(brainRel))
		for _, ref := range refs {
			if !markdownLinkRefMatchesSource(workspace, brainRel, ref, sourceVaultRel, sourceBase, sourceAbs, brainRoot) {
				continue
			}
			if len(results) >= workspaceMarkdownBacklinkResultCap {
				truncated = true
				break
			}
			results = append(results, workspaceMarkdownBacklink{
				SourcePath: fileVaultRel,
				LinkType:   ref.Type,
				LinkTarget: ref.Target,
				Excerpt:    extractMarkdownLinkExcerpt(text, ref.Target),
				FileURL:    workspaceMarkdownLinkFileURL(workspace, fileVaultRel),
			})
			break
		}
		if truncated {
			break
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		return results[i].SourcePath < results[j].SourcePath
	})
	return results, truncated, scanLimitReached, nil
}

func markdownLinkRefMatchesSource(workspace store.Workspace, candidateBrainRel string, ref workspaceMarkdownLinkRef, sourceVaultRel, sourceBase, sourceAbs, brainRoot string) bool {
	if isExternalMarkdownLink(ref.Target) {
		return false
	}
	linkType := ""
	if ref.Type == "wikilink" {
		linkType = "wikilink"
	}
	resolved := resolveWorkspaceMarkdownLink(workspace, candidateBrainRel, ref.Target, linkType)
	if resolved.OK {
		return resolved.VaultRelativePath == sourceVaultRel
	}
	if ref.Type != "wikilink" {
		return false
	}
	target := cleanMarkdownLinkTarget(ref.Target)
	if target == "" || strings.ContainsAny(target, `/\`) {
		return false
	}
	candidate := target
	if filepath.Ext(candidate) == "" {
		candidate += ".md"
	}
	if !strings.EqualFold(strings.TrimSuffix(candidate, ".md"), sourceBase) {
		return false
	}
	found, err := findWikilinkByBasename(brainRoot, target)
	if err != nil {
		return false
	}
	return filepath.Clean(found) == sourceAbs
}

func extractMarkdownLinkExcerpt(text, target string) string {
	if text == "" || target == "" {
		return ""
	}
	idx := strings.Index(text, target)
	if idx < 0 {
		return ""
	}
	start := idx
	for start > 0 && text[start-1] != '\n' {
		start--
	}
	end := idx + len(target)
	for end < len(text) && text[end] != '\n' {
		end++
	}
	excerpt := strings.TrimSpace(text[start:end])
	if len(excerpt) > workspaceMarkdownBacklinkExcerptCap {
		excerpt = excerpt[:workspaceMarkdownBacklinkExcerptCap-1] + "…"
	}
	return excerpt
}

func buildWorkspaceMarkdownLinkPanel(workspace store.Workspace, sourceRaw string) workspaceMarkdownLinkPanel {
	panel := workspaceMarkdownLinkPanel{Outgoing: []workspaceMarkdownLinkRef{}, Backlinks: []workspaceMarkdownBacklink{}}
	sourceRel, err := normalizeMarkdownSourcePath(sourceRaw)
	if err != nil {
		panel.Error = err.Error()
		return panel
	}
	sourceRel = stripBrainPrefixForWorkspace(workspace, sourceRel)
	brainRoot, _, err := brainWorkspaceRoots(workspace)
	if err != nil {
		panel.Error = err.Error()
		return panel
	}
	sourceAbs := filepath.Clean(filepath.Join(brainRoot, filepath.FromSlash(sourceRel)))
	if !pathInsideOrEqual(sourceAbs, brainRoot) {
		panel.Error = "source note is outside the brain workspace"
		return panel
	}
	if err := enforceWorkPersonalPath(sourceAbs); err != nil {
		panel.Error = workPersonalGuardrailMessage
		return panel
	}
	panel.SourcePath = sourceRel
	data, err := os.ReadFile(sourceAbs)
	if err != nil {
		panel.Error = "source note could not be read"
		return panel
	}
	refs := parseMarkdownLinkRefs(string(data))
	panel.Outgoing = resolveOutgoingMarkdownLinks(workspace, sourceRel, refs)
	for _, ref := range panel.Outgoing {
		if ref.Blocked || !ref.OK {
			panel.BrokenCount++
		}
	}
	backlinks, truncated, scanLimit, backErr := collectMarkdownBacklinks(workspace, sourceRel)
	if backErr != nil {
		panel.Error = backErr.Error()
		return panel
	}
	panel.Backlinks = backlinks
	panel.BacklinksTruncated = truncated
	panel.ScanLimitReached = scanLimit
	panel.OK = true
	return panel
}

func (a *App) handleWorkspaceMarkdownLinkPanel(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	workspace, err := a.resolveRuntimeWorkspaceByIDOrActive(chi.URLParam(r, "workspace_id"))
	if err != nil {
		if isNoRows(err) {
			http.Error(w, "workspace not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := enforceWorkPersonalWorkspace(workspace); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	panel := buildWorkspaceMarkdownLinkPanel(workspace, r.URL.Query().Get("source"))
	writeJSON(w, panel)
}
