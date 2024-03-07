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
	"path"
	"strings"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	cmmeta "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// MemberSpec defines the desired state of an etcd cluster member
type MemberSpec struct {
	Version           string   `json:"version,omitempty"`
	Backup            string   `json:"backup,omitempty"`
	ClusterName       string   `json:"clusterName,omitempty"`
	ClusterToken      string   `json:"clusterToken,omitempty"`
	Members           []string `json:"members,omitempty"`
	Broken            bool     `json:"broken,omitempty"`
	CertificateUpdate bool     `json:"certificateUpdate,omitempty"`
}

// MemberStatus defines the observed state of an etcd cluster member
type MemberStatus struct {
	Version            string      `json:"version,omitempty"`
	Phase              MemberPhase `json:"phase,omitempty"`
	FailedTime         metav1.Time `json:"failedTime,omitempty"`
	CertificateExpires bool        `json:"certificateExpires,omitempty"`
}

// MemberPhase defines status of specific etcd cluster member
type MemberPhase string

var (
	MemberCreating   MemberPhase = "Creating"
	MemberRecreating MemberPhase = "Recreating"
	MemberRunning    MemberPhase = "Running"
	MemberUpdating   MemberPhase = "Updating"
	MemberFailed     MemberPhase = "Failed"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Member is the Schema for the members API
type Member struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MemberSpec   `json:"spec,omitempty"`
	Status MemberStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MemberList contains a list of Member
type MemberList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Member `json:"items"`
}

func (in Member) GetCertificate(suffix string) *certv1.Certificate {
	return &certv1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.GetCertificateName(suffix),
			Namespace: in.Namespace,
			Finalizers: []string{
				metav1.FinalizerDeleteDependents,
			},
		},
		Spec: certv1.CertificateSpec{
			SecretName: fmt.Sprintf("%s-%s", in.Name, suffix),
			SecretTemplate: &certv1.CertificateSecretTemplate{
				Labels: map[string]string{
					ClusterLabel: in.Spec.ClusterName,
				},
			},
			PrivateKey: &certv1.CertificatePrivateKey{
				RotationPolicy: certv1.RotationPolicyAlways,
				Algorithm:      certv1.RSAKeyAlgorithm,
				Encoding:       certv1.PKCS1,
				Size:           2048,
			},
			IssuerRef: cmmeta.ObjectReference{
				Name:  in.Spec.ClusterName,
				Kind:  "Issuer",
				Group: "cert-manager.io",
			},
			DNSNames: []string{
				in.GetFQDN(),
			},
		},
	}
}

func (in Member) GetCertificateName(suffix string) string {
	return fmt.Sprintf("%s-%s", in.Name, suffix)
}

func (in Member) GetPVC() *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name,
			Namespace: in.Namespace,
			Labels: map[string]string{
				"name": in.Spec.ClusterName,
			},
			Finalizers: []string{
				metav1.FinalizerDeleteDependents,
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			Resources: corev1.ResourceRequirements{
				Requests: map[corev1.ResourceName]resource.Quantity{
					"storage": resource.MustParse("30Gi"),
				},
			},
		},
	}
}

func (in Member) GetPod() *corev1.Pod {
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.Name,
			Namespace: in.Namespace,
			Labels: map[string]string{
				"name": in.Spec.ClusterName,
			},
			Finalizers: []string{
				metav1.FinalizerDeleteDependents,
			},
		},
		Spec: corev1.PodSpec{
			Hostname:       in.Name,
			Subdomain:      in.Spec.ClusterName,
			InitContainers: in.GetInitContainers(),
			Containers:     in.GetContainers(),
			Volumes: []corev1.Volume{{
				Name: in.GetDataVolumeName(),
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: in.Name,
					},
				},
			}, {
				Name: in.GetPeerCertVolumeName(),
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: in.GetPeerCertSecret(),
					},
				},
			}, {
				Name: in.GetClientCertVolumeName(),
				VolumeSource: corev1.VolumeSource{
					Secret: &corev1.SecretVolumeSource{
						SecretName: in.GetClientCertSecret(),
					},
				},
			}},
		},
	}
}

func (in Member) GetInitContainers() []corev1.Container {
	if in.Spec.Backup == "" || in.Status.Phase == MemberRecreating {
		return nil
	}

	return []corev1.Container{{
		Name:  "etcd-restore",
		Image: in.GetImage(),
		Command: []string{
			"/usr/local/bin/etcdctl",
		},
		Args: []string{
			"snapshot",
			"restore",
			"snapshot.db",
			"--name", in.Name,
			"--initial-cluster", in.GetCluster(),
			"--initial-cluster-token", in.Spec.ClusterToken,
			"--initial-advertise-peer-urls", in.GetAdvertisePeerURL(),
			"--data-dir", in.GetDataPath(),
		},
		VolumeMounts: []corev1.VolumeMount{{
			MountPath: in.GetDataPath(),
			Name:      in.GetDataVolumeName(),
		}},
	}}
}

