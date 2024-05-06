//go:build linux

package processmanager

import (
	"log/slog"
	"os"
)

func (s *SpawningProcessManager) kill(proc *spawnedProcess) error {
	if proc.cmd.Process != nil {
		err := proc.cmd.Process.Signal(os.Interrupt)
		if err != nil {
			s.log.Error("Failed to interrupt agent process",
				slog.String("agent_id", proc.ID),
				slog.Int("pid", proc.cmd.Process.Pid),
				slog.String("err", err.Error()),
			)

			return proc.cmd.Process.Signal(os.Kill)
		}
	}

	return nil
}
