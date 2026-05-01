package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// brainSubcommands lists the known brain subcommand names.
var brainSubcommands = []string{
	"open", "search", "links", "backlinks", "link",
}

func firstPositionalArg(args []string) int {
	i := 0
	for i < len(args) && strings.HasPrefix(args[i], "-") {
		if strings.HasPrefix(args[i], "--") && !strings.Contains(args[i], " ") {
			if eq := strings.Index(args[i], "="); eq > 0 {
				i++
				continue
			}
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i += 2
			continue
		}
		i++
	}
	return i
}

// isBrainSubcommand checks if args start with a brain subcommand.
func isBrainSubcommand(args []string) bool {
	i := firstPositionalArg(args)
	if i >= len(args) {
		return false
	}
	if strings.ToLower(args[i]) != "brain" {
		return false
	}
	if i+1 >= len(args) {
		return true
	}
	sub := strings.ToLower(args[i+1])
	for _, sc := range brainSubcommands {
		if sc == sub {
			return true
		}
	}
	return false
}

func isTopLevelLinkFollow(args []string) bool {
	i := firstPositionalArg(args)
	return i+1 < len(args) &&
		strings.ToLower(args[i]) == "link" &&
		strings.ToLower(args[i+1]) == "follow"
}

func commandArgs(args []string) []string {
	i := firstPositionalArg(args)
	if i >= len(args) {
		return nil
	}
	switch strings.ToLower(args[i]) {
	case "brain":
		if i+1 >= len(args) {
			return []string{"brain"}
		}
		return args[i+1:]
	case "gtd":
		if i+1 >= len(args) {
			return []string{"gtd"}
		}
		return args[i+1:]
	}
	return args[i:]
}

// brainVaultRoot returns the brain vault root for a given sphere.
// It checks environment variables first, then falls back to the
// default Dropbox/Nextcloud paths.
func brainVaultRoot(sphere string) string {
	switch strings.ToLower(strings.TrimSpace(sphere)) {
	case "work":
		if v := strings.TrimSpace(os.Getenv("SLOPSHELL_BRAIN_WORK_ROOT")); v != "" {
			return v
		}
		return ""
	case "private":
		if v := strings.TrimSpace(os.Getenv("SLOPSHELL_BRAIN_PRIVATE_ROOT")); v != "" {
			return v
		}
		return ""
	}
	return ""
}

// brainPresetRoots returns the brain vault roots for configured spheres.
func brainPresetRoots() map[string]string {
	roots := make(map[string]string)
	for _, sphere := range []string{"work", "private"} {
		roots[sphere] = brainVaultRoot(sphere)
	}
	return roots
}

// findBrainRoots returns available brain vault roots keyed by sphere.
func findBrainRoots() map[string]string {
	roots := brainPresetRoots()
	available := make(map[string]string)
	for sphere, root := range roots {
		if root != "" && brainVaultAvailable(root) {
			available[sphere] = root
		}
	}
	return available
}

// brainVaultAvailable checks if a brain vault root exists and contains a brain/ directory.
func brainVaultAvailable(root string) bool {
	clean := filepath.Clean(root)
	if _, err := os.Stat(clean); err != nil {
		return false
	}
	brainDir := filepath.Join(clean, "brain")
	if _, err := os.Stat(brainDir); err != nil {
		return false
	}
	return true
}

// resolveSphere validates and returns the sphere name.
func resolveSphere(sphere string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(sphere))
	switch s {
	case "work", "private":
		return s, nil
	default:
		return "", fmt.Errorf("unknown sphere %q (want work|private)", sphere)
	}
}

// resolveSphereWithGuard returns the sphere and its root, checking for guardrails.
func resolveSphereWithGuard(sphere string) (string, string, error) {
	s, err := resolveSphere(sphere)
	if err != nil {
		return "", "", err
	}
	root := brainVaultRoot(s)
	if root == "" {
		return "", "", fmt.Errorf("brain vault for %s is not configured (set SLOPSHELL_BRAIN_%s_ROOT)", s, strings.ToUpper(s))
	}
	if !brainVaultAvailable(root) {
		return "", "", fmt.Errorf("brain vault for %s does not exist at %s", s, root)
	}
	// Guard work/personal: reject paths under personal/ subdirectory in work vault.
	if s == "work" && strings.Contains(root, "/personal/") {
		return "", "", fmt.Errorf("work sphere blocks personal/ subdirectory for privacy")
	}
	return s, root, nil
}