func (in Member) GetContainers() []corev1.Container {
	return []corev1.Container{{
		Name:           "etcd",
		Image:          in.GetImage(),
		ReadinessProbe: in.GetProbe(),
		Ports: []corev1.ContainerPort{{
			ContainerPort: 2379,
		}, {
			ContainerPort: 2380,
		}},
		Command: []string{
			"/usr/local/bin/etcd",
		},
		Args: []string{
			"--name", in.Name,
			"--initial-advertise-peer-urls", in.GetAdvertisePeerURL(),
			"--listen-peer-urls", "https://0.0.0.0:2380",
			"--advertise-client-urls", in.GetAdvertiseClientURL(),
			"--listen-client-urls", "https://0.0.0.0:2379",
			"--initial-cluster", in.GetCluster(),
			"--initial-cluster-state", in.GetState(),
			"--initial-cluster-token", in.Spec.ClusterToken,
			"--data-dir", in.GetDataPath(),
			"--peer-client-cert-auth",
			"--peer-trusted-ca-file", path.Join(in.GetPeerCertPath(), "ca.crt"),
			"--peer-cert-file", path.Join(in.GetPeerCertPath(), "tls.crt"),
			"--peer-key-file", path.Join(in.GetPeerCertPath(), "tls.key"),
			"--cert-file", path.Join(in.GetClientCertPath(), "tls.crt"),
			"--key-file", path.Join(in.GetClientCertPath(), "tls.key"),
		},
		VolumeMounts: []corev1.VolumeMount{{
			MountPath: in.GetDataPath(),
			Name:      in.GetDataVolumeName(),
		}, {
			MountPath: in.GetPeerCertPath(),
			Name:      in.GetPeerCertVolumeName(),
		}, {
			MountPath: in.GetClientCertPath(),
			Name:      in.GetClientCertVolumeName(),
		}},
	}}
}

func (in Member) GetDataVolumeName() string {
	return "data"
}

func (in Member) GetDataPath() string {
	return "/var/lib/etcd"
}

func (in Member) GetPeerCertVolumeName() string {
	return "peer-cert"
}

func (in Member) GetPeerCertSecret() string {
	return fmt.Sprintf("%s-peer", in.Name)
}

func (in Member) GetPeerCertPath() string {
	return "/var/lib/ssl/peer"
}

func (in Member) GetClientCertVolumeName() string {
	return "client-cert"
}

func (in Member) GetClientCertPath() string {
	return "/var/lib/ssl/client"
}

func (in Member) GetClientCertSecret() string {
	return fmt.Sprintf("%s-client", in.Name)
}

func (in Member) GetProbe() *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path:   "/health",
				Port:   intstr.FromInt(2379),
				Scheme: corev1.URISchemeHTTPS,
			},
		},
	}
}

func (in Member) GetImage() string {
	return fmt.Sprintf("quay.io/coreos/etcd:v%s", in.Spec.Version)
}

func (in Member) GetAdvertisePeerURL() string {
	return AdvertisePeerURL(in.Name, in.Namespace, in.Spec.ClusterName)
}

func (in Member) GetAdvertiseClientURL() string {
	return AdvertiseClientURL(in.Name, in.Namespace, in.Spec.ClusterName)
}

func (in Member) GetFQDN() string {
	return MemberFQDN(in.Name, in.Namespace, in.Spec.ClusterName)
}

func (in Member) GetCluster() string {
	var urls []string

	for _, member := range in.Spec.Members {
		urls = append(urls, fmt.Sprintf("%s=%s", member, AdvertisePeerURL(member, in.Namespace, in.Spec.ClusterName)))
	}

	return strings.Join(urls, ",")
}

func (in Member) GetEndpoints() []string {
	var endpoints []string

	for _, member := range in.Spec.Members {
		endpoints = append(endpoints, AdvertiseClientURL(member, in.Namespace, in.Spec.ClusterName))
	}

	return endpoints
}

func (in *Member) SetFailed() {
	if in.Status.Phase == MemberFailed {
		return
	}

	in.Status.FailedTime = metav1.Now()
	in.Status.Phase = MemberFailed
}

func (in Member) IsCreating() bool {
	return in.Status.Phase == MemberCreating || in.Status.Phase == MemberRecreating || in.Status.Phase == ""
}

func (in Member) ShouldUpdate() bool {
	return in.Status.Phase == MemberRunning &&
		(in.Status.Version != in.Spec.Version ||
			(in.Spec.CertificateUpdate && in.Status.CertificateExpires))
}

func (in Member) GetState() string {
	if in.Status.Phase == MemberRecreating {
		return "existing"
	}

	return "new"
}

func AdvertisePeerURL(name, namespace, service string) string {
	return fmt.Sprintf("https://%s:2380", MemberFQDN(name, namespace, service))
}

func AdvertiseClientURL(name, namespace, service string) string {
	return fmt.Sprintf("https://%s:2379", MemberFQDN(name, namespace, service))
}

func MemberFQDN(name, namespace, service string) string {
	return fmt.Sprintf("%s.%s.%s.svc.cluster.local", name, service, namespace)
}

func init() {
	SchemeBuilder.Register(&Member{}, &MemberList{})
}
