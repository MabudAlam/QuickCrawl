package utils

import (
	"os"
	"strings"

	"github.com/gookit/slog"
)

var Log *Logger

var DefaultLevel = "info"

func init() {
	InitLogger()
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "trace":
		return slog.TraceLevel
	case "debug":
		return slog.DebugLevel
	case "warn":
		return slog.WarnLevel
	case "error":
		return slog.ErrorLevel
	default:
		return slog.InfoLevel
	}
}

func InitLogger() {
	slog.Configure(func(l *slog.SugaredLogger) {
		f, ok := l.Formatter.(*slog.TextFormatter)
		if ok {
			f.EnableColor = true
		}

		level := parseLevel(DefaultLevel)
		if env := os.Getenv("LOG_LEVEL"); env != "" {
			level = parseLevel(env)
		}
		l.Level = level
	})

	Log = &Logger{}
}

type Logger struct{}

func (l *Logger) Trace(msg string, args ...any) {
	if len(args) == 0 {
		slog.Trace(msg)
		return
	}
	slog.Trace(msg, toData(args))
}

func (l *Logger) Debug(msg string, args ...any) {
	if len(args) == 0 {
		slog.Debug(msg)
		return
	}
	slog.Debug(msg, toData(args))
}

func (l *Logger) Info(msg string, args ...any) {
	if len(args) == 0 {
		slog.Info(msg)
		return
	}
	slog.Info(msg, toData(args))
}

func (l *Logger) Notice(msg string, args ...any) {
	if len(args) == 0 {
		slog.Notice(msg)
		return
	}
	slog.Notice(msg, toData(args))
}

func (l *Logger) Warn(msg string, args ...any) {
	if len(args) == 0 {
		slog.Warn(msg)
		return
	}
	slog.Warn(msg, toData(args))
}

func (l *Logger) Error(msg string, args ...any) {
	if len(args) == 0 {
		slog.Error(msg)
		return
	}
	slog.Error(msg, toData(args))
}

func toData(args []any) slog.M {
	data := make(slog.M, len(args)/2)
	for i := 0; i < len(args)-1; i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		data[key] = args[i+1]
	}
	return data
}