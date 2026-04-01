package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"vocis/internal/config"
)

var initForce bool

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create default config file",
	Long:  "Create a default config file. Use --force to overwrite an existing config.",
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInit(initForce)
	},
}

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false, "overwrite existing config with defaults")
}

func runInit(force bool) error {
	if force {
		path, err := config.Path()
		if err != nil {
			return err
		}
		if err := config.Save(path, config.Default()); err != nil {
			return err
		}
		fmt.Printf("wrote %s (forced)\n", path)
		return nil
	}

	path, err := config.InitDefault()
	if err != nil {
		return err
	}

	fmt.Printf("wrote %s\n", path)
	return nil
}
