package timer

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"ops-guarding/pkg/agentd/client"
	"ops-guarding/pkg/agentd/config"
	"ops-guarding/pkg/appframe/file"
	"ops-guarding/pkg/appframe/runner"
	"ops-guarding/pkg/appframe/sys"
	"os/exec"
	"os/user"
	"path"
	"strings"
	"sync"
	"time"
)

var errNoProcess = errors.New("no process to kill")

// killGrace is how long killProcess waits for runProcess to finish after a
// successful kill signal before it records the result itself (a detached
// grandchild holding the output pipe can keep Wait blocked).
var killGrace = 5 * time.Second

// lockedWriter lets exec's copy goroutines append to a Task buffer under the
// same lock the readers (GetStdout/GetStderr) use.
type lockedWriter struct {
	t   *Task
	buf *bytes.Buffer
}

func (w lockedWriter) Write(p []byte) (int, error) {
	w.t.Lock()
	defer w.t.Unlock()
	return w.buf.Write(p)
}

type Task struct {
	sync.Mutex

	Id     int64
	Clock  int64
	Action string
	Status string

	alive         bool
	killRequested bool          // set by killProcess before signalling; guarded by the lock
	exited        chan struct{} // closed by runProcess after Wait + persist; nil if no process
	Cmd           *exec.Cmd
	Stdout        bytes.Buffer
	Stderr        bytes.Buffer
	Stdin         *bytes.Reader

	Args     string
	Account  string
	StdinStr string
}

func (t *Task) SetStatus(status string) {
	t.Lock()
	t.Status = status
	t.Unlock()
}

func (t *Task) GetStatus() string {
	t.Lock()
	s := t.Status
	t.Unlock()
	return s
}

func (t *Task) GetAlive() bool {
	t.Lock()
	pa := t.alive
	t.Unlock()
	return pa
}

func (t *Task) SetAlive(pa bool) {
	t.Lock()
	t.alive = pa
	t.Unlock()
}

func (t *Task) GetStdout() string {
	t.Lock()
	out := t.Stdout.String()
	t.Unlock()
	return out
}

func (t *Task) GetStderr() string {
	t.Lock()
	out := t.Stderr.String()
	t.Unlock()
	return out
}

func (t *Task) setKillRequested(v bool) {
	t.Lock()
	t.killRequested = v
	t.Unlock()
}

func (t *Task) isKillRequested() bool {
	t.Lock()
	v := t.killRequested
	t.Unlock()
	return v
}

// failStart records a permanent start failure as a reportable `failed` result.
func (t *Task) failStart(err error) {
	t.Lock()
	t.Stderr.WriteString(fmt.Sprintf("agentd: cannot start task: %v\n", err))
	t.Unlock()
	t.SetStatus("failed")
	persistResult(t)
}

func (t *Task) ResetBuff() {
	t.Lock()
	t.Stdout.Reset()
	t.Stderr.Reset()
	t.Unlock()
}

func (t *Task) doneBefore() bool {
	doneFlag := path.Join(config.C.MetaDir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))
	return file.IsExist(doneFlag)
}

func (t *Task) loadResult() {
	metadir := config.C.MetaDir

	doneFlag := path.Join(metadir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))
	stdoutFile := path.Join(metadir, fmt.Sprint(t.Id), "stdout")
	stderrFile := path.Join(metadir, fmt.Sprint(t.Id), "stderr")

	var err error

	t.Status, err = file.ReadStringTrim(doneFlag)
	if err != nil {
		log.Printf("E: read file %s fail %v", doneFlag, err)
	}
	stdout, err := file.ReadString(stdoutFile)
	if err != nil {
		log.Printf("E: read file %s fail %v", stdoutFile, err)
	}
	stderr, err := file.ReadString(stderrFile)
	if err != nil {
		log.Printf("E: read file %s fail %v", stderrFile, err)
	}

	t.Stdout = *bytes.NewBufferString(stdout)
	t.Stderr = *bytes.NewBufferString(stderr)
}

