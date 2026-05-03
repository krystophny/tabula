package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/term"

	"github.com/sloppy-org/slopshell/internal/protocol"
	"github.com/sloppy-org/slopshell/internal/ptt"
	"github.com/sloppy-org/slopshell/internal/store"
	updater "github.com/sloppy-org/slopshell/internal/update"
	"github.com/sloppy-org/slopshell/internal/web"
)

const defaultBinaryVersion = "0.2.1"

var (
	version   = defaultBinaryVersion
	commit    = "dev"
	runUpdate = updater.Run
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		printHelp()
		return 2
	}
	switch args[0] {
	case "schema":
		return cmdSchema()
	case "bootstrap":
		return cmdBootstrap(args[1:])
	case "materialize":
		return cmdMaterialize(args[1:])
	case "archive":
		return cmdArchive(args[1:])
	case "server":
		return cmdServer(args[1:])
	case "set-password":
		return cmdSetPassword(args[1:])
	case "version":
		return cmdVersion()
	case "update":
		return cmdUpdate(args[1:])
	case "ptt-daemon":
		return cmdPTTDaemon(args[1:])
	case "google-auth":
		if err := cmdGoogleAuth(); err != nil {
			fmt.Fprintf(os.Stderr, "google-auth: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		printHelp()
		return 2
	}
}

func printHelp() {
	fmt.Println("slopshell <command> [flags]")
	fmt.Println("commands: schema bootstrap materialize archive server set-password version update ptt-daemon")
}

func cmdSchema() int {
	schema := map[string]interface{}{
		"title": "SlopshellCanvasEvent",
		"oneOf": []map[string]interface{}{
			{"type": "object", "properties": map[string]interface{}{"kind": map[string]interface{}{"const": "text_artifact"}}},
			{"type": "object", "properties": map[string]interface{}{"kind": map[string]interface{}{"const": "image_artifact"}}},
			{"type": "object", "properties": map[string]interface{}{"kind": map[string]interface{}{"const": "pdf_artifact"}}},
			{"type": "object", "properties": map[string]interface{}{"kind": map[string]interface{}{"const": "clear_canvas"}}},
		},
	}
	b, _ := json.MarshalIndent(schema, "", "  ")
	fmt.Println(string(b))
	return 0
}

type serverConfig struct {
	dataDir              string
	projectDir           string
	controlSocket        string
	webHost              string
	webPort              int
	webHTTPSPort         int
	webCertFile          string
	webKeyFile           string
	appServerURL         string
	model                string
	sparkReasoningEffort string
	ttsURL               string
	devRuntime           bool
}

func bindWorkspaceDirFlag(fs *flag.FlagSet, defaultValue string) *string {
	workspaceDir := fs.String("workspace-dir", defaultValue, "workspace dir")
	return workspaceDir
}

func cmdBootstrap(args []string) int {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	workspaceDir := bindWorkspaceDirFlag(fs, ".")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	res, err := protocol.BootstrapProject(*workspaceDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	fmt.Printf("workspace prepared: %s\n", res.Paths.WorkspaceDir)
	fmt.Printf("sloppy mcp config snippet: %s\n", res.Paths.MCPConfigPath)
	fmt.Println("workspace AGENTS.md files are left untouched")
	if res.GitInitialized {
		fmt.Println("git initialized")
	}
	return 0
}

func cmdServer(args []string) int {
	cfg, status := parseServerConfig(args)
	if status != 0 {
		return status
	}
	return runServer(cfg)
}

func parseServerConfig(args []string) (*serverConfig, int) {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	cfg := &serverConfig{
		dataDir: filepath.Join(os.Getenv("HOME"), ".slopshell-web"),
	}
	workspaceDir := bindWorkspaceDirFlag(fs, ".")
	fs.StringVar(&cfg.dataDir, "data-dir", cfg.dataDir, "data dir")
	fs.StringVar(&cfg.controlSocket, "control-socket", strings.TrimSpace(os.Getenv("SLOPSHELL_CONTROL_SOCKET")), "path to the private Slopshell runtime control unix socket (mode 0600); empty = default $XDG_RUNTIME_DIR/sloppy/control.sock")
	fs.StringVar(&cfg.webHost, "web-host", "127.0.0.1", "web listener host")
	fs.IntVar(&cfg.webPort, "web-port", web.DefaultPort, "web listener port")
	fs.IntVar(&cfg.webHTTPSPort, "web-https-port", 8443, "HTTPS web listener port (requires --web-cert-file and --web-key-file)")
	fs.StringVar(&cfg.webCertFile, "web-cert-file", "", "TLS certificate path for HTTPS web listener")
	fs.StringVar(&cfg.webKeyFile, "web-key-file", "", "TLS private key path for HTTPS web listener")
	fs.StringVar(&cfg.appServerURL, "app-server-url", web.DefaultAppServerURL, "Codex app-server websocket URL")
	fs.StringVar(&cfg.model, "model", "", "LLM model for chat (default: env SLOPSHELL_APP_SERVER_MODEL or "+web.DefaultModel+")")
	fs.StringVar(&cfg.sparkReasoningEffort, "spark-reasoning-effort", "", "Spark thinking budget, e.g. low|medium|high (default: env SLOPSHELL_APP_SERVER_SPARK_REASONING_EFFORT or low)")
	fs.StringVar(&cfg.ttsURL, "tts-url", "", "TTS server URL (default: env SLOPSHELL_TTS_URL or "+web.DefaultTTSURL+")")
	fs.BoolVar(&cfg.devRuntime, "dev-runtime", false, "dev runtime endpoint")
	if err := fs.Parse(args); err != nil {
		return nil, 2
	}
	cfg.projectDir = *workspaceDir
	hasCert := strings.TrimSpace(cfg.webCertFile) != ""
	hasKey := strings.TrimSpace(cfg.webKeyFile) != ""
	if hasCert != hasKey {
		fmt.Fprintln(os.Stderr, "HTTPS requires both --web-cert-file and --web-key-file")
		return nil, 2
	}
	return cfg, 0
}

func runServer(cfg *serverConfig) int {
	res, err := protocol.BootstrapProject(cfg.projectDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	app, err := web.New(
		cfg.dataDir,
		res.Paths.WorkspaceDir,
		cfg.controlSocket,
		cfg.appServerURL,
		cfg.model,
		cfg.ttsURL,
		cfg.sparkReasoningEffort,
		cfg.devRuntime,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	hasTLS := strings.TrimSpace(cfg.webCertFile) != "" && strings.TrimSpace(cfg.webKeyFile) != ""
	if hasTLS {
		go func() {
			if err := app.ListenTLS(cfg.webHost, cfg.webHTTPSPort, cfg.webCertFile, cfg.webKeyFile); err != nil {
				fmt.Fprintf(os.Stderr, "HTTPS listener failed: %v\n", err)
			}
		}()
	}
	startErr := app.Start(cfg.webHost, cfg.webPort)
	if startErr != nil {
		fmt.Fprintln(os.Stderr, startErr)
		return 1
	}
	return 0
}

func cmdSetPassword(args []string) int {
	fs := flag.NewFlagSet("set-password", flag.ContinueOnError)
	dataDir := fs.String("data-dir", filepath.Join(os.Getenv("HOME"), ".local/share/slopshell-web"), "data dir")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	dbPath := filepath.Join(*dataDir, "slopshell.db")
	s, err := store.New(dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open store %s: %v\n", dbPath, err)
		return 1
	}
	defer s.Close()
	var pw []byte
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(os.Stderr, "Enter password: ")
		pw, err = term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
	} else {
		pw, err = os.ReadFile("/dev/stdin")
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "read password: %v\n", err)
		return 1
	}
	pw = []byte(strings.TrimRight(string(pw), "\n\r"))
	if err := s.SetAdminPassword(string(pw)); err != nil {
		fmt.Fprintf(os.Stderr, "set password: %v\n", err)
		return 1
	}
	fmt.Println("Password set.")
	return 0
}

func cmdPTTDaemon(args []string) int {
	fs := flag.NewFlagSet("ptt-daemon", flag.ContinueOnError)
	cfg := ptt.DefaultConfig()
	fs.StringVar(&cfg.DevicePath, "device", cfg.DevicePath, "evdev device path (auto-detected if empty)")
	keyCode := fs.Int("key", int(cfg.KeyCode), "evdev key code to listen for (183=F13)")
	fs.StringVar(&cfg.STTURL, "stt-url", cfg.STTURL, "STT sidecar URL")
	fs.StringVar(&cfg.WebAPIURL, "web-api-url", cfg.WebAPIURL, "slopshell web API URL for STT replacements")
	fs.StringVar(&cfg.OutputMode, "output", cfg.OutputMode, "output mode: type (ydotool) or clipboard (wl-copy)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg.KeyCode = uint16(*keyCode)
	if err := ptt.Run(context.Background(), cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func cmdVersion() int {
	fmt.Println(formatVersionLine(version, commit, runtime.GOOS, runtime.GOARCH))
	return 0
}

func cmdUpdate(args []string) int {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	res, err := runUpdate(updater.Options{CurrentVersion: version})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if res.Updated {
		fmt.Printf("Updated slopshell %s -> %s. Restart service to apply.\n", res.CurrentVersion, res.LatestVersion)
		return 0
	}
	fmt.Printf("Already up to date (%s)\n", res.CurrentVersion)
	return 0
}

func formatVersionLine(rawVersion, rawCommit, goos, goarch string) string {
	release := strings.TrimSpace(rawVersion)
	if release == "" {
		release = "0.0.0"
	}
	if !strings.HasPrefix(strings.ToLower(release), "v") {
		release = "v" + release
	}
	shortCommit := strings.TrimSpace(rawCommit)
	if shortCommit == "" {
		shortCommit = "unknown"
	}
	return fmt.Sprintf("slopshell %s (%s) %s/%s", release, shortCommit, goos, goarch)
}
