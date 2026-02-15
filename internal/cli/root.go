package cli

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/ppiankov/pgspectre/internal/analyzer"
	"github.com/ppiankov/pgspectre/internal/baseline"
	"github.com/ppiankov/pgspectre/internal/config"
	"github.com/ppiankov/pgspectre/internal/logging"
	"github.com/ppiankov/pgspectre/internal/postgres"
	"github.com/ppiankov/pgspectre/internal/reporter"
	"github.com/ppiankov/pgspectre/internal/scanner"
	"github.com/ppiankov/pgspectre/internal/suppress"
	"github.com/spf13/cobra"
)

var (
	dbURL   string
	verbose bool
	cfg     config.Config
)

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:          "pgspectre",
		Short:        "PostgreSQL schema and usage auditor",
		Long:         "Scans codebases for table/column references, compares with live Postgres schema and statistics, detects drift.",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			logging.Init(verbose, cmd.ErrOrStderr())

			cwd, err := os.Getwd()
			if err != nil {
				cwd = "."
			}
			cfg, err = config.Load(cwd)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}
			slog.Debug("config loaded", "path", cwd)

			// Apply config defaults if flags not explicitly set
			if dbURL == "" {
				if envURL := os.Getenv("PGSPECTRE_DB_URL"); envURL != "" {
					dbURL = envURL
				} else if cfg.DBURL != "" {
					dbURL = cfg.DBURL
				}
			}
			return nil
		},
	}

	root.PersistentFlags().StringVar(&dbURL, "db-url", "", "PostgreSQL connection URL (or set PGSPECTRE_DB_URL)")
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable debug-level logging")

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newAuditCmd())
	root.AddCommand(newCheckCmd())
	root.AddCommand(newScanCmd())

	return root
}

func newVersionCmd(version string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("pgspectre " + version)
		},
	}
}

func newAuditCmd() *cobra.Command {
	var (
		format         string
		failOn         string
		baselinePath   string
		updateBaseline string
	)

	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Cluster-only analysis: unused tables, indexes, missing stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL == "" {
				return fmt.Errorf("--db-url is required")
			}

			// Use config format as default if flag not explicitly set
			if !cmd.Flags().Changed("format") && cfg.Defaults.Format != "" {
				format = cfg.Defaults.Format
			}

			timeout := cfg.TimeoutDuration()
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			inspector, err := postgres.NewInspector(ctx, postgres.Config{URL: dbURL})
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			defer inspector.Close()

			ver, err := inspector.ServerVersion(ctx)
			if err != nil {
				return fmt.Errorf("server version: %w", err)
			}
			slog.Info("connected", "version", ver)

			snap, err := inspector.Inspect(ctx)
			if err != nil {
				return fmt.Errorf("inspect: %w", err)
			}
			slog.Info("inspected", "tables", len(snap.Tables), "indexes", len(snap.Indexes), "constraints", len(snap.Constraints))

			findings := analyzer.Audit(snap, auditOptsFromConfig())

			// Save baseline before filtering
			if updateBaseline != "" {
				if err := baseline.Save(updateBaseline, findings); err != nil {
					return fmt.Errorf("save baseline: %w", err)
				}
				slog.Info("baseline saved", "path", updateBaseline, "findings", len(findings))
			}

			// Apply baseline + suppress filters
			findings, totalSuppressed, err := filterFindings(findings, baselinePath)
			if err != nil {
				return err
			}

			report := reporter.NewReport("audit", findings)
			if totalSuppressed > 0 {
				slog.Info("findings filtered", "total", report.Summary.Total+totalSuppressed, "suppressed", totalSuppressed)
			}

			if err := reporter.Write(cmd.OutOrStdout(), &report, reporter.Format(format)); err != nil {
				return fmt.Errorf("write report: %w", err)
			}

			if failOn != "" && shouldFailOn(findings, failOn) {
				os.Exit(2)
			}

			code := analyzer.ExitCode(report.MaxSeverity)
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, or sarif")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "exit 2 if findings match (comma-separated types or severity: high,medium)")
	cmd.Flags().StringVar(&baselinePath, "baseline", "", "path to baseline file (suppress known findings)")
	cmd.Flags().StringVar(&updateBaseline, "update-baseline", "", "save current findings as new baseline")

	return cmd
}

