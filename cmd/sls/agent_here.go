package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const agentHerePrompt = "Start agent here."

type agentHereResolution struct {
	StartPath    string
	TargetPath   string
	SourceCursor *chatMessageCursor
}

func isTopLevelAgentHere(args []string) bool {
	i := firstPositionalArg(args)
	return i < len(args) && strings.EqualFold(args[i], "agent-here")
}

func handleAgentHereCommand(args []string, opts cliOptions, stdout, stderr io.Writer) int {
	if len(args) == 0 || !strings.EqualFold(args[0], "agent-here") {
		fmt.Fprintln(stderr, "usage: sls agent-here <path-or-link>")
		return 2
	}
	spec := strings.TrimSpace(strings.Join(args[1:], " "))
	if spec == "" {
		fmt.Fprintln(stderr, "usage: sls agent-here <path-or-link>")
		return 2
	}
	res, err := resolveAgentHereSpec(spec, opts.projectDir)
	if err != nil {
		fmt.Fprintf(stderr, "sls agent-here: %v\n", err)
		return 1
	}

	ctx, cancel := context.WithTimeout(context.Background(), opts.timeout)
	defer cancel()

	client, err := newClient(ctx, clientConfig{
		baseURL:   opts.resolveBaseURL(),
		tokenFile: opts.effectiveTokenFile(),
		verbose:   opts.verbose,
		stderr:    stderr,
	})
	if err != nil {
		fmt.Fprintf(stderr, "sls agent-here: %v\n", err)
		return 1
	}

	renderer := newRenderer(stdout, opts.jsonOut, !opts.noColor && isTerminal(stdout))
	if err := runAgentHere(ctx, client, res, renderer, opts, stderr); err != nil {
		if errors.Is(err, errAssistantError) {
			fmt.Fprintf(stderr, "sls: %v\n", err)
			return 1
		}
		fmt.Fprintf(stderr, "sls agent-here: %v\n", err)
		return 1
	}
	return 0
}

func runAgentHere(ctx context.Context, client *chatClient, res agentHereResolution, renderer *renderer, opts cliOptions, stderr io.Writer) error {
	workspace, err := client.openLinkedWorkspace(ctx, res.StartPath)
	if err != nil {
		return err
	}
	if err := persistSessionForWorkspace(workspace.WorkspacePath, workspace.ChatSessionID); err != nil && opts.verbose {
		fmt.Fprintf(stderr, "sls: warning: persist session: %v\n", err)
	}
	turnCtx, cancel := context.WithTimeout(ctx, opts.timeout)
	defer cancel()
	final, err := client.sendAndWaitForFinal(turnCtx, workspace.ChatSessionID, agentHerePrompt, res.SourceCursor, renderer)
	if err != nil {
		return err
	}
	if !opts.jsonOut {
		trimmed := strings.TrimSpace(final)
		if trimmed != "" && !renderer.didEmitFinalText() {
			fmt.Fprintln(renderer.out, trimmed)
		}
	}
	return nil
}

func resolveAgentHereSpec(spec, cwd string) (agentHereResolution, error) {
	sourceRaw, targetRaw, hasSource := splitAgentHereSpec(spec)
	baseDir := absoluteCleanPath(cwd)
	if baseDir == "" {
		baseDir = "."
	}
	var sourcePath string
	var cursor *chatMessageCursor
	var sourceSphere string
	if hasSource {
		var err error
		sourcePath, cursor, sourceSphere, err = resolveAgentHereSource(sourceRaw, baseDir)
		if err != nil {
			return agentHereResolution{}, err
		}
	} else {
		sourcePath, cursor, sourceSphere = resolveAgentHereCurrentSource(baseDir)
	}
	targetPath, targetIsDir, err := resolveAgentHereTarget(targetRaw, baseDir, sourcePath, sourceSphere)
	if err != nil {
		return agentHereResolution{}, err
	}
	startPath := targetPath
	if !targetIsDir {
		startPath = filepath.Dir(targetPath)
	}
	return agentHereResolution{
		StartPath:    startPath,
		TargetPath:   targetPath,
		SourceCursor: cursor,
	}, nil
}

func splitAgentHereSpec(spec string) (string, string, bool) {
	left, right, ok := strings.Cut(spec, "::")
	if !ok {
		return "", strings.TrimSpace(spec), false
	}
	return strings.TrimSpace(left), strings.TrimSpace(right), true
}

