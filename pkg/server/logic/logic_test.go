package logic

import (
	"reflect"
	"testing"
	"time"

	"ops-guarding/pkg/appframe/cache"
	"ops-guarding/pkg/internal/testenv"
	"ops-guarding/pkg/models"
)

func status(t *testing.T, env *testenv.Env, id int64) map[string]string {
	t.Helper()
	return env.HostStatuses(t, id)
}

func countStatus(m map[string]string, s string) int {
	n := 0
	for _, v := range m {
		if v == s {
			n++
		}
	}
	return n
}

func TestScheduleTaskBatch(t *testing.T) {
	cases := []struct {
		name        string
		hosts       []string
		batch       int
		wantRunning int
	}{
		{"batch limits", []string{"a", "b", "c"}, 2, 2},
		{"batch larger than hosts", []string{"a", "b"}, 5, 2},
		{"batch 0 means all", []string{"a", "b", "c", "d"}, 0, 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testenv.New(t)
			id := env.SeedTask(t, testenv.Task{Hosts: c.hosts, Batch: c.batch})

			ScheduleTask(id)
			if got := countStatus(status(t, env, id), "running"); got != c.wantRunning {
				t.Fatalf("running = %d, want %d (%v)", got, c.wantRunning, status(t, env, id))
			}
			if got := len(env.Doing(t, id)); got != c.wantRunning {
				t.Fatalf("doing rows = %d, want %d", got, c.wantRunning)
			}

			// A second round must not start more hosts while the batch is busy.
			ScheduleTask(id)
			if got := countStatus(status(t, env, id), "running"); got != c.wantRunning {
				t.Fatalf("running after 2nd round = %d, want %d", got, c.wantRunning)
			}
		})
	}
}

func TestScheduleTaskRefillsAfterHostFinishes(t *testing.T) {
	env := testenv.New(t)
	id := env.SeedTask(t, testenv.Task{Hosts: []string{"a", "b", "c"}, Batch: 1, Tolerance: 5})

	ScheduleTask(id)
	if got := status(t, env, id); got["a"] != "running" || got["b"] != "waiting" {
		t.Fatalf("round 1 = %v", got)
	}

	doing, _ := models.DoingHostList("id=?", id)
	if err := models.MarkDoneStatus(id, doing[0].Clock, "a", "success", "", ""); err != nil {
		t.Fatal(err)
	}
	ScheduleTask(id)
	if got := status(t, env, id); got["a"] != "success" || got["b"] != "running" || got["c"] != "waiting" {
		t.Fatalf("round 2 = %v", got)
	}
}

func TestScheduleTaskToleranceExceededPauses(t *testing.T) {
	cases := []struct {
		name       string
		tolerance  int
		failStatus string
		wantAction string
		wantBRun   bool
	}{
		{"within tolerance continues", 1, "failed", "start", true},
		{"exceeded tolerance pauses", 0, "failed", "pause", false},
		{"timeout counts as unexpected", 0, "timeout", "pause", false},
		{"killfailed counts as unexpected", 0, "killfailed", "pause", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := testenv.New(t)
			id := env.SeedTask(t, testenv.Task{Hosts: []string{"a", "b"}, Batch: 1, Tolerance: c.tolerance})
			ScheduleTask(id)

			doing, _ := models.DoingHostList("id=?", id)
			if err := models.MarkDoneStatus(id, doing[0].Clock, "a", c.failStatus, "", ""); err != nil {
				t.Fatal(err)
			}
			ScheduleTask(id)

			if got := env.Action(t, id); got != c.wantAction {
				t.Fatalf("action = %q, want %q", got, c.wantAction)
			}
			if running := status(t, env, id)["b"] == "running"; running != c.wantBRun {
				t.Fatalf("b running = %v, want %v (%v)", running, c.wantBRun, status(t, env, id))
			}
		})
	}
}

func TestScheduleTaskPauseHostStopsBatch(t *testing.T) {
	env := testenv.New(t)
	id := env.SeedTask(t, testenv.Task{Hosts: []string{"a", "b", "c"}, Pause: "b"})

	ScheduleTask(id)

	// Hosts up to and including the pause host run, then the task pauses.
	want := map[string]string{"a": "running", "b": "running", "c": "waiting"}
	if got := status(t, env, id); !reflect.DeepEqual(got, want) {
		t.Fatalf("statuses = %v, want %v", got, want)
	}
	if got := env.Action(t, id); got != "pause" {
		t.Fatalf("action = %q, want pause", got)
	}
}

func TestScheduleTaskNonStartActionsDoNothing(t *testing.T) {
	for _, action := range []string{"pause", "cancel", "kill", "bogus"} {
		t.Run(action, func(t *testing.T) {
			env := testenv.New(t)
			id := env.SeedTask(t, testenv.Task{Hosts: []string{"a", "b"}, Action: action})

			ScheduleTask(id)

			if got := countStatus(status(t, env, id), "waiting"); got != 2 {
				t.Fatalf("waiting = %d, want 2 (%v)", got, status(t, env, id))
			}
			if len(env.Doing(t, id)) != 0 {
				t.Fatalf("doing rows created for action %q", action)
			}
		})
	}
}

