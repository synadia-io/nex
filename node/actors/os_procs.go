package actors

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
)

type OsProcess struct {
	name          string
	executionPath string
	env           map[string]string
	argv          []string
	logger        *slog.Logger
	cmd           *exec.Cmd
}

// Creates a new OS process based in the parameters
// NOTE:: if you want to ensure uniqueness in log eminations coming from this process, make sure you
// use a unique process name
func NewOsProcess(name string, executionPath string, env map[string]string, argv []string, logger *slog.Logger) (*OsProcess, error) {
	return &OsProcess{
		name:          name,
		executionPath: executionPath,
		env:           env,
		argv:          argv,
		logger:        logger,
	}, nil
}

// The Run function is blocking and as such needs to be run concurrently within an actor or using a goroutine.
// It will launch the indicated OS process using the supplied binary
// path, args, and environment variables.  The log capture struct will be used as the stdout/stderr
// targets for the process
func (proc *OsProcess) Run() error {
	cmd := exec.Command(proc.executionPath, proc.argv...)

	for k, v := range proc.env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	cmd.Stdout = logCapture{logger: proc.logger, stderr: false, name: proc.name}
	cmd.Stderr = logCapture{logger: proc.logger, stderr: true, name: proc.name}

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

	if proc.cmd.Process == nil {
		proc.logger.Debug("OS process terminated successfully", slog.Int("pid", proc.cmd.Process.Pid))
	}

	return nil
}

// Sends an interrupt signal to the running process (if it's running)
func (proc *OsProcess) Stop(reason string) error {
	if proc.cmd.Process != nil {
		return proc.cmd.Process.Signal(os.Interrupt)
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
	return proc.cmd.Process != nil
}

// NOTE: if there is an operating system specific thing that needs to be done to the command
// do it here in a os_procs_{os}.go file
func (proc *OsProcess) amendCommand(cmd *exec.Cmd) {

	// NOTE: example of something that might be OS specific is setting SysProcAttr
}

type logCapture struct {
	logger *slog.Logger
	stderr bool
	name   string
}

// Log capture implementation of io.Writer for stdout and stderr
func (cap logCapture) Write(p []byte) (n int, err error) {
	if cap.stderr {
		cap.logger.Error(string(p), slog.String("process_name", cap.name))
	} else {
		cap.logger.Debug(string(p), slog.String("process_name", cap.name))
	}
	return len(p), nil
}
