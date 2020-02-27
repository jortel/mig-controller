package container

import (
	"context"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	rbac "k8s.io/api/rbac/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

//
// A collection of k8s Role resources.
type Role struct {
	// Base
	BaseCollection
}

func (r *Role) AddWatch(dsController controller.Controller) error {
	err := dsController.Watch(
		&source.Kind{
			Type: &rbac.Role{},
		},
		&handler.EnqueueRequestForObject{},
		r)
	if err != nil {
		Log.Trace(err)
		return err
	}
	err = dsController.Watch(
		&source.Kind{
			Type: &rbac.ClusterRole{},
		},
		&handler.EnqueueRequestForObject{},
		r)
	if err != nil {
		Log.Trace(err)
		return err
	}

	return nil
}

func (r *Role) Reconcile() error {
	mark := time.Now()
	sr := SimpleReconciler{
		Db: r.ds.Container.Db,
	}
	err := sr.Reconcile(r)
	if err != nil {
		Log.Trace(err)
		return err
	}
	r.hasReconciled = true
	Log.Info(
		"Role reconciled.",
		"cluster",
		r.ds.Name(),
		"duration",
		time.Since(mark))

	return nil
}

func (r *Role) GetDiscovered() ([]model.Model, error) {
	models := []model.Model{}
	onCluster := rbac.RoleList{}
	err := r.ds.Client.List(context.TODO(), nil, &onCluster)
	if err != nil {
		Log.Trace(err)
		return nil, err
	}
	for _, discovered := range onCluster.Items {
		rb := &model.Role{
			Base: model.Base{
				Cluster: r.ds.Cluster.PK,
			},
		}
		rb.With(&discovered)
		models = append(models, rb)
	}
	onCluster2 := rbac.ClusterRoleList{}
	err = r.ds.Client.List(context.TODO(), nil, &onCluster2)
	if err != nil {
		Log.Trace(err)
		return nil, err
	}
	for _, discovered := range onCluster2.Items {
		rb := &model.Role{
			Base: model.Base{
				Cluster: r.ds.Cluster.PK,
			},
		}
		rb.With2(&discovered)
		models = append(models, rb)
	}

	return models, nil
}

func (r *Role) GetStored() ([]model.Model, error) {
	models := []model.Model{}
	list, err := model.Role{
		Base: model.Base{
			Cluster: r.ds.Cluster.PK,
		},
	}.List(r.ds.Container.Db, model.ListOptions{})
	if err != nil {
		Log.Trace(err)
		return nil, err
	}
	for _, rb := range list {
		models = append(models, rb)
	}

	return models, nil
}

//
// Predicate methods.
//

func (r *Role) Create(e event.CreateEvent) bool {
	Log.Reset()
	role := &model.Role{
		Base: model.Base{
			Cluster: r.ds.Cluster.PK,
		},
	}
	object, cast := e.Object.(*rbac.Role)
	if cast {
		role.With(object)
	} else {
		object, cast := e.Object.(*rbac.ClusterRole)
		if cast {
			role.With2(object)
		} else {
			return false
		}
	}
	r.ds.Create(role)

	return false
}

func (r *Role) Update(e event.UpdateEvent) bool {
	Log.Reset()
	role := &model.Role{
		Base: model.Base{
			Cluster: r.ds.Cluster.PK,
		},
	}
	new, cast := e.ObjectNew.(*rbac.Role)
	if cast {
		role.With(new)
	} else {
		object, cast := e.ObjectNew.(*rbac.ClusterRole)
		if cast {
			role.With2(object)
		} else {
			return false
		}
	}
	r.ds.Update(role)

	return false
}

func (r *Role) Delete(e event.DeleteEvent) bool {
	Log.Reset()
	role := &model.Role{
		Base: model.Base{
			Cluster: r.ds.Cluster.PK,
		},
	}
	object, cast := e.Object.(*rbac.Role)
	if cast {
		role.With(object)
	} else {
		object, cast := e.Object.(*rbac.ClusterRole)
		if cast {
			role.With2(object)
		} else {
			return false
		}
	}
	r.ds.Delete(role)

	return false
}

func (r *Role) Generic(e event.GenericEvent) bool {
	return false
}
