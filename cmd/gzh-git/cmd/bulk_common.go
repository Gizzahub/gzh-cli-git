package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/gizzahub/gzh-cli-git/pkg/repository"
)

// BulkCommandFlags holds common flags for bulk operations (fetch, pull, push)
type BulkCommandFlags struct {
	Depth             int
	Parallel          int
	DryRun            bool
	IncludeSubmodules bool
	Include           string
	Exclude           string
	Format            string
	Watch             bool
	Interval          time.Duration
}

// addBulkFlags registers common bulk operation flags to a command
func addBulkFlags(cmd *cobra.Command, flags *BulkCommandFlags) {
	cmd.Flags().IntVarP(&flags.Depth, "depth", "d", repository.DefaultBulkMaxDepth, "directory depth to scan")
	cmd.Flags().IntVarP(&flags.Parallel, "parallel", "j", repository.DefaultBulkParallel, "number of parallel operations")
	cmd.Flags().BoolVarP(&flags.DryRun, "dry-run", "n", false, "show what would be done without doing it")
	cmd.Flags().BoolVarP(&flags.IncludeSubmodules, "recursive", "r", false, "recursively include nested repositories and submodules")
	cmd.Flags().StringVar(&flags.Include, "include", "", "regex pattern to include repositories")
	cmd.Flags().StringVar(&flags.Exclude, "exclude", "", "regex pattern to exclude repositories")
	cmd.Flags().StringVar(&flags.Format, "format", "default", "output format: default, compact")
	cmd.Flags().BoolVar(&flags.Watch, "watch", false, "continuously run at intervals")
	cmd.Flags().DurationVar(&flags.Interval, "interval", 5*time.Minute, "interval when watching")
}

// validateBulkDirectory parses and validates the directory argument
// Returns the directory path (defaults to ".") or an error
func validateBulkDirectory(args []string) (string, error) {
	directory := "."
	if len(args) > 0 {
		directory = args[0]
	}

	// Validate directory exists
	if _, err := os.Stat(directory); err != nil {
		return "", fmt.Errorf("directory does not exist: %s", directory)
	}

	return directory, nil
}

// validateBulkDepth validates the depth flag
// Returns an error if depth is explicitly set to 0
func validateBulkDepth(cmd *cobra.Command, depth int) error {
	if cmd.Flags().Changed("depth") && depth == 0 {
		return fmt.Errorf("depth must be at least 1 (use --depth 1 to scan current directory and immediate subdirectories)")
	}
	return nil
}

// createBulkLogger creates a logger for bulk operations
// Returns a logger if verbose mode is enabled, nil otherwise
func createBulkLogger(verbose bool) repository.Logger {
	if verbose {
		return repository.NewWriterLogger(os.Stdout)
	}
	return nil
}

// createProgressCallback creates a progress callback function for bulk operations
// The callback is used to display progress during bulk operations
func createProgressCallback(operationName string, format string, quiet bool) func(int, int, string) {
	return func(current, total int, repo string) {
		if !quiet && format != "compact" {
			fmt.Printf("[%d/%d] %s %s...\n", current, total, operationName, repo)
		}
	}
}