func resolveAgentHereSource(raw, cwd string) (string, *chatMessageCursor, string, error) {
	clean := cleanAgentHerePath(raw)
	if clean == "" {
		return "", nil, "", errors.New("source note is required")
	}
	sourceSphere := agentHereSphereForResolvedContext(cwd)
	if info, err := os.Stat(cwd); err == nil && !info.IsDir() {
		if path, isDir, ok := resolveAgentHereDirectPath(clean, cwd, cwd, sourceSphere); ok {
			if resolvedSphere := agentHereSphereForPath(path); resolvedSphere != "" {
				sourceSphere = resolvedSphere
			}
			if err := rejectAgentHereResolvedPath(path, sourceSphere); err != nil {
				return "", nil, "", err
			}
			return path, &chatMessageCursor{
				Path:  clean,
				IsDir: isDir,
			}, sourceSphere, nil
		}
	}
	path, isDir, err := resolveAgentHereExistingPath(clean, cwd, sourceSphere)
	if err != nil {
		return "", nil, "", fmt.Errorf("source note %q not found", raw)
	}
	if resolvedSphere := agentHereSphereForPath(path); resolvedSphere != "" {
		sourceSphere = resolvedSphere
	}
	if err := rejectAgentHereResolvedPath(path, sourceSphere); err != nil {
		return "", nil, "", err
	}
	return path, &chatMessageCursor{
		Path:  clean,
		IsDir: isDir,
	}, sourceSphere, nil
}

func resolveAgentHereCurrentSource(cwd string) (string, *chatMessageCursor, string) {
	sphere := agentHereSphereForResolvedContext(cwd)
	info, err := os.Stat(cwd)
	if err != nil {
		return "", nil, sphere
	}
	if info.IsDir() {
		return "", nil, sphere
	}
	path := absoluteCleanPath(cwd)
	if path == "" {
		return "", nil, sphere
	}
	return path, &chatMessageCursor{
		Path:  path,
		IsDir: false,
	}, sphere
}

func resolveAgentHereTarget(raw, cwd, sourcePath, sourceSphere string) (string, bool, error) {
	clean := cleanAgentHerePath(raw)
	if clean == "" {
		return "", false, errors.New("target path is required")
	}
	if path, isDir, ok := resolveAgentHereDirectPath(clean, cwd, sourcePath, sourceSphere); ok {
		if err := rejectAgentHereResolvedPath(path, sourceSphere); err != nil {
			return "", false, err
		}
		return path, isDir, nil
	}
	if path, isDir, ok := resolveAgentHereBrainPath(clean, sourceSphere); ok {
		if err := rejectAgentHereResolvedPath(path, sourceSphere); err != nil {
			return "", false, err
		}
		return path, isDir, nil
	}
	if path, isDir, ok := resolveAgentHereBrainBasename(clean, sourceSphere); ok {
		if err := rejectAgentHereResolvedPath(path, sourceSphere); err != nil {
			return "", false, err
		}
		return path, isDir, nil
	}
	if err := resolveAgentHereCrossSphereGuard(clean, sourceSphere); err != nil {
		return "", false, err
	}
	return "", false, fmt.Errorf("target %q not found", raw)
}

func resolveAgentHereExistingPath(raw, cwd, sourceSphere string) (string, bool, error) {
	if path, isDir, ok := resolveAgentHereDirectPath(raw, cwd, "", sourceSphere); ok {
		if err := rejectAgentHereResolvedPath(path, sourceSphere); err != nil {
			return "", false, err
		}
		return path, isDir, nil
	}
	if path, isDir, ok := resolveAgentHereBrainPath(raw, sourceSphere); ok {
		if err := rejectAgentHereResolvedPath(path, sourceSphere); err != nil {
			return "", false, err
		}
		return path, isDir, nil
	}
	if path, isDir, ok := resolveAgentHereBrainBasename(raw, sourceSphere); ok {
		if err := rejectAgentHereResolvedPath(path, sourceSphere); err != nil {
			return "", false, err
		}
		return path, isDir, nil
	}
	if err := resolveAgentHereCrossSphereGuard(raw, sourceSphere); err != nil {
		return "", false, err
	}
	return "", false, fmt.Errorf("target %q not found", raw)
}

