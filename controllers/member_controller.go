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
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/go-logr/zapr"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clientv3 "go.etcd.io/etcd/client/v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	api "github.com/elemir/etcdops/api/v1alpha1"
)

// MemberReconciler reconciles a Member object
type MemberReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=operator.etcd.io,resources=members,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=operator.etcd.io,resources=members/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=operator.etcd.io,resources=members/finalizers,verbs=update
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;delete
//+kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=get;list;watch;create;update;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MemberReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var member api.Member
	if err := r.Get(ctx, req.NamespacedName, &member); err != nil {
		if !errors.IsNotFound(err) {
			l.Error(err, "unable to fetch member")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if member.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}

	defer func() {
		if err := r.Status().Update(ctx, &member); err != nil && !errors.IsConflict(err) {
			l.Error(err, "unable to update member status")
		}
	}()

	if member.Spec.Broken {
		if result, err := r.Repair(ctx, &member); err != nil || !result.IsZero() {
			return result, err
		}
	}

	if result, err := r.EnsureCertificates(ctx, &member); err != nil || !result.IsZero() {
		return result, err
	}
	if result, err := r.CheckCertificateExpires(ctx, &member); err != nil || !result.IsZero() {
		return result, err
	}
	if result, err := r.EnsurePVC(ctx, &member); err != nil || !result.IsZero() {
		return result, err
	}
	if member.ShouldUpdate() {
		if result, err := r.DeletePod(ctx, &member); err != nil || !result.IsZero() {
			return result, err
		}
		member.Status.Phase = api.MemberUpdating
	}
	if result, err := r.EnsurePod(ctx, &member); err != nil || !result.IsZero() {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *MemberReconciler) EnsureCertificates(ctx context.Context, member *api.Member) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	prepare := false
	for _, suffix := range []string{"client", "peer"} {
		cert := member.GetCertificate(suffix)
		if err := controllerutil.SetControllerReference(member, cert, r.Scheme); err != nil {
			l.Error(err, "unable to set controller reference on certificate", "type", suffix)
			return ctrl.Result{}, err
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cert, SkipUpdate); err != nil {
			l.Error(err, "unable to create certificate", "type", suffix)
			return ctrl.Result{}, err
		}

		prepare = prepare || cert.Status.NotBefore.IsZero()
	}

	if prepare {
		return Requeue(), nil
	}
	return ctrl.Result{}, nil
}

func (r *MemberReconciler) EnsurePVC(ctx context.Context, member *api.Member) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	if !member.IsCreating() {
		return ctrl.Result{}, nil
	}

	pvc := member.GetPVC()
	if err := controllerutil.SetControllerReference(member, pvc, r.Scheme); err != nil {
		l.Error(err, "unable to set controller reference on pvc")
		return ctrl.Result{}, err
	}

	if opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, pvc, SkipUpdate); err != nil {
		l.Error(err, "unable to create pvc")
		return ctrl.Result{}, err
	} else if opResult == controllerutil.OperationResultCreated {
		return Requeue(), nil
	}

	return ctrl.Result{}, nil
}

func (r *MemberReconciler) EnsurePod(ctx context.Context, member *api.Member) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	pod := member.GetPod()
	if err := controllerutil.SetControllerReference(member, pod, r.Scheme); err != nil {
		l.Error(err, "unable to set controller reference on pod")
		return ctrl.Result{}, err
	}

	if opResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, pod, SkipUpdate); err != nil {
		l.Error(err, "unable to create pod")
		return ctrl.Result{}, err
	} else if opResult == controllerutil.OperationResultCreated {
		if member.Status.Phase == api.MemberRunning {
			member.SetFailed()
		}
		return Requeue(), nil
	}

	ready := len(pod.Status.ContainerStatuses) == len(pod.Spec.Containers)
	for _, status := range pod.Status.ContainerStatuses {
		ready = ready && status.Ready
	}

	if member.Status.Phase == api.MemberRunning && !ready {
		member.SetFailed()
	} else if ready {
		member.Status.Phase = api.MemberRunning
		member.Status.Version = member.Spec.Version
	}

	return ctrl.Result{}, nil
}

