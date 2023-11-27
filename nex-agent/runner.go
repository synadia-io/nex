package nexagent

import (
	"fmt"
	"os/exec"
	"strings"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

func RunWorkload(vmId string, name string, totalBytes int32, tempFileName string, runtimeEnvironment map[string]string) error {
	// This has to be backgrounded because the workload could be a long-running process/service
	go func() {
		cmd := exec.Command(tempFileName)
		cmd.Stdout = &logEmitter{stderr: false, name: name}
		cmd.Stderr = &logEmitter{stderr: true, name: name}

		envVars := make([]string, len(runtimeEnvironment))
		for k, v := range runtimeEnvironment {
			item := fmt.Sprintf("%s=%s", strings.ToUpper(k), v)
			envVars = append(envVars, item)
		}
		cmd.Env = envVars
		err := cmd.Start()

		if err != nil {
			msg := fmt.Sprintf("Failed to start workload: %s", err)
			LogError(msg)
			PublishWorkloadStopped(vmId, name, true, msg)
			return
		}
		PublishWorkloadStarted(vmId, name, totalBytes)

		// if cmd.Wait has unblocked, it means the workload has stopped
		err = cmd.Wait()
		msg := ""
		if err != nil {
			msg = fmt.Sprintf("Workload stopped unexpectedly: %s", err)
		} else {
			msg = "OK"
		}
		PublishWorkloadStopped(vmId, name, err != nil, msg)

	}()

	return nil

}

// This implements the writer interface that allows us to capture a workload's
// stdout and stderr so that we can then publish those logs to the host node
type logEmitter struct {
	stderr bool
	name   string
}

func (l *logEmitter) Write(bytes []byte) (int, error) {
	var lvl agentapi.LogLevel
	if l.stderr {
		lvl = agentapi.LogLevelError
	} else {
		lvl = agentapi.LogLevelInfo
	}
	entry := &agentapi.LogEntry{
		Source: l.name,
		Level:  lvl,
		Text:   string(bytes),
	}
	agentLogs <- entry

	return len(bytes), nil
}
