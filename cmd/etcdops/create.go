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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
)

type createParams struct {
	size                  int
	fromBackup            string
	backupCreationPeriod  time.Duration
	backupRetentionPeriod time.Duration
}

var (
	cp createParams

	createCmd = &cobra.Command{
		Use:   "create [flags] <CLUSTER-NAME> <VERSION>",
		Short: "Create an etcd cluster",
		Args:  cobra.MatchAll(cobra.MinimumNArgs(1), cobra.MaximumNArgs(2), oneOfVersionFromBackup),
		RunE:  create,
	}
)

func init() {
	createCmd.PersistentFlags().IntVar(&cp.size, "size", 3, "Number of cluster members")
	createCmd.PersistentFlags().StringVar(&cp.fromBackup, "from-backup", "", "Backup used for a cluster restoration")
	createCmd.PersistentFlags().DurationVar(&cp.backupCreationPeriod, "backup-creation-period", 24*time.Hour, "Creation policy of automated backups")
	createCmd.PersistentFlags().DurationVar(&cp.backupRetentionPeriod, "backup-retention-period", 7*24*time.Hour, "Retention policy of automated backups")

}

func oneOfVersionFromBackup(cmd *cobra.Command, args []string) error {
	if len(args) == 1 && cp.fromBackup == "" {
		return fmt.Errorf("one of --from-backup or VERSION is required")
	}

	return nil
}

func create(cmd *cobra.Command, args []string) error {
	ctx := context.Background()
	client, err := cli.NewClient()
	if err != nil {
		return err
	}

	name := args[0]
	version := ""
	if len(args) == 2 {
		version = args[1]
	}

	cluster := api.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: client.Namespace,
			Finalizers: []string{
				metav1.FinalizerDeleteDependents,
				api.CleanupSecretFinalizer,
			},
		},
		Spec: api.ClusterSpec{
			Size:                  cp.size,
			Version:               version,
			Backup:                cp.fromBackup,
			BackupCreationPeriod:  cp.backupCreationPeriod,
			BackupRetentionPeriod: cp.backupRetentionPeriod,
		},
	}

	err = client.Create(ctx, &cluster)
	if err != nil {
		return err
	}

	watcher, err := client.Watch(ctx, &api.ClusterList{})
	defer func() {
		watcher.Stop()
	}()

	obj := watcher.Wait(func(event watch.Event) bool {
		cluster, ok := event.Object.(*api.Cluster)

		return ok && cluster.Status.Phase == api.ClusterRunning && cluster.Name == name
	})

	return cli.PrettyPrint(obj, output)
}
