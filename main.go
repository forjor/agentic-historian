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
		runCommand(line, sessionDir)
	}
}

func runCommand(cmdLine, sessionDir string) {
	timestamp := time.Now()
	outFileName := fmt.Sprintf("%s/%s_%d", sessionDir, timestamp.Format("2006-01-02_15-04-05"), timestamp.Unix())

	outFile, err := os.Create(outFileName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error creating output file: %v\n", err)
		return
	}
	defer outFile.Close()

	fmt.Fprintf(outFile, "$ %s\n", cmdLine)

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
		io.Copy(io.MultiWriter(os.Stdout, outFile), stdoutPipe)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(io.MultiWriter(os.Stderr, outFile), stderrPipe)
		done <- struct{}{}
	}()
	<-done
	<-done

	if err := cmd.Wait(); err != nil {
		fmt.Fprintf(outFile, "\n[exit: %v]\n", err)
	}

	fmt.Fprintf(os.Stderr, "\n[recorded: %s]\n", outFileName)
}
