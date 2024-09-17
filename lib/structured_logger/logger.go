package structured_logger

import (
	"lib"
	"os"

	"github.com/rs/zerolog"
)

type Logger zerolog.Logger
type LogEntry struct {
	*zerolog.Event
}
type LogContext zerolog.Context

func NewLogger(logLevel string) *Logger {
	zerolog.LevelFieldName = "severity"
	zerolog.LevelTraceValue = "DEBUG"
	zerolog.LevelDebugValue = "DEBUG"
	zerolog.LevelInfoValue = "INFO"
	zerolog.LevelWarnValue = "WARNING"
	zerolog.LevelErrorValue = "ERROR"
	zerolog.LevelFatalValue = "CRITICAL"
	zerolog.LevelPanicValue = "ALERT"

	if l, err := zerolog.ParseLevel(logLevel); err == nil {
		zerolog.SetGlobalLevel(l)
	}
	ret := Logger(zerolog.New(os.Stderr).With().Timestamp().Logger())
	return &ret
}

func (l *Logger) Debug() lib.LogEntry {
	return LogEntry{(*zerolog.Logger)(l).Debug()}
}

func (l *Logger) Info() lib.LogEntry {
	return LogEntry{(*zerolog.Logger)(l).Info()}
}

func (l *Logger) Warn() lib.LogEntry {
	return LogEntry{(*zerolog.Logger)(l).Warn()}
}

func (l *Logger) Err() lib.LogEntry {
	return LogEntry{(*zerolog.Logger)(l).Error()}
}

func (l *Logger) Fatal() lib.LogEntry {
	return LogEntry{(*zerolog.Logger)(l).Fatal()}
}

func (l *Logger) With() lib.LogContext {
	return LogContext((*zerolog.Logger)(l).With())
}

func (l LogContext) Caller(skip ...int) lib.LogContext {
	if len(skip) == 0 {
		return LogContext((zerolog.Context)(l).Caller())
	}

	return LogContext((zerolog.Context)(l).CallerWithSkipFrameCount(skip[0]))
}

func (l LogContext) Value(key string, value any) lib.LogContext {
	c := (zerolog.Context)(l)
	switch value.(type) {
	case string:
		return LogContext(c.Str(key, value.(string)))
	case int8, int16, int32, int64, int:
		return LogContext(c.Int(key, lib.Cast[int](value)))
	case uint8, uint16, uint32, uint64, uint:
		return LogContext(c.Uint(key, lib.Cast[uint](value)))
	case float32, float64:
		return LogContext(c.Float64(key, lib.Cast[float64](value)))
	case bool:
		return LogContext(c.Bool(key, value.(bool)))
	default:
		return LogContext(c.Str(key, lib.Cast[string](value)))
	}
}

func (l LogContext) Logger() lib.Logger {
	ret := Logger((zerolog.Context)(l).Logger())
	return &ret
}

func (l LogEntry) Caller(skip ...int) lib.LogEntry {
	return LogEntry{l.Event.Caller(skip...)}
}

func (l LogEntry) Value(key string, value any) lib.LogEntry {
	switch value.(type) {
	case string:
		return LogEntry{l.Str(key, value.(string))}
	case int8, int16, int32, int64, int:
		return LogEntry{l.Int(key, lib.Cast[int](value))}
	case uint8, uint16, uint32, uint64, uint:
		return LogEntry{l.Uint(key, lib.Cast[uint](value))}
	case float32, float64:
		return LogEntry{l.Float64(key, lib.Cast[float64](value))}
	case bool:
		return LogEntry{l.Bool(key, value.(bool))}
	default:
		return LogEntry{l.Str(key, lib.Cast[string](value))}
	}
}

func (l LogEntry) Msg(msg string) {
	l.Event.Msg(msg)
}

func (l LogEntry) Error(err error, skip ...int) {
	l.Event.Caller(skip...).Err(err).Msg(err.Error())
}
