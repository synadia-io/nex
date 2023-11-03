package nexagent

import (
	"fmt"
	"os"
	"os/exec"

	agentapi "github.com/ConnectEverything/nex/agent-api"
)

func RunWorkload(name string, totalBytes int32, tempFile *os.File) error {
	// This has to be backgrounded because the workload could be a long-running process/service
	go func() {
		cmd := exec.Command(tempFile.Name())
		cmd.Stdout = &logEmitter{stderr: false}
		cmd.Stderr = &logEmitter{stderr: true}
		err := cmd.Start()

		if err != nil {
			// TODO: handle error
			fmt.Println(err)
			return
		}
		PublishWorkloadStarted(name, totalBytes)
		err = cmd.Wait()
		msg := ""
		if err != nil {
			msg = fmt.Sprintf("%v", err)
		} else {
			msg = "OK"
		}
		PublishWorkloadStopped(name, err != nil, msg)
	}()

	return nil

}

type logEmitter struct {
	stderr bool
}

func (l *logEmitter) Write(bytes []byte) (int, error) {
	var lvl agentapi.LogLevel
	if l.stderr {
		lvl = agentapi.LogLevel_LEVEL_ERROR
	} else {
		lvl = agentapi.LogLevel_LEVEL_DEBUG
	}
	entry := &agentapi.LogEntry{
		Source: "workload",
		Level:  lvl,
		Text:   string(bytes),
	}
	agentLogs <- entry

	return len(bytes), nil
}
