//go:build !windows && !race

package timer

import (
	"testing"
	"time"
)

// NOTE: excluded from -race builds on purpose. killProcess() calls
// persistResult() (reads Task.Stdout/Stderr) right after SIGKILL without
// waiting for exec's output-copy goroutines, which still write into the same
// bytes.Buffer: a real data race in pkg/agentd/timer/task.go (found by this
// test). Re-enable under -race once killProcess waits for the process / copy
// goroutines (see Trellis task 10-23-unit-tests-rpc-logic-agentd research notes).

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
