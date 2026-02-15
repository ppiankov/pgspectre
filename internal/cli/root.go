package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/ppiankov/pgspectre/internal/analyzer"
	"github.com/ppiankov/pgspectre/internal/baseline"
	"github.com/ppiankov/pgspectre/internal/config"
	"github.com/ppiankov/pgspectre/internal/postgres"
	"github.com/ppiankov/pgspectre/internal/reporter"
	"github.com/ppiankov/pgspectre/internal/scanner"
	"github.com/ppiankov/pgspectre/internal/suppress"
	"github.com/spf13/cobra"
)

var (
	dbURL string
	cfg   config.Config
)

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:          "pgspectre",
		Short:        "PostgreSQL schema and usage auditor",
		Long:         "Scans codebases for table/column references, compares with live Postgres schema and statistics, detects drift.",
		SilenceUsage: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				cwd = "."
			}
			cfg, err = config.Load(cwd)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

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
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connected to PostgreSQL %s\n", ver)

			snap, err := inspector.Inspect(ctx)
			if err != nil {
				return fmt.Errorf("inspect: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Inspected %d tables, %d indexes, %d constraints\n",
				len(snap.Tables), len(snap.Indexes), len(snap.Constraints))

			findings := analyzer.Audit(snap, auditOptsFromConfig())

			// Save baseline before filtering
			if updateBaseline != "" {
				if err := baseline.Save(updateBaseline, findings); err != nil {
					return fmt.Errorf("save baseline: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Baseline saved to %s (%d findings)\n", updateBaseline, len(findings))
			}

			// Apply baseline + suppress filters
			findings, totalSuppressed, err := filterFindings(findings, baselinePath)
			if err != nil {
				return err
			}

			report := reporter.NewReport("audit", findings)
			if totalSuppressed > 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%d findings (%d suppressed)\n",
					report.Summary.Total+totalSuppressed, totalSuppressed)
			}

			if err := reporter.Write(cmd.OutOrStdout(), &report, reporter.Format(format)); err != nil {
				return fmt.Errorf("write report: %w", err)
			}

			code := analyzer.ExitCode(report.MaxSeverity)
			if code != 0 {
				os.Exit(code)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, or sarif")
	cmd.Flags().StringVar(&baselinePath, "baseline", "", "path to baseline file (suppress known findings)")
	cmd.Flags().StringVar(&updateBaseline, "update-baseline", "", "save current findings as new baseline")

	return cmd
}

func newCheckCmd() *cobra.Command {
	var (
		repo           string
		format         string
		failOnMissing  bool
		baselinePath   string
		updateBaseline string
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
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Scanning repo %s...\n", repo)
			scan, err := scanner.Scan(repo)
			if err != nil {
				return fmt.Errorf("scan repo: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Found %d table references in %d files\n",
				len(scan.Refs), scan.FilesScanned)

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
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Connected to PostgreSQL %s\n", ver)

			snap, err := inspector.Inspect(ctx)
			if err != nil {
				return fmt.Errorf("inspect: %w", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Inspected %d tables, %d indexes, %d constraints\n",
				len(snap.Tables), len(snap.Indexes), len(snap.Constraints))

			// Run diff analysis
			findings := analyzer.Diff(&scan, snap, auditOptsFromConfig())

			// Save baseline before filtering
			if updateBaseline != "" {
				if err := baseline.Save(updateBaseline, findings); err != nil {
					return fmt.Errorf("save baseline: %w", err)
				}
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "Baseline saved to %s (%d findings)\n", updateBaseline, len(findings))
			}

			// Apply baseline + suppress filters
			findings, totalSuppressed, err := filterFindings(findings, baselinePath)
			if err != nil {
				return err
			}

			report := reporter.NewReport("check", findings)
			if totalSuppressed > 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%d findings (%d suppressed)\n",
					report.Summary.Total+totalSuppressed, totalSuppressed)
			}

			if err := reporter.Write(cmd.OutOrStdout(), &report, reporter.Format(format)); err != nil {
				return fmt.Errorf("write report: %w", err)
			}

			if failOnMissing {
				for _, f := range findings {
					if f.Type == analyzer.FindingMissingTable {
						os.Exit(2)
					}
				}
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
	cmd.Flags().BoolVar(&failOnMissing, "fail-on-missing", false, "exit 2 if any MISSING_TABLE found")
	cmd.Flags().StringVar(&baselinePath, "baseline", "", "path to baseline file (suppress known findings)")
	cmd.Flags().StringVar(&updateBaseline, "update-baseline", "", "save current findings as new baseline")

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
