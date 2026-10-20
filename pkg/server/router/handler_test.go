package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"ops-guarding/pkg/models"

	"github.com/gin-gonic/gin"
)

type stub struct {
	meta   *models.TaskMeta
	action *models.TaskAction
	counts map[string]int64
	host   *models.TaskHost
	err    error
}

func install(t *testing.T, s stub) {
	t.Helper()
	om, oa, oc, oh := taskMetaGetFn, taskActionGetFn, taskHostCountsFn, taskHostGetFn
	t.Cleanup(func() { taskMetaGetFn, taskActionGetFn, taskHostCountsFn, taskHostGetFn = om, oa, oc, oh })
	taskMetaGetFn = func(interface{}) (*models.TaskMeta, error) { return s.meta, s.err }
	taskActionGetFn = func(int64) (*models.TaskAction, error) { return s.action, nil }
	taskHostCountsFn = func(int64) (map[string]int64, error) { return s.counts, nil }
	taskHostGetFn = func(int64, string) (*models.TaskHost, error) { return s.host, nil }
}

func do(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/task/:id/state", taskState)
	r.GET("/task/:id/host/:host/stdout", taskHostStdout)
	r.GET("/task/:id/host/:host/stderr", taskHostStderr)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("%s: status %d body %s", path, w.Code, w.Body.String())
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &m); err != nil {
		t.Fatalf("%s: bad json %s", path, w.Body.String())
	}
	return m
}

func errOf(m map[string]json.RawMessage) string {
	var s string
	_ = json.Unmarshal(m["err"], &s)
	return s
}

func TestTaskStateUnknownTask(t *testing.T) {
	install(t, stub{})
	m := do(t, "/task/42/state")
	if errOf(m) != "task 42 not found" {
		t.Fatalf("err = %q", errOf(m))
	}
}

func TestTaskStateLookupError(t *testing.T) {
	install(t, stub{err: errors.New("db down")})
	if e := errOf(do(t, "/task/1/state")); e != "db down" {
		t.Fatalf("err = %q", e)
	}
}

func TestTaskStateAggregated(t *testing.T) {
	install(t, stub{
		meta:   &models.TaskMeta{Id: 7},
		action: &models.TaskAction{Id: 7, Action: "start"},
		counts: map[string]int64{"success": 3, "failed": 1},
	})
	m := do(t, "/task/7/state")
	var dat string
	_ = json.Unmarshal(m["dat"], &dat)
	if dat != "failed" || errOf(m) != "" {
		t.Fatalf("dat=%q err=%q", dat, errOf(m))
	}
}

func TestTaskStateDetail(t *testing.T) {
	install(t, stub{
		meta:   &models.TaskMeta{Id: 7},
		action: &models.TaskAction{Id: 7, Action: "start"},
		counts: map[string]int64{"running": 2, "success": 1},
	})
	m := do(t, "/task/7/state?detail=1")
	var d TaskStateDetail
	if err := json.Unmarshal(m["dat"], &d); err != nil {
		t.Fatal(err)
	}
	if d.State != "start" || d.Action != "start" || d.Total != 3 || d.Counts["running"] != 2 {
		t.Fatalf("detail = %+v", d)
	}
}

func TestTaskHostOutputUnknownHostNoPanic(t *testing.T) {
	install(t, stub{})
	for _, p := range []string{"/task/9/host/h1/stdout", "/task/9/host/h1/stderr"} {
		if e := errOf(do(t, p)); e != "host h1 not found in task 9" {
			t.Fatalf("%s err = %q", p, e)
		}
	}
}

func TestTaskHostOutputFound(t *testing.T) {
	install(t, stub{host: &models.TaskHost{Stdout: "out", Stderr: "errtxt"}})
	var s string
	_ = json.Unmarshal(do(t, "/task/9/host/h1/stdout")["dat"], &s)
	if s != "out" {
		t.Fatalf("stdout dat = %q", s)
	}
	_ = json.Unmarshal(do(t, "/task/9/host/h1/stderr")["dat"], &s)
	if s != "errtxt" {
		t.Fatalf("stderr dat = %q", s)
	}
}
