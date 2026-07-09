package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const hookMarker = "fjgo"

type hookInstallResult struct {
	Status       string   `json:"status"`
	Integrations []string `json:"integrations"`
	Command      string   `json:"command"`
	Capture      string   `json:"capture_command"`
	Files        []string `json:"files"`
	Changed      []string `json:"changed,omitempty"`
	CheckOnly    bool     `json:"check_only,omitempty"`
}

func runSetup(args []string, stdout io.Writer) error {
	if hasHelp(args) {
		return writeHelp(stdout, setupHelp())
	}
	if len(args) == 0 || args[0] != "hooks" {
		return newUsageError("usage: fjgo setup hooks [--check]", "Run `fjgo setup hooks`")
	}
	args = args[1:]
	check := false
	for _, arg := range args {
		switch arg {
		case "--check":
			check = true
		default:
			if strings.HasPrefix(arg, "-") {
				return unknownFlagError("setup hooks", arg, []string{"--check"})
			}
			return newUsageError("usage: fjgo setup hooks [--check]", "Run `fjgo setup hooks`")
		}
	}
	result, err := installHooks(check)
	if err != nil {
		return err
	}
	return writeTOON(stdout, toonBlocks{
		map[string]any{
			"hooks": map[string]any{
				"status":          result.Status,
				"command":         result.Command,
				"capture_command": result.Capture,
				"check_only":      result.CheckOnly,
			},
		},
		tableBlock("integrations", []string{"name"}, namesRows(result.Integrations)),
		tableBlock("files", []string{"path"}, namesRows(result.Files)),
		helpBlock([]string{
			"Restart your agent session after installing hooks",
			"Run `fjgo setup hooks --check` to verify managed hook target paths",
			"Run `fjgo skill install --force` to install the lower-overhead on-demand skill too",
		}),
	})
}

func namesRows(values []string) []map[string]any {
	rows := make([]map[string]any, 0, len(values))
	for _, value := range values {
		rows = append(rows, map[string]any{"name": value, "path": collapseHome(value)})
	}
	return rows
}

func installHooks(checkOnly bool) (hookInstallResult, error) {
	home, err := hookHomeDir()
	if err != nil {
		return hookInstallResult{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return hookInstallResult{}, err
	}
	hookExe := portableHookExecutable(exe)
	command := hookCommandString(hookExe, "-R", "origin")
	capture := hookCommandString(hookExe, "hook", "capture")
	result := hookInstallResult{
		Status:       "installed",
		Integrations: []string{"Claude Code", "Codex", "OpenCode"},
		Command:      command,
		Capture:      capture,
		CheckOnly:    checkOnly,
		Files: []string{
			filepath.Join(home, ".claude", "settings.json"),
			filepath.Join(home, ".codex", "hooks.json"),
			filepath.Join(home, ".codex", "config.toml"),
			filepath.Join(home, ".config", "opencode", "plugins", "axi-fjgo.js"),
		},
	}
	if checkOnly {
		result.Status = "check"
		return result, nil
	}
	jsonTargets := []string{
		filepath.Join(home, ".claude", "settings.json"),
		filepath.Join(home, ".codex", "hooks.json"),
	}
	for _, target := range jsonTargets {
		changed, err := installJSONHooks(target, command, capture)
		if err != nil {
			return result, err
		}
		if changed {
			result.Changed = append(result.Changed, target)
		}
	}
	changed, err := installCodexConfig(filepath.Join(home, ".codex", "config.toml"))
	if err != nil {
		return result, err
	}
	if changed {
		result.Changed = append(result.Changed, filepath.Join(home, ".codex", "config.toml"))
	}
	changed, err = installOpenCodePlugin(filepath.Join(home, ".config", "opencode", "plugins", "axi-fjgo.js"), hookExe)
	if err != nil {
		return result, err
	}
	if changed {
		result.Changed = append(result.Changed, filepath.Join(home, ".config", "opencode", "plugins", "axi-fjgo.js"))
	}
	return result, nil
}

func hookHomeDir() (string, error) {
	if v := os.Getenv("FJGO_HOOK_HOME"); v != "" {
		return v, nil
	}
	return os.UserHomeDir()
}

func portableHookExecutable(exe string) string {
	abs, err := filepath.Abs(exe)
	if err == nil {
		exe = abs
	}
	if path, err := exec.LookPath("fjgo"); err == nil && sameExecutable(path, exe) {
		return "fjgo"
	}
	return exe
}

func sameExecutable(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return false
	}
	return ra == rb
}

