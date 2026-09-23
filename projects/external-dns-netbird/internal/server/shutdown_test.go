package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

// TestRunDrainsBothListeners documents the graceful-shutdown contract:
// cancelling the Run context shuts down BOTH listeners (webhook + ops)
// with "shutting down" + "drained" log lines, and Run returns nil.
func TestRunDrainsBothListeners(t *testing.T) {
	webhookLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("webhook listen: %v", err)
	}
	webhookAddr := webhookLn.Addr().String()
	_ = webhookLn.Close()
	opsLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("ops listen: %v", err)
	}
	opsAddr := opsLn.Addr().String()
	_ = opsLn.Close()

	s := testServerWithAddrs(t, webhookAddr, opsAddr)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()

	waitUp := func(url string) {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			resp, err := http.Get(url) //nolint:gosec,noctx,bodyclose
			if err == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Fatalf("listener %s never came up", url)
	}
	waitUp("http://" + webhookAddr + "/")
	waitUp("http://" + opsAddr + "/healthz")

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil drain", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not return after context cancellation")
	}

	// Both listeners must be closed: dials fail after drain.
	for _, addr := range []string{webhookAddr, opsAddr} {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			t.Errorf("listener %s still accepting after drain", addr)
		}
	}
}

func testServerWithAddrs(t *testing.T, webhookAddr, opsAddr string) *Server {
	t.Helper()
	s := testServer()
	s.webhookAddr = webhookAddr
	s.metricsAddr = opsAddr
	return s
}
