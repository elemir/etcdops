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

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"go.uber.org/multierr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	api "github.com/elemir/etcdops/api/v1alpha1"
)

const (
	minorFailedTimeout = 5 * time.Minute
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	ClusterIssuer string
}

//+kubebuilder:rbac:groups=operator.etcd.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.etcd.io,resources=clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.etcd.io,resources=clusters/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;delete
//+kubebuilder:rbac:groups=cert-manager.io,resources=certificates,verbs=get;list;watch;create;update
//+kubebuilder:rbac:groups=cert-manager.io,resources=issuers,verbs=get;list;watch;create;update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var cluster api.Cluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if !errors.IsNotFound(err) {
			l.Error(err, "unable to fetch cluster")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if cluster.DeletionTimestamp != nil {
		return r.CleanupSecrets(ctx, &cluster)
	}

	if cluster.Status.Phase == "" {
		cluster.Status.Phase = api.ClusterCreating
	}

	defer func() {
		if err := r.Status().Update(ctx, &cluster); err != nil && !errors.IsConflict(err) {
			l.Error(err, "unable to update cluster")
		}
	}()

	if result, err := r.EnsureBackupSchedule(ctx, &cluster); err != nil || !result.IsZero() {
		return result, err
	}
	if result, err := r.EnsureService(ctx, &cluster); err != nil || !result.IsZero() {
		return result, err
	}
	if result, err := r.EnsureCA(ctx, &cluster); err != nil || !result.IsZero() {
		return result, err
	}
	if result, err := r.EnsureMembers(ctx, &cluster); err != nil || !result.IsZero() {
		return result, err
	}

	if cluster.Status.Phase == api.ClusterMinorFailure {
		if result, err := r.RepairMembers(ctx, &cluster); err != nil || !result.IsZero() {
			return result, err
		}
	}

	if cluster.ShouldUpdate() {
		if result, err := r.UpdateMembers(ctx, &cluster); err != nil || !result.IsZero() {
			return result, err
		}
	}

	cluster.Status.Version = cluster.Spec.Version

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) EnsureBackupSchedule(ctx context.Context, cluster *api.Cluster) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	schedule := cluster.GetBackupSchedule()
	if err := controllerutil.SetControllerReference(cluster, schedule, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, schedule, SkipUpdate); err != nil {
		l.Error(err, "unable to schedule backups")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) EnsureService(ctx context.Context, cluster *api.Cluster) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	service := cluster.GetService()
	if err := controllerutil.SetControllerReference(cluster, service, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, service, SkipUpdate); err != nil {
		l.Error(err, "unable to create service")
		return ctrl.Result{}, err
	} else if opResult == controllerutil.OperationResultCreated {
		return Requeue(), nil
	}

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) EnsureCA(ctx context.Context, cluster *api.Cluster) (ctrl.Result, error) {
	if result, err := r.EnsureCACertificate(ctx, cluster); err != nil || !result.IsZero() {
		return result, err
	}
	if result, err := r.EnsureCAIssuer(ctx, cluster); err != nil || !result.IsZero() {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) EnsureCACertificate(ctx context.Context, cluster *api.Cluster) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	ca := cluster.GetCACertificate(r.ClusterIssuer)
	if err := controllerutil.SetControllerReference(cluster, ca, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, ca, SkipUpdate); err != nil {
		l.Error(err, "unable to create CA certificate")
		return ctrl.Result{}, err
	} else if opResult == controllerutil.OperationResultCreated {
		return Requeue(), nil
	}

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) EnsureCAIssuer(ctx context.Context, cluster *api.Cluster) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	ca := cluster.GetCAIssuer()
	if err := controllerutil.SetControllerReference(cluster, ca, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}

	if opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, ca, SkipUpdate); err != nil {
		l.Error(err, "unable to create issuer with CA")
		return ctrl.Result{}, err
	} else if opResult == controllerutil.OperationResultCreated {
		return Requeue(), nil
	}

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) EnsureMembers(ctx context.Context, cluster *api.Cluster) (ctrl.Result, error) {
	var errs error

	failedCount := 0
	creatingCount := 0
	certificateExpires := false

	for i := 0; i < cluster.Spec.Size; i++ {
		member, err := r.EnsureMember(ctx, cluster, i)
		errs = multierr.Append(errs, err)

		if member.Status.Phase == api.MemberFailed {
			failedCount++
		} else if member.IsCreating() {
			creatingCount++
		}

		certificateExpires = certificateExpires || member.Status.CertificateExpires || member.Spec.CertificateUpdate
	}

	if failedCount == 0 && creatingCount == 0 {
		cluster.Status.Phase = api.ClusterRunning
	} else if failedCount == 0 && creatingCount > 0 {
		cluster.Status.Phase = api.ClusterCreating
	} else if (failedCount+creatingCount)*2 < cluster.Spec.Size {
		cluster.Status.Phase = api.ClusterMinorFailure
	} else {
		cluster.Status.Phase = api.ClusterFailed
	}
	cluster.Status.CertificateExpires = certificateExpires

	return ctrl.Result{}, errs
}

