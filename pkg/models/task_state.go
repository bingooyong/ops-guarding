package models

// Task states returned by GET /task/:id/state.
const (
	TaskStateStart  = "start"
	TaskStatePause  = "pause"
	TaskStateCancel = "cancel"
	TaskStateKill   = "kill"
	TaskStateDone   = "done"
	TaskStateFailed = "failed"
)

// ingHostStatuses are host statuses that are not terminal yet.
var ingHostStatuses = []string{"waiting", "running", "killing"}

// unexpectedHostStatuses are terminal host statuses counted as failures
// (same set as UnexpectedHostCount).
var unexpectedHostStatuses = []string{"failed", "timeout", "killfailed"}

// AggregateState derives the task state from the task_action row (if any)
// and the per-status host counts of the task. It is pure so it can be
// unit-tested without a database.
//
//   - any host still waiting/running/killing: the current action
//     (or "start" when the action row is already gone);
//   - all hosts terminal and at least one failed/timeout/killfailed: "failed";
//   - otherwise: "done".
func AggregateState(action string, hasAction bool, counts map[string]int64) string {
	var ing, unexpected int64
	for _, s := range ingHostStatuses {
		ing += counts[s]
	}
	for _, s := range unexpectedHostStatuses {
		unexpected += counts[s]
	}

	if ing > 0 {
		if hasAction && action != "" {
			return action
		}
		return TaskStateStart
	}

	if unexpected > 0 {
		return TaskStateFailed
	}

	return TaskStateDone
}
