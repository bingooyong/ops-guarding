package testenv

import (
	"testing"

	"ops-guarding/pkg/models"
)

// Task describes a task to seed. Zero values: account "root", script
// "echo hi", creator "tester", timeout 30, action "start".
type Task struct {
	Hosts     []string
	Batch     int
	Tolerance int
	Timeout   int
	Pause     string
	Action    string
	Account   string
	Script    string
}

// SeedTask stores a task exactly as POST /ibex/v1/tasks does (meta, action,
// scheduler row owned by nobody, waiting hosts) and returns its id.
func (e *Env) SeedTask(t testing.TB, in Task) int64 {
	t.Helper()
	if len(in.Hosts) == 0 {
		in.Hosts = []string{"h1"}
	}
	if in.Action == "" {
		in.Action = "start"
	}
	if in.Account == "" {
		in.Account = "root"
	}
	if in.Script == "" {
		in.Script = "echo hi"
	}
	if in.Timeout == 0 {
		in.Timeout = 30
	}

	meta := &models.TaskMeta{
		Title: "seed", Account: in.Account, Batch: in.Batch, Tolerance: in.Tolerance,
		Timeout: in.Timeout, Pause: in.Pause, Script: in.Script, Creator: "tester",
	}
	if err := meta.Save(in.Hosts, in.Action); err != nil {
		t.Fatalf("seed task: %v", err)
	}
	return meta.Id
}

// HostStatuses returns host -> status of a task (all shards via models).
func (e *Env) HostStatuses(t testing.TB, id int64) map[string]string {
	t.Helper()
	hosts, err := models.TaskHostStatus(id)
	if err != nil {
		t.Fatalf("host statuses of %d: %v", id, err)
	}
	out := make(map[string]string, len(hosts))
	for _, h := range hosts {
		out[h.Host] = h.Status
	}
	return out
}

// Doing returns the task_host_doing hosts of a task (host -> action).
func (e *Env) Doing(t testing.TB, id int64) map[string]string {
	t.Helper()
	list, err := models.DoingHostList("id=?", id)
	if err != nil {
		t.Fatalf("doing list of %d: %v", id, err)
	}
	out := make(map[string]string, len(list))
	for _, d := range list {
		out[d.Host] = d.Action
	}
	return out
}

// Action returns the current task_action action ("" if the row is gone).
func (e *Env) Action(t testing.TB, id int64) string {
	t.Helper()
	a, err := models.TaskActionGet("id=?", id)
	if err != nil {
		t.Fatalf("action of %d: %v", id, err)
	}
	if a == nil {
		return ""
	}
	return a.Action
}
