package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/zoncoen/scenarigo"
	"github.com/zoncoen/scenarigo/reporter"
)

var listCmd = &cobra.Command{
	Use:           "list",
	Short:         "list the test scenario files",
	Long:          "Lists the test scenario files as relative paths from the current directory.",
	RunE:          list,
	SilenceErrors: true,
	SilenceUsage:  true,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

func list(cmd *cobra.Command, args []string) error {
	return listWithConfig(cmd, args, configFile)
}

func listWithConfig(cmd *cobra.Command, args []string, configPath string) error {
	opts := []func(*scenarigo.Runner) error{}
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}
	if cfg != nil {
		if len(args) > 0 {
			cfg.Scenarios = nil
		}
		opts = append(opts, scenarigo.WithConfig(cfg))
	}
	if len(args) > 0 {
		opts = append(opts, scenarigo.WithScenarios(args...))
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	for _, arg := range args {
		opts = append(opts, scenarigo.WithScenarios(arg))
	}
	r, err := scenarigo.NewRunner(opts...)
	if err != nil {
		return err
	}

	var retErr error
	reporterOpts := []reporter.Option{reporter.WithWriter(io.Discard)}
	reporter.Run(func(rptr reporter.Reporter) {
		for _, file := range r.ScenarioFiles() {
			rel, err := filepath.Rel(wd, file)
			if err != nil {
				retErr = fmt.Errorf("failed to get releative path: %w", err)
				break
			}
			fmt.Fprintln(cmd.OutOrStdout(), rel)
		}
	}, reporterOpts...)
	return retErr
}
