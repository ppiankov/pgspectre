package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"runtime"
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

// ExitError carries a non-zero exit code without calling os.Exit directly.
// This allows tests to inspect exit codes without terminating the process.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("exit code %d", e.Code)
}

// BuildInfo holds version and build metadata.
type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"goVersion"`
}

var (
	dbURL        string
	verbose      bool
	cfg          config.Config
	buildVersion string
)

func newRootCmd(info BuildInfo) *cobra.Command {
	buildVersion = info.Version
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
			if !config.Exists(cwd) {
				slog.Debug("no .pgspectre.yml found, using defaults", "path", cwd)
			} else {
				slog.Debug("config loaded", "path", cwd)
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
	root.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable debug-level logging")

	root.AddCommand(newVersionCmd(info))
	root.AddCommand(newAuditCmd())
	root.AddCommand(newCheckCmd())
	root.AddCommand(newScanCmd())

	return root
}

func newVersionCmd(info BuildInfo) *cobra.Command {
	var jsonOutput bool

	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print the version",
		Run: func(cmd *cobra.Command, args []string) {
			if jsonOutput {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(info)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "pgspectre %s (commit: %s, built: %s, go: %s)\n",
					info.Version, info.Commit, info.Date, info.GoVersion)
			}
		},
	}

	cmd.Flags().BoolVar(&jsonOutput, "json", false, "output version as JSON")

	return cmd
}

func newAuditCmd() *cobra.Command {
	var (
		format         string
		failOn         string
		baselinePath   string
		updateBaseline string
		minSeverity    string
		typeFilter     string
		schemaFlag     string
		noColor        bool
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

			schemas := resolveSchemaFlag(schemaFlag)
			snap = postgres.FilterSnapshot(snap, schemas)
			slog.Info("inspected", "tables", len(snap.Tables), "indexes", len(snap.Indexes), "constraints", len(snap.Constraints), "schemas", schemas)

			if len(snap.Tables) == 0 {
				schemaHint := "public"
				if len(schemas) > 0 {
					schemaHint = strings.Join(schemas, ", ")
				}
				slog.Warn("no tables found", "schemas", schemaHint)
			}

			findings := analyzer.Audit(snap, auditOptsFromConfig(schemas))
			totalBeforeFilter := len(findings)

			// Apply report filters (severity, type)
			findings = applyReportFilters(findings, minSeverity, typeFilter)

			// Save baseline before baseline/suppress filtering
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

			report := reporter.NewReport("audit", findings, buildVersion)
			report.Metadata.URIHash = reporter.HashURI(dbURL)
			report.Metadata.Database = extractDatabase(dbURL)
			report.Scanned = reporter.ScanContext{
				Tables:  len(snap.Tables),
				Indexes: len(snap.Indexes),
				Schemas: countSchemas(snap),
			}
			filtered := totalBeforeFilter - len(findings) - totalSuppressed
			if totalSuppressed > 0 || filtered > 0 {
				slog.Info("findings filtered",
					"showing", len(findings),
					"total", totalBeforeFilter,
					"suppressed", totalSuppressed,
					"filtered", filtered)
			}

			if err := reporter.Write(cmd.OutOrStdout(), &report, reporter.Format(format), reporter.WriteOptions{NoColor: noColor}); err != nil {
				return fmt.Errorf("write report: %w", err)
			}

			if failOn != "" && shouldFailOn(findings, failOn) {
				return &ExitError{Code: 2}
			}

			code := analyzer.ExitCode(report.MaxSeverity)
			if code != 0 {
				return &ExitError{Code: code}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, sarif, or spectrehub")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "exit 2 if findings match (comma-separated types or severity: high,medium)")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "", "show only findings at or above this severity (high, medium, low, info)")
	cmd.Flags().StringVar(&typeFilter, "type", "", "show only these finding types (comma-separated, e.g. UNUSED_INDEX,BLOATED_INDEX)")
	cmd.Flags().StringVar(&schemaFlag, "schema", "", "schemas to analyze (comma-separated, or 'all' for all non-system schemas)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI color output")
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
		failOnDrift    bool
		minSeverity    string
		typeFilter     string
		schemaFlag     string
		noColor        bool
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

			schemas := resolveSchemaFlag(schemaFlag)
			snap = postgres.FilterSnapshot(snap, schemas)
			slog.Info("inspected", "tables", len(snap.Tables), "indexes", len(snap.Indexes), "constraints", len(snap.Constraints), "schemas", schemas)

			if len(snap.Tables) == 0 {
				schemaHint := "public"
				if len(schemas) > 0 {
					schemaHint = strings.Join(schemas, ", ")
				}
				slog.Warn("no tables found", "schemas", schemaHint)
			}

			// Run diff analysis
			findings := analyzer.Diff(&scan, snap, auditOptsFromConfig(schemas))
			totalBeforeFilter := len(findings)

			// Apply report filters (severity, type)
			findings = applyReportFilters(findings, minSeverity, typeFilter)

			// Save baseline before baseline/suppress filtering
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

			report := reporter.NewReport("check", findings, buildVersion)
			report.Metadata.URIHash = reporter.HashURI(dbURL)
			report.Metadata.Database = extractDatabase(dbURL)
			report.Scanned = reporter.ScanContext{
				Tables:  len(snap.Tables),
				Indexes: len(snap.Indexes),
				Schemas: countSchemas(snap),
			}
			filtered := totalBeforeFilter - len(findings) - totalSuppressed
			if totalSuppressed > 0 || filtered > 0 {
				slog.Info("findings filtered",
					"showing", len(findings),
					"total", totalBeforeFilter,
					"suppressed", totalSuppressed,
					"filtered", filtered)
			}

			if err := reporter.Write(cmd.OutOrStdout(), &report, reporter.Format(format), reporter.WriteOptions{NoColor: noColor}); err != nil {
				return fmt.Errorf("write report: %w", err)
			}

			// Backward-compatible aliases for common check failures.
			effectiveFailOn := resolveCheckFailOn(failOn, failOnMissing, failOnDrift)
			if effectiveFailOn != "" && shouldFailOn(findings, effectiveFailOn) {
				return &ExitError{Code: 2}
			}

			code := analyzer.ExitCode(report.MaxSeverity)
			if code != 0 {
				return &ExitError{Code: code}
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repo, "repo", "", "path to code repository to scan")
	cmd.Flags().StringVar(&format, "format", "text", "output format: text, json, sarif, or spectrehub")
	cmd.Flags().StringVar(&failOn, "fail-on", "", "exit 2 if findings match (comma-separated types or severity: high,medium)")
	cmd.Flags().BoolVar(&failOnMissing, "fail-on-missing", false, "exit 2 if any MISSING_TABLE found (deprecated, use --fail-on)")
	cmd.Flags().BoolVar(&failOnDrift, "fail-on-drift", false, "exit 2 if any schema drift found (alias for MISSING_COLUMN, deprecated, use --fail-on)")
	cmd.Flags().StringVar(&minSeverity, "min-severity", "", "show only findings at or above this severity (high, medium, low, info)")
	cmd.Flags().StringVar(&typeFilter, "type", "", "show only these finding types (comma-separated, e.g. MISSING_TABLE,UNUSED_INDEX)")
	cmd.Flags().StringVar(&schemaFlag, "schema", "", "schemas to analyze (comma-separated, or 'all' for all non-system schemas)")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "disable ANSI color output")
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
			t := canonicalFindingType(p)
			if t != "" {
				types[t] = true
			}
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

// resolveCheckFailOn resolves check-specific fail aliases when --fail-on is not explicitly set.
func resolveCheckFailOn(failOn string, failOnMissing, failOnDrift bool) string {
	if strings.TrimSpace(failOn) != "" {
		return failOn
	}
	var parts []string
	if failOnMissing {
		parts = append(parts, string(analyzer.FindingMissingTable))
	}
	if failOnDrift {
		parts = append(parts, string(analyzer.FindingMissingColumn))
	}
	return strings.Join(parts, ",")
}

// applyReportFilters applies --min-severity and --type filters to findings.
func applyReportFilters(findings []analyzer.Finding, minSeverity, typeFilter string) []analyzer.Finding {
	if minSeverity != "" {
		findings = filterBySeverity(findings, minSeverity)
	}
	if typeFilter != "" {
		findings = filterByType(findings, typeFilter)
	}
	return findings
}

// filterBySeverity keeps only findings at or above the given severity level.
func filterBySeverity(findings []analyzer.Finding, minSev string) []analyzer.Finding {
	threshold, ok := severityOrder[strings.ToLower(minSev)]
	if !ok {
		return findings // unknown severity, no filtering
	}

	var result []analyzer.Finding
	for _, f := range findings {
		if severityOrder[string(f.Severity)] >= threshold {
			result = append(result, f)
		}
	}
	return result
}

var severityOrder = map[string]int{
	"info":   0,
	"low":    1,
	"medium": 2,
	"high":   3,
}

// findingTypeAliases maps legacy names to current finding types.
var findingTypeAliases = map[string]string{
	"SCHEMA_DRIFT": string(analyzer.FindingMissingColumn),
}

func canonicalFindingType(t string) string {
	t = strings.ToUpper(strings.TrimSpace(t))
	if t == "" {
		return ""
	}
	if alias, ok := findingTypeAliases[t]; ok {
		return alias
	}
	return t
}

// filterByType keeps only findings matching the given types (comma-separated).
func filterByType(findings []analyzer.Finding, typeFilter string) []analyzer.Finding {
	types := make(map[string]bool)
	for _, t := range strings.Split(typeFilter, ",") {
		t = canonicalFindingType(t)
		if t != "" {
			types[t] = true
		}
	}
	if len(types) == 0 {
		return findings
	}

	var result []analyzer.Finding
	for _, f := range findings {
		if types[string(f.Type)] {
			result = append(result, f)
		}
	}
	return result
}

// countSchemas returns the number of unique schemas in a snapshot.
// extractDatabase returns the database name from a PostgreSQL connection URL.
func extractDatabase(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}

func countSchemas(snap *postgres.Snapshot) int {
	schemas := make(map[string]bool)
	for _, t := range snap.Tables {
		schemas[t.Schema] = true
	}
	return len(schemas)
}

// resolveSchemaFlag parses the --schema flag value and falls back to config.
func resolveSchemaFlag(flag string) []string {
	if flag != "" {
		parts := strings.Split(flag, ",")
		return postgres.ResolveSchemas(parts)
	}
	if len(cfg.Schemas) > 0 {
		return postgres.ResolveSchemas(cfg.Schemas)
	}
	return nil
}

func auditOptsFromConfig(includeSchemas []string) analyzer.AuditOptions {
	// Include wins over exclude: remove included schemas from the exclude list
	excludeSchemas := cfg.Exclude.Schemas
	if len(includeSchemas) > 0 {
		includeSet := make(map[string]bool, len(includeSchemas))
		for _, s := range includeSchemas {
			includeSet[strings.ToLower(s)] = true
		}
		filtered := make([]string, 0, len(excludeSchemas))
		for _, s := range excludeSchemas {
			if !includeSet[strings.ToLower(s)] {
				filtered = append(filtered, s)
			}
		}
		excludeSchemas = filtered
	}

	return analyzer.AuditOptions{
		VacuumDays:          cfg.Thresholds.VacuumDays,
		UnusedIndexMinBytes: cfg.Thresholds.UnusedIndexMinBytes,
		BloatMinBytes:       cfg.Thresholds.BloatMinBytes,
		ExcludeTables:       cfg.Exclude.Tables,
		ExcludeSchemas:      excludeSchemas,
	}
}

// Execute runs the root command.
func Execute(v, commit, date string) error {
	info := BuildInfo{
		Version:   v,
		Commit:    commit,
		Date:      date,
		GoVersion: runtime.Version(),
	}
	return newRootCmd(info).Execute()
}
