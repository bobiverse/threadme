package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// useInterpreter points jobs at a known shell and restores the package default.
func useInterpreter(t *testing.T, shell string) {
	t.Helper()

	previous := cmdInterpreter
	cmdInterpreter = shell
	t.Cleanup(func() { cmdInterpreter = previous })
}

// waitFor polls cond until it holds, and reports whether it did in time.
func waitFor(within time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Every job used to leave its kill-timer waiting in a goroutine for the whole
// time limit, so a batch of quick jobs piled up one leaked goroutine each.
func TestRunBashWithTimeoutDoesNotLeakPerJobGoroutines(t *testing.T) {
	useInterpreter(t, "/bin/sh")

	const jobs = 50

	before := runtime.NumGoroutine()
	for i := 0; i < jobs; i++ {
		if _, _, err := runBashWithTimeout(5*time.Minute, "true"); err != nil {
			t.Fatalf("job %d returned unexpected error: %v", i, err)
		}
	}

	// tolerate a few unrelated runtime goroutines, but not one per job
	const tolerance = 10
	settled := waitFor(2*time.Second, func() bool {
		return runtime.NumGoroutine()-before <= tolerance
	})

	if !settled {
		t.Fatalf("goroutines leaked across %d jobs: before=%d, after=%d", jobs, before, runtime.NumGoroutine())
	}
}

// A job that finishes inside its time limit must not be killed afterwards.
// The job leaves a background process in its group and returns at once; the
// marker only appears if no timeout kill lands on that group later.
func TestRunBashWithTimeoutDoesNotKillAfterJobFinished(t *testing.T) {
	useInterpreter(t, "/bin/sh")

	marker := filepath.Join(t.TempDir(), "survived")
	// stdout is redirected so the background child does not hold the pipe open
	job := fmt.Sprintf("(sleep 1; touch %s) >/dev/null 2>&1 & exit 0", marker)

	start := time.Now()
	if _, _, err := runBashWithTimeout(300*time.Millisecond, job); err != nil {
		t.Fatalf("runBashWithTimeout returned unexpected error: %v", err)
	}

	if elapsed := time.Since(start); elapsed > 300*time.Millisecond {
		t.Skipf("machine too loaded: the job took %s and hit its own time limit, nothing to prove", elapsed)
	}

	if !waitFor(3*time.Second, func() bool { return fileExists(marker) }) {
		t.Fatal("process group was killed after the job had already finished: stale timeout kill fired")
	}
}

// A job that really does exceed its time limit must still be killed.
func TestRunBashWithTimeoutKillsJobOverTimeLimit(t *testing.T) {
	useInterpreter(t, "/bin/sh")

	start := time.Now()
	_, _, err := runBashWithTimeout(300*time.Millisecond, "sleep 30")
	if err == nil {
		t.Fatal("expected the job to be killed, got no error")
	}

	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("job was not killed near its time limit: took %s", elapsed)
	}
}

// Cancellation (--stop-on / --while) must kill child process groups that are
// still running, instead of orphaning them when the program exits.
func TestKillRunningStopsInFlightJobs(t *testing.T) {
	useInterpreter(t, "/bin/sh")

	done := make(chan error, 1)
	go func() {
		_, _, err := runBashWithTimeout(0, "sleep 30")
		done <- err
	}()

	// wait until the job registered itself as running
	if !waitFor(2*time.Second, func() bool { return len(runningPgids()) > 0 }) {
		t.Fatal("job never registered a running process group")
	}

	killRunning()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a signal error from the killed job, got none")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child process survived killRunning()")
	}

	if got := runningPgids(); len(got) != 0 {
		t.Fatalf("registry not cleaned up after job returned: %v", got)
	}
}

// A finished job must leave nothing behind for cancellation to kill.
func TestRunBashWithTimeoutUnregistersFinishedJob(t *testing.T) {
	useInterpreter(t, "/bin/sh")

	if _, _, err := runBashWithTimeout(5*time.Second, "true"); err != nil {
		t.Fatalf("runBashWithTimeout returned unexpected error: %v", err)
	}

	if got := runningPgids(); len(got) != 0 {
		t.Fatalf("finished job still registered as running: %v", got)
	}
}

func TestResolveInterpreter(t *testing.T) {
	tests := []struct {
		name    string
		flag    string
		want    string
		wantErr bool
	}{
		{name: "shell from environment", flag: "/bin/bash", want: "/bin/bash"},
		{name: "explicit interpreter", flag: "/bin/zsh", want: "/bin/zsh"},
		{name: "nothing set", flag: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveInterpreter(tt.flag)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got interpreter %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
