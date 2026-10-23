//go:build !windows

package timer

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ops-guarding/pkg/agentd/config"
	"ops-guarding/pkg/types"
)

// agentEnv points the agent globals at a temp MetaDir and a fresh task table.
type agentEnv struct {
	t       *testing.T
	metaDir string
	account string
}

func newAgentEnv(t *testing.T) *agentEnv {
	t.Helper()
	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}

	oldCfg, oldLocals := config.C, Locals
	dir := t.TempDir()
	config.C = &config.Config{MetaDir: dir}
	Locals = &LocalTasksT{M: make(map[int64]*Task)}
	t.Cleanup(func() { config.C, Locals = oldCfg, oldLocals })

	return &agentEnv{t: t, metaDir: dir, account: u.Username}
}

// prepared stores a task the way Task.prepare would after the first RPC fetch,
// so execution tests need no server (prepare reads it back from disk).
func (e *agentEnv) prepared(id int64, script, args, stdin string) string {
	e.t.Helper()
	dir := filepath.Join(e.metaDir, fmt.Sprint(id))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.t.Fatal(err)
	}
	files := map[string]string{"args": args, "account": e.account, "stdin": stdin, ".write": ""}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "script"), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		e.t.Fatal(err)
	}
	return dir
}

func (e *agentEnv) read(id int64, name string) string {
	e.t.Helper()
	b, err := os.ReadFile(filepath.Join(e.metaDir, fmt.Sprint(id), name))
	if err != nil {
		e.t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

// waitStatus polls until the local task reaches one of the wanted statuses.
func waitStatus(t *testing.T, id int64, want ...string) *Task {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if task, ok := Locals.GetTask(id); ok {
			s := task.GetStatus()
			for _, w := range want {
				if s == w && !task.GetAlive() {
					return task
				}
			}
		}
		if time.Now().After(deadline) {
			task, _ := Locals.GetTask(id)
			status := "<no task>"
			if task != nil {
				status = task.GetStatus()
			}
			t.Fatalf("task %d: status %q, want one of %v", id, status, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func assign(id, clock int64, action string) {
	Locals.AssignTask(types.AssignTask{Id: id, Clock: clock, Action: action})
}

func TestTaskExecution(t *testing.T) {
	cases := []struct {
		name       string
		script     string
		args       string
		stdin      string
		wantStatus string
		wantStdout string
		wantStderr string
	}{
		{"exit 0 is success", "echo out; echo err >&2; exit 0\n", "", "", "success", "out\n", "err\n"},
		{"non-zero exit is failed", "echo partial; exit 3\n", "", "", "failed", "partial\n", ""},
		{"stdin is passed through", "cat\n", "", "hello-stdin", "success", "hello-stdin", ""},
		{"args are split on ,, and quoted", "echo \"$1|$2\"\n", "a,,b c", "", "success", "a|b c\n", ""},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e := newAgentEnv(t)
			id, clock := int64(100+i), int64(1000+i)
			e.prepared(id, c.script, c.args, c.stdin)

			assign(id, clock, "start")
			task := waitStatus(t, id, c.wantStatus)

			if got := task.GetStdout(); got != c.wantStdout {
				t.Fatalf("stdout = %q, want %q", got, c.wantStdout)
			}
			if got := task.GetStderr(); got != c.wantStderr {
				t.Fatalf("stderr = %q, want %q", got, c.wantStderr)
			}

			// Results are persisted for replays after an agent restart.
			if got := e.read(id, "stdout"); got != c.wantStdout {
				t.Fatalf("persisted stdout = %q", got)
			}
			if got := e.read(id, "stderr"); got != c.wantStderr {
				t.Fatalf("persisted stderr = %q", got)
			}
			if got := e.read(id, fmt.Sprintf("%d.done", clock)); got != c.wantStatus {
				t.Fatalf("done flag = %q, want %q", got, c.wantStatus)
			}

			// And reported to the server.
			rts := Locals.ReportTasks()
			if len(rts) != 1 || rts[0].Id != id || rts[0].Clock != clock || rts[0].Status != c.wantStatus || rts[0].Stdout != c.wantStdout {
				t.Fatalf("ReportTasks = %+v", rts)
			}
		})
	}
}

func TestTaskPrepareFetchesFromServerOnlyOnce(t *testing.T) {
	// With .write present prepare must read from disk and never call the server
	// (no RPC client is configured in this test; a fetch would block/fail).
	e := newAgentEnv(t)
	e.prepared(7, "echo cached\n", "", "")

	assign(7, 1, "start")
	task := waitStatus(t, 7, "success")
	if task.Account != e.account {
		t.Fatalf("account = %q, want %q (read from disk)", task.Account, e.account)
	}
}

func TestAssignIgnoresDuplicateAndRerunsOnNewClock(t *testing.T) {
	e := newAgentEnv(t)
	counter := filepath.Join(e.metaDir, "runs")
	e.prepared(5, fmt.Sprintf("echo x >> %s\n", counter), "", "")
	runs := func() int {
		b, _ := os.ReadFile(counter)
		return strings.Count(string(b), "x")
	}

	assign(5, 1, "start")
	waitStatus(t, 5, "success")
	assign(5, 1, "start") // same id/clock/action: ignored
	assign(5, 1, "start")
	time.Sleep(100 * time.Millisecond)
	if got := runs(); got != 1 {
		t.Fatalf("runs after duplicate assignments = %d, want 1", got)
	}

	assign(5, 2, "start") // new clock (redo): executes again
	deadline := time.Now().Add(10 * time.Second)
	for runs() != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("runs = %d, want 2 after new clock", runs())
		}
		time.Sleep(10 * time.Millisecond)
	}
	waitStatus(t, 5, "success")
	if task, _ := Locals.GetTask(5); task.Clock != 2 {
		t.Fatalf("clock = %d, want 2", task.Clock)
	}
}

func TestAssignReplaysFinishedResultWithoutRunning(t *testing.T) {
	e := newAgentEnv(t)
	marker := filepath.Join(e.metaDir, "ran")
	dir := e.prepared(9, fmt.Sprintf("touch %s\n", marker), "", "")
	// Simulate an agent restart: the result of clock 77 is already on disk.
	for name, content := range map[string]string{"77.done": "failed\n", "stdout": "old out", "stderr": "old err"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	assign(9, 77, "start")

	task, ok := Locals.GetTask(9)
	if !ok || task.GetStatus() != "failed" || task.GetStdout() != "old out" || task.GetStderr() != "old err" {
		t.Fatalf("replayed task = %+v", task)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("script ran again for an already finished clock")
	}
	if rts := Locals.ReportTasks(); len(rts) != 1 || rts[0].Status != "failed" || rts[0].Stdout != "old out" {
		t.Fatalf("ReportTasks = %+v", rts)
	}
}

func TestAssignKillUnknownTaskIsIgnored(t *testing.T) {
	newAgentEnv(t)
	assign(404, 1, "kill")
	if _, ok := Locals.GetTask(404); ok {
		t.Fatal("kill for an unknown local task must not create a task")
	}
}

func TestAssignUnknownActionDoesNotRun(t *testing.T) {
	e := newAgentEnv(t)
	marker := filepath.Join(e.metaDir, "ran")
	e.prepared(11, fmt.Sprintf("touch %s\n", marker), "", "")

	assign(11, 1, "bogus")

	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("script ran for an unknown action")
	}
}

func TestReportTasksSkipsIntermediateAndTruncates(t *testing.T) {
	newAgentEnv(t)
	big := strings.Repeat("a", 70000) + "TAIL"

	Locals.SetTask(&Task{Id: 1, Clock: 1, Status: "running"})
	Locals.SetTask(&Task{Id: 2, Clock: 2, Status: "killing"})
	done := &Task{Id: 3, Clock: 3, Status: "success"}
	done.Stdout.WriteString(big)
	done.Stderr.WriteString("e" + big)
	Locals.SetTask(done)

	rts := Locals.ReportTasks()
	if len(rts) != 1 || rts[0].Id != 3 {
		t.Fatalf("ReportTasks = %d entries, want only the finished task", len(rts))
	}
	if len(rts[0].Stdout) != 65535 || !strings.HasSuffix(rts[0].Stdout, "TAIL") {
		t.Fatalf("stdout len=%d, want last 65535 bytes", len(rts[0].Stdout))
	}
	if len(rts[0].Stderr) != 65535 || !strings.HasSuffix(rts[0].Stderr, "TAIL") {
		t.Fatalf("stderr len=%d, want last 65535 bytes", len(rts[0].Stderr))
	}
}

func TestCleanKeepsAssignedAndRunning(t *testing.T) {
	newAgentEnv(t)
	Locals.SetTask(&Task{Id: 1, Status: "success"})
	Locals.SetTask(&Task{Id: 2, Status: "success"})
	Locals.SetTask(&Task{Id: 3, Status: "running"})

	Locals.Clean(map[int64]struct{}{1: {}})

	if _, ok := Locals.GetTask(1); !ok {
		t.Error("assigned task 1 was removed")
	}
	if _, ok := Locals.GetTask(2); ok {
		t.Error("finished, unassigned task 2 should be dropped")
	}
	if _, ok := Locals.GetTask(3); !ok {
		t.Error("running task 3 must be kept so its result is still reported")
	}
}

func TestReportTasksTruncationBoundary(t *testing.T) {
	newAgentEnv(t)
	exact := strings.Repeat("x", 65535)
	over := "HEAD" + strings.Repeat("y", 65531) + "Z" // 65536 bytes

	for _, c := range []struct {
		name, stdout, want string
	}{
		{"exactly the limit is kept whole", exact, exact},
		{"one byte over drops the oldest byte", over, over[1:]},
	} {
		t.Run(c.name, func(t *testing.T) {
			Locals.M = map[int64]*Task{}
			task := &Task{Id: 1, Clock: 1, Status: "success"}
			task.Stdout.WriteString(c.stdout)
			Locals.SetTask(task)

			rts := Locals.ReportTasks()
			if len(rts) != 1 || rts[0].Stdout != c.want {
				t.Fatalf("stdout len=%d, want len=%d", len(rts[0].Stdout), len(c.want))
			}
		})
	}
}
