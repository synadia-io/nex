package nexagent

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

func RunWorkload(name string, totalBytes int32, tempFile *os.File, runtimeEnvironment map[string]string) error {
	// This has to be backgrounded because the workload could be a long-running process/service
	go func() {
		cmd := exec.Command(tempFile.Name())
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
			PublishWorkloadStopped(name, true, msg)
			return
		}
		PublishWorkloadStarted(name, totalBytes)
		err = cmd.Wait()
		msg := ""
		if err != nil {
			msg = fmt.Sprintf("Workload stopped unexpectedly: %s", err)
		} else {
			msg = "OK"
		}
		PublishWorkloadStopped(name, err != nil, msg)
	}()

	return nil

}

type logEmitter struct {
	stderr bool
	name   string
}

func (l *logEmitter) Write(bytes []byte) (int, error) {
	var lvl agentapi.LogLevel
	if l.stderr {
		lvl = agentapi.LogLevel_LEVEL_ERROR
	} else {
		lvl = agentapi.LogLevel_LEVEL_DEBUG
	}
	entry := &agentapi.LogEntry{
		Source: l.name,
		Level:  lvl,
		Text:   string(bytes),
	}
	agentLogs <- entry

	return len(bytes), nil
}
