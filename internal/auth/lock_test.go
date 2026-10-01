package auth

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestLockFileExcludesSecondHolder(t *testing.T) {
	unlock, err := lockFile(context.Background())
	if err != nil {
		t.Fatalf("lockFile: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := lockFile(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("second lockFile err = %v, want a deadline error while the lock is held", err)
	}

	unlock()
	unlock2, err := lockFile(context.Background())
	if err != nil {
		t.Fatalf("lockFile after unlock: %v", err)
	}
	unlock2()
}

// Re-executed as a child by TestLockFileExcludesOtherProcess.
func TestHelperHoldLock(t *testing.T) {
	if os.Getenv("OKU_TEST_HOLD_LOCK") != "1" {
		t.Skip("helper process only")
	}
	unlock, err := lockFile(context.Background())
	if err != nil {
		t.Fatalf("lockFile: %v", err)
	}
	defer unlock()
	os.Stdout.WriteString("locked\n")
	// hold until the parent closes our stdin
	_, _ = os.Stdin.Read(make([]byte, 1))
}

func TestLockFileExcludesOtherProcess(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperHoldLock$")
	cmd.Env = append(os.Environ(), "OKU_TEST_HOLD_LOCK=1", "OKU_TEST_DATA_DIR="+os.Getenv("XDG_DATA_HOME"))
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Wait() })

	buf := make([]byte, len("locked\n"))
	if _, err := stdout.Read(buf); err != nil || string(buf) != "locked\n" {
		t.Fatalf("child did not report the lock: %q, %v", buf, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if unlock, err := lockFile(ctx); err == nil {
		unlock()
		t.Fatal("lockFile succeeded while another process holds the lock")
	}

	// the lock is released when the holder exits
	_ = stdin.Close()
	_ = cmd.Wait()
	unlock, err := lockFile(context.Background())
	if err != nil {
		t.Fatalf("lockFile after holder exited: %v", err)
	}
	unlock()
}

// Each goroutine stands in for a separate oku process holding the same
// expired token; the server rejects a replayed refresh token.
func TestConcurrentRefreshesHitServerOnce(t *testing.T) {
	freshKeychain(t)
	rs := newRotatingServer(t, "r0")
	mustStore(t, expiredToken("old", "r0"))

	const n = 8
	tokens := make([]*oauth2.Token, n)
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Go(func() {
			tokens[i], errs[i] = rs.source(expiredToken("old", "r0")).Token()
		})
	}
	wg.Wait()

	for i := range n {
		if errs[i] != nil {
			t.Fatalf("source %d: %v", i, errs[i])
		}
		if tokens[i].AccessToken != "access-1" {
			t.Fatalf("source %d AccessToken = %q, want %q", i, tokens[i].AccessToken, "access-1")
		}
	}
	if calls := rs.callCount(); calls != 1 {
		t.Fatalf("token endpoint called %d times, want 1", calls)
	}
}

func TestRefreshWorksWhenLockUnavailable(t *testing.T) {
	freshKeychain(t)
	// a file where the data dir should be makes MkdirAll fail
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_DATA_HOME", blocker)

	rs := newRotatingServer(t, "r0")
	mustStore(t, expiredToken("old", "r0"))

	got, err := rs.source(expiredToken("old", "r0")).Token()
	if err != nil {
		t.Fatalf("Token() = %v, want a refresh despite the lock failing", err)
	}
	if got.AccessToken != "access-1" {
		t.Fatalf("AccessToken = %q, want %q", got.AccessToken, "access-1")
	}
}
