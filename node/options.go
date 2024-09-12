package node

import (
	"log/slog"
)

func WithLogger(s *slog.Logger) NexOption {
	return func(n *NexNode) {
		n.logger = s
	}
}
