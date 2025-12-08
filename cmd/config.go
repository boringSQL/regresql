package cmd

import (
	"fmt"
	"os"

	"github.com/boringsql/regresql/regresql"
	"github.com/spf13/cobra"
)

var (
	configCwd      string
	configTestConn bool

	configCmd = &cobra.Command{
		Use:   "config",
		Short: "View and update configuration",
		Long: `View and update regresql configuration.

Examples:
  # Get current pguri
  regresql config get pguri

  # Set pguri with connection test
  regresql config set pguri "postgres://user@host/db" --test

  # Set pguri without testing
  regresql config set pguri "postgres://user@host/db"`,
	}

	configGetCmd = &cobra.Command{
		Use:   "get <key>",
		Short: "Get a configuration value",
		Long: `Get a configuration value from regress.yaml.

Supported keys:
  pguri   - PostgreSQL connection string
  root    - Root directory for SQL file discovery`,
		Args: cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(configCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}

			if err := runConfigGet(args[0]); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}

	configSetCmd = &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value",
		Long: `Set a configuration value in regress.yaml.

Supported keys:
  pguri   - PostgreSQL connection string

Examples:
  # Set pguri and test connection
  regresql config set pguri "postgres://user@host/db" --test

  # Set pguri without testing
  regresql config set pguri "postgres://user@host/db"`,
		Args: cobra.ExactArgs(2),
		Run: func(cmd *cobra.Command, args []string) {
			if err := checkDirectory(configCwd); err != nil {
				fmt.Print(err.Error())
				os.Exit(1)
			}

			if err := runConfigSet(args[0], args[1]); err != nil {
				fmt.Printf("Error: %s\n", err.Error())
				os.Exit(1)
			}
		},
	}
)

func init() {
	RootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)

	configCmd.PersistentFlags().StringVarP(&configCwd, "cwd", "C", ".", "Change to directory")
	configSetCmd.Flags().BoolVar(&configTestConn, "test", false, "Test connection before saving (for pguri)")
}

func runConfigGet(key string) error {
	cfg, err := regresql.ReadConfig(configCwd)
	if err != nil {
		return fmt.Errorf("failed to read config: %w (have you run 'regresql init'?)", err)
	}

	switch key {
	case "pguri":
		fmt.Println(cfg.PgUri)
	case "root":
		fmt.Println(cfg.Root)
	default:
		return fmt.Errorf("unknown config key: %s (supported: pguri, root)", key)
	}

	return nil
}

func runConfigSet(key, value string) error {
	switch key {
	case "pguri":
		if configTestConn {
			fmt.Printf("Testing connection to %s... ", maskConnectionString(value))
			if err := regresql.TestConnectionString(value); err != nil {
				fmt.Println("✗")
				return fmt.Errorf("connection test failed: %w", err)
			}
			fmt.Println("✓")
		}

		if err := regresql.UpdateConfigField(configCwd, "pguri", value); err != nil {
			return err
		}
		fmt.Printf("Updated pguri in regress.yaml\n")

	default:
		return fmt.Errorf("unknown or read-only config key: %s (can set: pguri)", key)
	}

	return nil
}
