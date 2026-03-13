package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/chzyer/readline"
	"github.com/google/uuid"
)

const defaultHistoricalSessionPath = "/workspaces/workspace/ai/agentic-history"

var (
	sessionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("135")).
			Italic(true)

	promptStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("214")).
			Bold(true)

	exitStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("183"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("43"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("196"))
)

// HistoryEntry holds metadata for each command recorded in the session history.
type HistoryEntry struct {
	ID        string `json:"id"`
	Command   string `json:"cmd"`
	Timestamp string `json:"ts"`
	DeltaMs   int64  `json:"delta_ms"`
	ExitCode  int    `json:"exit"`
}

func main() {
	historicalSessionPath := os.Getenv("HISTORICAL_SESSION_PATH")
	if historicalSessionPath == "" {
		historicalSessionPath = defaultHistoricalSessionPath
	}

	now := time.Now()
	sessionID := fmt.Sprintf("%s_%d", now.Format("2006-01-02_15-04-05"), now.Unix())
	sessionDir := filepath.Join(historicalSessionPath, "session-"+sessionID)

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating session directory: %v\n", err)
		os.Exit(1)
	}

	os.Setenv("HISTORICAL_PATH", sessionDir)

	logPath := filepath.Join(sessionDir, "history.jsonl")
	logFile, err := os.Create(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating history log: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	fmt.Println(sessionStyle.Render(fmt.Sprintf("◆ history path: %s", sessionDir)))
	fmt.Println(sessionStyle.Render("  type 'exit!' to end | 'agent [-p|-e] <prompt>' to invoke AI"))
	fmt.Println()

	historyFile := filepath.Join(os.Getenv("HOME"), ".historian_history")
	cwd, _ := os.Getwd()

	rl, err := readline.NewEx(&readline.Config{
		Prompt:          buildPrompt(cwd),
		HistoryFile:     historyFile,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing readline: %v\n", err)
		os.Exit(1)
	}
	defer rl.Close()

	var lastCommandTime time.Time
	encoder := json.NewEncoder(logFile)

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt {
			if len(line) == 0 {
				break
			}
			continue
		}
		if err == io.EOF {
			break
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if line == "exit!" {
			fmt.Println(exitStyle.Render("history closed."))
			break
		}

		var deltaMs int64
		commandTime := time.Now()
		if !lastCommandTime.IsZero() {
			deltaMs = commandTime.Sub(lastCommandTime).Milliseconds()
		}
		lastCommandTime = commandTime

		var exitCode int
		if strings.HasPrefix(line, "agent ") {
			rest := strings.TrimPrefix(line, "agent ")
			mode := "-e"
			prompt := rest
			if strings.HasPrefix(rest, "-p ") {
				mode = "-p"
				prompt = strings.TrimPrefix(rest, "-p ")
			} else if strings.HasPrefix(rest, "-e ") {
				prompt = strings.TrimPrefix(rest, "-e ")
			}
			exitCode = runAgent(mode, prompt, sessionDir, logFile)
		} else if line == "agent" {
			fmt.Println(errorStyle.Render("usage: agent [-p|-e] <prompt>"))
			continue
		} else {
			exitCode = runCommand(line, logFile)
		}

		entry := HistoryEntry{
			ID:        uuid.New().String()[:8],
			Command:   line,
			Timestamp: commandTime.Format(time.RFC3339),
			DeltaMs:   deltaMs,
			ExitCode:  exitCode,
		}
		encoder.Encode(entry)

		newCwd, _ := os.Getwd()
		if newCwd != cwd {
			cwd = newCwd
			rl.SetPrompt(buildPrompt(cwd))
		}
	}
}

func buildPrompt(cwd string) string {
	dir := filepath.Base(cwd)
	if dir == "/" {
		dir = "/"
	}
	return promptStyle.Render(dir) + " ⟩ "
}

func runCommand(cmdLine string, logFile *os.File) int {
	cmd := exec.Command("bash", "-c", cmdLine)
	cmd.Env = os.Environ()
	cmd.Stdin = os.Stdin

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating stdout pipe: %v\n", err)
		return 1
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating stderr pipe: %v\n", err)
		return 1
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting command: %v\n", err)
		return 1
	}

	done := make(chan struct{}, 2)
	go func() {
		io.Copy(io.MultiWriter(os.Stdout, logFile), stdoutPipe)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(io.MultiWriter(os.Stderr, logFile), stderrPipe)
		done <- struct{}{}
	}()
	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}

func runAgent(mode string, prompt string, sessionDir string, logFile *os.File) int {
	invokeScript := findHistorianScript()
	if invokeScript == "" {
		fmt.Println(errorStyle.Render("historian.sh not found"))
		return 1
	}

	cmd := exec.Command(invokeScript, mode, prompt)
	cmd.Env = append(os.Environ(), "HISTORICAL_SESSION_PATH="+sessionDir)
	cmd.Stdin = os.Stdin
	cmd.Stdout = io.MultiWriter(os.Stdout, logFile)
	cmd.Stderr = io.MultiWriter(os.Stderr, logFile)

	fmt.Println(sessionStyle.Render("invoking agent..."))

	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			fmt.Println(errorStyle.Render(fmt.Sprintf("agent exited: %d", code)))
			return code
		}
		fmt.Println(errorStyle.Render(fmt.Sprintf("agent error: %v", err)))
		return 1
	}

	fmt.Println(successStyle.Render("agent completed"))
	return 0
}

func findHistorianScript() string {
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "historian.sh")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	cwd, err := os.Getwd()
	if err == nil {
		candidate := filepath.Join(cwd, "historian.sh")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	path, err := exec.LookPath("historian.sh")
	if err == nil {
		return path
	}

	return ""
}
