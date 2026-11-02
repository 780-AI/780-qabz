// Package zap adapts go.uber.org/zap to the key-value logger that app uses.
package zap

import (
	"fmt"

	uzap "go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger is a zap-backed logger. Arguments after msg are key-value pairs.
type Logger struct{ s *uzap.SugaredLogger }

// New builds a JSON logger at the given level (debug, info, warn, error).
func New(level, service string) (*Logger, error) {
	var lvl zapcore.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		return nil, fmt.Errorf("log level %q: %w", level, err)
	}
	cfg := uzap.NewProductionConfig()
	cfg.Level = uzap.NewAtomicLevelAt(lvl)
	cfg.DisableStacktrace = true
	l, err := cfg.Build(uzap.AddCallerSkip(1))
	if err != nil {
		return nil, err
	}
	return &Logger{s: l.Sugar().With("service", service)}, nil
}

func (l *Logger) Debug(msg string, kv ...any) { l.s.Debugw(msg, kv...) }
func (l *Logger) Info(msg string, kv ...any)  { l.s.Infow(msg, kv...) }
func (l *Logger) Warn(msg string, kv ...any)  { l.s.Warnw(msg, kv...) }
func (l *Logger) Error(msg string, kv ...any) { l.s.Errorw(msg, kv...) }

// Sync flushes buffered entries.
func (l *Logger) Sync() error { return l.s.Sync() }
