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
	"context"

	api "github.com/elemir/etcdops/api/v1alpha1"
	"github.com/elemir/etcdops/pkg/cli"
	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	listCmd = &cobra.Command{
		Use:   "list",
		Short: "List available etcd clusters",
		RunE:  list,
	}
)

func list(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	cl, err := cli.NewClient()
	if err != nil {
		return err
	}

	var clusters api.ClusterList

	err = cl.List(ctx, &clusters, client.InNamespace(namespace))
	if err != nil {
		return err
	}

	return cli.PrettyPrint(clusters, output)
}