func resolveAgentHereDirectPath(raw, cwd, sourcePath, sourceSphere string) (string, bool, bool) {
	for _, candidate := range agentHerePathCandidates(raw, cwd, sourcePath, sourceSphere) {
		info, err := os.Stat(candidate)
		if err == nil {
			return candidate, info.IsDir(), true
		}
	}
	return "", false, false
}

func resolveAgentHereBrainPath(raw, sourceSphere string) (string, bool, bool) {
	roots := agentHereBrainRoots(sourceSphere)
	if len(roots) == 0 {
		return "", false, false
	}
	for _, root := range roots {
		brainDir := filepath.Join(root, "brain")
		for _, candidate := range agentHereBrainCandidates(brainDir, raw) {
			info, err := os.Stat(candidate)
			if err == nil {
				return candidate, info.IsDir(), true
			}
		}
	}
	return "", false, false
}

func resolveAgentHereBrainBasename(raw, sourceSphere string) (string, bool, bool) {
	if raw == "" || strings.ContainsAny(raw, `/\`) {
		return "", false, false
	}
	needle := filepath.Base(raw)
	alts := []string{needle}
	if filepath.Ext(needle) == "" {
		alts = append(alts, needle+".md")
	}
	roots := agentHereBrainRoots(sourceSphere)
	for _, root := range roots {
		brainDir := filepath.Join(root, "brain")
		var matches []string
		_ = filepath.WalkDir(brainDir, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if entry.IsDir() {
				if strings.EqualFold(entry.Name(), "personal") {
					return filepath.SkipDir
				}
				return nil
			}
			if pathInWorkPersonalGuardrail(path) {
				return nil
			}
			for _, alt := range alts {
				if strings.EqualFold(entry.Name(), alt) {
					matches = append(matches, path)
					break
				}
			}
			return nil
		})
		if len(matches) == 0 {
			continue
		}
		sort.Strings(matches)
		info, err := os.Stat(matches[0])
		if err != nil {
			continue
		}
		return matches[0], info.IsDir(), true
	}
	return "", false, false
}

func agentHereBrainRoots(sourceSphere string) []string {
	roots := findBrainRoots()
	if strings.TrimSpace(sourceSphere) == "" {
		return sortedBrainRoots(roots)
	}
	root := strings.TrimSpace(roots[sourceSphere])
	if root == "" {
		return nil
	}
	return []string{root}
}

func rejectAgentHereResolvedPath(path, sourceSphere string) error {
	if err := rejectAgentHerePersonalPath(path); err != nil {
		return err
	}
	if strings.TrimSpace(sourceSphere) == "" {
		return nil
	}
	if sphere := agentHereSphereForPath(path); sphere != "" && sphere != sourceSphere {
		return errors.New("target leaves the originating sphere")
	}
	return nil
}

func agentHereSphereForResolvedContext(path string) string {
	return agentHereSphereForPath(path)
}

func agentHereSphereForPath(path string) string {
	roots := findBrainRoots()
	for _, sphere := range []string{"work", "private"} {
		root := strings.TrimSpace(roots[sphere])
		if root == "" {
			continue
		}
		if pathInsideOrEqual(path, root) {
			return sphere
		}
	}
	return ""
}

func resolveAgentHereCrossSphereGuard(raw, sourceSphere string) error {
	if strings.TrimSpace(sourceSphere) == "" {
		return nil
	}
	for _, sphere := range []string{"work", "private"} {
		if sphere == sourceSphere {
			continue
		}
		if path, _, ok := resolveAgentHereBrainPath(raw, sphere); ok {
			if agentHereSphereForPath(path) == sphere {
				return errors.New("target leaves the originating sphere")
			}
		}
		if path, _, ok := resolveAgentHereBrainBasename(raw, sphere); ok {
			if agentHereSphereForPath(path) == sphere {
				return errors.New("target leaves the originating sphere")
			}
		}
	}
	return nil
}

func agentHerePathCandidates(raw, cwd, sourcePath, sourceSphere string) []string {
	clean := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	if clean == "" {
		return nil
	}
	candidates := make([]string, 0, 8)
	add := func(path string) {
		if path == "" {
			return
		}
		cleaned := filepath.Clean(path)
		for _, existing := range candidates {
			if existing == cleaned {
				return
			}
		}
		candidates = append(candidates, cleaned)
	}
	addVariants := func(path string) {
		add(path)
		if filepath.Ext(path) == "" && !strings.HasSuffix(path, string(filepath.Separator)) {
			add(path + ".md")
		}
	}
	if filepath.IsAbs(clean) {
		addVariants(clean)
		return candidates
	}
	if sourcePath != "" {
		addVariants(filepath.Join(filepath.Dir(sourcePath), filepath.FromSlash(clean)))
	} else {
		addVariants(filepath.Join(cwd, filepath.FromSlash(clean)))
	}
	for _, root := range agentHereBrainRoots(sourceSphere) {
		brainDir := filepath.Join(root, "brain")
		addVariants(filepath.Join(brainDir, filepath.FromSlash(clean)))
	}
	return candidates
}

func agentHereBrainCandidates(brainDir, raw string) []string {
	clean := strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
	if clean == "" {
		return nil
	}
	candidates := make([]string, 0, 4)
	add := func(path string) {
		if path == "" {
			return
		}
		cleaned := filepath.Clean(path)
		for _, existing := range candidates {
			if existing == cleaned {
				return
			}
		}
		candidates = append(candidates, cleaned)
	}
	add(filepath.Join(brainDir, filepath.FromSlash(clean)))
	if filepath.Ext(clean) == "" && !strings.HasSuffix(clean, string(filepath.Separator)) {
		add(filepath.Join(brainDir, filepath.FromSlash(clean)+".md"))
	}
	return candidates
}

func cleanAgentHerePath(raw string) string {
	target := strings.TrimSpace(raw)
	if target == "" {
		return ""
	}
	if strings.HasPrefix(target, "<") && strings.HasSuffix(target, ">") {
		target = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(target, "<"), ">"))
	}
	if strings.HasPrefix(strings.ToLower(target), "slopshell-wiki:") {
		if decoded, err := url.PathUnescape(target[len("slopshell-wiki:"):]); err == nil {
			target = decoded
		}
	}
	if strings.HasPrefix(target, "[[") && strings.HasSuffix(target, "]]") {
		target = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(target, "[["), "]]"))
	}
	if idx := strings.Index(target, "|"); idx >= 0 {
		target = strings.TrimSpace(target[:idx])
	}
	if idx := strings.Index(target, "#"); idx >= 0 {
		target = strings.TrimSpace(target[:idx])
	}
	if idx := strings.Index(target, "?"); idx >= 0 {
		target = strings.TrimSpace(target[:idx])
	}
	return strings.TrimSpace(strings.ReplaceAll(target, "\\", "/"))
}

func rejectAgentHerePersonalPath(path string) error {
	if pathInWorkPersonalGuardrail(path) {
		return errors.New("work personal subtree is blocked")
	}
	return nil
}

func pathInWorkPersonalGuardrail(path string) bool {
	return pathInsideOrEqual(path, workPersonalGuardrailRoot())
}

func workPersonalGuardrailRoot() string {
	root := strings.TrimSpace(brainVaultRoot("work"))
	if root == "" {
		return ""
	}
	clean := filepath.Clean(root)
	if filepath.Base(clean) == "brain" {
		return filepath.Join(filepath.Dir(clean), "personal")
	}
	return filepath.Join(clean, "personal")
}

func pathInsideOrEqual(path, root string) bool {
	cleanPath := absoluteCleanPath(path)
	cleanRoot := absoluteCleanPath(root)
	if cleanPath == "" || cleanRoot == "" {
		return false
	}
	if cleanPath == cleanRoot {
		return true
	}
	rel, err := filepath.Rel(cleanRoot, cleanPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func absoluteCleanPath(path string) string {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return ""
	}
	abs, err := filepath.Abs(clean)
	if err != nil {
		return filepath.Clean(clean)
	}
	return filepath.Clean(abs)
}

func sortedBrainRoots(roots map[string]string) []string {
	order := []string{"work", "private"}
	out := make([]string, 0, len(roots))
	seen := map[string]bool{}
	for _, sphere := range order {
		root := strings.TrimSpace(roots[sphere])
		if root == "" {
			continue
		}
		out = append(out, root)
		seen[root] = true
	}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || seen[root] {
			continue
		}
		out = append(out, root)
	}
	return out
}
