//go:build linux

package journald_logger

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"lib"
	"net"
	"runtime"
	"strings"
	"sync"
)

type Logger struct {
	socket  *lib.Socket
	mapBuf  *sync.Pool
	ch      chan map[string]any
	onError func(error, []byte) bool
	closed  chan struct{}
}

type LogEntry struct {
	logger *Logger
	dict   map[string]any
}

type LogContext LogEntry
type LogContextLogger LogContext

func (sl *Logger) Start() {
	defer close(sl.closed)
	buf := &bytes.Buffer{}

	var m map[string]any
	var ok bool
	for {
		select {
		case m, ok = <-sl.ch:
			if !ok {
				return
			}
		}

		buf.Reset()

		// https://.io/JOURNAL_NATIVE_PROTOCOL/
		for k, v := range m {
			buf.WriteString(k)

			if s, ok := v.(string); ok {
				buf.WriteString("\n")
				binary.Write(buf, binary.LittleEndian, int64(len(s)))
				buf.WriteString(s)
			} else {
				buf.WriteString("=")
				buf.WriteString(lib.Cast[string](v))
			}

			buf.WriteString("\n")
		}

		clear(m)
		sl.mapBuf.Put(m)

		msg := buf.Bytes()
		_, err := sl.socket.EnsureWrite(msg)

		if err != nil && sl.onError != nil && sl.onError(err, msg) {
			return
		}
	}
}

// NewLogger creates a logger that goes into journald
// onError is an optional callback if journald socket is not available. If it is unset or returns false, error will be ignored.
func NewLogger(onError func(error, []byte) bool) (ret *Logger, err error) {
	conn, err := net.Dial("unixgram", "/run/systemd/journal/socket")
	if err != nil {
		return
	}

	s, err := lib.NewSocket(conn)
	if err != nil {
		conn.Close()
		return
	}

	ret = &Logger{
		socket: s,
		mapBuf: &sync.Pool{
			New: func() any {
				return map[string]any{}
			},
		},
		ch:      make(chan map[string]any, 100),
		closed:  make(chan struct{}),
		onError: onError,
	}

	return
}

func (sl *Logger) Close(captureUnhandledError bool) {
	if captureUnhandledError {
		if e := recover(); e != nil {
			if err, ok := e.(error); ok {
				sl.Err().Caller(1).Msg(err.Error())
			} else {
				sl.Fatal().Msg(fmt.Sprint(e))
			}
		}
	}

	close(sl.ch)
	<-sl.closed

	sl.socket.Close()
}

func (sl *Logger) createLogger(logLevel int, dict map[string]any) lib.LogEntry {
	nd := sl.mapBuf.Get().(map[string]any)

	for k, v := range dict {
		nd[k] = v
	}

	nd["PRIORITY"] = logLevel

	return &LogEntry{
		logger: sl,
		dict:   nd,
	}
}

func (sl *Logger) Debug() lib.LogEntry {
	return sl.createLogger(7, nil)
}

func (sl *Logger) Info() lib.LogEntry {
	return sl.createLogger(6, nil)
}

func (sl *Logger) Warn() lib.LogEntry {
	return sl.createLogger(4, nil)
}

func (sl *Logger) Err() lib.LogEntry {
	return sl.createLogger(3, nil)
}

func (sl *Logger) Fatal() lib.LogEntry {
	return sl.createLogger(2, nil)
}

func (sl *Logger) With() lib.LogContext {
	return LogContext{
		logger: sl,
		dict:   sl.mapBuf.Get().(map[string]any),
	}
}

// func (slc *LogContext) Unit(service string) lib.LogContext {
// 	slc.dict["UNIT"] = service + ".service"
// 	return slc
// }

func (slc LogContext) Caller(skip ...int) lib.LogContext {
	s := 0
	if len(skip) > 0 {
		s = skip[0]
	}

	_, file, line, ok := runtime.Caller(s + 1)
	if ok {
		slc.dict["CODE_FILE"] = file
		slc.dict["CODE_LINE"] = line
	}

	return slc
}

func (slc LogContext) Value(key string, value any) lib.LogContext {
	slc.dict[strings.ToUpper(key)] = value
	return slc
}

func (slc LogContext) Logger() lib.Logger {
	return LogContextLogger{
		logger: slc.logger,
		dict:   slc.dict,
	}
}

func (slcl LogContextLogger) Debug() lib.LogEntry {
	return slcl.logger.createLogger(7, slcl.dict)
}

func (slcl LogContextLogger) Info() lib.LogEntry {
	return slcl.logger.createLogger(6, slcl.dict)
}

func (slcl LogContextLogger) Warn() lib.LogEntry {
	return slcl.logger.createLogger(4, slcl.dict)
}

func (slcl LogContextLogger) Err() lib.LogEntry {
	return slcl.logger.createLogger(3, slcl.dict)
}

func (slcl LogContextLogger) Fatal() lib.LogEntry {
	return slcl.logger.createLogger(2, slcl.dict)
}

func (slcl LogContextLogger) With() lib.LogContext {
	nd := slcl.logger.mapBuf.Get().(map[string]any)

	for k, v := range slcl.dict {
		nd[k] = v
	}

	return &LogContext{
		logger: slcl.logger,
		dict:   nd,
	}
}

func (sle *LogEntry) Caller(skip ...int) lib.LogEntry {
	s := 0
	if len(skip) > 0 {
		s = skip[0]
	}

	_, file, line, ok := runtime.Caller(s + 1)
	if ok {
		sle.dict["CODE_FILE"] = file
		sle.dict["CODE_LINE"] = line
	}

	return sle
}

func (sle *LogEntry) Value(key string, value any) lib.LogEntry {
	sle.dict[strings.ToUpper(key)] = value
	return sle
}

func (sle *LogEntry) Error(err error, skip ...int) {
	if len(skip) > 0 {
		sle.Caller(skip[0] + 1)
	}

	sle.dict["MESSAGE"] = err.Error()

	sle.logger.ch <- sle.dict
}

func (sle *LogEntry) Msg(msg string) {
	sle.dict["MESSAGE"] = msg

	sle.logger.ch <- sle.dict
}
