package updater

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type updateCheckServer struct {
	server  *httptest.Server
	release chan struct{}
	started chan struct{}
}

// newUpdateCheckServer answers the release check only once the test closes
// release, which is what lets a test observe the in-flight state.
func newUpdateCheckServer(t *testing.T, tag string) *updateCheckServer {
	t.Helper()

	uc := &updateCheckServer{release: make(chan struct{}), started: make(chan struct{})}
	uc.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case <-uc.started:
		default:
			close(uc.started)
		}
		<-uc.release
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"` + tag + `","body":"notes"}`))
	}))
	t.Cleanup(uc.server.Close)

	t.Setenv("GOVARD_UPDATE_CHECK_URL", uc.server.URL)
	t.Setenv("GOVARD_HOME_DIR", t.TempDir())
	return uc
}

func waitForRequest(t *testing.T, uc *updateCheckServer) {
	t.Helper()
	select {
	case <-uc.started:
	case <-time.After(2 * time.Second):
		t.Fatal("the update check never issued its request")
	}
}

func recordNotifications(t *testing.T) *int32 {
	t.Helper()
	var count int32
	restore := SetUpdateCheckNotifierForTest(func(string, string) { atomic.AddInt32(&count, 1) })
	t.Cleanup(restore)
	return &count
}

func TestStartUpdateCheckReturnsImmediately(t *testing.T) {
	uc := newUpdateCheckServer(t, "v9.9.9")
	count := recordNotifications(t)

	started := time.Now()
	finish := StartUpdateCheck("1.0.0")
	elapsed := time.Since(started)
	if elapsed > 50*time.Millisecond {
		t.Fatalf("StartUpdateCheck took %s, want an immediate return while the request is in flight", elapsed)
	}
	if atomic.LoadInt32(count) != 0 {
		t.Fatal("StartUpdateCheck notified before the request had been answered")
	}

	waitForRequest(t, uc)
	close(uc.release)
	_ = finish
}

func TestUpdateCheckFinishSkipsPendingResult(t *testing.T) {
	uc := newUpdateCheckServer(t, "v9.9.9")
	count := recordNotifications(t)

	finish := StartUpdateCheck("1.0.0")
	waitForRequest(t, uc)

	started := time.Now()
	finish()
	elapsed := time.Since(started)
	if elapsed > 50*time.Millisecond {
		t.Fatalf("finish blocked for %s, want it to return without waiting for the request", elapsed)
	}
	if atomic.LoadInt32(count) != 0 {
		t.Fatal("finish notified for a result that had not arrived yet")
	}

	close(uc.release)
	time.Sleep(100 * time.Millisecond)
	if got := atomic.LoadInt32(count); got != 0 {
		t.Fatalf("a result that arrived after the command finished still notified %d times", got)
	}
}

func TestUpdateCheckFinishNotifiesArrivedResult(t *testing.T) {
	uc := newUpdateCheckServer(t, "v9.9.9")
	count := recordNotifications(t)

	finish := StartUpdateCheck("1.0.0")
	waitForRequest(t, uc)
	close(uc.release)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		finish()
		if atomic.LoadInt32(count) > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := atomic.LoadInt32(count); got != 1 {
		t.Fatalf("got %d notifications for an arrived newer release, want exactly 1", got)
	}
}
