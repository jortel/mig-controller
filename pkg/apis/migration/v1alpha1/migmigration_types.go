/*
Copyright 2019 Red Hat Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"time"

	kapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
)

var log = logf.Log.WithName("controller")

// MigMigrationSpec defines the desired state of MigMigration
type MigMigrationSpec struct {
	MigPlanRef *kapi.ObjectReference `json:"migPlanRef,omitempty"`
}

// MigMigrationStatus defines the observed state of MigMigration
type MigMigrationStatus struct {
	Conditions

	MigrationRunning   bool `json:"migrationStarted,omitempty"`
	MigrationCompleted bool `json:"migrationCompleted,omitempty"`

	StartTimestamp      *metav1.Time `json:"startTimestamp,omitempty"`
	CompletionTimestamp *metav1.Time `json:"completionTimestamp,omitempty"`

	SrcBackupRef   *kapi.ObjectReference `json:"srcBackupRef,omitempty"`
	DestRestoreRef *kapi.ObjectReference `json:"destRestoreRef,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MigMigration is the Schema for the migmigrations API
// +k8s:openapi-gen=true
type MigMigration struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MigMigrationSpec   `json:"spec,omitempty"`
	Status MigMigrationStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MigMigrationList contains a list of MigMigration
type MigMigrationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MigMigration `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MigMigration{}, &MigMigrationList{})
}

func (r *MigMigration) GetCorrelationLabels() map[string]string {
	return buildCorrelationLabels(r, r.UID)
}

func (r *MigMigration) GetNamespace() string {
	return r.Namespace
}

func (r *MigMigration) GetName() string {
	return r.Name
}

// GetPlan - Get the migration plan.
// Returns `nil` when the reference cannot be resolved.
func (r *MigMigration) GetPlan(client k8sclient.Client) (*MigPlan, error) {
	return GetPlan(client, r.Spec.MigPlanRef)
}

// MarkAsRunning marks the MigMigration status as 'Running'. Returns true if changed.
func (r *MigMigration) MarkAsRunning() bool {
	if r.Status.MigrationCompleted == true || r.Status.MigrationRunning == true {
		return false
	}
	r.Status.MigrationRunning = true
	r.Status.MigrationCompleted = false
	r.Status.StartTimestamp = &metav1.Time{Time: time.Now()}
	return true
}

// MarkAsCompleted marks the MigMigration status as 'Completed'. Returns true if changed.
func (r *MigMigration) MarkAsCompleted() bool {
	if r.Status.MigrationRunning == false || r.Status.MigrationCompleted == true {
		return false
	}
	r.Status.MigrationRunning = false
	r.Status.MigrationCompleted = true
	r.Status.CompletionTimestamp = &metav1.Time{Time: time.Now()}
	return true
}
