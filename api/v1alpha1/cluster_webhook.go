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

package v1alpha1

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

// log is for logging in this package.
var clusterlog = logf.Log.WithName("cluster-resource")

func (r *Cluster) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

//+kubebuilder:webhook:path=/validate,mutating=false,failurePolicy=fail,sideEffects=None,groups=operator.etcd.io,resources=clusters,verbs=create;update,versions=v1alpha1,name=vcluster.kb.io,admissionReviewVersions=v1

var _ webhook.Validator = &Cluster{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Cluster) ValidateCreate() error {
	if r.Spec.Size%2 == 0 {
		return fmt.Errorf("size of cluster should be odd, got %d", r.Spec.Size)
	}

	return nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Cluster) ValidateUpdate(old runtime.Object) error {
	oldCluster, ok := old.(*Cluster)
	if !ok {
		return fmt.Errorf("updated object is not cluster")
	}
	if oldCluster.Status.Phase == ClusterUpdating && r.Spec.Version != oldCluster.Spec.Version {
		return fmt.Errorf("unable to change cluster version on updating cluster")
	}
	if oldCluster.Spec.Backup != r.Spec.Backup {
		return fmt.Errorf("unable to restore working cluster from backup, please create new one")
	}
	if oldCluster.Spec.Size != r.Spec.Size {
		return fmt.Errorf("changing cluster size currently not supported")
	}

	return nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Cluster) ValidateDelete() error {
	return nil
}