func (t *Task) prepare() error {
	if t.Account != "" {
		// already prepared
		return nil
	}

	IdDir := path.Join(config.C.MetaDir, fmt.Sprint(t.Id))
	err := file.EnsureDir(IdDir)
	if err != nil {
		log.Printf("E: mkdir -p %s fail: %v", IdDir, err)
		return err
	}

	writeFlag := path.Join(IdDir, ".write")
	if file.IsExist(writeFlag) {
		// 从磁盘读取
		argsFile := path.Join(IdDir, "args")
		args, err := file.ReadStringTrim(argsFile)
		if err != nil {
			log.Printf("E: read %s fail %v", argsFile, err)
			return err
		}

		accountFile := path.Join(IdDir, "account")
		account, err := file.ReadStringTrim(accountFile)
		if err != nil {
			log.Printf("E: read %s fail %v", accountFile, err)
			return err
		}

		stdinFile := path.Join(IdDir, "stdin")
		stdin, err := file.ReadStringTrim(stdinFile)
		if err != nil {
			log.Printf("E: read %s fail %v", stdinFile, err)
			return err
		}

		t.Args = args
		t.Account = account
		t.StdinStr = stdin

	} else {
		// 从远端读取，再写入磁盘
		script, args, account, stdin, err := client.Meta(t.Id)
		if err != nil {
			log.Println("E: query task meta fail:", err)
			return err
		}

		scriptFile := path.Join(IdDir, "script")
		_, err = file.WriteString(scriptFile, script)
		if err != nil {
			log.Printf("E: write script to %s fail: %v", scriptFile, err)
			return err
		}

		out, err := sys.CmdOutTrim("chmod", "+x", scriptFile)
		if err != nil {
			log.Printf("E: chmod +x %s fail %v. output: %s", scriptFile, err, out)
			return err
		}

		argsFile := path.Join(IdDir, "args")
		_, err = file.WriteString(argsFile, args)
		if err != nil {
			log.Printf("E: write args to %s fail: %v", argsFile, err)
			return err
		}

		accountFile := path.Join(IdDir, "account")
		_, err = file.WriteString(accountFile, account)
		if err != nil {
			log.Printf("E: write account to %s fail: %v", accountFile, err)
			return err
		}

		stdinFile := path.Join(IdDir, "stdin")
		_, err = file.WriteString(stdinFile, stdin)
		if err != nil {
			log.Printf("E: write tags to %s fail: %v", stdinFile, err)
			return err
		}

		_, err = file.WriteString(writeFlag, "")
		if err != nil {
			log.Printf("E: create %s flag file fail: %v", writeFlag, err)
			return err
		}

		t.Args = args
		t.Account = account
		t.StdinStr = stdin
	}

	t.Stdin = bytes.NewReader([]byte(t.StdinStr))

	return nil
}

