//go:build !windows

package timer

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestKillRunningTask(t *testing.T) {
	e := newAgentEnv(t)
	e.prepared(21, "sleep 30\n", "", "")

	assign(21, 1, "start")
	task, _ := Locals.GetTask(21)
	deadline := time.Now().Add(5 * time.Second)
	for !task.GetAlive() { // process started
		if time.Now().After(deadline) {
			t.Fatal("task never started")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if rts := Locals.ReportTasks(); len(rts) != 0 {
		t.Fatalf("running task must not be reported yet: %+v", rts)
	}

	assign(21, 2, "kill")
	waitStatus(t, 21, "killed")

	if got := e.read(21, "2.done"); got != "killed" {
		t.Fatalf("done flag = %q, want killed", got)
	}
	rts := Locals.ReportTasks()
	if len(rts) != 1 || rts[0].Status != "killed" || rts[0].Clock != 2 {
		t.Fatalf("ReportTasks = %+v", rts)
	}
}

// Race witness: output is read (GetStdout / persistResult, the same getters ReportTasks uses) while the
// process is still writing and while it is being killed. Must be clean under -race.
func TestKillWhileOutputIsStreaming(t *testing.T) {
	e := newAgentEnv(t)
	e.prepared(31, "while :; do echo out; echo err >&2; done\n", "", "")

	assign(31, 1, "start")
	task, _ := Locals.GetTask(31)
	deadline := time.Now().Add(5 * time.Second)
	for task.GetStdout() == "" || task.GetStderr() == "" {
		if time.Now().After(deadline) {
			t.Fatal("task produced no output")
		}
		time.Sleep(2 * time.Millisecond)
	}

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = task.GetStdout()
				_ = task.GetStderr()
			}
		}
	}()

	assign(31, 2, "kill")
	waitStatus(t, 31, "killed")
	close(stop)
	wg.Wait()

	if got := e.read(31, "2.done"); got != "killed" {
		t.Fatalf("done flag = %q, want killed", got)
	}
	if rts := Locals.ReportTasks(); len(rts) != 1 || rts[0].Status != "killed" || rts[0].Stdout == "" {
		t.Fatalf("ReportTasks = %+v", rts)
	}
	// The persisted output must be the complete final output (written after Wait).
	if got, want := e.read(31, "stdout"), task.GetStdout(); got != want {
		t.Fatalf("persisted stdout (%d bytes) != final buffer (%d bytes)", len(got), len(want))
	}
	if out := task.GetStdout(); out == "" || strings.Trim(out, "out\n") != "" || !strings.HasSuffix(out, "\n") {
		t.Fatalf("unexpected stdout tail: %q", out[max(0, len(out)-20):])
	}
}

// kill for a task that has no live process must not panic and must end terminal.
func TestKillTaskWithoutProcess(t *testing.T) {
	t.Run("replayed finished result", func(t *testing.T) {
		e := newAgentEnv(t)
		dir := e.prepared(41, "echo never\n", "", "")
		writeFiles(t, dir, map[string]string{"1.done": "success\n", "stdout": "old", "stderr": ""})

		assign(41, 1, "start") // restart replay: Cmd stays nil
		assign(41, 2, "kill")
		waitStatus(t, 41, "killed")

		if got := e.read(41, "2.done"); got != "killed" {
			t.Fatalf("done flag = %q, want killed", got)
		}
		rts := Locals.ReportTasks()
		if len(rts) != 1 || rts[0].Status != "killed" || rts[0].Clock != 2 || rts[0].Stdout != "old" {
			t.Fatalf("ReportTasks = %+v", rts)
		}
	})

	for name, cmd := range map[string]*exec.Cmd{"nil Cmd": nil, "unstarted Cmd": {}} {
		t.Run(name, func(t *testing.T) {
			e := newAgentEnv(t)
			e.prepared(42, "true\n", "", "")
			task := &Task{Id: 42, Clock: 3, Cmd: cmd}
			killProcess(task)
			if got := task.GetStatus(); got != "killed" {
				t.Fatalf("status = %q, want killed", got)
			}
			if got := e.read(42, "3.done"); got != "killed" {
				t.Fatalf("done flag = %q, want killed", got)
			}
		})
	}

	t.Run("CmdKill guards", func(t *testing.T) {
		if err := CmdKill(nil); err == nil {
			t.Fatal("CmdKill(nil) returned nil error")
		}
		if err := CmdKill(&exec.Cmd{}); err == nil {
			t.Fatal("CmdKill(unstarted) returned nil error")
		}
	})
}

// Fallback branch: the signal was delivered but runProcess never signals `exited`
// (e.g. a detached grandchild keeps the output pipe open so Wait blocks).
// Deterministic: no runProcess goroutine, exited is simply never closed.
func TestKillGraceFallbackRecordsResult(t *testing.T) {
	e := newAgentEnv(t)
	e.prepared(61, "true\n", "", "")

	old := killGrace
	killGrace = 50 * time.Millisecond
	t.Cleanup(func() { killGrace = old })

	cmd := exec.Command("sleep", "30")
	if err := CmdStart(cmd); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Wait() }) // reap the killed child

	task := &Task{Id: 61, Clock: 4, Cmd: cmd, exited: make(chan struct{})}
	task.Stdout.WriteString("partial")

	start := time.Now()
	killProcess(task)
	if d := time.Since(start); d < 50*time.Millisecond || d > 3*time.Second {
		t.Fatalf("killProcess returned after %v, want ~killGrace", d)
	}
	if got := task.GetStatus(); got != "killed" {
		t.Fatalf("status = %q, want killed", got)
	}
	if got := e.read(61, "4.done"); got != "killed" {
		t.Fatalf("done flag = %q", got)
	}
	if got := e.read(61, "stdout"); got != "partial" {
		t.Fatalf("stdout = %q", got)
	}
}

// start() must mark the task alive before it returns, otherwise a second start
// (redo assignment) could pass the GetAlive guard while the first run is spawning.
func TestStartMarksTaskAliveSynchronously(t *testing.T) {
	e := newAgentEnv(t)
	e.prepared(62, "sleep 30\n", "", "")

	assign(62, 1, "start")
	task, _ := Locals.GetTask(62)
	if !task.GetAlive() {
		t.Fatal("task not alive right after start()")
	}

	assign(62, 2, "kill")
	waitStatus(t, 62, "killed")
}
