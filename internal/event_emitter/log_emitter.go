package eventemitter

import (
	"context"
	"errors"
	"log/slog"

	"github.com/synadia-io/nex/models"
)

var _ models.EventEmitter = (*LogEmitter)(nil)

type LogEmitter struct {
	ctx    context.Context
	logger *slog.Logger
	level  slog.Leveler
}

func NewLogEmitter(ctx context.Context, logger *slog.Logger, level slog.Leveler) *LogEmitter {
	return &LogEmitter{
		ctx:    ctx,
		logger: logger,
		level:  level,
	}
}

func (e *LogEmitter) EmitEvent(from string, event any) error {
	if e.logger == nil {
		return errors.New("logger is nil/not set")
	}

	e.logger.Log(e.ctx, e.level.Level(), "event emitted", slog.Any("event", event))

	return nil
}
