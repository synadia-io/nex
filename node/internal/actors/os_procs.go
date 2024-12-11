package actors

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"

	"github.com/nats-io/nats.go"
)

type OsProcess struct {
	ctx           context.Context
	name          string
	executionPath string
	env           map[string]string
	argv          []string
	logger        *slog.Logger
	stdout        logCapture
	stderr        logCapture
	cmd           *exec.Cmd
}

// Creates a new OS process based in the parameters
// NOTE:: if you want to ensure uniqueness in log eminations coming from this process, make sure you
// use a unique process name
func NewOsProcess(ctx context.Context, name string, executionPath string, env map[string]string, argv []string, logger *slog.Logger, stdout, stderr logCapture) (*OsProcess, error) {
	return &OsProcess{
		ctx:           ctx,
		name:          name,
		executionPath: executionPath,
		env:           env,
		argv:          argv,
		logger:        logger,
		stdout:        stdout,
		stderr:        stderr,
	}, nil
}

// The Run function is blocking and as such needs to be run concurrently within an actor or using a goroutine.
// It will launch the indicated OS process using the supplied binary
// path, args, and environment variables.  The log capture struct will be used as the stdout/stderr
// targets for the process
func (proc *OsProcess) Run() error {
	cmd := exec.CommandContext(context.TODO(), proc.executionPath, proc.argv...)

	for k, v := range proc.env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	cmd.Stdout = proc.stdout
	cmd.Stderr = proc.stderr
	cmd.SysProcAttr = sysProcAttr()

	proc.amendCommand(cmd)

	proc.cmd = cmd

	if err := cmd.Start(); err != nil {
		proc.logger.Error("Failed to start OS process", slog.Any("error", err))
		return err
	}
	if proc.cmd.Process != nil {
		proc.logger.Debug("OS process started", slog.Int("pid", proc.cmd.Process.Pid))
	}

	if err := cmd.Wait(); err != nil {
		if proc.cmd.Process != nil {
			proc.logger.Debug("OS process terminated with error", slog.Any("error", err), slog.Int("pid", proc.cmd.Process.Pid))
		}
		return err
	}

	proc.logger.Debug("OS process terminated successfully", slog.Int("pid", proc.cmd.Process.Pid))
	proc.cmd = nil

	return nil
}

// Sends an interrupt signal to the running process (if it's running)
func (proc *OsProcess) Interrupt(reason string) error {
	if proc.cmd.Process != nil {
		return stopProcess(proc.cmd.Process)
	}
	return nil
}

// Kills the running process (if it's running). Note that this sends
// a signal and then exits, which means the actual OS process might stop
// "some time" later
func (proc *OsProcess) Kill() error {
	if proc.cmd.Process != nil {
		return proc.cmd.Process.Kill()
	}
	return nil
}

func (proc *OsProcess) IsRunning() bool {
	return proc.cmd != nil
}

// NOTE: if there is an operating system specific thing that needs to be done to the command
// do it here in a os_procs_{os}.go file
func (proc *OsProcess) amendCommand(cmd *exec.Cmd) {

	// NOTE: example of something that might be OS specific is setting SysProcAttr
}

type logCapture struct {
	logger    *slog.Logger
	nc        *nats.Conn
	namespace string
	stderr    bool
	name      string
}

// Log capture implementation of io.Writer for stdout and stderr
func (cap logCapture) Write(p []byte) (n int, err error) {
	if !cap.stderr {
		_ = cap.nc.Publish(fmt.Sprintf("$NEX.logs.%s.%s.stdout", cap.namespace, cap.name), p)
		cap.logger.Debug(string(p), slog.String("process_name", cap.name))
	} else {
		_ = cap.nc.Publish(fmt.Sprintf("$NEX.logs.%s.%s.stderr", cap.namespace, cap.name), p)
		cap.logger.Error(string(p), slog.String("process_name", cap.name))
	}
	return len(p), nil
}
