package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"

	"github.com/nimishr2/shroud/internal/masker"
	"github.com/nimishr2/shroud/internal/proxy"
	"github.com/nimishr2/shroud/internal/session"
	"github.com/nimishr2/shroud/internal/ui"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "shroud",
	Short: "Use AI on real data — without leaking secrets",
}

var runCmd = &cobra.Command{
	Use:   "run [--ui] <tool> [args...]",
	Short: "Run an AI tool with secrets masking",
	Args:  cobra.MinimumNArgs(1),
	RunE:  runTool,
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Show the last session's masked secrets summary",
	RunE:  showLogs,
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show shroud status",
	RunE:  showStatus,
}

var uiFlag bool
var debugHeadersFlag bool

func init() {
	runCmd.Flags().BoolVar(&uiFlag, "ui", false, "Open live dashboard in browser")
	runCmd.Flags().BoolVar(&debugHeadersFlag, "debug-headers", false, "Log incoming provider/auth headers with secrets redacted")
	rootCmd.AddCommand(runCmd, logsCmd, statusCmd)
}

func runTool(cmd *cobra.Command, args []string) error {
	tool := args[0]
	toolArgs := args[1:]

	m := masker.New()

	logger, err := session.NewLogger(tool)
	if err != nil {
		return fmt.Errorf("session logger: %w", err)
	}
	defer logger.Close()

	// UI hub: only created when --ui is set. The proxy holds a nil hub otherwise
	// and skips all emit calls — zero overhead on the hot path.
	var hub *ui.Hub
	var uiServer *http.Server
	var uiAddr string

	if uiFlag {
		hub = ui.NewHub()

		// Emit session_start so the browser knows which tool is running
		hub.Publish(ui.Event{Type: "session_start", Tool: tool})

		// Serve the dashboard + WebSocket on a second random port
		uiMux := http.NewServeMux()
		uiMux.HandleFunc("/ws", hub.ServeHTTP)
		uiMux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(ui.DashboardHTML))
		})

		uiListener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("ui server: %w", err)
		}
		uiAddr = "http://" + uiListener.Addr().String()
		uiServer = &http.Server{Handler: uiMux}
		go uiServer.Serve(uiListener)
	}

	// Capture the original upstream URLs BEFORE we overwrite them.
	// If the user has a custom endpoint (e.g., Azure OpenAI, AWS Bedrock),
	// Shroud will route there automatically.
	upstreams := map[string]string{
		"anthropic": envOrDefault("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
		"openai":    envOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
	}

	openAIProxy := proxy.New(m, logger, hub, upstreams, "openai", debugHeadersFlag)
	openAIAddr, err := openAIProxy.Start()
	if err != nil {
		return fmt.Errorf("openai proxy: %w", err)
	}
	defer openAIProxy.Stop()

	anthropicProxy := proxy.New(m, logger, hub, upstreams, "anthropic", debugHeadersFlag)
	anthropicAddr, err := anthropicProxy.Start()
	if err != nil {
		return fmt.Errorf("anthropic proxy: %w", err)
	}
	defer anthropicProxy.Stop()

	fmt.Fprintf(os.Stderr, "🛡  Shroud active — secrets will be masked\n")
	if uiFlag {
		fmt.Fprintf(os.Stderr, "🌐 Dashboard: %s\n", uiAddr)
		openBrowser(uiAddr)
	}

	// Inject provider-specific proxy addresses into the child environment.
	// Tools choose the provider-specific env var they already understand.
	env := os.Environ()
	env = append(env,
		"ANTHROPIC_BASE_URL="+anthropicAddr,
		"OPENAI_BASE_URL="+openAIAddr,
		"SHROUD_ANTHROPIC_PROXY="+anthropicAddr,
		"SHROUD_OPENAI_PROXY="+openAIAddr,
	)

	childCmdArgs := toolArgs
	if isCodexTool(tool) {
		env = withoutEnv(env, "OPENAI_BASE_URL")
		childCmdArgs = append([]string{
			"-c",
			fmt.Sprintf("openai_base_url=%q", openAIAddr),
		}, toolArgs...)
	}

	child := exec.Command(tool, childCmdArgs...)
	child.Env = env
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr

	if err := child.Start(); err != nil {
		return fmt.Errorf("starting %s: %w", tool, err)
	}

	// Forward OS signals to the child process
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for sig := range sigCh {
			if child.Process != nil {
				child.Process.Signal(sig)
			}
		}
	}()

	err = child.Wait()
	signal.Stop(sigCh)
	close(sigCh)

	if uiServer != nil {
		uiServer.Close()
	}

	// Session summary
	mappings := m.Mappings()
	if len(mappings) > 0 {
		fmt.Fprintf(os.Stderr, "\n🛡  Session summary: %d secret(s) masked\n", len(mappings))
		for ph := range mappings {
			fmt.Fprintf(os.Stderr, "   %s\n", ph)
		}
	}
	fmt.Fprintf(os.Stderr, "📄 Session log: %s\n", logger.Path())

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		return err
	}
	return nil
}