func resolveBrainPath(root, relPath string) (string, error) {
	brainDir := filepath.Clean(filepath.Join(root, "brain"))
	fullPath := filepath.Clean(filepath.Join(brainDir, relPath))
	rel, err := filepath.Rel(brainDir, fullPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes brain vault")
	}
	return fullPath, nil
}

func rejectWorkPersonalPath(sphere, root, fullPath string) error {
	if sphere != "work" {
		return nil
	}
	rel, err := filepath.Rel(filepath.Clean(filepath.Join(root, "brain")), filepath.Clean(fullPath))
	if err != nil {
		return fmt.Errorf("path escapes brain vault")
	}
	if rel == "personal" || strings.HasPrefix(rel, "personal"+string(os.PathSeparator)) {
		return fmt.Errorf("work sphere blocks brain/personal")
	}
	return nil
}

// runRg runs ripgrep with the given args and returns stdout.
func runRg(args []string) (string, error) {
	cmd := exec.Command("rg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// rg returns exit code 1 for no matches, which is not an error.
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", nil
		}
		return "", fmt.Errorf("rg failed: %w: %s", err, string(out))
	}
	return string(out), nil
}

// brainOpen activates a brain preset workspace via the slopshell API.
func brainOpen(sphere string, baseURL, tokenFile string) error {
	s, root, err := resolveSphereWithGuard(sphere)
	if err != nil {
		return err
	}

	presetID := "brain." + s
	url := baseURL + "/api/runtime/workspace-presets/" + presetID + "/activate"

	// Use the existing CLI auth flow: login via token, then POST.
	// We build a minimal HTTP client like the chat client does.
	client, err := newClientForBrain(baseURL, tokenFile)
	if err != nil {
		return fmt.Errorf("auth: %w", err)
	}

	resp, err := client.Post(url, "application/json", strings.NewReader(""))
	if err != nil {
		return fmt.Errorf("activate preset: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body := make([]byte, 1024)
		n, _ := resp.Body.Read(body)
		return fmt.Errorf("activate preset: status %d: %s", resp.StatusCode, string(body[:n]))
	}

	fmt.Printf("Activated brain workspace %s at %s\n", presetID, root)
	return nil
}

// brainSearch runs rg across brain vaults for the given query.
func brainSearch(query string, limit int) error {
	roots := findBrainRoots()
	if len(roots) == 0 {
		return fmt.Errorf("no brain vaults configured or available")
	}

	var results []string
	for sphere, root := range roots {
		brainDir := filepath.Join(root, "brain")
		// rg search: match in note content, output file:line:match
		output, err := runRg([]string{
			"--files-with-matches",
			"--glob", "!personal/**",
			"--glob", "*.md",
			"-n",
			"-S", // case insensitive
			query,
			brainDir,
		})
		if err != nil {
			return err
		}
		if output == "" {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
			if line == "" {
				continue
			}
			results = append(results, fmt.Sprintf("  [%s] %s", sphere, line))
		}
	}

	if len(results) == 0 {
		fmt.Println("(no matches)")
		return nil
	}

	// Sort results for deterministic output
	sort.Strings(results)
	for _, r := range results {
		fmt.Println(r)
	}
	return nil
}

// wikilinkPattern matches [[wiki links]] in markdown content.
var wikilinkPattern = regexp.MustCompile(`\[\[([^\]]+)\]\]`)

// brainLinks extracts links from a brain note file.
func brainLinks(notePath string, sphere string, baseURL, tokenFile string) error {
	if sphere != "" {
		s, root, err := resolveSphereWithGuard(sphere)
		if err != nil {
			return err
		}
		return brainLinksInVault(notePath, s, root)
	}

	roots := findBrainRoots()
	if len(roots) == 0 {
		return fmt.Errorf("no brain vaults configured or available")
	}
	for sphere, root := range roots {
		fmt.Printf("  [%s]\n", sphere)
		if err := brainLinksInVault(notePath, sphere, root); err != nil {
			return err
		}
	}
	return nil
}

func brainLinksInVault(notePath, sphere string, root string) error {
	fullPath, err := resolveBrainPath(root, notePath)
	if err != nil {
		return fmt.Errorf("note path escapes brain vault for %s", sphere)
	}
	if err := rejectWorkPersonalPath(sphere, root, fullPath); err != nil {
		return err
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("read note %s: %w", notePath, err)
	}

	matches := wikilinkPattern.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		fmt.Printf("  (no links in %s)\n", notePath)
		return nil
	}

	seen := make(map[string]bool)
	for _, m := range matches {
		target := strings.TrimSpace(m[1])
		if seen[target] {
			continue
		}
		seen[target] = true
		fmt.Printf("  %s\n", target)
	}
	return nil
}

