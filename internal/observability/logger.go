package observability

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.uber.org/fx/fxevent"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"seam/internal/config"
	"seam/internal/supervise"
)

// Rotation limits. A session writes little unless --verbose is on; this
// keeps a few verbose sessions without growing without bound.
const (
	maxSizeMB  = 10
	maxBackups = 3
	maxAgeDays = 30
)

// Logger is the process logger together with the file it rotates. Its owner
// is the session in cmd/seam, which closes it after the fx graph has stopped.
type Logger struct {
	zap  *zap.Logger
	file *lumberjack.Logger
}

// NewLogger opens the log file as JSON.
func NewLogger(cfg config.Config) (*Logger, error) {
	// lumberjack creates a missing directory itself, but only on the first
	// write, where a failure ends up as a zap internal error on stderr. Doing
	// it here turns an unwritable location into an ordinary start-up error.
	if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	file := &lumberjack.Logger{
		Filename:   cfg.LogFile,
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
		Compress:   true,
	}

	level := zapcore.InfoLevel
	if cfg.Verbose {
		level = zapcore.DebugLevel
	}
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(file),
		level,
	)
	// zap reports its own write errors on stderr by default, which would
	// garble the TUI; they go to the void instead.
	opts := []zap.Option{
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.DPanicLevel),
		zap.ErrorOutput(zapcore.AddSync(io.Discard)),
	}
	return &Logger{zap: zap.New(core, opts...), file: file}, nil
}

// Zap returns the logger to write to.
func (l *Logger) Zap() *zap.Logger { return l.zap }

// Close flushes the log and releases the file. Nothing may log afterwards:
// lumberjack would silently reopen the file.
func (l *Logger) Close() error {
	return errors.Join(l.zap.Sync(), l.file.Close())
}

// NewFxLogger routes fx's own events into the log file.
func NewFxLogger(l *Logger) *fxevent.ZapLogger {
	events := &fxevent.ZapLogger{Logger: l.zap}
	// fx narrates every provide and invoke; at info level that would be most
	// of the file. Lifecycle errors keep their own level.
	events.UseLogLevel(zapcore.DebugLevel)
	return events
}

// PanicFunc logs panics caught by supervise groups.
func PanicFunc(l *zap.Logger) supervise.PanicFunc {
	return func(owner string, f supervise.Failure) {
		l.Error("panic in background task",
			zap.String("owner", owner),
			zap.String("task", f.Task),
			zap.Any("value", f.Value),
			zap.ByteString("stack", f.Stack),
		)
	}
}
