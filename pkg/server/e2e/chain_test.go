// Package e2e holds the task-chain regression tests: create task (HTTP) ->
// schedule -> agent RPC report/assign -> status/result/output (HTTP).
// They run on sqlite by default and on MySQL with -tags integration and
// OPS_MYSQL_DSN (see pkg/internal/testenv).
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"testing"

	"ops-guarding/pkg/internal/testenv"
	"ops-guarding/pkg/models"
	"ops-guarding/pkg/server/logic"
	"ops-guarding/pkg/server/router"
	"ops-guarding/pkg/server/rpc"
	"ops-guarding/pkg/server/timer"
	"ops-guarding/pkg/types"

	"github.com/gin-gonic/gin"
)

type chain struct {
	t   *testing.T
	env *testenv.Env
	r   *gin.Engine
	rpc rpc.Server
}

func newChain(t *testing.T) *chain {
	t.Helper()
	env := testenv.New(t)
	return &chain{t: t, env: env, r: router.New("test")}
}

// call performs an authenticated API request and returns the decoded envelope.
func (c *chain) call(method, path string, body interface{}) (int, map[string]json.RawMessage) {
	c.t.Helper()
	return c.callAuth(method, path, body, testenv.BasicAuthUser, testenv.BasicAuthPass)
}

func (c *chain) callAuth(method, path string, body interface{}, user, pass string) (int, map[string]json.RawMessage) {
	c.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			c.t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if user != "" {
		req.SetBasicAuth(user, pass)
	}
	w := httptest.NewRecorder()
	c.r.ServeHTTP(w, req)

	var env map[string]json.RawMessage
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	return w.Code, env
}

func dat[T any](c *chain, env map[string]json.RawMessage) T {
	c.t.Helper()
	var v T
	if e := errField(env); e != "" {
		c.t.Fatalf("api error: %s", e)
	}
	if err := json.Unmarshal(env["dat"], &v); err != nil {
		c.t.Fatalf("decode dat %s: %v", env["dat"], err)
	}
	return v
}

func errField(env map[string]json.RawMessage) string {
	var s string
	_ = json.Unmarshal(env["err"], &s)
	return s
}

type form map[string]interface{}

func (c *chain) createTask(f form) int64 {
	c.t.Helper()
	base := form{"title": "e2e", "account": "root", "script": "echo hello", "action": "start", "creator": "tester", "timeout": 30}
	for k, v := range f {
		base[k] = v
	}
	code, env := c.call(http.MethodPost, "/ibex/v1/tasks", base)
	if code != http.StatusOK {
		c.t.Fatalf("create task: status %d", code)
	}
	id := dat[int64](c, env)
	if id <= 0 {
		c.t.Fatalf("create task: id = %d", id)
	}
	return id
}

// schedule runs one scheduler round for the task as the server timer would.
func (c *chain) schedule(id int64) {
	c.t.Helper()
	if _, err := models.TakeOverTask(id, "", testenv.Scheduler); err != nil {
		c.t.Fatal(err)
	}
	logic.CheckTimeout(id)
	logic.ScheduleTask(id)
	if err := timer.RefreshHostDoing(); err != nil {
		c.t.Fatal(err)
	}
}

func (c *chain) state(id int64) string {
	c.t.Helper()
	_, env := c.call(http.MethodGet, fmt.Sprintf("/ibex/v1/task/%d/state", id), nil)
	return dat[string](c, env)
}

func (c *chain) result(id int64) map[string][]string {
	c.t.Helper()
	_, env := c.call(http.MethodGet, fmt.Sprintf("/ibex/v1/task/%d/result", id), nil)
	return dat[map[string][]string](c, env)
}

// report simulates one agent heartbeat.
func (c *chain) report(ident string, done ...types.ReportTask) types.ReportResponse {
	c.t.Helper()
	var resp types.ReportResponse
	if err := c.rpc.Report(types.ReportRequest{Ident: ident, ReportTasks: done}, &resp); err != nil {
		c.t.Fatal(err)
	}
	if resp.Message != "" {
		c.t.Fatalf("report %s: %s", ident, resp.Message)
	}
	return resp
}

func (c *chain) assigned(ident string) []types.AssignTask {
	c.t.Helper()
	return c.report(ident).AssignTasks
}

