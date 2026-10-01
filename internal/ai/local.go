package ai

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// Local runs llama.cpp's llama-server as a child process, for server owners
// who want the model on their own machine with nothing to set up but a
// model file. The game talks to it like any other OpenAI-compatible server,
// on a private port, and stops it on shutdown.
type Local struct {
	cmd     *exec.Cmd
	BaseURL string
	done    chan struct{}
}

// StartLocal launches bin (llama-server) with the GGUF model at modelPath
// and waits until it's ready, which can take minutes for a large model.
func StartLocal(ctx context.Context, bin, modelPath string, contextSize int, extra []string) (*Local, error) {
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("local model: %w", err)
	}
	path, err := exec.LookPath(bin)
	if err != nil {
		return nil, fmt.Errorf("local model server %q: %w", bin, err)
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}
	args := append([]string{"-m", modelPath, "--host", "127.0.0.1", "--port", fmt.Sprint(port),
		"-c", fmt.Sprint(contextSize), "--alias", filepath.Base(modelPath)}, extra...)
	cmd := exec.Command(path, args...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr // its log goes to ours
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", bin, err)
	}
	l := &Local{cmd: cmd, BaseURL: fmt.Sprintf("http://127.0.0.1:%d/v1", port), done: make(chan struct{})}
	go func() { cmd.Wait(); close(l.done) }()
	if err := waitHealthy(ctx, fmt.Sprintf("http://127.0.0.1:%d/health", port), l.done); err != nil {
		l.Stop()
		return nil, err
	}
	return l, nil
}

// Stop ends the child process.
func (l *Local) Stop() {
	if l == nil || l.cmd.Process == nil {
		return
	}
	l.cmd.Process.Signal(os.Interrupt)
	select {
	case <-l.done:
	case <-time.After(10 * time.Second):
		l.cmd.Process.Kill()
		<-l.done
	}
}

// Exited is closed if the child process stops.
func (l *Local) Exited() <-chan struct{} { return l.done }

func waitHealthy(ctx context.Context, url string, died <-chan struct{}) error {
	client := &http.Client{Timeout: 3 * time.Second}
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if res, err := client.Get(url); err == nil {
			res.Body.Close()
			if res.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("local model server didn't become ready: %w", ctx.Err())
		case <-died:
			return fmt.Errorf("local model server exited while loading; see its log above")
		case <-tick.C:
		}
	}
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}
