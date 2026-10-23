//go:build !windows

package timer

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"testing"
	"time"

	"ops-guarding/pkg/agentd/client"
	"ops-guarding/pkg/agentd/config"
	"ops-guarding/pkg/internal/testenv"
	"ops-guarding/pkg/models"
	"ops-guarding/pkg/server/logic"
	"ops-guarding/pkg/server/rpc"
	srvtimer "ops-guarding/pkg/server/timer"
)

// TestAgentServerLoopback runs the real agentd heartbeat / RPC client / task
// execution against an in-process server RPC listener and the test database:
// create -> schedule -> assign -> agent runs script -> report -> final state.
func TestAgentServerLoopback(t *testing.T) {
	testenv.New(t)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	go rpc.Serve(l)

	// Agent-side globals; restore afterwards.
	oldCfg, oldLocals := config.C, Locals
	config.C = &config.Config{
		MetaDir:   t.TempDir(),
		Heartbeat: config.Heartbeat{Servers: []string{l.Addr().String()}, Host: "loop-agent", Interval: 50},
	}
	Locals = &LocalTasksT{M: make(map[int64]*Task)}
	client.CloseCli()
	t.Cleanup(func() {
		client.CloseCli()
		l.Close()
		config.C, Locals = oldCfg, oldLocals
	})

	u, err := user.Current()
	if err != nil {
		t.Fatal(err)
	}

	meta := &models.TaskMeta{
		Title:   "loopback",
		Account: u.Username, // as root runs directly, otherwise the current user is used anyway
		Script:  "#!/bin/sh\necho hello-from-agent\necho oops >&2\n",
		Creator: "tester",
		Timeout: 30,
	}
	if err := meta.Save([]string{"loop-agent"}, "start"); err != nil {
		t.Fatal(err)
	}
	id := meta.Id

	if _, err := models.TakeOverTask(id, "", testenv.Scheduler); err != nil {
		t.Fatal(err)
	}
	logic.ScheduleTask(id)

	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := srvtimer.RefreshHostDoing(); err != nil {
			t.Fatal(err)
		}
		heartbeat()

		h, err := models.TaskHostGet(id, "loop-agent")
		if err != nil || h == nil {
			t.Fatalf("TaskHostGet = %v, %v", h, err)
		}
		if h.Status == "success" {
			if h.Stdout != "hello-from-agent\n" || h.Stderr != "oops\n" {
				t.Fatalf("recorded output stdout=%q stderr=%q", h.Stdout, h.Stderr)
			}
			break
		}
		if h.Status != "running" {
			t.Fatalf("unexpected host status %q (stdout=%q stderr=%q)", h.Status, h.Stdout, h.Stderr)
		}
		if time.Now().After(deadline) {
			t.Fatalf("host still %q after 10s", h.Status)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Agent-side artifacts persisted under MetaDir/<id>/.
	dir := filepath.Join(config.C.MetaDir, fmt.Sprint(id))
	for _, name := range []string{"script", "stdout", "stderr"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing agent artifact %s: %v", name, err)
		}
	}

	logic.ScheduleTask(id) // cleaner sees no ing hosts
	counts, err := models.TaskHostStatusCounts(id)
	if err != nil || counts["success"] != 1 {
		t.Fatalf("counts = %v, %v", counts, err)
	}
	if got := models.AggregateState("", false, counts); got != models.TaskStateDone {
		t.Fatalf("state = %q, want done", got)
	}
}
