package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"vocis/internal/securestore"
)

var keyCmd = &cobra.Command{
	Use:   "key",
	Short: "Manage OpenAI API key",
}

var keySetCmd = &cobra.Command{
	Use:   "set",
	Short: "Store API key in system keyring",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := securestore.New()
		key, err := readSecret()
		if err != nil {
			return err
		}
		if err := store.SetAPIKey(key); err != nil {
			return err
		}
		fmt.Println("stored OpenAI API key in the system keyring")
		return nil
	},
}

var keyClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove API key from system keyring",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := securestore.New()
		if err := store.ClearAPIKey(); err != nil {
			return err
		}
		fmt.Println("removed OpenAI API key from the system keyring")
		return nil
	},
}

var keyShowSourceCmd = &cobra.Command{
	Use:   "show-source",
	Short: "Show where the API key comes from",
	RunE: func(cmd *cobra.Command, args []string) error {
		store := securestore.New()
		source, err := store.Source()
		if err != nil {
			return err
		}
		fmt.Println(source)
		return nil
	},
}

func init() {
	keyCmd.AddCommand(keySetCmd)
	keyCmd.AddCommand(keyClearCmd)
	keyCmd.AddCommand(keyShowSourceCmd)
}
