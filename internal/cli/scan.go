package cli

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/ppiankov/pgspectre/internal/scanner"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	var (
		repo   string
		format string
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

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Scanning %s...\n", repo)
			result, err := scanner.Scan(repo)
			if err != nil {
				return fmt.Errorf("scan: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Scanned %d files (%d skipped), found %d table references, %d column references\n",
				result.FilesScanned, result.FilesSkipped, len(result.Refs), len(result.ColumnRefs))

			return writeScanResult(cmd.OutOrStdout(), &result, format)
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "path to code repository to scan (required)")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, or sarif")

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
