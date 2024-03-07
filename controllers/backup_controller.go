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
	"crypto/tls"
	"io"
	"path"
	"time"

	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/go-logr/zapr"
	clientv3 "go.etcd.io/etcd/client/v3"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/elemir/etcdops/api/v1alpha1"
)

// BackupReconciler reconciles a Backup object
type BackupReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	S3API      s3iface.S3API
	S3Uploader *s3manager.Uploader
	S3Bucket   string
	S3Prefix   string
}

//+kubebuilder:rbac:groups=operator.etcd.io,resources=backups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.etcd.io,resources=backups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.etcd.io,resources=backups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *BackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var backup api.Backup
	if err := r.Get(ctx, req.NamespacedName, &backup); err != nil {
		if !errors.IsNotFound(err) {
			l.Error(err, "unable to fetch backup")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if backup.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	defer func() {
		if err := r.Status().Update(ctx, &backup); err != nil && !errors.IsConflict(err) {
			l.Error(err, "unable to update backup")
		}
	}()

	if backup.Status.Finished.IsZero() {
		if result, err := r.UploadBackup(ctx, &backup); err != nil || !result.IsZero() {
			return result, err
		}
	}

	return r.RemoveStale(ctx, &backup)
}

func (r *BackupReconciler) UploadBackup(ctx context.Context, backup *api.Backup) (ctrl.Result, error) {
	snapshot, err := r.Snapshot(ctx, backup)
	if err != nil || snapshot == nil {
		return ctrl.Result{}, err
	}
	defer snapshot.Close()

	key := path.Join(r.S3Prefix, backup.Labels[api.ClusterLabel], backup.Name)
	output, err := r.S3Uploader.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: &r.S3Bucket,
		Key:    &key,
		Body:   snapshot,
	})
	if err != nil {
		return ctrl.Result{}, err
	}
	backup.Status.URL = output.Location
	backup.Status.Finished = metav1.Now()

	return ctrl.Result{}, nil
}

func (r *BackupReconciler) Snapshot(ctx context.Context, backup *api.Backup) (io.ReadCloser, error) {
	l := log.FromContext(ctx)

	var cluster api.Cluster
	if err := r.Get(ctx, types.NamespacedName{
		Name:      backup.Labels[api.ClusterLabel],
		Namespace: backup.Namespace,
	}, &cluster); err != nil {
		return nil, client.IgnoreNotFound(err)
	}

	etcdConfig := clientv3.Config{
		Endpoints:   cluster.GetEndpoints(),
		DialTimeout: 5 * time.Second,
		TLS: &tls.Config{
			InsecureSkipVerify: true,
		},
	}

	if zapLogger, ok := l.GetSink().(zapr.Underlier); ok {
		etcdConfig.Logger = zapLogger.GetUnderlying()
	}

	etcd, err := clientv3.New(etcdConfig)
	if err != nil {
		l.Error(err, "failed to instanate etcd client")
		return nil, err
	}

	return etcd.Snapshot(ctx)
}

func (r *BackupReconciler) RemoveStale(ctx context.Context, backup *api.Backup) (ctrl.Result, error) {
	retentionTime := backup.CreationTimestamp.Add(backup.Spec.RetentionPeriod)
	if retentionTime.Before(time.Now()) {
		key := path.Join(r.S3Prefix, backup.Labels[api.ClusterLabel], backup.Name)
		if _, err := r.S3API.DeleteObjectWithContext(ctx, &s3.DeleteObjectInput{
			Bucket: &r.S3Bucket,
			Key:    &key,
		}); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Delete(ctx, backup); err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}
	}

	return ctrl.Result{
		RequeueAfter: retentionTime.Sub(time.Now()),
	}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.Backup{}).
		Complete(r)
}
