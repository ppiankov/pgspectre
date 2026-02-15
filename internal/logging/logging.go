package logging

import (
	"io"
	"log/slog"
	"os"
)

// Init configures the default slog logger.
// verbose=true sets LevelDebug, otherwise LevelWarn (silent unless problems).
// output defaults to os.Stderr if nil.
func Init(verbose bool, output io.Writer) {
	if output == nil {
		output = os.Stderr
	}

	level := slog.LevelWarn
	if verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(output, &slog.HandlerOptions{
		Level: level,
	})

	slog.SetDefault(slog.New(handler))
}
