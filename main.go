package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
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

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("$ ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
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