// brainBacklinks finds notes that link to the target note.
func brainBacklinks(targetNote string, sphere string, baseURL, tokenFile string) error {
	if sphere != "" {
		s, root, err := resolveSphereWithGuard(sphere)
		if err != nil {
			return err
		}
		return brainBacklinksInVault(targetNote, s, root)
	}

	roots := findBrainRoots()
	if len(roots) == 0 {
		return fmt.Errorf("no brain vaults configured or available")
	}
	for sphere, root := range roots {
		fmt.Printf("  [%s]\n", sphere)
		if err := brainBacklinksInVault(targetNote, sphere, root); err != nil {
			return err
		}
	}
	return nil
}

func brainBacklinksInVault(targetNote, sphere string, root string) error {
	brainDir := filepath.Join(root, "brain")

	// Escape the target for rg regex.
	escapedTarget := regexp.QuoteMeta(targetNote)
	pattern := `\[?\[?\[` + escapedTarget + `\]?\]?\]?`

	output, err := runRg([]string{
		"--files-with-matches",
		"--glob", "!personal/**",
		"--glob", "*.md",
		"-S",
		pattern,
		brainDir,
	})
	if err != nil {
		return err
	}
	if output == "" {
		fmt.Printf("  (no backlinks to %s in %s)\n", targetNote, sphere)
		return nil
	}

	var results []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		rel, err := filepath.Rel(filepath.Join(root, "brain"), line)
		if err != nil {
			rel = line
		}
		results = append(results, fmt.Sprintf("  %s", rel))
	}

	sort.Strings(results)
	for _, r := range results {
		fmt.Println(r)
	}
	return nil
}

// linkFollow resolves a link target to a file path and prints it.
func linkFollow(sourceNote string, linkTarget string, sphere string, baseURL, tokenFile string) error {
	if sphere != "" {
		s, root, err := resolveSphereWithGuard(sphere)
		if err != nil {
			return err
		}
		return linkFollowInVault(sourceNote, linkTarget, s, root)
	}

	roots := findBrainRoots()
	if len(roots) == 0 {
		return fmt.Errorf("no brain vaults configured or available")
	}

	// Check personal guard across all vaults before attempting resolution.
	for sphere, root := range roots {
		if err := checkLinkFollowPersonal(sourceNote, linkTarget, sphere, root); err != nil {
			return err
		}
	}

	var lastErr error
	for sphere, root := range roots {
		err := linkFollowInVault(sourceNote, linkTarget, sphere, root)
		if err == nil {
			return nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return lastErr
	}
	return fmt.Errorf("link target %q not found in any brain vault", linkTarget)
}

func checkLinkFollowPersonal(sourceNote, linkTarget, sphere string, root string) error {
	if sphere != "work" {
		return nil
	}
	brainDir := filepath.Join(root, "brain")

	// Check source note path.
	if strings.TrimSpace(sourceNote) != "" {
		sourcePath, err := resolveBrainPath(root, sourceNote)
		if err == nil {
			if err := rejectWorkPersonalPath(sphere, root, sourcePath); err != nil {
				return err
			}
		}
	}

	// Check link target candidate.
	candidate := filepath.Clean(filepath.Join(brainDir, linkTarget))
	if strings.HasPrefix(candidate, filepath.Clean(brainDir)) {
		if err := rejectWorkPersonalPath(sphere, root, candidate); err != nil {
			return err
		}
	}
	return nil
}

func linkFollowInVault(sourceNote, linkTarget, sphere string, root string) error {
	brainDir := filepath.Join(root, "brain")

	// Try resolving the link target as a path relative to the source note's directory.
	var sourceDir string
	if strings.TrimSpace(sourceNote) != "" {
		sourcePath, err := resolveBrainPath(root, sourceNote)
		if err != nil {
			return fmt.Errorf("source note escapes brain vault for %s", sphere)
		}
		if err := rejectWorkPersonalPath(sphere, root, sourcePath); err != nil {
			return err
		}
		sourceDir = filepath.Dir(sourcePath)
	} else {
		sourceDir = brainDir
	}
	candidate := filepath.Clean(filepath.Join(sourceDir, linkTarget))

	// If candidate is under brain dir, check if it exists.
	if strings.HasPrefix(candidate, filepath.Clean(brainDir)) {
		if _, err := os.Stat(candidate); err == nil {
			if err := rejectWorkPersonalPath(sphere, root, candidate); err != nil {
				return err
			}
			rel, _ := filepath.Rel(root, candidate)
			fmt.Printf("%s\n", rel)
			return nil
		}
	}

	// Fallback: search across brain vault for a matching note filename.
	output, err := runRg([]string{
		"--files-with-matches",
		"--glob", "!personal/**",
		"--glob", "*.md",
		"-i",
		linkTarget,
		brainDir,
	})
	if err != nil {
		return err
	}
	if output == "" {
		return fmt.Errorf("link target %q not found in %s brain", linkTarget, sphere)
	}

	// Return first match.
	first := strings.Split(strings.TrimSpace(output), "\n")[0]
	rel, err := filepath.Rel(root, first)
	if err != nil {
		rel = first
	}
	fmt.Printf("%s\n", rel)
	return nil
}

// handleBrainCommand dispatches brain subcommands.
func handleBrainCommand(args []string, opts cliOptions) int {
	if len(args) == 0 || strings.ToLower(args[0]) == "brain" {
		printBrainUsage()
		return 2
	}
	rest := args[1:]

	switch strings.ToLower(args[0]) {
	case "open":
		return handleBrainOpen(rest, opts)
	case "search":
		return handleBrainSearch(rest)
	case "links":
		return handleBrainLinks(rest, opts)
	case "backlinks":
		return handleBrainBacklinks(rest, opts)
	case "link":
		return handleBrainLink(rest, opts)
	default:
		fmt.Fprintf(os.Stderr, "unknown brain subcommand %q (want open|search|links|backlinks|link)\n", args[0])
		return 2
	}
}

func printBrainUsage() {
	fmt.Fprintln(os.Stderr, "usage: sls brain <subcommand> [args...]")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "subcommands:")
	fmt.Fprintln(os.Stderr, "  open work|private        activate a brain workspace preset")
	fmt.Fprintln(os.Stderr, "  search <query> [--limit N]  search brain vaults with rg")
	fmt.Fprintln(os.Stderr, "  links <note> [<sphere>]  show links in a brain note")
	fmt.Fprintln(os.Stderr, "  backlinks <note> [<sphere>] find backlinks to a brain note")
	fmt.Fprintln(os.Stderr, "  link follow <note> <target> [<sphere>] resolve a wiki link")
}

