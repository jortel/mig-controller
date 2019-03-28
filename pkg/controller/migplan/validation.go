package migplan

import (
	"context"
	migapi "github.com/fusor/mig-controller/pkg/apis/migration/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

// Types
const (
	// Type
	Ready = "Ready"
	InvalidSourceClusterRef = "InvalidSourceClusterRef"
	InvalidDestinationClusterRef = "InvalidDestinationClusterRef"
	InvalidStorageRef = "InvalidStorageRef"
	InvalidAssetCollectionRef = "InvalidAssetCollectionRef"
	SourceClusterNotReady = "SourceClusterNotReady"
	DestinationClusterNotReady = "DestinationClusterNotReady"
	StorageNotReady = "StorageNotReady"
	AssetCollectionNotReady = "AssetCollectionNotReady"
)
// Reasons
const (
	NotSet = "NotSet"
	NotFound = "NotFound"
)
// Status
const (
	True = "True"
	False = "False"
)

func (r ReconcileMigPlan) validate(plan *migapi.MigPlan) (error, int) {
	total := 0
	err, count := r.validateStorage(plan)
	if err != nil {
		return err, total
	}
	err = r.Update(context.TODO(), plan)
	return err, count
}

func (r ReconcileMigPlan) validateStorage(plan *migapi.MigPlan) (error, int) {
	count := 0

	plan.Status.ClearConditions(InvalidStorageRef, StorageNotReady)

	// NotSet
	if plan.Spec.MigStorageRef == nil {
		condition := migapi.Condition{
			Type: InvalidStorageRef,
			Status: True,
			Reason: NotSet,
		}
		plan.Status.SetCondition(condition)
		count++
	}

	// Exists
	ref := plan.Spec.MigStorageRef
	storage := migapi.MigStorage{}
	name := types.NamespacedName{
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}
	err := r.Get(context.TODO(), name, &storage)
	if err != nil {
		// NotReady
		_, ready := storage.Status.FindCondition(Ready)
		if ready == nil {
			condition := migapi.Condition{
				Type: StorageNotReady,
				Status: True,
			}
			plan.Status.SetCondition(condition)
			count++
		}
	} else {
		// NotFound
		condition := migapi.Condition{
			Type: InvalidStorageRef,
			Status: True,
			Reason: NotFound,
		}
		plan.Status.SetCondition(condition)
		count++
	}

	return err, count
}
