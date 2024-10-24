package logger

import (
	"context"
	"fmt"
	"io"
	golog "log"
	"log/slog"
	"os"

	"disorder.dev/shandler"
	"github.com/tochemey/goakt/v2/log"
)

// DefaultSlogLogger represents the default Log to use
// This Log wraps slog under the hood
var DefaultSlogLogger = NewSlog(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
var DiscardSlogLogger = NewSlog(slog.NewJSONHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))

// Log implements Logger interface with the underlying zap as
// the underlying logging library
type SlogLog struct {
	logger *slog.Logger
	level  slog.Level
}

// enforce compilation and linter error
var _ log.Logger = &SlogLog{}

// New creates an instance of SlogLog
func NewSlog(handler slog.Handler) *SlogLog {
	levelPanic := shandler.LevelFatal + 2
	levels := []slog.Level{levelPanic, shandler.LevelFatal, slog.LevelError, slog.LevelWarn, slog.LevelInfo, slog.LevelDebug, shandler.LevelTrace}
	l := levelPanic

	for i, level := range levels {
		if !handler.Enabled(context.TODO(), level) {
			l = levels[i-1]
			break
		}
	}

	logger := slog.New(handler)
	return &SlogLog{
		logger: logger,
		level:  l,
	}
}

// Debug starts a message with debug level
func (l *SlogLog) Debug(v ...any) {
	l.logger.Debug(fmt.Sprint(v...))
}

// Debugf starts a message with debug level
func (l *SlogLog) Debugf(format string, v ...any) {
	l.logger.Debug(fmt.Sprintf(format, v...))
}

// Panic starts a new message with panic level. The panic() function
// is called which stops the ordinary flow of a goroutine.
func (l *SlogLog) Panic(v ...any) {
	l.logger.Log(context.TODO(), shandler.LevelFatal+2, fmt.Sprint(v...))
	panic(v)
}

// Panicf starts a new message with panic level. The panic() function
// is called which stops the ordinary flow of a goroutine.
func (l *SlogLog) Panicf(format string, v ...any) {
	l.logger.Log(context.TODO(), shandler.LevelFatal+2, fmt.Sprintf(format, v...))
	panic(v)
}

// Fatal starts a new message with fatal level. The os.Exit(1) function
// is called which terminates the program immediately.
func (l *SlogLog) Fatal(v ...any) {
	l.logger.Log(context.TODO(), shandler.LevelFatal, fmt.Sprint(v...))
}

// Fatalf starts a new message with fatal level. The os.Exit(1) function
// is called which terminates the program immediately.
func (l *SlogLog) Fatalf(format string, v ...any) {
	l.logger.Log(context.TODO(), shandler.LevelFatal, fmt.Sprintf(format, v...))
}

// Error starts a new message with error level.
func (l *SlogLog) Error(v ...any) {
	l.logger.Error(fmt.Sprint(v...))
}

// Errorf starts a new message with error level.
func (l *SlogLog) Errorf(format string, v ...any) {
	l.logger.Error(fmt.Sprintf(format, v...))
}

// Warn starts a new message with warn level
func (l *SlogLog) Warn(v ...any) {
	l.logger.Warn(fmt.Sprint(v...))
}

// Warnf starts a new message with warn level
func (l *SlogLog) Warnf(format string, v ...any) {
	l.logger.Warn(fmt.Sprintf(format, v...))
}

// Info starts a message with info level
func (l *SlogLog) Info(v ...any) {
	l.logger.Info(fmt.Sprint(v...))
}

// Infof starts a message with info level
func (l *SlogLog) Infof(format string, v ...any) {
	l.logger.Info(fmt.Sprintf(format, v...))
}

// LogLevel returns the log level that is used
func (l *SlogLog) LogLevel() log.Level {
	switch l.level {
	case shandler.LevelFatal:
		return log.FatalLevel
	case slog.LevelError:
		return log.ErrorLevel
	case slog.LevelInfo:
		return log.InfoLevel
	case slog.LevelDebug:
		return log.DebugLevel
	case slog.LevelWarn:
		return log.WarningLevel
	default:
		return log.InvalidLevel
	}
}

// LogOutput returns the log output that is set
func (l *SlogLog) LogOutput() []io.Writer {
	return nil
}

// StdLogger returns the standard logger associated to the logger
func (l *SlogLog) StdLogger() *golog.Logger {
	return slog.NewLogLogger(l.logger.Handler(), l.level)
}