func hookCommandString(exe string, args ...string) string {
	parts := []string{shellQuote(exe)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return !(r == '/' || r == '.' || r == '_' || r == '-' || r == ':' || r == '=' || r == '+' || r == ',' || r == '@' || r == '%' || r == '~' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z')
	}) == -1 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func installJSONHooks(path, startCommand, endCommand string) (bool, error) {
	settings := map[string]any{}
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) != 0 {
		if err := json.Unmarshal(b, &settings); err != nil {
			return false, fmt.Errorf("%s: parse json: %w", path, err)
		}
	} else if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	changed := false
	var next any
	next, changed = upsertHook(settings, "SessionStart", startCommand, changed)
	settings = next.(map[string]any)
	next, changed = upsertHook(settings, "SessionEnd", endCommand, changed)
	settings = next.(map[string]any)
	if !changed {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return false, err
	}
	b = append(b, '\n')
	return true, os.WriteFile(path, b, 0o644)
}

func upsertHook(settings map[string]any, event, command string, changed bool) (any, bool) {
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		hooks = map[string]any{}
		settings["hooks"] = hooks
		changed = true
	}
	groups, _ := hooks[event].([]any)
	for _, groupValue := range groups {
		group, ok := groupValue.(map[string]any)
		if !ok {
			continue
		}
		items, _ := group["hooks"].([]any)
		for _, itemValue := range items {
			item, ok := itemValue.(map[string]any)
			if !ok {
				continue
			}
			existing, _ := item["command"].(string)
			if !strings.Contains(existing, hookMarker) {
				continue
			}
			if item["type"] == "command" && item["command"] == command && hookTimeoutIs10(item["timeout"]) {
				return settings, changed
			}
			item["type"] = "command"
			item["command"] = command
			item["timeout"] = 10
			return settings, true
		}
	}
	hooks[event] = append(groups, map[string]any{
		"matcher": "",
		"hooks": []any{
			map[string]any{
				"type":    "command",
				"command": command,
				"timeout": 10,
			},
		},
	})
	return settings, true
}

func hookTimeoutIs10(value any) bool {
	switch x := value.(type) {
	case int:
		return x == 10
	case int64:
		return x == 10
	case float64:
		return x == 10
	default:
		return false
	}
}

func installCodexConfig(path string) (bool, error) {
	current := ""
	if b, err := os.ReadFile(path); err == nil {
		current = string(b)
	} else if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	updated, changed := computeCodexConfigUpdate(current)
	if !changed {
		return false, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(updated), 0o644)
}

func computeCodexConfigUpdate(content string) (string, bool) {
	newline := "\n"
	if strings.Contains(content, "\r\n") {
		newline = "\r\n"
	}
	if strings.TrimSpace(content) == "" {
		return "[features]" + newline + "hooks = true" + newline, true
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	inFeatures := false
	sawFeatures := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if inFeatures {
				lines = slicesInsert(lines, i, "hooks = true")
				return strings.Join(lines, newline), true
			}
			name := strings.Trim(trimmed, "[] ")
			inFeatures = name == "features"
			sawFeatures = sawFeatures || inFeatures
			continue
		}
		if !inFeatures {
			continue
		}
		if strings.HasPrefix(trimmed, "hooks") && strings.Contains(trimmed, "=") {
			if strings.Contains(trimmed, "true") {
				return content, false
			}
			lines[i] = strings.Replace(line, "false", "true", 1)
			return strings.Join(lines, newline), true
		}
	}
	if sawFeatures {
		if !strings.HasSuffix(content, newline) {
			content += newline
		}
		return content + "hooks = true" + newline, true
	}
	separator := newline + newline
	if strings.HasSuffix(content, newline) {
		separator = newline
	}
	return content + separator + "[features]" + newline + "hooks = true" + newline, true
}

func slicesInsert(values []string, index int, value string) []string {
	values = append(values, "")
	copy(values[index+1:], values[index:])
	values[index] = value
	return values
}

func installOpenCodePlugin(path, exe string) (bool, error) {
	const managed = "axi-sdk-js managed opencode plugin: fjgo"
	source := openCodePluginSource(exe)
	current, err := os.ReadFile(path)
	if err == nil {
		if !strings.Contains(string(current), managed) {
			return false, fmt.Errorf("%s: refusing to overwrite unmanaged OpenCode plugin", path)
		}
		if string(current) == source {
			return false, nil
		}
	} else if err != nil && !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(source), 0o644)
}

