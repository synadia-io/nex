package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"testing"

	"disorder.dev/shandler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tochemey/goakt/v2/log"
)

func TestSlogDebug(t *testing.T) {
	t.Run("With Debug log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
		// assert Debug log
		logger.Debug("test debug")
		expected := "test debug"
		actual, err := extractMessage(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		lvl, err := extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.DebugLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.DebugLevel, logger.LogLevel())

		// reset the buffer
		buffer.Reset()
		// assert Debug log
		name := "world"
		logger.Debugf("hello %s", name)
		actual, err = extractMessage(buffer.Bytes())
		require.NoError(t, err)
		expected = "hello world"
		require.Equal(t, expected, actual)

		lvl, err = extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.DebugLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.DebugLevel, logger.LogLevel())
	})
	t.Run("With Info log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
		// assert Debug log
		logger.Debug("test debug")
		require.Empty(t, buffer.String())
	})
	t.Run("With Error log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelError}))
		// assert Debug log
		logger.Debug("test debug")
		require.Empty(t, buffer.String())
	})
}

func TestSlogInfo(t *testing.T) {
	t.Run("With Info log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
		// assert Debug log
		logger.Info("test debug")
		expected := "test debug"
		actual, err := extractMessage(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		lvl, err := extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.InfoLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.InfoLevel, logger.LogLevel())

		// reset the buffer
		buffer.Reset()
		// assert Debug log
		name := "world"
		logger.Infof("hello %s", name)
		actual, err = extractMessage(buffer.Bytes())
		require.NoError(t, err)
		expected = "hello world"
		require.Equal(t, expected, actual)

		lvl, err = extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.InfoLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.InfoLevel, logger.LogLevel())
	})
	t.Run("With Debug log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
		// assert Debug log
		logger.Info("test debug")
		expected := "test debug"
		actual, err := extractMessage(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		lvl, err := extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.InfoLevel.String(), strings.ToLower(lvl))

		// reset the buffer
		buffer.Reset()
		// assert Debug log
		name := "world"
		logger.Infof("hello %s", name)
		actual, err = extractMessage(buffer.Bytes())
		require.NoError(t, err)
		expected = "hello world"
		require.Equal(t, expected, actual)

		lvl, err = extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.InfoLevel.String(), strings.ToLower(lvl))
	})
	t.Run("With Error log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelError}))
		// assert Debug log
		logger.Info("test debug")
		require.Empty(t, buffer.String())
	})
}

func TestSlogWarn(t *testing.T) {
	t.Run("With Warn log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelWarn}))
		// assert Debug log
		logger.Warn("test debug")
		expected := "test debug"
		actual, err := extractMessage(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		lvl, err := extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.WarningLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.WarningLevel, logger.LogLevel())

		// reset the buffer
		buffer.Reset()
		// assert Debug log
		name := "world"
		logger.Warnf("hello %s", name)
		actual, err = extractMessage(buffer.Bytes())
		require.NoError(t, err)
		expected = "hello world"
		require.Equal(t, expected, actual)

		lvl, err = extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.WarningLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.WarningLevel, logger.LogLevel())
	})
	t.Run("With Debug log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
		// assert Debug log
		logger.Warn("test debug")
		expected := "test debug"
		actual, err := extractMessage(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		lvl, err := extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.WarningLevel.String(), strings.ToLower(lvl))

		// reset the buffer
		buffer.Reset()
		// assert Debug log
		name := "world"
		logger.Warnf("hello %s", name)
		actual, err = extractMessage(buffer.Bytes())
		require.NoError(t, err)
		expected = "hello world"
		require.Equal(t, expected, actual)

		lvl, err = extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.WarningLevel.String(), strings.ToLower(lvl))
	})
	t.Run("With Error log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelError}))
		// assert Debug log
		logger.Warn("test debug")
		require.Empty(t, buffer.String())
	})
}