func sorted(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

func TestChainSuccessAndFailure(t *testing.T) {
	c := newChain(t)
	id := c.createTask(form{"hosts": []string{"h1", "h2"}, "batch": 1, "tolerance": 1})

	// Created: both hosts waiting, nothing assigned yet.
	if got := c.state(id); got != "start" {
		t.Fatalf("state after create = %q, want start", got)
	}
	if got := c.result(id); !reflect.DeepEqual(got, map[string][]string{"waiting": {"h1", "h2"}}) {
		t.Fatalf("result after create = %v", got)
	}
	c.schedule(id)

	// batch=1: only h1 is running and only h1 gets work.
	if a := c.assigned("h2"); len(a) != 0 {
		t.Fatalf("h2 assigned %v before its turn", a)
	}
	a := c.assigned("h1")
	if len(a) != 1 || a[0].Id != id || a[0].Action != "start" || a[0].Clock == 0 {
		t.Fatalf("h1 assignment = %+v", a)
	}

	// Agent fetches the task meta over RPC.
	var meta types.TaskMetaResponse
	if err := c.rpc.GetTaskMeta(id, &meta); err != nil || meta.Script != "echo hello" || meta.Account != "root" {
		t.Fatalf("GetTaskMeta = %+v, %v", meta, err)
	}

	// h1 succeeds; the next scheduler round starts h2.
	c.report("h1", types.ReportTask{Id: id, Clock: a[0].Clock, Status: "success", Stdout: "hello\n"})
	c.schedule(id)
	if got := c.result(id); !reflect.DeepEqual(got, map[string][]string{"success": {"h1"}, "running": {"h2"}}) {
		t.Fatalf("result mid-run = %v", got)
	}
	if got := c.state(id); got != "start" {
		t.Fatalf("state mid-run = %q, want start", got)
	}
	if left := c.assigned("h1"); len(left) != 0 {
		t.Fatalf("h1 still assigned after done: %v", left)
	}

	// h2 fails (within tolerance=1): task finishes as failed.
	b := c.assigned("h2")
	if len(b) != 1 {
		t.Fatalf("h2 assignment = %+v", b)
	}
	c.report("h2", types.ReportTask{Id: id, Clock: b[0].Clock, Status: "failed", Stdout: "partial", Stderr: "boom"})
	c.schedule(id)

	if got := c.state(id); got != "failed" {
		t.Fatalf("final state = %q, want failed", got)
	}
	res := c.result(id)
	if !reflect.DeepEqual(sorted(res["success"]), []string{"h1"}) || !reflect.DeepEqual(sorted(res["failed"]), []string{"h2"}) || len(res) != 2 {
		t.Fatalf("final result = %v", res)
	}

	// Cleaner removed the scheduling rows (task is done).
	if act, err := models.TaskActionGet("id=?", id); err != nil || act != nil {
		t.Fatalf("task_action after done = %+v, %v", act, err)
	}

	// Output endpoints return the stored data.
	for path, want := range map[string]string{
		fmt.Sprintf("/ibex/v1/task/%d/host/h1/stdout", id): "hello\n",
		fmt.Sprintf("/ibex/v1/task/%d/host/h2/stdout", id): "partial",
		fmt.Sprintf("/ibex/v1/task/%d/host/h2/stderr", id): "boom",
	} {
		_, env := c.call(http.MethodGet, path, nil)
		if got := dat[string](c, env); got != want {
			t.Errorf("GET %s = %q, want %q", path, got, want)
		}
	}

	// Detail view.
	_, env := c.call(http.MethodGet, fmt.Sprintf("/ibex/v1/task/%d/state?detail=1", id), nil)
	d := dat[router.TaskStateDetail](c, env)
	if d.State != "failed" || d.Total != 2 || d.Counts["success"] != 1 || d.Counts["failed"] != 1 {
		t.Fatalf("detail = %+v", d)
	}
}

func TestChainAllSuccessIsDone(t *testing.T) {
	c := newChain(t)
	id := c.createTask(form{"hosts": []string{"h1", "h2"}}) // batch 0 = all at once
	c.schedule(id)

	for _, h := range []string{"h1", "h2"} {
		a := c.assigned(h)
		if len(a) != 1 {
			t.Fatalf("%s assignment = %+v", h, a)
		}
		c.report(h, types.ReportTask{Id: id, Clock: a[0].Clock, Status: "success"})
	}
	c.schedule(id)

	if got := c.state(id); got != "done" {
		t.Fatalf("state = %q, want done", got)
	}
}

func TestChainToleranceExceededPauses(t *testing.T) {
	c := newChain(t)
	id := c.createTask(form{"hosts": []string{"h1", "h2"}, "batch": 1, "tolerance": 0})
	c.schedule(id)

	a := c.assigned("h1")
	c.report("h1", types.ReportTask{Id: id, Clock: a[0].Clock, Status: "failed", Stderr: "bad"})
	c.schedule(id)

	// 1 failure > tolerance 0: scheduler pauses the task and does not start h2.
	_, env := c.call(http.MethodGet, fmt.Sprintf("/ibex/v1/task/%d", id), nil)
	var detail struct {
		Action string `json:"action"`
	}
	if err := json.Unmarshal(env["dat"], &detail); err != nil || detail.Action != "pause" {
		t.Fatalf("task action = %q (%v), want pause; env=%s", detail.Action, err, env["dat"])
	}
	if got := c.state(id); got != "pause" {
		t.Fatalf("state = %q, want pause", got)
	}
	if a := c.assigned("h2"); len(a) != 0 {
		t.Fatalf("h2 started while paused: %v", a)
	}
	if got := c.result(id); !reflect.DeepEqual(got, map[string][]string{"failed": {"h1"}, "waiting": {"h2"}}) {
		t.Fatalf("result = %v", got)
	}
}

func TestChainTimeoutThenLateReport(t *testing.T) {
	c := newChain(t)
	id := c.createTask(form{"hosts": []string{"h1"}, "timeout": 1})
	c.schedule(id)
	a := c.assigned("h1")
	if len(a) != 1 {
		t.Fatalf("assignment = %+v", a)
	}

	// Age the doing row beyond timeout+3s, then run the scheduler round.
	if err := c.env.DB.Exec("UPDATE task_host_doing SET clock = clock - 60 WHERE id = ?", id).Error; err != nil {
		t.Fatal(err)
	}
	c.schedule(id)

	if got := c.result(id); !reflect.DeepEqual(got, map[string][]string{"timeout": {"h1"}}) {
		t.Fatalf("result after timeout = %v", got)
	}
	if got := c.state(id); got != "failed" {
		t.Fatalf("state after timeout = %q, want failed", got)
	}
	if left := c.assigned("h1"); len(left) != 0 {
		t.Fatalf("timed-out host still assigned: %v", left)
	}

	// The agent finishes late: status and output are still recorded.
	c.report("h1", types.ReportTask{Id: id, Clock: a[0].Clock, Status: "success", Stdout: "late output"})
	if got := c.result(id); !reflect.DeepEqual(got, map[string][]string{"success": {"h1"}}) {
		t.Fatalf("result after late report = %v", got)
	}
	_, env := c.call(http.MethodGet, fmt.Sprintf("/ibex/v1/task/%d/host/h1/stdout", id), nil)
	if got := dat[string](c, env); got != "late output" {
		t.Fatalf("stdout = %q", got)
	}
}

func TestChainUnknownTaskAndHost(t *testing.T) {
	c := newChain(t)
	_, env := c.call(http.MethodGet, "/ibex/v1/task/424242/state", nil)
	if got := errField(env); got != "task 424242 not found" {
		t.Fatalf("unknown task err = %q", got)
	}

	id := c.createTask(form{"hosts": []string{"h1"}})
	_, env = c.call(http.MethodGet, fmt.Sprintf("/ibex/v1/task/%d/host/nope/stdout", id), nil)
	if got := errField(env); got != fmt.Sprintf("host nope not found in task %d", id) {
		t.Fatalf("unknown host err = %q", got)
	}

	var meta types.TaskMetaResponse
	if err := c.rpc.GetTaskMeta(424242, &meta); err != nil || meta.Message != "task 424242 not found" {
		t.Fatalf("GetTaskMeta unknown = %+v, %v", meta, err)
	}
}

func TestChainAuth(t *testing.T) {
	c := newChain(t)

	// /ping is open.
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	c.r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "pong" {
		t.Fatalf("/ping = %d %q", w.Code, w.Body.String())
	}

	cases := []struct {
		name       string
		user, pass string
		want       int
	}{
		{"missing credentials", "", "", http.StatusUnauthorized},
		{"wrong password", testenv.BasicAuthUser, "nope", http.StatusUnauthorized},
		{"unknown user", "mallory", testenv.BasicAuthPass, http.StatusUnauthorized},
		{"valid", testenv.BasicAuthUser, testenv.BasicAuthPass, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _ := c.callAuth(http.MethodGet, "/ibex/v1/tasks", nil, tc.user, tc.pass)
			if code != tc.want {
				t.Fatalf("GET /ibex/v1/tasks = %d, want %d", code, tc.want)
			}
		})
	}
}
