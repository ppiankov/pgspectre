package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"

	"github.com/ppiankov/pgspectre/internal/scanner"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var (
		repo     string
		format   string
		parallel int
	)

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Scan code repo for SQL table/column references (no database required)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}

			// Use config format as default if flag not explicitly set
			if !cmd.Flags().Changed("format") && cfg.Defaults.Format != "" {
				format = cfg.Defaults.Format
			}

			slog.Debug("scanning repo", "path", repo)
			result, err := scanner.ScanParallel(repo, parallel)
			if err != nil {
				return fmt.Errorf("scan: %w", err)
			}
			slog.Info("scan complete",
				"files", result.FilesScanned,
				"skipped", result.FilesSkipped,
				"tables", len(result.Refs),
				"columns", len(result.ColumnRefs))

			return writeScanResult(cmd.OutOrStdout(), &result, format)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "path to code repository to scan (required)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, or sarif")
	cmd.Flags().IntVar(&parallel, "parallel", 0, "number of scanner goroutines (0=NumCPU, 1=sequential)")

	return cmd
}

func writeScanResult(w io.Writer, result *scanner.ScanResult, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	return writeScanResultText(w, result)
}

func writeScanResultText(w io.Writer, result *scanner.ScanResult) error {
	if len(result.Tables) == 0 {
		_, err := fmt.Fprintln(w, "No table references found.")
		return err
	}

	_, _ = fmt.Fprintf(w, "Tables (%d):\n", len(result.Tables))
	for _, t := range result.Tables {
		_, _ = fmt.Fprintf(w, "  %s\n", t)
	}

	if len(result.Columns) > 0 {
		_, _ = fmt.Fprintf(w, "\nColumns (%d):\n", len(result.Columns))
		for _, c := range result.Columns {
			_, _ = fmt.Fprintf(w, "  %s\n", c)
		}
	}

	_, _ = fmt.Fprintf(w, "\nReferences (%d):\n", len(result.Refs))
	for _, r := range result.Refs {
		loc := fmt.Sprintf("%s:%d", r.File, r.Line)
		_, _ = fmt.Fprintf(w, "  %-30s %-20s [%s] %s\n", loc, r.Table, r.Context, r.Pattern)
	}

	_, err := fmt.Fprintf(w, "\nSummary: %d tables, %d columns, %d references in %d files\n",
		len(result.Tables), len(result.Columns), len(result.Refs), result.FilesScanned)
	return err
}