func (r *ClusterReconciler) EnsureMember(ctx context.Context, cluster *api.Cluster, num int) (*api.Member, error) {
	l := log.FromContext(ctx)

	member := cluster.GetMember(num)
	if err := controllerutil.SetControllerReference(cluster, member, r.Scheme); err != nil {
		return nil, err
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, member, SkipUpdate); err != nil {
		l.Error(err, "unable to create member")
		return nil, err
	}

	return member, nil
}

func (r *ClusterReconciler) UpdateMembers(ctx context.Context, cluster *api.Cluster) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	var errs error

	l.Info("update members", "cluster", cluster.Name, "namespace", cluster.Namespace)

	for i := 0; i < cluster.Spec.Size; i++ {
		member, err := r.EnsureMember(ctx, cluster, i)
		errs = multierr.Append(errs, err)

		if member.IsCreating() || member.Status.Phase == api.MemberUpdating {
			return Requeue(), nil
		}

		if cluster.Spec.Version != member.Spec.Version {
			member.Spec.Version = cluster.Spec.Version
			cluster.Status.Phase = api.ClusterUpdating

			return Requeue(), r.Update(ctx, member)
		}

		if cluster.Status.CertificateExpires || member.Spec.CertificateUpdate {
			member.Spec.CertificateUpdate = member.Status.CertificateExpires
			if err := r.Update(ctx, member); err != nil {
				return ctrl.Result{}, err
			}
			if member.Spec.CertificateUpdate {
				cluster.Status.Phase = api.ClusterUpdating

				return Requeue(), nil
			}
		}
		if cluster.Spec.Version != member.Status.Version {
			return Requeue(), nil
		}
	}

	if errs != nil {
		return ctrl.Result{}, errs
	}

	cluster.Status.Phase = api.ClusterRunning
	cluster.Status.CertificateExpires = false

	return ctrl.Result{}, nil
}

func (r *ClusterReconciler) RepairMembers(ctx context.Context, cluster *api.Cluster) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var errs error
	var failedMember *api.Member

	failedCount := 0
	minFailedTime := time.Now().Add(-minorFailedTimeout)

	for i := 0; i < cluster.Spec.Size; i++ {
		member, err := r.EnsureMember(ctx, cluster, i)
		errs = multierr.Append(errs, err)

		if member.IsCreating() || member.Spec.Broken {
			return Requeue(), nil
		}

		if member.Status.Phase != api.MemberFailed {
			continue
		}

		failedCount++
		if member.Status.FailedTime.Time.Before(minFailedTime) {
			minFailedTime = member.Status.FailedTime.Time
			failedMember = member
		}
	}
	if failedMember == nil {
		return Requeue(), nil
	}

	if failedCount*2 > cluster.Spec.Size || failedCount == 0 {
		return ctrl.Result{}, nil
	}

	l.Info("starting repair process", "member", failedMember.Name, "namespace", failedMember.Namespace)
	failedMember.Spec.Broken = true

	return ctrl.Result{}, r.Update(ctx, failedMember)
}

func (r *ClusterReconciler) CleanupSecrets(ctx context.Context, cluster *api.Cluster) (ctrl.Result, error) {
	if controllerutil.ContainsFinalizer(cluster, metav1.FinalizerDeleteDependents) {
		return Requeue(), nil
	}

	var secrets corev1.SecretList

	if err := r.List(ctx, &secrets, client.InNamespace(cluster.Namespace), client.MatchingLabels{
		api.ClusterLabel: cluster.Name,
	}); client.IgnoreNotFound(err) != nil {
		return ctrl.Result{}, err
	}

	for _, secret := range secrets.Items {
		if err := r.Delete(ctx, &secret); err != nil {
			return ctrl.Result{}, err
		}
	}

	controllerutil.RemoveFinalizer(cluster, api.CleanupSecretFinalizer)
	return ctrl.Result{}, r.Update(ctx, cluster)
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.Cluster{}).
		Owns(&api.Member{}).
		Owns(&api.BackupSchedule{}).
		Owns(&corev1.Service{}).
		Owns(&certv1.Certificate{}).
		Owns(&certv1.Issuer{}).
		Complete(r)
}

func SkipUpdate() error {
	return nil
}
