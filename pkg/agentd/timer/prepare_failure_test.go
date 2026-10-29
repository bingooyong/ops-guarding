//go:build !windows

package timer

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ops-guarding/pkg/agentd/client"
	"ops-guarding/pkg/agentd/config"
	"ops-guarding/pkg/internal/testenv"
	"ops-guarding/pkg/server/rpc"
)

// assertPrepareFailed checks that a prepare() failure ends as a reported,
// persisted `failed` result instead of a silent `running`.
func assertPrepareFailed(t *testing.T, e *agentEnv, id int64, wantErr string) {
	t.Helper()
	task, ok := Locals.GetTask(id)
	if !ok {
		t.Fatalf("task %d not in Locals", id)
	}
	if got := task.GetStatus(); got != "failed" {
		t.Fatalf("status = %q, want failed", got)
	}
	if got := task.GetStderr(); !strings.Contains(got, "prepare: ") || !strings.Contains(got, wantErr) {
		t.Fatalf("stderr = %q, want prepare error containing %q", got, wantErr)
	}
	if got := e.read(id, "1.done"); got != "failed" {
		t.Fatalf("done flag = %q, want failed", got)
	}
	if rts := Locals.ReportTasks(); len(rts) != 1 || rts[0].Id != id || rts[0].Status != "failed" || rts[0].Stderr == "" {
		t.Fatalf("ReportTasks = %+v", rts)
	}
	if task.GetAlive() {
		t.Fatal("alive must be false after a failed prepare")
	}
}

func TestPrepareMetaRPCErrorReportsFailed(t *testing.T) {
	testenv.New(t)
	e := newAgentEnv(t)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen on loopback: %v", err)
	}
	go rpc.Serve(l)
	config.C.Heartbeat.Servers = []string{l.Addr().String()}
	client.CloseCli()
	t.Cleanup(func() {
		client.CloseCli()
		l.Close()
	})

	// No .write cache: prepare asks the server, which does not know task 71.
	assign(71, 1, "start")
	assertPrepareFailed(t, e, 71, "task 71 not found")
}

func TestPrepareCacheErrorReportsFailedAndRedoRecovers(t *testing.T) {
	e := newAgentEnv(t)
	dir := e.prepared(72, "echo recovered\n", "", "")
	if err := os.Remove(filepath.Join(dir, "args")); err != nil {
		t.Fatal(err)
	}

	assign(72, 1, "start")
	assertPrepareFailed(t, e, 72, "args")

	// Operator fixes the cause and redoes the task (new clock): it runs.
	if err := os.WriteFile(filepath.Join(dir, "args"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	assign(72, 2, "start")
	task := waitStatus(t, 72, "success")
	if got := task.GetStdout(); got != "recovered\n" {
		t.Fatalf("redo stdout = %q", got)
	}
	if got := task.GetStderr(); got != "" {
		t.Fatalf("redo stderr = %q, want empty (buffers reset)", got)
	}
	if got := e.read(72, "2.done"); got != "success" {
		t.Fatalf("done flag = %q", got)
	}
}
