package reporter

import (
	"io"
	"os"

	"github.com/ppiankov/pgspectre/internal/analyzer"
)

// ANSI escape codes for severity colors.
const (
	colorReset  = "\033[0m"
	colorRed    = "\033[31m"
	colorYellow = "\033[33m"
	colorCyan   = "\033[36m"
	colorGray   = "\033[37m"
	colorBold   = "\033[1m"
)

var severityColor = map[analyzer.Severity]string{
	analyzer.SeverityHigh:   colorRed,
	analyzer.SeverityMedium: colorYellow,
	analyzer.SeverityLow:    colorCyan,
	analyzer.SeverityInfo:   colorGray,
}

// isTTY returns true if the writer is a terminal.
func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
