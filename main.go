package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/chzyer/readline"
)

const defaultHistoricalSessionPath = "/workspaces/workspace/ai/agentic-history"

func main() {
	historicalSessionPath := os.Getenv("HISTORICAL_SESSION_PATH")
	if historicalSessionPath == "" {
		historicalSessionPath = defaultHistoricalSessionPath
	}

	now := time.Now()
	sessionID := fmt.Sprintf("%s_%d", now.Format("2006-01-02_15-04-05"), now.Unix())
	sessionDir := fmt.Sprintf("%s/session-%s", historicalSessionPath, sessionID)

	if err := os.MkdirAll(sessionDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error creating session directory: %v\n", err)
		os.Exit(1)
	}

	// Export HISTORICAL_PATH so child commands inherit it
	os.Setenv("HISTORICAL_PATH", sessionDir)

	logPath := fmt.Sprintf("%s/session_log.txt", sessionDir)
	logFile, err := os.Create(logPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating session log: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close()

	fmt.Printf("Session started: %s\n", sessionDir)
	fmt.Printf("Type 'exit!' to end the session.\n\n")

	historyFile := fmt.Sprintf("%s/.historian_history", os.Getenv("HOME"))
	rl, err := readline.NewEx(&readline.Config{
		Prompt:          "$ ",
		HistoryFile:     historyFile,
		InterruptPrompt: "^C",
		EOFPrompt:       "exit",
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error initializing readline: %v\n", err)
		os.Exit(1)
	}
	defer rl.Close()

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
			fmt.Println("Session ended.")
			break
		}
		runCommand(line, logFile)
	}
}

func runCommand(cmdLine string, logFile *os.File) {
	timestamp := time.Now()
	fmt.Fprintf(logFile, "\n[%s]\n$ %s\n", timestamp.Format("2006-01-02 15:04:05"), cmdLine)

	cmd := exec.Command("bash", "-c", cmdLine)
	cmd.Env = os.Environ()

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating stdout pipe: %v\n", err)
		return
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating stderr pipe: %v\n", err)
		return
	}

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting command: %v\n", err)
		return
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
		fmt.Fprintf(logFile, "\n[exit: %v]\n", err)
	}
}
