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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BackupScheduleSpec defines the desired state of BackupSchedule
type BackupScheduleSpec struct {
	CreationPeriod  time.Duration `json:"creationPeriod,omitempty"`
	RetentionPeriod time.Duration `json:"retentionPeriod,omitempty"`
}

// BackupScheduleStatus defines the observed state of BackupSchedule
type BackupScheduleStatus struct {
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BackupSchedule is the Schema for the backupschedules API
type BackupSchedule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupScheduleSpec   `json:"spec,omitempty"`
	Status BackupScheduleStatus `json:"status,omitempty"`
}

func (in *BackupSchedule) GetBackup() *Backup {
	return &Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-%d", in.Name, time.Now().Unix()),
			Namespace: in.Namespace,
			Labels: map[string]string{
				ClusterLabel: in.Name,
			},
		},
		Spec: BackupSpec{
			RetentionPeriod: in.Spec.RetentionPeriod,
		},
	}
}

//+kubebuilder:object:root=true

// BackupScheduleList contains a list of BackupSchedule
type BackupScheduleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupSchedule `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupSchedule{}, &BackupScheduleList{})
}