func (r *MemberReconciler) CheckCertificateExpires(ctx context.Context, member *api.Member) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var pod corev1.Pod
	err := r.Get(ctx, types.NamespacedName{
		Name:      member.Name,
		Namespace: member.Namespace,
	}, &pod)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	for _, suffix := range []string{"client", "peer"} {
		var cert certv1.Certificate
		err := r.Get(ctx, types.NamespacedName{
			Name:      member.GetCertificateName(suffix),
			Namespace: member.Namespace,
		}, &cert)
		if err != nil {
			return ctrl.Result{}, client.IgnoreNotFound(err)
		}

		if cert.Status.NotBefore.After(pod.CreationTimestamp.Time) {
			l.Info("certificate expires")
			member.Status.CertificateExpires = true
		} else {
			member.Status.CertificateExpires = false
		}
	}

	return ctrl.Result{}, nil
}

func (r *MemberReconciler) Repair(ctx context.Context, member *api.Member) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	if result, err := r.DeletePod(ctx, member); err != nil || !result.IsZero() {
		return result, err
	}
	if result, err := r.DeletePVC(ctx, member); err != nil || !result.IsZero() {
		return result, err
	}
	if result, err := r.ReaddToCluster(ctx, member); err != nil || !result.IsZero() {
		return result, err
	}

	member.Status.Phase = api.MemberRecreating
	member.Status.FailedTime = metav1.Time{}
	if err := r.Status().Update(ctx, member); err != nil && !errors.IsConflict(err) {
		l.Error(err, "unable to update member")
	}

	member.Spec.Broken = false
	if err := r.Update(ctx, member); err != nil && !errors.IsConflict(err) {
		l.Error(err, "unable to update member")
	}

	return ctrl.Result{}, nil
}

func (r *MemberReconciler) ReaddToCluster(ctx context.Context, member *api.Member) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	etcdConfig := clientv3.Config{
		Endpoints:   member.GetEndpoints(),
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
		return ctrl.Result{}, err
	}

	resp, err := etcd.MemberList(ctx)

	if err != nil {
		l.Error(err, "failed to get member list")
		return ctrl.Result{}, err
	}

	for _, m := range resp.Members {
		if m.Name != member.Name {
			continue
		}

		_, err := etcd.MemberRemove(ctx, m.ID)
		if err != nil {
			l.Error(err, "failed to remove member")
			return ctrl.Result{}, err
		}
		l.Info("removed broken member from cluster", "member", member.Name, "namespace", member.Namespace)
	}

	_, err = etcd.MemberAdd(ctx, []string{
		member.GetAdvertisePeerURL(),
	})
	if err != nil {
		l.Error(err, "failed to add new version of member")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *MemberReconciler) DeletePod(ctx context.Context, member *api.Member) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	pod := member.GetPod()
	if err := r.Delete(ctx, pod); err != nil {
		if !errors.IsNotFound(err) {
			l.Error(err, "unable to fetch pod")

			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}
	l.Info("removed member's pod", "member", member.Name, "namespace", member.Namespace)

	return Requeue(), nil
}

func (r *MemberReconciler) DeletePVC(ctx context.Context, member *api.Member) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	pvc := member.GetPVC()
	if err := r.Delete(ctx, pvc); err != nil {
		if !errors.IsNotFound(err) {
			l.Error(err, "unable to fetch pvc")

			return ctrl.Result{}, err
		}

		return ctrl.Result{}, nil
	}
	l.Info("removed broken member's pvc", "member", member.Name, "namespace", member.Namespace)

	return Requeue(), nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemberReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&api.Member{}).
		Owns(&corev1.Pod{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Owns(&certv1.Certificate{}).
		Complete(r)
}
