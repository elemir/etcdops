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
	"fmt"

	api "github.com/elemir/etcdops/api/v1alpha1"
	"github.com/elemir/etcdops/pkg/cli"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

var (
	getCmd = &cobra.Command{
		Use:   "get [flags] <CLUSTER-NAME>",
		Short: "Get information about an etcd cluster",
		Args:  cobra.ExactArgs(1),
		RunE:  getCluster,
	}
)

func getCluster(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := cli.NewClient()
	if err != nil {
		return err
	}

	name := args[0]

	var cluster api.Cluster

	err = client.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: client.Namespace,
	}, &cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("cluster \"%s\" not found", name)
		}
		return err
	}

	return cli.PrettyPrint(cluster, output)
}
