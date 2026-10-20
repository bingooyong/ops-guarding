package models

import "testing"

func TestAggregateState(t *testing.T) {
	cases := []struct {
		name      string
		action    string
		hasAction bool
		counts    map[string]int64
		want      string
	}{
		{"no hosts no action", "", false, nil, TaskStateDone},
		{"running with start", "start", true, map[string]int64{"running": 1, "success": 2}, TaskStateStart},
		{"waiting with pause", "pause", true, map[string]int64{"waiting": 3, "failed": 2}, TaskStatePause},
		{"killing with kill", "kill", true, map[string]int64{"killing": 1}, TaskStateKill},
		{"ing without action row", "", false, map[string]int64{"waiting": 1}, TaskStateStart},
		{"all success, action not yet cleaned", "start", true, map[string]int64{"success": 5}, TaskStateDone},
		{"all success, action cleaned", "", false, map[string]int64{"success": 5}, TaskStateDone},
		{"one failed", "start", true, map[string]int64{"success": 4, "failed": 1}, TaskStateFailed},
		{"timeout", "", false, map[string]int64{"timeout": 1}, TaskStateFailed},
		{"killfailed", "kill", true, map[string]int64{"killfailed": 1, "killed": 1}, TaskStateFailed},
		{"cancelled and killed only", "cancel", true, map[string]int64{"cancelled": 2, "killed": 1}, TaskStateDone},
		{"paused but finished with failures", "pause", true, map[string]int64{"failed": 3, "cancelled": 2}, TaskStateFailed},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := AggregateState(c.action, c.hasAction, c.counts); got != c.want {
				t.Fatalf("AggregateState(%q,%v,%v) = %q, want %q", c.action, c.hasAction, c.counts, got, c.want)
			}
		})
	}
}