func handleBrainOpen(args []string, opts cliOptions) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sls brain open work|private")
		return 2
	}
	if err := brainOpen(args[0], opts.baseURL, opts.effectiveTokenFile()); err != nil {
		fmt.Fprintf(os.Stderr, "sls brain open: %v\n", err)
		return 1
	}
	return 0
}

func handleBrainSearch(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: sls brain search <query> [--limit N]")
		return 2
	}
	query := strings.Join(searchQueryArgs(args), " ")
	if err := brainSearch(query, 0); err != nil {
		fmt.Fprintf(os.Stderr, "sls brain search: %v\n", err)
		return 1
	}
	return 0
}

func searchQueryArgs(args []string) []string {
	var queryParts []string
	for _, arg := range args {
		if arg == "--limit" || strings.HasPrefix(arg, "--limit=") {
			continue
		}
		queryParts = append(queryParts, arg)
	}
	return queryParts
}

func handleBrainLinks(args []string, opts cliOptions) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: sls brain links <note> [<sphere>]")
		return 2
	}
	sphere := ""
	if len(args) >= 2 {
		sphere = args[1]
	}
	if err := brainLinks(args[0], sphere, opts.baseURL, opts.effectiveTokenFile()); err != nil {
		fmt.Fprintf(os.Stderr, "sls brain links: %v\n", err)
		return 1
	}
	return 0
}

func handleBrainBacklinks(args []string, opts cliOptions) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: sls brain backlinks <note> [<sphere>]")
		return 2
	}
	sphere := ""
	if len(args) >= 2 {
		sphere = args[1]
	}
	if err := brainBacklinks(args[0], sphere, opts.baseURL, opts.effectiveTokenFile()); err != nil {
		fmt.Fprintf(os.Stderr, "sls brain backlinks: %v\n", err)
		return 1
	}
	return 0
}

func handleBrainLink(args []string, opts cliOptions) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: sls brain link follow <note> <target> [<sphere>]")
		return 2
	}
	if strings.ToLower(args[0]) != "follow" {
		fmt.Fprintf(os.Stderr, "unknown link subcommand %q (want follow)\n", args[0])
		return 2
	}
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: sls brain link follow <note> <target> [<sphere>]")
		return 2
	}
	sphere := ""
	if len(args) >= 4 {
		sphere = args[3]
	}
	if err := linkFollow(args[1], args[2], sphere, opts.baseURL, opts.effectiveTokenFile()); err != nil {
		fmt.Fprintf(os.Stderr, "sls brain link follow: %v\n", err)
		return 1
	}
	return 0
}
