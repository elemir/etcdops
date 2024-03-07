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
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/duration"
)

// BackupSpec defines the desired state of Backup
type BackupSpec struct {
	RetentionPeriod time.Duration `json:"retentionPeriod,omitempty"`
}

// BackupStatus defines the observed state of Backup
type BackupStatus struct {
	Finished metav1.Time `json:"finishedTime,omitempty"`
	URL      string      `json:"url,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// Backup is the Schema for the backups API
type Backup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupSpec   `json:"spec,omitempty"`
	Status BackupStatus `json:"status,omitempty"`
}

type PrettyBackup struct {
	Name            string `json:"name,omitempty" yaml:"name,omitempty"`
	Namespace       string `json:"namespace,omitempty" yaml:"namespace,omitempty"`
	Cluster         string `json:"cluster,omitempty" yaml:"cluster,omitempty"`
	RetentionPeriod string `json:"retentionPeriod,omitempty" yaml:"retentionPeriod,omitempty"`
	Finished        string `json:"finished,omitempty" yaml:"finished,omitempty"`
	URL             string `json:"url,omitempty" yaml:"url,omitempty"`
}

func (in Backup) Prettify() interface{} {
	return PrettyBackup{
		Name:            in.Name,
		Namespace:       in.Namespace,
		Cluster:         in.Labels[ClusterLabel],
		RetentionPeriod: duration.HumanDuration(in.Spec.RetentionPeriod),
		Finished:        in.Status.Finished.Format("2006-01-02 15:04:05"),
		URL:             in.Status.URL,
	}
}

//+kubebuilder:object:root=true

// BackupList contains a list of Backup
type BackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backup `json:"items"`
}

func (in BackupList) Prettify() interface{} {
	var pretties []interface{}

	for _, backup := range in.Items {
		pretties = append(pretties, backup.Prettify())
	}

	return pretties
}

func (in BackupList) Header() table.Row {
	return table.Row{
		"NAME",
		"CLUSTER",
		"FINISHED AT",
		"RETENTION",
		"URL",
	}
}

func (in BackupList) Rows() []table.Row {
	var rows []table.Row

	for _, backup := range in.Items {
		rows = append(rows, table.Row{
			backup.Name,
			backup.Labels[ClusterLabel],
			backup.Status.Finished.Format("2006-01-02 15:04:05"),
			duration.HumanDuration(backup.Spec.RetentionPeriod),
			backup.Status.URL,
		})
	}

	return rows
}

func init() {
	SchemeBuilder.Register(&Backup{}, &BackupList{})
}
