package rpc

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"ops-guarding/pkg/appframe/cache"
	"ops-guarding/pkg/internal/testenv"
	"ops-guarding/pkg/models"
	"ops-guarding/pkg/types"
)

func TestPing(t *testing.T) {
	var out string
	if err := new(Server).Ping("anything", &out); err != nil || out != "pong" {
		t.Fatalf("Ping() = %q, %v; want pong, nil", out, err)
	}
}

func TestGetTaskMeta(t *testing.T) {
	env := testenv.New(t)
	id := env.SeedTask(t, testenv.Task{Account: "ops", Script: "#!/bin/sh\nexit 0\n"})
	var s Server

	t.Run("found", func(t *testing.T) {
		var resp types.TaskMetaResponse
		if err := s.GetTaskMeta(id, &resp); err != nil {
			t.Fatal(err)
		}
		want := types.TaskMetaResponse{Script: "#!/bin/sh\nexit 0\n", Account: "ops"}
		if !reflect.DeepEqual(resp, want) {
			t.Fatalf("resp = %+v, want %+v", resp, want)
		}
	})

	t.Run("not found reports message, no error", func(t *testing.T) {
		var resp types.TaskMetaResponse
		if err := s.GetTaskMeta(424242, &resp); err != nil {
			t.Fatalf("err = %v, want nil (failures go in Message)", err)
		}
		if resp.Message != "task 424242 not found" || resp.Script != "" {
			t.Fatalf("resp = %+v", resp)
		}
	})

	t.Run("db error reports message, no panic", func(t *testing.T) {
		if err := cache.Instance.Flush(); err != nil {
			t.Fatal(err)
		}
		if err := env.DB.Exec("DROP TABLE task_meta").Error; err != nil {
			t.Fatal(err)
		}
		var resp types.TaskMetaResponse
		if err := s.GetTaskMeta(id, &resp); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
		if resp.Message == "" {
			t.Fatal("expected an error message from the database failure")
		}
	})
}

func startedTask(t *testing.T, env *testenv.Env, hosts ...string) (int64, map[string]int64) {
	t.Helper()
	id := env.SeedTask(t, testenv.Task{Hosts: hosts})
	// Run every host through the real scheduler path to create doing rows.
	for _, h := range hosts {
		if err := env.DB.Exec("UPDATE "+shardOf(id)+" SET status='running' WHERE id=? AND host=?", id, h).Error; err != nil {
			t.Fatal(err)
		}
		if err := env.DB.Create(&models.TaskHostDoing{Id: id, Host: h, Clock: time.Now().Unix(), Action: "start"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	doing, err := models.DoingHostList("id=?", id)
	if err != nil {
		t.Fatal(err)
	}
	clocks := map[string]int64{}
	for _, d := range doing {
		clocks[d.Host] = d.Clock
	}
	return id, clocks
}

func shardOf(id int64) string { return fmt.Sprintf("task_host_%d", id%100) }

func setDoingCache(t *testing.T) {
	t.Helper()
	list, err := models.DoingHostList("")
	if err != nil {
		t.Fatal(err)
	}
	set := map[string][]models.TaskHostDoing{}
	for _, d := range list {
		set[d.Host] = append(set[d.Host], d)
	}
	models.SetDoingCache(set)
}

func TestReportAssignsFromDoingCache(t *testing.T) {
	env := testenv.New(t)
	id1, clocks1 := startedTask(t, env, "h1", "h2")
	id2, _ := startedTask(t, env, "h1")
	setDoingCache(t)

	var s Server
	t.Run("host with two tasks", func(t *testing.T) {
		var resp types.ReportResponse
		if err := s.Report(types.ReportRequest{Ident: "h1"}, &resp); err != nil || resp.Message != "" {
			t.Fatalf("Report: %v %q", err, resp.Message)
		}
		got := map[int64]types.AssignTask{}
		for _, a := range resp.AssignTasks {
			got[a.Id] = a
		}
		if len(got) != 2 || got[id1].Action != "start" || got[id1].Clock != clocks1["h1"] || got[id2].Action != "start" {
			t.Fatalf("assign = %+v", resp.AssignTasks)
		}
	})

	t.Run("unknown ident gets empty non-nil list", func(t *testing.T) {
		var resp types.ReportResponse
		if err := s.Report(types.ReportRequest{Ident: "nobody"}, &resp); err != nil {
			t.Fatal(err)
		}
		if resp.AssignTasks == nil || len(resp.AssignTasks) != 0 {
			t.Fatalf("assign = %#v, want empty non-nil", resp.AssignTasks)
		}
	})

	t.Run("empty report list is a pure heartbeat", func(t *testing.T) {
		var resp types.ReportResponse
		req := types.ReportRequest{Ident: "h2", ReportTasks: []types.ReportTask{}}
		if err := s.Report(req, &resp); err != nil || len(resp.AssignTasks) != 1 {
			t.Fatalf("Report: %v %+v", err, resp)
		}
	})
}

func TestReportPersistsDoneTasks(t *testing.T) {
	env := testenv.New(t)
	id, clocks := startedTask(t, env, "h1", "h2")
	var s Server

	var resp types.ReportResponse
	req := types.ReportRequest{Ident: "h1", ReportTasks: []types.ReportTask{
		{Id: id, Clock: clocks["h1"], Status: "failed", Stdout: "o", Stderr: "e"},
	}}
	if err := s.Report(req, &resp); err != nil || resp.Message != "" {
		t.Fatalf("Report: %v %q", err, resp.Message)
	}

	h, err := models.TaskHostGet(id, "h1")
	if err != nil || h == nil || h.Status != "failed" || h.Stdout != "o" || h.Stderr != "e" {
		t.Fatalf("host h1 = %+v, %v", h, err)
	}
	if other, _ := models.TaskHostGet(id, "h2"); other.Status != "running" {
		t.Fatalf("host h2 status = %q, must stay running", other.Status)
	}
	if got := env.Doing(t, id); !reflect.DeepEqual(got, map[string]string{"h2": "start"}) {
		t.Fatalf("doing = %v, want only h2", got)
	}
}

func TestReportIgnoresStaleClock(t *testing.T) {
	env := testenv.New(t)
	id, clocks := startedTask(t, env, "h1")
	var s Server

	var resp types.ReportResponse
	req := types.ReportRequest{Ident: "h1", ReportTasks: []types.ReportTask{
		{Id: id, Clock: clocks["h1"] - 100, Status: "success", Stdout: "stale"},
	}}
	if err := s.Report(req, &resp); err != nil || resp.Message != "" {
		t.Fatalf("Report: %v %q", err, resp.Message)
	}
	if h, _ := models.TaskHostGet(id, "h1"); h.Status != "running" || h.Stdout != "" {
		t.Fatalf("stale report applied: %+v", h)
	}
}

func TestReportStoreErrorBecomesMessage(t *testing.T) {
	env := testenv.New(t)
	id, clocks := startedTask(t, env, "h1")
	if err := env.DB.Exec("DROP TABLE " + shardOf(id)).Error; err != nil {
		t.Fatal(err)
	}

	var resp types.ReportResponse
	req := types.ReportRequest{Ident: "h1", ReportTasks: []types.ReportTask{
		{Id: id, Clock: clocks["h1"], Status: "success"},
	}}
	if err := new(Server).Report(req, &resp); err != nil {
		t.Fatalf("err = %v, want nil (failures go in Message)", err)
	}
	if resp.Message == "" {
		t.Fatal("expected Message for storage failure")
	}
	if resp.AssignTasks != nil {
		t.Fatalf("no assignments expected on failure, got %+v", resp.AssignTasks)
	}
}
