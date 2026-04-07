//go:build integration && unix

package main_test

import (
	"bufio"
	"encoding/json"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

func Test_Server_With_SIGINT_Should_ShutdownCleanly(t *testing.T) {
	t.Parallel()
	testSignalShutdown(t, syscall.SIGINT)
}

func Test_Server_With_SIGTERM_Should_ShutdownCleanly(t *testing.T) {
	t.Parallel()
	testSignalShutdown(t, syscall.SIGTERM)
}

func testSignalShutdown(t *testing.T, sig syscall.Signal) {
	t.Helper()

	cmd := exec.Command(testBinary)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	// Collect stderr lines in background
	stderrCh := make(chan []string, 1)
	go func() {
		var lines []string
		scanner := bufio.NewScanner(stderrPipe)
		for scanner.Scan() {
			lines = append(lines, scanner.Text())
		}
		stderrCh <- lines
	}()

	// Wait for server_started by reading stderr indirectly — send initialize
	// and wait for the response on stdout to confirm the server is running.
	initReq := `{"jsonrpc":"2.0","method":"initialize","id":1,"params":{"capabilities":{}}}` + "\n"
	if _, err := stdin.Write([]byte(initReq)); err != nil {
		t.Fatalf("write init: %v", err)
	}

	// Read initialize response from stdout
	stdoutScanner := bufio.NewScanner(stdoutPipe)
	gotResponse := make(chan struct{})
	go func() {
		if stdoutScanner.Scan() {
			close(gotResponse)
		}
	}()

	select {
	case <-gotResponse:
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for initialize response")
	}

	// Act — send signal, then write a ping to unblock the decoder. The loop
	// will process the ping and then check ctx.Done() at the top of the next
	// iteration, observing the cancelled context from the signal.
	if err := cmd.Process.Signal(sig); err != nil {
		_ = stdin.Close()
		t.Fatalf("signal failed: %v", err)
	}

	// Give the signal a moment to be delivered and cancel the context.
	time.Sleep(100 * time.Millisecond)

	// Send a ping to unblock the decoder — the server processes it, loops
	// back, and the ctx.Done() select case fires.
	pingReq := `{"jsonrpc":"2.0","method":"ping","id":2,"params":{}}` + "\n"
	_, _ = stdin.Write([]byte(pingReq))

	// Wait for process to exit
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case waitErr := <-done:
		if waitErr != nil {
			t.Errorf("expected clean exit, got: %v", waitErr)
		}
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("timed out waiting for process exit")
	}

	_ = stdin.Close()

	// Collect stderr
	var stderrLines []string
	select {
	case stderrLines = <-stderrCh:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out collecting stderr")
	}

	// Assert — stderr contains shutdown log with non-eof reason
	allStderr := strings.Join(stderrLines, "\n")
	dec := json.NewDecoder(strings.NewReader(allStderr))
	foundShutdown := false
	for {
		var entry map[string]any
		if err := dec.Decode(&entry); err != nil {
			break
		}
		if entry["msg"] == "server_stopped" {
			foundShutdown = true
			reason, _ := entry["reason"].(string)
			if reason == "eof" {
				t.Error("expected non-eof shutdown reason for signal")
			}
			break
		}
	}
	if !foundShutdown {
		t.Errorf("expected server_stopped log entry; stderr: %s", allStderr)
	}
}
