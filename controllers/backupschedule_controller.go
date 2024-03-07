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

package controllers

import (
	"context"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/elemir/etcdops/api/v1alpha1"
)

// BackupScheduleReconciler reconciles a BackupSchedule object
type BackupScheduleReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=operator.etcd.io,resources=backupschedules,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.etcd.io,resources=backupschedules/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.etcd.io,resources=backupschedules/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BackupScheduleReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var schedule api.BackupSchedule
	if err := r.Get(ctx, req.NamespacedName, &schedule); err != nil {
		if !errors.IsNotFound(err) {
			l.Error(err, "unable to fetch schedule")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if schedule.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	if result, err := r.Schedule(ctx, &schedule); err != nil || !result.IsZero() {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *BackupScheduleReconciler) Schedule(ctx context.Context, schedule *api.BackupSchedule) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	var backups api.BackupList

	err := r.List(ctx, &backups, client.InNamespace(schedule.Namespace), client.MatchingLabels{
		api.ClusterLabel: schedule.Name,
	})
	if err != nil {
		return ctrl.Result{}, err
	}

	var latestBackupTime time.Time

	inProgress := false
	for _, backup := range backups.Items {
		if backup.Status.Finished.IsZero() {
			inProgress = true
		}

		if backup.Status.Finished.After(latestBackupTime) {
			latestBackupTime = backup.Status.Finished.Time
		}
	}

	if inProgress {
		return Requeue(), err
	}

	nextRunTime := latestBackupTime.Add(schedule.Spec.CreationPeriod)
	if nextRunTime.Before(time.Now()) {
		return Requeue(), r.CreateBackup(ctx, schedule)
	}

	l.Info("scheduled next run", "nextRun", nextRunTime)
	return ctrl.Result{
		RequeueAfter: nextRunTime.Sub(time.Now()),
	}, nil
}

func (r *BackupScheduleReconciler) CreateBackup(ctx context.Context, schedule *api.BackupSchedule) error {
	l := log.FromContext(ctx)

	backup := schedule.GetBackup()

	if err := r.Create(ctx, backup); err != nil {
		l.Info("unable to create backup", "name", backup.Name)
		return err
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupScheduleReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.BackupSchedule{}).
		Owns(&api.Backup{}).
		Complete(r)
}
