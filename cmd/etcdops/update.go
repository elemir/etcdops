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
	"time"

	api "github.com/elemir/etcdops/api/v1alpha1"
	"github.com/elemir/etcdops/pkg/cli"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
)

type updateParams struct {
	size                  int
	version               string
	backupCreationPeriod  time.Duration
	backupRetentionPeriod time.Duration
}

var (
	up        updateParams
	updateCmd = &cobra.Command{
		Use:   "update [flags] <CLUSTER-NAME>",
		Short: "Update an etcd cluster",
		Args:  cobra.ExactArgs(1),
		RunE:  update,
	}
)

func init() {
	updateCmd.PersistentFlags().IntVar(&up.size, "size", 0, "Number of cluster members")
	updateCmd.PersistentFlags().StringVar(&up.version, "version", "", "Version used in cluster")
	updateCmd.PersistentFlags().DurationVar(&up.backupCreationPeriod, "backup-creation-period", 0, "Creation policy of automated backups")
	updateCmd.PersistentFlags().DurationVar(&up.backupRetentionPeriod, "backup-retention-period", 0, "Retention policy of automated backups")
}

func update(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := cli.NewClient()
	if err != nil {
		return err
	}

	var cluster api.Cluster

	name := args[0]
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

	if up.size != 0 {
		cluster.Spec.Size = up.size
	}
	if up.version != "" {
		cluster.Spec.Version = up.version
	}
	if up.backupRetentionPeriod != 0 {
		cluster.Spec.BackupRetentionPeriod = up.backupRetentionPeriod
	}
	if up.backupCreationPeriod != 0 {
		cluster.Spec.BackupRetentionPeriod = up.backupRetentionPeriod
	}

	err = client.Update(ctx, &cluster)
	if err != nil {
		return err
	}

	watcher, err := client.Watch(ctx, &api.ClusterList{
		Items: []api.Cluster{
			cluster,
		},
	})
	defer func() {
		watcher.Stop()
	}()

	obj := watcher.Wait(func(event watch.Event) bool {
		cluster, ok := event.Object.(*api.Cluster)

		return ok && cluster.Status.Version == up.version && cluster.Name == name
	})

	return cli.PrettyPrint(obj, output)
}