func TestScheduleTaskMissingPieces(t *testing.T) {
	t.Run("no action row", func(t *testing.T) {
		env := testenv.New(t)
		id := env.SeedTask(t, testenv.Task{Hosts: []string{"a"}})
		if err := env.DB.Exec("DELETE FROM task_action WHERE id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		ScheduleTask(id) // must not panic or start anything
		if got := status(t, env, id)["a"]; got != "waiting" {
			t.Fatalf("status = %q, want waiting", got)
		}
	})

	t.Run("meta lost", func(t *testing.T) {
		env := testenv.New(t)
		id := env.SeedTask(t, testenv.Task{Hosts: []string{"a"}})
		if err := env.DB.Exec("DELETE FROM task_meta WHERE id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		ScheduleTask(id) // must not panic
		if got := status(t, env, id)["a"]; got != "waiting" {
			t.Fatalf("status = %q, want waiting", got)
		}
	})

	t.Run("unknown task", func(t *testing.T) {
		testenv.New(t)
		ScheduleTask(987654) // no rows at all: no panic
	})
}

func TestScheduleTaskCleansDoneTask(t *testing.T) {
	t.Run("all hosts terminal", func(t *testing.T) {
		env := testenv.New(t)
		id := env.SeedTask(t, testenv.Task{Hosts: []string{"a"}})
		if _, err := models.TakeOverTask(id, "", testenv.Scheduler); err != nil {
			t.Fatal(err)
		}
		ScheduleTask(id)
		doing, _ := models.DoingHostList("id=?", id)
		if err := models.MarkDoneStatus(id, doing[0].Clock, "a", "success", "", ""); err != nil {
			t.Fatal(err)
		}

		ScheduleTask(id)

		if got := env.Action(t, id); got != "" {
			t.Fatalf("task_action = %q, want removed", got)
		}
		if ids, _ := models.TasksOfScheduler(testenv.Scheduler); len(ids) != 0 {
			t.Fatalf("task_scheduler rows left: %v", ids)
		}
	})

	t.Run("running host keeps task", func(t *testing.T) {
		env := testenv.New(t)
		id := env.SeedTask(t, testenv.Task{Hosts: []string{"a", "b"}, Batch: 0})
		ScheduleTask(id) // both running, no waiting left
		ScheduleTask(id) // waiting==0 -> cleanDoneTask must see ing hosts and keep it
		if got := env.Action(t, id); got != "start" {
			t.Fatalf("task_action = %q, want start (still running)", got)
		}
	})
}

func TestCheckTimeout(t *testing.T) {
	setup := func(t *testing.T, ageSeconds int64) (*testenv.Env, int64) {
		env := testenv.New(t)
		id := env.SeedTask(t, testenv.Task{Hosts: []string{"a"}, Timeout: 5})
		ScheduleTask(id)
		if err := env.DB.Exec("UPDATE task_host_doing SET clock = ? WHERE id = ?", time.Now().Unix()-ageSeconds, id).Error; err != nil {
			t.Fatal(err)
		}
		return env, id
	}

	t.Run("within timeout+3s grace", func(t *testing.T) {
		env, id := setup(t, 6) // timeout 5 (+3 grace) = 8 > 6
		CheckTimeout(id)
		if got := status(t, env, id)["a"]; got != "running" {
			t.Fatalf("status = %q, want running", got)
		}
	})

	t.Run("beyond timeout+3s", func(t *testing.T) {
		env, id := setup(t, 30)
		CheckTimeout(id)
		if got := status(t, env, id)["a"]; got != "timeout" {
			t.Fatalf("status = %q, want timeout", got)
		}
		if len(env.Doing(t, id)) != 0 {
			t.Fatal("doing row not removed on timeout")
		}
	})

	t.Run("no doing hosts", func(t *testing.T) {
		env := testenv.New(t)
		id := env.SeedTask(t, testenv.Task{})
		CheckTimeout(id) // nothing to do
		if got := status(t, env, id)["h1"]; got != "waiting" {
			t.Fatalf("status = %q", got)
		}
	})

	t.Run("meta lost", func(t *testing.T) {
		env, id := setup(t, 30)
		if err := env.DB.Exec("DELETE FROM task_meta WHERE id = ?", id).Error; err != nil {
			t.Fatal(err)
		}
		if err := cache.Instance.Flush(); err != nil { // meta is cached after the first lookup
			t.Fatal(err)
		}
		CheckTimeout(id) // must not panic nor change status
		if got := status(t, env, id)["a"]; got != "running" {
			t.Fatalf("status = %q, want running", got)
		}
	})
}
