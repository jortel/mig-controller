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

package discovery

import (
	"context"
	migapi "github.com/konveyor/mig-controller/pkg/apis/migration/v1alpha1"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/auth"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/container"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/web"
	"github.com/konveyor/mig-controller/pkg/logging"
	"github.com/konveyor/mig-controller/pkg/settings"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"reflect"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"time"
)

var log logging.Logger

// Application settings.
var Settings = &settings.Settings

func init() {
	log = logging.WithName("discovery")
	model.Log = &log
	container.Log = &log
	web.Log = &log
	auth.Log = &log
}

func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	db, err := model.Create()
	if err != nil {
		panic(err)
	}
	restCfg, _ := config.GetConfig()
	if err != nil {
		panic(err)
	}
	nClient, err := client.New(
		restCfg,
		client.Options{
			Scheme: scheme.Scheme,
		})
	if err != nil {
		panic(err)
	}
	container := container.NewContainer(nClient, db)
	web := &web.WebServer{
		Container: container,
	}
	reconciler := ReconcileDiscovery{
		client:    nClient,
		scheme:    mgr.GetScheme(),
		container: container,
		web:       web,
	}

	return &reconciler
}

func add(mgr manager.Manager, r reconcile.Reconciler) error {
	options := controller.Options{
		MaxConcurrentReconciles: 10,
		Reconciler:              r,
	}
	c, err := controller.New("discovery", mgr, options)
	if err != nil {
		log.Trace(err)
		return err
	}
	//
	// Add watches.
	err = c.Watch(
		&source.Kind{
			Type: &migapi.MigCluster{},
		},
		&handler.EnqueueRequestForObject{},
		&ClusterPredicate{})
	if err != nil {
		log.Trace(err)
		return err
	}
	err = c.Watch(
		&source.Kind{
			Type: &migapi.MigPlan{},
		},
		&handler.EnqueueRequestForObject{},
		&PlanPredicate{
			Container: r.(*ReconcileDiscovery).container,
		})
	if err != nil {
		log.Trace(err)
		return err
	}
	//
	// Add the `host` cluster.
	cluster := &migapi.MigCluster{
		Spec: migapi.MigClusterSpec{
			IsHostCluster: true,
		},
	}
	err = r.(*ReconcileDiscovery).container.Add(
		cluster,
		container.Collections{
			&container.RoleBinding{},
			&container.Role{},
		})
	//
	// Start Web
	r.(*ReconcileDiscovery).web.Start()

	return nil
}

var _ reconcile.Reconciler = &ReconcileDiscovery{}

type ReconcileDiscovery struct {
	client    client.Client
	scheme    *runtime.Scheme
	container *container.Container
	web       *web.WebServer
}

func (r *ReconcileDiscovery) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	log.Reset()
	reQueue := reconcile.Result{RequeueAfter: time.Second * 10}
	err := r.container.Prune()
	if err != nil {
		log.Trace(err)
		return reQueue, nil
	}
	cluster := &migapi.MigCluster{}
	err = r.client.Get(context.TODO(), request.NamespacedName, cluster)
	if err != nil {
		if errors.IsNotFound(err) {
			r.container.Delete(request.NamespacedName)
			return reconcile.Result{}, nil
		}
		log.Trace(err)
		return reQueue, nil
	}
	if !r.IsValid(cluster) {
		return reconcile.Result{}, nil
	}
	err = r.container.Add(
		cluster,
		container.Collections{
			&container.RoleBinding{},
			&container.Role{},
			&container.Namespace{},
			&container.Service{},
			&container.PVC{},
			&container.Pod{},
			&container.PV{},
		})
	if err != nil {
		log.Trace(err)
		return reQueue, nil
	}

	return reconcile.Result{}, nil
}

func (r *ReconcileDiscovery) IsValid(cluster *migapi.MigCluster) bool {
	if cluster.Spec.IsHostCluster {
		return true
	}
	ref := cluster.Spec.ServiceAccountSecretRef
	if ref == nil {
		return false
	}
	secret, err := cluster.GetServiceAccountSecret(r.client)
	if err != nil {
		log.Trace(err)
		return false
	}
	if secret == nil {
		return false
	}
	if _, found := secret.Data[migapi.SaToken]; !found {
		return false
	}

	return true
}

//
// Cluster predicate
type ClusterPredicate struct {
}

func (r ClusterPredicate) Create(e event.CreateEvent) bool {
	_, cast := e.Object.(*migapi.MigCluster)
	if !cast {
		return false
	}

	return true
}

func (r ClusterPredicate) Update(e event.UpdateEvent) bool {
	o, cast := e.ObjectOld.(*migapi.MigCluster)
	if !cast {
		return false
	}
	n, cast := e.ObjectNew.(*migapi.MigCluster)
	if !cast {
		return false
	}
	changed := o.Spec.URL != n.Spec.URL ||
		!reflect.DeepEqual(o.Spec.ServiceAccountSecretRef, n.Spec.ServiceAccountSecretRef) ||
		!reflect.DeepEqual(o.Spec.CABundle, n.Spec.CABundle)
	return changed
}

func (r *ClusterPredicate) Delete(e event.DeleteEvent) bool {
	return true
}

func (r *ClusterPredicate) Generic(e event.GenericEvent) bool {
	return false
}

//
// Plan predicate
type PlanPredicate struct {
	Container *container.Container
}

func (r PlanPredicate) Create(e event.CreateEvent) bool {
	log.Reset()
	object, cast := e.Object.(*migapi.MigPlan)
	if !cast {
		return false
	}
	plan := model.Plan{}
	plan.With(object)
	err := plan.Insert(r.Container.Db)
	if err != nil {
		log.Trace(err)
	}

	return false
}

func (r PlanPredicate) Update(e event.UpdateEvent) bool {
	log.Reset()
	o, cast := e.ObjectOld.(*migapi.MigPlan)
	if !cast {
		return false
	}
	n, cast := e.ObjectNew.(*migapi.MigPlan)
	if !cast {
		return false
	}
	changed := !reflect.DeepEqual(
		o.Spec.SrcMigClusterRef,
		n.Spec.SrcMigClusterRef) ||
		!reflect.DeepEqual(
			o.Spec.DestMigClusterRef,
			n.Spec.DestMigClusterRef)
	if changed {
		plan := model.Plan{}
		plan.With(n)
		err := plan.Update(r.Container.Db)
		if err != nil {
			log.Trace(err)
		}
	}

	return false
}

func (r *PlanPredicate) Delete(e event.DeleteEvent) bool {
	log.Reset()
	object, cast := e.Object.(*migapi.MigPlan)
	if !cast {
		return false
	}
	plan := model.Plan{}
	plan.With(object)
	err := plan.Delete(r.Container.Db)
	if err != nil {
		log.Trace(err)
	}

	return false
}

func (r *PlanPredicate) Generic(e event.GenericEvent) bool {
	return false
}
