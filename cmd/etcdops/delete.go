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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

var (
	deleteCmd = &cobra.Command{
		Use:   "delete [flags] <CLUSTER-NAME>",
		Short: "Delete an etcd cluster",
		Args:  cobra.ExactArgs(1),
		RunE:  deleteCluster,
	}
)

func deleteCluster(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	cl, err := cli.NewClient()
	if err != nil {
		return err
	}

	name := args[0]

	cluster := api.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: cl.Namespace,
		},
	}

	watcher, err := cl.Watch(ctx, &api.ClusterList{})

	defer func() {
		watcher.Stop()
	}()

	if err := cl.Delete(ctx, &cluster); err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("cluster \"%s\" not found", name)
		}
		return err
	}

	watcher.Wait(func(event watch.Event) bool {
		cluster, ok := event.Object.(*api.Cluster)

		return ok && event.Type == watch.Deleted && cluster.Name == name
	})

	return nil
}
