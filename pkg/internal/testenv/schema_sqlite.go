package testenv

import "fmt"

// sqliteTables mirrors sql/ibex.sql (columns and key constraints that matter
// to the models). TestSchemaMatchesIbexSQL guards against drift.
var sqliteTables = map[string]string{
	"task_meta": "CREATE TABLE task_meta (" +
		"id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL DEFAULT ''," +
		"account TEXT NOT NULL, batch INTEGER NOT NULL DEFAULT 0, tolerance INTEGER NOT NULL DEFAULT 0," +
		"timeout INTEGER NOT NULL DEFAULT 0, pause TEXT NOT NULL DEFAULT '', script TEXT NOT NULL," +
		"args TEXT NOT NULL DEFAULT '', stdin TEXT NOT NULL DEFAULT '', creator TEXT NOT NULL DEFAULT ''," +
		"created TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)",
	"task_action": "CREATE TABLE task_action (" +
		"id INTEGER PRIMARY KEY, action TEXT NOT NULL, clock INTEGER NOT NULL DEFAULT 0)",
	"task_scheduler": "CREATE TABLE task_scheduler (" +
		"id INTEGER NOT NULL, scheduler TEXT NOT NULL DEFAULT '')",
	"task_scheduler_health": "CREATE TABLE task_scheduler_health (" +
		"scheduler TEXT NOT NULL UNIQUE, clock INTEGER NOT NULL)",
	"task_host_doing": "CREATE TABLE task_host_doing (" +
		"id INTEGER NOT NULL, host TEXT NOT NULL, clock INTEGER NOT NULL DEFAULT 0, action TEXT NOT NULL)",
}

const taskHostShards = 100

func taskHostDDL(n int) string {
	return fmt.Sprintf("CREATE TABLE task_host_%d ("+
		"ii INTEGER PRIMARY KEY AUTOINCREMENT, id INTEGER NOT NULL, host TEXT NOT NULL,"+
		"status TEXT NOT NULL, stdout TEXT, stderr TEXT, UNIQUE (id, host))", n)
}

// SQLiteSchema returns the DDL statements creating all tables.
func SQLiteSchema() []string {
	order := []string{"task_meta", "task_action", "task_scheduler", "task_scheduler_health", "task_host_doing"}
	out := make([]string, 0, len(order)+taskHostShards)
	for _, name := range order {
		out = append(out, sqliteTables[name])
	}
	for i := 0; i < taskHostShards; i++ {
		out = append(out, taskHostDDL(i))
	}
	return out
}
