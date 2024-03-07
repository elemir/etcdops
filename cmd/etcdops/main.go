/*
Copyright 2022 Evgenii Omelchenko.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, version 3.

This program is distributed in the hope that it will be useful, but
WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the GNU
General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program. If not, see <http://www.gnu.org/licenses/>.
*/

package main

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	kubeconfigPath string
	namespace      string
	output         string

	rootCmd = &cobra.Command{
		Use:          "etcdops",
		Short:        "etcdops controls etcd clusters",
		SilenceUsage: true,
	}
)

func init() {
	rootCmd.PersistentFlags().StringVar(&kubeconfigPath, "kubeconfig", "", "Path to the kubeconfig file to use for CLI requests (default is $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&namespace, "namespace", "", "If present, the namespace scope for this CLI request")
	rootCmd.PersistentFlags().StringVar(&output, "output", "text", "Set the output format: text, yaml or json")
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

func main() {
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(listBackupsCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