func showLogs(cmd *cobra.Command, args []string) error {
	dir := filepath.Join(os.Getenv("HOME"), ".shroud", "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("no sessions found at %s", dir)
	}

	type fileInfo struct {
		path    string
		modTime int64
	}
	var files []fileInfo
	for _, e := range entries {
		if !e.IsDir() {
			info, err := e.Info()
			if err != nil {
				continue
			}
			files = append(files, fileInfo{
				path:    filepath.Join(dir, e.Name()),
				modTime: info.ModTime().UnixNano(),
			})
		}
	}
	if len(files) == 0 {
		fmt.Println("No sessions recorded yet.")
		return nil
	}
	// Sort by modification time (most recent last)
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime < files[j].modTime
	})
	latest := files[len(files)-1].path

	fmt.Printf("📄 Session: %s\n\n", filepath.Base(latest))

	f, err := os.Open(latest)
	if err != nil {
		return err
	}
	defer f.Close()

	var masked, rehydrated, requests int
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			continue
		}
		switch e["type"] {
		case "mask_event":
			masked++
			fmt.Printf("  🔴 MASKED    %-8s → %s\n", e["entity"], e["placeholder"])
		case "rehydrate_event":
			rehydrated++
		case "session_start":
			fmt.Printf("  Tool: %v  PID: %v  Started: %v\n\n", e["tool"], e["pid"], e["ts"])
		case "session_end":
			if v, ok := e["request_count"].(float64); ok {
				requests = int(v)
			}
		}
	}

	fmt.Printf("\n  %d request(s)  │  %d secret(s) masked  │  %d rehydrated\n",
		requests, masked, rehydrated)
	return nil
}

func showStatus(cmd *cobra.Command, args []string) error {
	dir := filepath.Join(os.Getenv("HOME"), ".shroud", "sessions")
	entries, _ := os.ReadDir(dir)
	fmt.Printf("🛡  Shroud\n")
	fmt.Printf("   Sessions logged: %d\n", len(entries))
	fmt.Printf("   Session dir:     %s\n", dir)
	return nil
}

// openBrowser opens the given URL in the default system browser.
func openBrowser(url string) {
	var cmd string
	var cmdArgs []string
	switch runtime.GOOS {
	case "darwin":
		cmd, cmdArgs = "open", []string{url}
	case "linux":
		cmd, cmdArgs = "xdg-open", []string{url}
	default:
		cmd, cmdArgs = "cmd", []string{"/c", "start", url}
	}
	exec.Command(cmd, cmdArgs...).Start()
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// envOrDefault returns the value of the environment variable, or the fallback
// if it is empty or unset.
func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func isCodexTool(tool string) bool {
	return strings.EqualFold(filepath.Base(tool), "codex")
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	filtered := env[:0]
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