func TestSlogError(t *testing.T) {
	t.Run("With the Error log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
		// assert Debug log
		logger.Error("test debug")
		expected := "test debug"
		actual, err := extractMessage(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		lvl, err := extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.ErrorLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.InfoLevel, logger.LogLevel())

		// reset the buffer
		buffer.Reset()
		// assert Debug log
		name := "world"
		logger.Errorf("hello %s", name)
		actual, err = extractMessage(buffer.Bytes())
		require.NoError(t, err)
		expected = "hello world"
		require.Equal(t, expected, actual)

		lvl, err = extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.ErrorLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.InfoLevel, logger.LogLevel())
	})
	t.Run("With the Debug log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelDebug}))
		// assert Debug log
		logger.Error("test debug")
		expected := "test debug"
		actual, err := extractMessage(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		lvl, err := extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.ErrorLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.DebugLevel, logger.LogLevel())

		// reset the buffer
		buffer.Reset()
		// assert Debug log
		name := "world"
		logger.Errorf("hello %s", name)
		actual, err = extractMessage(buffer.Bytes())
		require.NoError(t, err)
		expected = "hello world"
		require.Equal(t, expected, actual)

		lvl, err = extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.ErrorLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.DebugLevel, logger.LogLevel())
	})
	t.Run("With the Info log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelInfo}))
		// assert Debug log
		logger.Error("test debug")
		expected := "test debug"
		actual, err := extractMessage(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		lvl, err := extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.ErrorLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.InfoLevel, logger.LogLevel())

		// reset the buffer
		buffer.Reset()
		// assert Debug log
		name := "world"
		logger.Errorf("hello %s", name)
		actual, err = extractMessage(buffer.Bytes())
		require.NoError(t, err)
		expected = "hello world"
		require.Equal(t, expected, actual)

		lvl, err = extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.ErrorLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.InfoLevel, logger.LogLevel())
	})
	t.Run("With the Warn log level", func(t *testing.T) {
		// create a bytes buffer that implements an io.Writer
		buffer := new(bytes.Buffer)
		// create an instance of Log
		logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: slog.LevelWarn}))
		// assert Debug log
		logger.Error("test debug")
		expected := "test debug"
		actual, err := extractMessage(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, expected, actual)

		lvl, err := extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.ErrorLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.WarningLevel, logger.LogLevel())

		// reset the buffer
		buffer.Reset()
		// assert Debug log
		name := "world"
		logger.Errorf("hello %s", name)
		actual, err = extractMessage(buffer.Bytes())
		require.NoError(t, err)
		expected = "hello world"
		require.Equal(t, expected, actual)

		lvl, err = extractLevel(buffer.Bytes())
		require.NoError(t, err)
		require.Equal(t, log.ErrorLevel.String(), strings.ToLower(lvl))
		require.Equal(t, log.WarningLevel, logger.LogLevel())
	})
}

func TestSlogPanic(t *testing.T) {
	panicLvl := shandler.LevelFatal + 2
	// create a bytes buffer that implements an io.Writer
	buffer := new(bytes.Buffer)
	// create an instance of Log
	logger := NewSlog(slog.NewJSONHandler(buffer, &slog.HandlerOptions{Level: panicLvl}))
	// assert Debug log
	assert.Panics(t, func() {
		logger.Panic("test debug")
	})
}

func extractMessage(bytes []byte) (string, error) {
	// a map container to decode the JSON structure into
	c := make(map[string]json.RawMessage)

	// unmarshal JSON
	if err := json.Unmarshal(bytes, &c); err != nil {
		return "", err
	}
	for k, v := range c {
		if k == "msg" {
			return strconv.Unquote(string(v))
		}
	}

	return "", nil
}

func extractLevel(bytes []byte) (string, error) {
	// a map container to decode the JSON structure into
	c := make(map[string]json.RawMessage)

	// unmarshal JSON
	if err := json.Unmarshal(bytes, &c); err != nil {
		return "", err
	}
	for k, v := range c {
		if k == "level" {
			return strconv.Unquote(string(v))
		}
	}

	return "", nil
}
