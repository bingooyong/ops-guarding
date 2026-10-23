package timer

import (
	"reflect"
	"testing"
	"time"

	"ops-guarding/pkg/internal/testenv"
	"ops-guarding/pkg/models"
)

func TestCacheHostDoingGroupsByHost(t *testing.T) {
	env := testenv.New(t)
	for _, d := range []models.TaskHostDoing{
		{Id: 1, Host: "h1", Clock: 10, Action: "start"},
		{Id: 2, Host: "h1", Clock: 11, Action: "kill"},
		{Id: 2, Host: "h2", Clock: 12, Action: "start"},
	} {
		if err := env.DB.Create(&d).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := RefreshHostDoing(); err != nil {
		t.Fatal(err)
	}

	if got := models.GetDoingCache("h1"); len(got) != 2 {
		t.Fatalf("h1 doing = %+v, want 2 entries", got)
	}
	got := models.GetDoingCache("h2")
	if len(got) != 1 || got[0].Id != 2 || got[0].Action != "start" {
		t.Fatalf("h2 doing = %+v", got)
	}
	if got := models.GetDoingCache("unknown"); len(got) != 0 {
		t.Fatalf("unknown host doing = %+v", got)
	}

	// Rows removed from the table disappear on the next refresh.
	if err := env.DB.Exec("DELETE FROM task_host_doing").Error; err != nil {
		t.Fatal(err)
	}
	if err := RefreshHostDoing(); err != nil {
		t.Fatal(err)
	}
	if got := models.GetDoingCache("h1"); len(got) != 0 {
		t.Fatalf("h1 doing after delete = %+v", got)
	}
}

func TestScheduleOrphanTakesOverAndSchedules(t *testing.T) {
	env := testenv.New(t)
	run := env.SeedTask(t, testenv.Task{Hosts: []string{"a"}})
	paused := env.SeedTask(t, testenv.Task{Hosts: []string{"b"}, Action: "pause"})
	noAction := env.SeedTask(t, testenv.Task{Hosts: []string{"c"}})
	if err := env.DB.Exec("DELETE FROM task_action WHERE id = ?", noAction).Error; err != nil {
		t.Fatal(err)
	}

	scheduleOrphan()

	mine, err := models.TasksOfScheduler(testenv.Scheduler)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(mine, []int64{run}) {
		t.Fatalf("tasks owned = %v, want [%d] (paused and action-less stay orphans)", mine, run)
	}
	if got := env.HostStatuses(t, run)["a"]; got != "running" {
		t.Fatalf("taken-over task host status = %q, want running", got)
	}
	if got := env.HostStatuses(t, paused)["b"]; got != "waiting" {
		t.Fatalf("paused task host status = %q, want waiting", got)
	}
	orphans, _ := models.OrphanTaskIds()
	if len(orphans) != 2 {
		t.Fatalf("orphans = %v, want paused+noAction", orphans)
	}
}

func TestScheduleOrphanDoesNotStealOwnedTasks(t *testing.T) {
	env := testenv.New(t)
	id := env.SeedTask(t, testenv.Task{Hosts: []string{"a"}})
	if _, err := models.TakeOverTask(id, "", "other:20090"); err != nil {
		t.Fatal(err)
	}

	scheduleOrphan()

	if ids, _ := models.TasksOfScheduler("other:20090"); len(ids) != 1 {
		t.Fatalf("owner changed: %v", ids)
	}
	if got := env.HostStatuses(t, id)["a"]; got != "waiting" {
		t.Fatalf("host scheduled by non-owner: %q", got)
	}
}

func TestScheduleMineSchedulesAndTimesOut(t *testing.T) {
	env := testenv.New(t)
	id := env.SeedTask(t, testenv.Task{Hosts: []string{"a", "b"}, Batch: 1, Timeout: 5})
	if _, err := models.TakeOverTask(id, "", testenv.Scheduler); err != nil {
		t.Fatal(err)
	}

	scheduleMine() // a starts
	if got := env.HostStatuses(t, id); got["a"] != "running" || got["b"] != "waiting" {
		t.Fatalf("round 1 = %v", got)
	}

	// a never reports: after timeout+3s the owner marks it timeout and moves on.
	if err := env.DB.Exec("UPDATE task_host_doing SET clock = ? WHERE id = ?", time.Now().Unix()-60, id).Error; err != nil {
		t.Fatal(err)
	}
	scheduleMine()
	if got := env.HostStatuses(t, id)["a"]; got != "timeout" {
		t.Fatalf("a = %q, want timeout", got)
	}
}

func TestCleanLongTaskCancelsOldTasks(t *testing.T) {
	env := testenv.New(t)
	old := env.SeedTask(t, testenv.Task{Hosts: []string{"a", "b"}})
	fresh := env.SeedTask(t, testenv.Task{Hosts: []string{"c"}})
	if err := env.DB.Exec("UPDATE task_action SET clock = ? WHERE id = ?", time.Now().Unix()-15*86400, old).Error; err != nil {
		t.Fatal(err)
	}

	cleanLongTask()

	if got := env.Action(t, old); got != "cancel" {
		t.Fatalf("old task action = %q, want cancel", got)
	}
	for h, s := range env.HostStatuses(t, old) {
		if s != "cancelled" {
			t.Fatalf("old task host %s = %q, want cancelled", h, s)
		}
	}
	if got := env.Action(t, fresh); got != "start" {
		t.Fatalf("fresh task action = %q, want start", got)
	}
	if got := env.HostStatuses(t, fresh)["c"]; got != "waiting" {
		t.Fatalf("fresh task host = %q", got)
	}
}

func TestHeartbeatTakesOverDeadScheduler(t *testing.T) {
	env := testenv.New(t)
	id := env.SeedTask(t, testenv.Task{Hosts: []string{"a"}})
	const dead = "dead:20090"
	if _, err := models.TakeOverTask(id, "", dead); err != nil {
		t.Fatal(err)
	}
	if err := env.DB.Create(&models.TaskSchedulerHealth{Scheduler: dead, Clock: time.Now().Unix() - 60}).Error; err != nil {
		t.Fatal(err)
	}

	heartbeat()

	if ids, _ := models.TasksOfScheduler(testenv.Scheduler); !reflect.DeepEqual(ids, []int64{id}) {
		t.Fatalf("tasks of live scheduler = %v, want [%d]", ids, id)
	}
	count := func(scheduler string) int64 {
		var n int64
		env.DB.Model(&models.TaskSchedulerHealth{}).Where("scheduler = ?", scheduler).Count(&n)
		return n
	}
	if count(testenv.Scheduler) != 1 {
		t.Fatalf("own health rows = %d, want 1", count(testenv.Scheduler))
	}
	// The dead scheduler's health row is only dropped once it owns no tasks,
	// i.e. on the heartbeat after the takeover.
	if count(dead) != 1 {
		t.Fatal("dead scheduler row removed in the same round as the takeover")
	}

	heartbeat()
	if count(dead) != 0 {
		t.Fatal("dead scheduler health row not removed once it owns no tasks")
	}
}
