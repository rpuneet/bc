package slackgw

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/socketmode"
)

func TestAppTokenLockPathUsesHashNotSecret(t *testing.T) {
	t.Setenv(socketLockDirEnv, t.TempDir())
	// Path hashing fixture — avoid credential-shaped strings (gosec G101).
	const appCred = "fixture-app-cred-should-never-appear-in-path"
	path, err := appTokenLockPath(appCred)
	if err != nil {
		t.Fatalf("appTokenLockPath: %v", err)
	}
	if strings.Contains(path, appCred) {
		t.Fatalf("lock path %q contains raw app credential", path)
	}
	if strings.Contains(path, "fixture-app-cred") {
		t.Fatalf("lock path %q leaks credential prefix material", path)
	}
	if !strings.HasSuffix(path, ".lock") {
		t.Fatalf("lock path %q: want .lock suffix", path)
	}
}

func TestAcquireAppTokenLockExclusive(t *testing.T) {
	t.Setenv(socketLockDirEnv, t.TempDir())
	const appCred = "fixture-app-cred-lock-value"

	release1, err := acquireAppTokenLock(appCred)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer release1()

	_, err = acquireAppTokenLock(appCred)
	if err == nil {
		t.Fatal("second acquire succeeded; want exclusive lock error")
	}
	if !strings.Contains(err.Error(), "another local process") {
		t.Fatalf("error = %q, want mention of another local process", err)
	}
	if strings.Contains(err.Error(), appCred) {
		t.Fatalf("error leaked app credential: %q", err)
	}

	release1()
	release2, err := acquireAppTokenLock(appCred)
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	release2()
}

func TestAcquireAppTokenLockEmptyTokenNoop(t *testing.T) {
	release, err := acquireAppTokenLock("")
	if err != nil {
		t.Fatalf("empty token: %v", err)
	}
	release()
}

func TestSocketConnectionErrorAttrs(t *testing.T) {
	t.Parallel()
	underlying := errors.New("websocket: close 1006 (abnormal closure)")
	msg, attrs := socketConnectionErrorAttrs(&slack.ConnectionErrorEvent{
		Attempt:  4,
		Backoff:  1500 * time.Millisecond,
		ErrorObj: underlying,
	})
	if msg != underlying.Error() {
		t.Fatalf("msg = %q, want %q", msg, underlying.Error())
	}
	got := attrsToMap(attrs)
	if got["error"] != underlying.Error() {
		t.Errorf("error attr = %v", got["error"])
	}
	if got["attempt"] != 4 {
		t.Errorf("attempt = %v, want 4", got["attempt"])
	}
	if got["backoff"] != "1.5s" {
		t.Errorf("backoff = %v, want 1.5s", got["backoff"])
	}

	msg, attrs = socketConnectionErrorAttrs(nil)
	if msg != "unknown connection error" {
		t.Errorf("nil data msg = %q", msg)
	}
	if attrsToMap(attrs)["error"] != "unknown connection error" {
		t.Errorf("nil data attrs = %v", attrs)
	}
}

func TestHandleConnectionErrorRecordsDetailAndFlap(t *testing.T) {
	a := New("bot-fixture", "app-fixture")
	underlying := errors.New("dial tcp: connection refused")

	for i := 0; i < socketFlapThreshold-1; i++ {
		a.handleConnectionError(&slack.ConnectionErrorEvent{
			Attempt:  i + 1,
			Backoff:  time.Second,
			ErrorObj: underlying,
		})
	}
	st := a.Status()
	if st.Connected {
		t.Fatal("Connected = true after connection errors, want false")
	}
	if st.Error != underlying.Error() {
		t.Fatalf("Status.Error = %q, want %q", st.Error, underlying.Error())
	}
	a.chatMu.RLock()
	warned := a.flapWarned
	a.chatMu.RUnlock()
	if warned {
		t.Fatal("flapWarned early, before threshold")
	}

	a.handleConnectionError(&slack.ConnectionErrorEvent{
		Attempt:  socketFlapThreshold,
		Backoff:  time.Second,
		ErrorObj: underlying,
	})
	a.chatMu.RLock()
	warned = a.flapWarned
	errs := a.flapErrors
	a.chatMu.RUnlock()
	if !warned {
		t.Fatal("flapWarned = false after threshold, want true")
	}
	if errs < socketFlapThreshold {
		t.Fatalf("flapErrors = %d, want >= %d", errs, socketFlapThreshold)
	}
}

func TestProcessEventHelloResetsFlap(t *testing.T) {
	a := New("bot-fixture", "app-fixture")
	a.handleConnectionError(&slack.ConnectionErrorEvent{
		Attempt:  1,
		ErrorObj: errors.New("boom"),
	})
	a.processEvent(nil, socketmode.Event{Type: socketmode.EventTypeHello})
	a.chatMu.RLock()
	defer a.chatMu.RUnlock()
	if a.flapErrors != 0 {
		t.Fatalf("flapErrors = %d after hello, want 0", a.flapErrors)
	}
	if a.flapWarned {
		t.Fatal("flapWarned still set after hello")
	}
	if !a.connected {
		t.Fatal("connected = false after hello")
	}
	if a.lastError != "" {
		t.Fatalf("lastError = %q after hello, want empty", a.lastError)
	}
}

func TestNoteConnectionErrorWindowReset(t *testing.T) {
	a := New("bot-fixture", "app-fixture")
	now := time.Now()
	a.chatMu.Lock()
	if a.noteConnectionErrorLocked(now) {
		t.Fatal("unexpected flap warn on first error")
	}
	a.flapErrors = socketFlapThreshold - 1
	// Outside window → counter resets, should not warn yet.
	if a.noteConnectionErrorLocked(now.Add(socketFlapWindow + time.Second)) {
		t.Fatal("flap warn after window reset on first error in new window")
	}
	if a.flapErrors != 1 {
		t.Fatalf("flapErrors = %d after window reset, want 1", a.flapErrors)
	}
	a.chatMu.Unlock()
}

func TestPluginDocsMentionSingleSocketClient(t *testing.T) {
	docs := strings.Join(plugin{}.Describe().Docs, "\n")
	if !strings.Contains(docs, "SLACK_APP_TOKEN") {
		t.Fatalf("Docs missing SLACK_APP_TOKEN dual-client guidance:\n%s", docs)
	}
}

func attrsToMap(attrs []any) map[string]any {
	out := make(map[string]any, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		key, ok := attrs[i].(string)
		if !ok {
			continue
		}
		out[key] = attrs[i+1]
	}
	return out
}

func TestLockFileWrittenWithPID(t *testing.T) {
	t.Setenv(socketLockDirEnv, t.TempDir())
	const appCred = "fixture-app-cred-pid-check"
	release, err := acquireAppTokenLock(appCred)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer release()

	path, err := appTokenLockPath(appCred)
	if err != nil {
		t.Fatalf("path: %v", err)
	}
	// path is under t.TempDir via socketLockDirEnv.
	data, err := os.ReadFile(path) //nolint:gosec // G304: temp lock path from appTokenLockPath
	if err != nil {
		if os.IsNotExist(err) {
			t.Skip("lock file not created on this platform")
		}
		t.Fatalf("read lock: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got == "" {
		t.Skip("lock file empty on this platform (no flock PID write)")
	}
	pid, err := strconv.Atoi(got)
	if err != nil {
		t.Fatalf("lock contents %q: want pid: %v", got, err)
	}
	if pid != os.Getpid() {
		t.Fatalf("lock pid = %d, want %d", pid, os.Getpid())
	}
}