func (t *Task) start() {
	if t.GetAlive() {
		return
	}

	err := t.prepare()
	if err != nil {
		return
	}

	// A redo (new clock) starts from empty output and a fresh stdin.
	t.ResetBuff()
	t.Stdin = bytes.NewReader([]byte(t.StdinStr))
	t.setKillRequested(false)

	args := t.Args
	if args != "" {
		args = strings.Replace(args, ",,", "' '", -1)
		args = "'" + args + "'"
	}

	scriptFile := path.Join(config.C.MetaDir, fmt.Sprint(t.Id), "script")
	if !path.IsAbs(scriptFile) {
		scriptFile = path.Join(runner.Cwd, scriptFile)
	}

	sh := fmt.Sprintf("%s %s", scriptFile, args)
	var cmd *exec.Cmd

	loginUser, err := user.Current()
	if err != nil {
		log.Println("E: cannot get current login user:", err)
		t.failStart(err)
		return
	}

	if loginUser.Username == "root" {
		// current login user is root
		if t.Account == "root" {
			cmd = exec.Command("sh", "-c", sh)
			cmd.Dir = loginUser.HomeDir
		} else {
			cmd = exec.Command("su", "-c", sh, "-", t.Account)
		}
	} else {
		// current login user not root
		cmd = exec.Command("sh", "-c", sh)
		cmd.Dir = loginUser.HomeDir
	}

	cmd.Stdout = lockedWriter{t, &t.Stdout}
	cmd.Stderr = lockedWriter{t, &t.Stderr}
	cmd.Stdin = t.Stdin

	t.Cmd = cmd

	err = CmdStart(cmd)
	if err != nil {
		log.Printf("E: cannot start cmd of task[%d]: %v", t.Id, err)
		t.failStart(err)
		return
	}

	// Mark alive before the goroutine runs so a second start() cannot slip in.
	exited := make(chan struct{})
	t.Lock()
	t.exited = exited
	t.alive = true
	t.Unlock()

	go runProcess(t, exited)
}

func (t *Task) kill() {
	go killProcess(t)
}

func runProcess(t *Task, exited chan struct{}) {
	defer close(exited) // runs last: alive is already false when killProcess resumes
	defer t.SetAlive(false)

	// Wait also waits for the output copy goroutines, so the buffers are
	// complete once it returns.
	err := t.Cmd.Wait()
	if t.isKillRequested() {
		t.SetStatus("killed")
		log.Printf("D: process of task[%d] killed", t.Id)
	} else if err != nil {
		if strings.Contains(err.Error(), "signal: killed") {
			t.SetStatus("killed")
			log.Printf("D: process of task[%d] killed", t.Id)
		} else if strings.Contains(err.Error(), "signal: terminated") {
			// kill children process manually
			t.SetStatus("killed")
			log.Printf("D: process of task[%d] terminated", t.Id)
		} else {
			t.SetStatus("failed")
			log.Printf("D: process of task[%d] return error: %v", t.Id, err)
		}
	} else {
		t.SetStatus("success")
		log.Printf("D: process of task[%d] done", t.Id)
	}

	persistResult(t)
}

func persistResult(t *Task) {
	metadir := config.C.MetaDir

	stdout := path.Join(metadir, fmt.Sprint(t.Id), "stdout")
	stderr := path.Join(metadir, fmt.Sprint(t.Id), "stderr")
	doneFlag := path.Join(metadir, fmt.Sprint(t.Id), fmt.Sprintf("%d.done", t.Clock))

	file.WriteString(stdout, t.GetStdout())
	file.WriteString(stderr, t.GetStderr())
	file.WriteString(doneFlag, t.GetStatus())
}

func killProcess(t *Task) {
	log.Printf("D: begin kill process of task[%d]", t.Id)

	cmd := t.Cmd
	if cmd == nil || cmd.Process == nil {
		// Nothing is running: a finished result replayed from disk after a restart,
		// or a start that failed. Record the requested outcome so it converges.
		t.SetAlive(true)
		defer t.SetAlive(false)
		log.Printf("D: task[%d] has no process to kill", t.Id)
		t.SetStatus("killed")
		persistResult(t)
		return
	}

	t.Lock()
	t.killRequested = true
	exited := t.exited
	t.Unlock()

	if err := CmdKill(cmd); err != nil {
		t.SetStatus("killfailed")
		log.Printf("D: kill process of task[%d] fail: %v", t.Id, err)
		persistResult(t)
		return
	}

	if exited == nil {
		return
	}

	// runProcess owns the final state: it sets `killed` and persists the
	// complete output once Wait returns.
	select {
	case <-exited:
		log.Printf("D: process of task[%d] killed", t.Id)
	case <-time.After(killGrace):
		log.Printf("W: task[%d] still not reaped %s after kill, recording result", t.Id, killGrace)
		t.SetStatus("killed")
		persistResult(t)
	}
}