func openCodePluginSource(exe string) string {
	commandJSON, _ := json.Marshal(exe)
	return `// axi-sdk-js managed opencode plugin: fjgo
// Generated by fjgo. Remove the managed marker above before taking manual ownership.
import { spawn } from "node:child_process";

const command = ` + string(commandJSON) + `;
const args = ["-R", "origin"];
const captureArgs = ["hook", "capture"];
const timeoutMs = 10000;
let captureRegistered = false;

function runFjgoHome(directory) {
  return new Promise((resolve) => {
    const child = spawn(command, args, {
      cwd: typeof directory === "string" && directory.length > 0 ? directory : process.cwd(),
      env: process.env,
      shell: false,
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    let settled = false;
    const timer = setTimeout(() => {
      if (settled) return;
      settled = true;
      child.kill("SIGTERM");
      resolve("error: fjgo ambient context timed out after " + timeoutMs + "ms");
    }, timeoutMs);
    child.stdout?.setEncoding("utf-8");
    child.stderr?.setEncoding("utf-8");
    child.stdout?.on("data", (chunk) => { stdout += chunk; });
    child.stderr?.on("data", (chunk) => { stderr += chunk; });
    child.on("error", (error) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      resolve("error: fjgo ambient context failed: " + error.message);
    });
    child.on("close", (code) => {
      if (settled) return;
      settled = true;
      clearTimeout(timer);
      if (code === 0) return resolve(stdout.trim());
      resolve("error: fjgo ambient context failed: " + (stderr || stdout || ("exit " + code)).trim());
    });
  });
}

function runFjgoCapture(directory) {
  try {
    const child = spawn(command, captureArgs, {
      cwd: typeof directory === "string" && directory.length > 0 ? directory : process.cwd(),
      env: process.env,
      shell: false,
      stdio: "ignore",
    });
    child.unref?.();
  } catch {
    // Best-effort session capture must not affect OpenCode shutdown.
  }
}

function registerFjgoCapture(directory) {
  if (captureRegistered) return;
  captureRegistered = true;
  process.once("beforeExit", () => runFjgoCapture(directory));
}

export const AxiFjgoAmbientContextPlugin = async ({ directory }) => {
  const sessionCache = new Map();
  registerFjgoCapture(directory);
  return {
    "experimental.chat.system.transform": async (input, output) => {
      const sessionID = input.sessionID ?? "__global__";
      let homeView = sessionCache.get(sessionID);
      if (homeView === undefined) {
        homeView = await runFjgoHome(directory);
        sessionCache.set(sessionID, homeView);
      }
      if (homeView.length === 0) return;
      output.system.push("## AXI ambient context: fjgo\n" + homeView);
    },
  };
};
`
}

func runHook(args []string, stdout io.Writer) error {
	if hasHelp(args) {
		return writeHelp(stdout, "usage: fjgo hook capture\nCapture lightweight session-end context for future fjgo diagnostics.")
	}
	if len(args) == 0 || args[0] != "capture" {
		return newUsageError("usage: fjgo hook capture")
	}
	if err := rejectUnknownFlags(args[1:], "hook capture", nil, nil); err != nil {
		return err
	}
	if len(args) != 1 {
		return newUsageError("usage: fjgo hook capture")
	}
	home, err := hookHomeDir()
	if err != nil {
		return err
	}
	stateDir := filepath.Join(home, ".local", "state", "fjgo")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return err
	}
	record := hookCaptureRecord()
	lineBytes, err := json.Marshal(record)
	if err != nil {
		return err
	}
	line := string(lineBytes) + "\n"
	if err := appendFile(filepath.Join(stateDir, "sessions.log"), line); err != nil {
		return err
	}
	return writeTOON(stdout, map[string]any{"capture": "recorded"})
}

func hookCaptureRecord() map[string]any {
	cwd, _ := os.Getwd()
	record := map[string]any{
		"time":   time.Now().UTC().Format(time.RFC3339),
		"goos":   runtime.GOOS,
		"goarch": runtime.GOARCH,
		"cwd":    cwd,
	}
	if branch := gitOutput("rev-parse", "--abbrev-ref", "HEAD"); branch != "" {
		record["branch"] = branch
	}
	if head := gitOutput("rev-parse", "--short", "HEAD"); head != "" {
		record["head"] = head
	}
	if status := gitOutput("status", "--porcelain"); status != "" {
		record["dirty_files"] = len(strings.Split(status, "\n"))
	} else if isGitWorktree() {
		record["dirty_files"] = 0
	}
	if ref := gitRemoteRepo("origin"); ref != "" {
		record["repo"] = ref
	}
	return record
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func isGitWorktree() bool {
	return gitOutput("rev-parse", "--is-inside-work-tree") == "true"
}

func gitRemoteRepo(remote string) string {
	raw := gitOutput("remote", "get-url", remote)
	if raw == "" {
		return ""
	}
	ref, err := parseRemoteRepo(raw, "")
	if err != nil {
		return ""
	}
	return ref.Owner + "/" + ref.Repo
}

func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.WriteString(f, text)
	return err
}
