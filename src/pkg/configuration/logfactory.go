package configuration

import (
	"io"
	"log/slog"
	"os"
)

const (
	TEXT_HANDLER int8       = 0
	JSON_HANDLER int8       = 1
	LVL_DEBUG    slog.Level = slog.LevelDebug
	LVL_INFO     slog.Level = slog.LevelInfo
	LVL_WARN     slog.Level = slog.LevelWarn
	LVL_ERROR    slog.Level = slog.LevelError
)

// logFactory makes and returns logs
func logFactory(w io.Writer, opts *slog.HandlerOptions, handlerType int8) *slog.Logger {
	var handler slog.Handler
	switch handlerType {
	case TEXT_HANDLER:
		handler = slog.NewTextHandler(w, opts)
	case JSON_HANDLER:
		handler = slog.NewJSONHandler(w, opts)
	}

	log := slog.New(handler)
	return log
}

func makeStdJSONLogger(opts *slog.HandlerOptions) *slog.Logger {
	return logFactory(os.Stdout, opts, JSON_HANDLER)
}

func StdJsonLoggerLevel(level slog.Level) *slog.Logger {
	if level == slog.LevelDebug {
		return StdJsonLoggerDebug()
	}
	return makeStdJSONLogger(&slog.HandlerOptions{Level: level})
}

func StdJsonLoggerDebug(with ...any) *slog.Logger {
	opts := slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}

	return withAttrs(makeStdJSONLogger(&opts), with...)
}

func StdJsonLoggerInfo(with ...any) *slog.Logger {
	return withAttrs(StdJsonLoggerLevel(slog.LevelInfo), with...)
}

func StdJsonLoggerWarn(with ...any) *slog.Logger {
	return withAttrs(StdJsonLoggerLevel(slog.LevelWarn), with...)
}

func StdJsonLoggerError(with ...any) *slog.Logger {
	return withAttrs(StdJsonLoggerLevel(slog.LevelError), with...)
}

func makeStdTextLogger(opts *slog.HandlerOptions) *slog.Logger {
	return logFactory(os.Stdout, opts, TEXT_HANDLER)
}

func StdTextLoggerLevel(level slog.Level) *slog.Logger {
	if level == slog.LevelDebug {
		return StdTextLoggerDebug()
	}
	return makeStdTextLogger(&slog.HandlerOptions{Level: level})
}

func StdTextLoggerDebug(with ...any) *slog.Logger {
	opts := slog.HandlerOptions{
		AddSource: true,
		Level:     slog.LevelDebug,
	}

	return withAttrs(makeStdTextLogger(&opts), with...)
}

func StdTextLoggerInfo(with ...any) *slog.Logger {
	return withAttrs(StdTextLoggerLevel(slog.LevelInfo), with...)
}

func StdTextLoggerWarn(with ...any) *slog.Logger {
	return withAttrs(StdTextLoggerLevel(slog.LevelWarn), with...)
}

func StdTextLoggerError(with ...any) *slog.Logger {
	return withAttrs(StdTextLoggerLevel(slog.LevelError), with...)
}

func withAttrs(logger *slog.Logger, a ...any) *slog.Logger {
	// Loop through two at a time. If an odd number of items are added to a, the last
	// will be dropped.
	for i := 0; i < (len(a))/2; i++ {
		j := i * 2
		logger = logger.With(a[j], a[j+1])
	}

	return logger
}
