package tests

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"govard/internal/try"
)

func TestWaitReadySucceedsOnFirstOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	origin := try.Origin{URL: srv.URL, HostHeader: "shop.test"}
	if err := try.WaitReady(context.Background(), srv.Client(), origin, time.Second, 10*time.Millisecond); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
}

func TestWaitReadyRetriesUntilGatewayIsUp(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	origin := try.Origin{URL: srv.URL, HostHeader: "shop.test"}
	if err := try.WaitReady(context.Background(), srv.Client(), origin, 2*time.Second, 10*time.Millisecond); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("server saw %d requests, want 3", got)
	}
}

func TestWaitReadyTimesOutWithLastStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	origin := try.Origin{URL: srv.URL, HostHeader: "shop.test"}
	err := try.WaitReady(context.Background(), srv.Client(), origin, 200*time.Millisecond, 20*time.Millisecond)
	if err == nil {
		t.Fatalf("WaitReady returned nil for a gateway that never came up")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want it to name the last status and the timeout", err)
	}
	if !errors.Is(err, try.ErrNotReady) {
		t.Fatalf("error = %v, want it to wrap try.ErrNotReady", err)
	}
}

func TestWaitReadySendsProjectHostHeader(t *testing.T) {
	var seenHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	origin := try.Origin{URL: srv.URL, HostHeader: "shop.test"}
	if err := try.WaitReady(context.Background(), srv.Client(), origin, time.Second, 10*time.Millisecond); err != nil {
		t.Fatalf("WaitReady: %v", err)
	}
	if seenHost != "shop.test" {
		t.Fatalf("server saw Host %q, want shop.test", seenHost)
	}
}
