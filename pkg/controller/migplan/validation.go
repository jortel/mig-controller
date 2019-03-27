package migplan

import (
	"context"
	migapi "github.com/fusor/mig-controller/pkg/apis/migration/v1alpha1"
	"k8s.io/apimachinery/pkg/types"
)

func (r ReconcileMigPlan) validate(plan *migapi.MigPlan) error {
	status := plan.Status.Validation
	status.Reset()
	r.validateStorage(plan)
	return nil
}

func (r ReconcileMigPlan) validateStorage(plan *migapi.MigPlan) error {
	// Reference:nil
	status := plan.Status.Validation
	if plan.Spec.MigStorageRef == nil {
		status.Failed("Storage not specified.")
		return nil
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
		if storage.Status.Validation.Invalid {
			status.Failed("Storage is invalid.")
		}
	} else {
		status.Failed("Storage not found.")
	}

	return err
}
