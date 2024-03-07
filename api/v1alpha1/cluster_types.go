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
	"time"

	"github.com/jedib0t/go-pretty/v6/table"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
)

const (
	ClusterLabel           = "cluster.operator.etcd.io"
	CleanupSecretFinalizer = "cleanup-secret.operator.etcd.io"
)

// ClusterSpec defines the desired state of etcd cluster
type ClusterSpec struct {
	Version               string        `json:"version,omitempty"`
	Size                  int           `json:"size,omitempty"`
	Backup                string        `json:"backup,omitempty"`
	BackupCreationPeriod  time.Duration `json:"backupCreationPeriod,omitempty"`
	BackupRetentionPeriod time.Duration `json:"backupRetentionPeriod,omitempty"`
}

// ClusterStatus defines the observed state of etcd cluster
type ClusterStatus struct {
	Phase              ClusterPhase `json:"phase,omitempty" yaml:"phase,omitempty"`
	Version            string       `json:"version,omitempty" yaml:"version,omitempty"`
	CertificateExpires bool         `json:"certificateExpires,omitempty" yaml:"certificateExpires,omitempty"`
}

type ClusterPhase string

var (
	ClusterCreating     ClusterPhase = "Creating"
	ClusterRunning      ClusterPhase = "Running"
	ClusterUpdating     ClusterPhase = "Updating"
	ClusterMinorFailure ClusterPhase = "MinorFailure"
	ClusterFailed       ClusterPhase = "Failed"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.phase`
//+kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`

// Cluster is the Schema for the clusters API
type Cluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterSpec   `json:"spec,omitempty"`
	Status ClusterStatus `json:"status,omitempty"`
}

func (in *Cluster) GetCACertificate(clusterIssuer string) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name,
			Namespace: in.Namespace,
			Finalizers: []string{
				metav1.FinalizerDeleteDependents,
			},
		},
		Spec: certv1.CertificateSpec{
			IsCA:       true,
			CommonName: in.GetCommonName(),
			SecretName: in.GetCASecretName(),
			SecretTemplate: &certv1.CertificateSecretTemplate{
				Labels: map[string]string{
					ClusterLabel: in.Name,
				},
			},
			PrivateKey: &certv1.CertificatePrivateKey{
				Algorithm: certv1.ECDSAKeyAlgorithm,
				Size:      256,
			},
			IssuerRef: cmmeta.ObjectReference{
				Name:  clusterIssuer,
				Kind:  "ClusterIssuer",
				Group: "cert-manager.io",
			},
		},
	}
}

func (in *Cluster) GetCASecretName() string {
	return fmt.Sprintf("%s-ca", in.Name)
}

func (in *Cluster) GetCommonName() string {
	return fmt.Sprintf("%s.%s.svc.cluster.local", in.Name, in.Namespace)
}

func (in *Cluster) GetCAIssuer() *certv1.Issuer {
	return &certv1.Issuer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name,
			Namespace: in.Namespace,
			Finalizers: []string{
				metav1.FinalizerDeleteDependents,
			},
		},
		Spec: certv1.IssuerSpec{
			IssuerConfig: certv1.IssuerConfig{
				CA: &certv1.CAIssuer{
					SecretName: in.GetCASecretName(),
				},
			},
		},
	}
}

func (in *Cluster) GetService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name,
			Namespace: in.Namespace,
			Finalizers: []string{
				metav1.FinalizerDeleteDependents,
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"name": in.Name,
			},
			ClusterIP:                "None",
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{{
				Name: "etcd-server-ssl",
				Port: 2380,
			}, {
				Name: "etcd-client-ssl",
				Port: 2379,
			}},
		},
	}
}

func (in *Cluster) GetBackupSchedule() *BackupSchedule {
	return &BackupSchedule{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name,
			Namespace: in.Namespace,
			Finalizers: []string{
				metav1.FinalizerDeleteDependents,
			},
		},
		Spec: BackupScheduleSpec{
			CreationPeriod:  in.Spec.BackupCreationPeriod,
			RetentionPeriod: in.Spec.BackupRetentionPeriod,
		},
	}
}

func (in *Cluster) GetMember(num int) *Member {
	var members []string

	for num := 0; num < in.Spec.Size; num++ {
		members = append(members, in.GetMemberName(num))
	}

	return &Member{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.GetMemberName(num),
			Namespace: in.Namespace,
			Finalizers: []string{
				metav1.FinalizerDeleteDependents,
			},
		},
		Spec: MemberSpec{
			Version:      in.Spec.Version,
			ClusterToken: string(in.GetUID()),
			ClusterName:  in.Name,
			Members:      members,
			Backup:       in.Spec.Backup,
		},
	}
}

func (in *Cluster) GetMemberName(num int) string {
	return fmt.Sprintf("%s-%d", in.Name, num)
}

func (in *Cluster) ShouldUpdate() bool {
	return in.Status.Phase == ClusterRunning && (in.Status.Version != in.Spec.Version || in.Status.CertificateExpires)
}

func (in Cluster) GetEndpoints() []string {
	var endpoints []string

	for num := 0; num < in.Spec.Size; num++ {
		endpoints = append(endpoints, AdvertiseClientURL(in.GetMemberName(num), in.Namespace, in.Name))
	}

	return endpoints
}

type PrettyClusterSpec struct {
	Size                  int    `json:"size,omitempty" yaml:"size,omitempty"`
	Version               string `json:"version,omitempty" yaml:"version,omitempty"`
	Backup                string `json:"backup,omitempty" yaml:"backup,omitempty"`
	BackupCreationPeriod  string `json:"backupCreationPeriod,omitempty" yaml:"backupCreationPeriod,omitempty"`
	BackupRetentionPeriod string `json:"backupRetentionPeriod,omitempty" yaml:"backupRetentionPeriod,omitempty"`
}

type PrettyCluster struct {
	Name      string            `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace string            `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Spec      PrettyClusterSpec `json:"spec,omitempty" yaml:"spec,omitempty"`
	Status    ClusterStatus     `json:"status,omitempty" yaml:"status,omitempty"`
}

func (in Cluster) Prettify() interface{} {
	return PrettyCluster{
		Name:      in.Name,
		Namespace: in.Namespace,
		Spec: PrettyClusterSpec{
			Version:               in.Spec.Version,
			Size:                  in.Spec.Size,
			Backup:                in.Spec.Backup,
			BackupCreationPeriod:  duration.HumanDuration(in.Spec.BackupCreationPeriod),
			BackupRetentionPeriod: duration.HumanDuration(in.Spec.BackupRetentionPeriod),
		},
		Status: in.Status,
	}
}

//+kubebuilder:object:root=true

// ClusterList contains a list of Cluster
type ClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Cluster `json:"items"`
}

func (in ClusterList) Prettify() interface{} {
	var pretties []interface{}

	for _, cluster := range in.Items {
		pretties = append(pretties, cluster.Prettify())
	}

	return pretties
}

func (in ClusterList) Header() table.Row {
	return table.Row{
		"NAME",
		"CREATED AT",
		"SIZE",
		"VERSION",
		"STATUS",
	}
}

func (in ClusterList) Rows() []table.Row {
	var rows []table.Row

	for _, cluster := range in.Items {
		rows = append(rows, table.Row{
			cluster.Name,
			cluster.CreationTimestamp.Format("2006-01-02 15:04:05"),
			cluster.Spec.Size,
			cluster.Spec.Version,
			cluster.Status.Phase,
		})
	}

	return rows
}

func init() {
	SchemeBuilder.Register(&Cluster{}, &ClusterList{})
}
