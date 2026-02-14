package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var dbURL string

func newRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:          "pgspectre",
		Short:        "PostgreSQL schema and usage auditor",
		Long:         "Scans codebases for table/column references, compares with live Postgres schema and statistics, detects drift.",
		SilenceUsage: true,
	}

	root.PersistentFlags().StringVar(&dbURL, "db-url", "", "PostgreSQL connection URL (or set PGSPECTRE_DB_URL)")

	root.AddCommand(newVersionCmd(version))
	root.AddCommand(newAuditCmd())
	root.AddCommand(newCheckCmd())

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
	return &cobra.Command{
		Use:   "audit",
		Short: "Cluster-only analysis: unused tables, indexes, missing stats",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("audit command not implemented yet")
		},
	}
}

func newCheckCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Code repo + cluster: missing tables, schema drift, unindexed queries",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("check command not implemented yet")
		},
	}
}

// Execute runs the root command.
func Execute(version string) error {
	return newRootCmd(version).Execute()
}