func newCheckCmd() *cobra.Command {
	var (
		repo           string
		format         string
		failOn         string
		failOnMissing  bool
		baselinePath   string
		updateBaseline string
		parallel       int
	)

	cmd := &cobra.Command{
		Use:   "check",
		Short: "Code repo + cluster: missing tables, schema drift, unindexed queries",
		RunE: func(cmd *cobra.Command, args []string) error {
			if dbURL == "" {
				return fmt.Errorf("--db-url is required")
			}
			if repo == "" {
				return fmt.Errorf("--repo is required")
			}

			// Use config format as default if flag not explicitly set
			if !cmd.Flags().Changed("format") && cfg.Defaults.Format != "" {
				format = cfg.Defaults.Format
			}

			// Scan code repo (no timeout needed — local filesystem)
			slog.Debug("scanning repo", "path", repo)
			scan, err := scanner.ScanParallel(repo, parallel)
			if err != nil {
				return fmt.Errorf("scan repo: %w", err)
			}
			slog.Info("scan complete", "refs", len(scan.Refs), "files", scan.FilesScanned)

			// Connect to PostgreSQL
			timeout := cfg.TimeoutDuration()
			ctx, cancel := context.WithTimeout(cmd.Context(), timeout)
			defer cancel()

			inspector, err := postgres.NewInspector(ctx, postgres.Config{URL: dbURL})
			if err != nil {
				return fmt.Errorf("connect: %w", err)
			}
			defer inspector.Close()

			ver, err := inspector.ServerVersion(ctx)
			if err != nil {
				return fmt.Errorf("server version: %w", err)
			}
			slog.Info("connected", "version", ver)

			snap, err := inspector.Inspect(ctx)
			if err != nil {
				return fmt.Errorf("inspect: %w", err)
			}
			slog.Info("inspected", "tables", len(snap.Tables), "indexes", len(snap.Indexes), "constraints", len(snap.Constraints))

			// Run diff analysis
			findings := analyzer.Diff(&scan, snap, auditOptsFromConfig())

			// Save baseline before filtering
			if updateBaseline != "" {
				if err := baseline.Save(updateBaseline, findings); err != nil {
					return fmt.Errorf("save baseline: %w", err)
				}
				slog.Info("baseline saved", "path", updateBaseline, "findings", len(findings))
			}

			// Apply baseline + suppress filters
			findings, totalSuppressed, err := filterFindings(findings, baselinePath)
			if err != nil {
				return err
			}

			report := reporter.NewReport("check", findings)
			if totalSuppressed > 0 {
				slog.Info("findings filtered", "total", report.Summary.Total+totalSuppressed, "suppressed", totalSuppressed)
			}

			if err := reporter.Write(cmd.OutOrStdout(), &report, reporter.Format(format)); err != nil {
				return fmt.Errorf("write report: %w", err)
			}

			// --fail-on-missing is an alias for --fail-on MISSING_TABLE
			effectiveFailOn := failOn
			if failOnMissing && effectiveFailOn == "" {
				effectiveFailOn = "MISSING_TABLE"
			}
			if effectiveFailOn != "" && shouldFailOn(findings, effectiveFailOn) {
				os.Exit(2)
			}

			code := analyzer.ExitCode(report.MaxSeverity)
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "path to code repository to scan")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, or sarif")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "exit 2 if findings match (comma-separated types or severity: high,medium)")
	cmd.Flags().BoolVar(&failOnMissing, "fail-on-missing", false, "exit 2 if any MISSING_TABLE found (deprecated, use --fail-on)")
	cmd.Flags().StringVar(&baselinePath, "baseline", "", "path to baseline file (suppress known findings)")
	cmd.Flags().StringVar(&updateBaseline, "update-baseline", "", "save current findings as new baseline")
	cmd.Flags().IntVar(&parallel, "parallel", 0, "number of scanner goroutines (0=NumCPU, 1=sequential)")

	return cmd
}

// filterFindings applies baseline and suppression rules to findings.
func filterFindings(findings []analyzer.Finding, baselinePath string) ([]analyzer.Finding, int, error) {
	totalSuppressed := 0

	// Apply baseline filtering
	if baselinePath != "" {
		bl, err := baseline.Load(baselinePath)
		if err != nil {
			return nil, 0, fmt.Errorf("load baseline: %w", err)
		}
		var n int
		findings, n = bl.Filter(findings)
		totalSuppressed += n
	}

	// Apply suppress rules (.pgspectre-ignore.yml + config exclude.findings)
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	rules, err := suppress.LoadRules(cwd)
	if err != nil {
		return nil, 0, fmt.Errorf("load suppress rules: %w", err)
	}
	rules.WithConfigFindings(cfg.Exclude.Findings)

	var n int
	findings, n = rules.Filter(findings)
	totalSuppressed += n

	return findings, totalSuppressed, nil
}

// shouldFailOn returns true if any finding matches the fail-on criteria.
// Criteria can be finding types (MISSING_TABLE) or severity levels (high, medium).
func shouldFailOn(findings []analyzer.Finding, failOn string) bool {
	parts := strings.Split(failOn, ",")
	types := make(map[string]bool)
	severities := make(map[string]bool)

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		lower := strings.ToLower(p)
		switch lower {
		case "high", "medium", "low", "info":
			severities[lower] = true
		default:
			types[strings.ToUpper(p)] = true
		}
	}

	for _, f := range findings {
		if types[string(f.Type)] {
			return true
		}
		if severities[string(f.Severity)] {
			return true
		}
	}
	return false
}

func auditOptsFromConfig() analyzer.AuditOptions {
	return analyzer.AuditOptions{
		VacuumDays:     cfg.Thresholds.VacuumDays,
		BloatMinBytes:  cfg.Thresholds.BloatMinBytes,
		ExcludeTables:  cfg.Exclude.Tables,
		ExcludeSchemas: cfg.Exclude.Schemas,
	}
}

// Execute runs the root command.
func Execute(version string) error {
	return newRootCmd(version).Execute()
}
