package container

import (
	"context"
	"database/sql"
	migapi "github.com/fusor/mig-controller/pkg/apis/migration/v1alpha1"
	"github.com/fusor/mig-controller/pkg/controller/discovery/model"
	"github.com/fusor/mig-controller/pkg/logging"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sync"
)

// Shared logger.
var Log *logging.Logger

//
// DataSource Key
type DsKey types.NamespacedName

//
// Container of DataSources.
// Each DataSource corresponds to a MigCluster.
// The container is responsible for the lifecycle of DataSources based
// on add,update,delete of MigCluster.
type Container struct {
	// A k8s client for the host cluster.
	Client client.Client
	// A database connection.
	Db *sql.DB
	// The mapping of MigClusters to: DataSource.
	sources map[DsKey]*DataSource
	// Delete model.Cluster for MigCluster deleted while
	// the container was stopped.
	pruned bool
	// Protect the map.
	mutex sync.RWMutex
}

//
// Construct a new container.
func NewContainer(cnt client.Client, db *sql.DB) *Container {
	return &Container{
		sources: map[DsKey]*DataSource{},
		Client:  cnt,
		Db:      db,
	}
}

//
// Get the DataSource for a cluster.
func (r *Container) GetDs(cluster *model.Cluster) (*DataSource, bool) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	key := DsKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	ds, found := r.sources[key]
	return ds, found
}

//
// Add a cluster to the container.
// Build/update a DataSource for the cluster as needed.
func (r *Container) Add(cluster *migapi.MigCluster) error {
	// Build
	r.mutex.RLock()
	key := DsKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if ds, found := r.sources[key]; found {
		ds.Stop(false)
	}
	ds := &DataSource{
		Container:   r,
		StopChannel: make(chan struct{}),
		Collections: Collections{
			&NsCollection{},
			&PvCollection{},
			&PodCollection{},
		},
	}
	r.sources[key] = ds
	r.mutex.RUnlock()
	// Start
	restCfg, err := cluster.BuildRestConfig(r.Client)
	if err != nil {
		Log.Trace(err)
		return err
	}
	ds.RestCfg = restCfg
	dsManager, err := manager.New(restCfg, manager.Options{})
	if err != nil {
		Log.Trace(err)
		return err
	}
	dsController, err := controller.New(
		cluster.Name,
		dsManager,
		controller.Options{
			Reconciler: ds,
		})
	if err != nil {
		Log.Trace(err)
		return err
	}
	err = ds.AddWatches(dsController)
	if err != nil {
		Log.Trace(err)
		return err
	}
	err = ds.Start(cluster)
	if err != nil {
		Log.Trace(err)
		return err
	}

	go dsManager.Start(ds.StopChannel)

	return nil
}

//
// Delete the DataSrource for a deleted MigCluster.
func (r *Container) Delete(cluster types.NamespacedName) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	key := DsKey{
		Namespace: cluster.Namespace,
		Name:      cluster.Name,
	}
	if ds, found := r.sources[key]; found {
		Log.Info("Deleted.", "cluster", key)
		delete(r.sources, key)
		ds.Stop(true)
	}
}

//
// Prune (delete) clusters in the DB that no longer exist.
// Intended to call called once on container initialization.
func (r *Container) Prune() error {
	if r.pruned {
		return nil
	}
	stored, err := model.ClusterList(r.Db, nil)
	if err != nil {
		Log.Trace(err)
		return err
	}
	list := migapi.MigClusterList{}
	err = r.Client.List(context.TODO(), nil, &list)
	if err != nil {
		Log.Trace(err)
		return err
	}
	wanted := map[string]bool{}
	for _, cluster := range list.Items {
		wanted[string(cluster.UID)] = true
	}
	for _, cluster := range stored {
		if _, found := wanted[cluster.UID]; !found {
			cluster.Delete(r.Db)
		}
	}

	r.pruned = true

	return nil
}

//
// Determine of a MigCluster actually exists.
func (r *Container) HasCluster(cluster *model.Cluster) (bool, error) {
	found := migapi.MigCluster{}
	err := r.Client.Get(
		context.TODO(),
		types.NamespacedName{
			Namespace: cluster.Namespace,
			Name:      cluster.Name,
		},
		&found)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		} else {
			Log.Trace(err)
			return false, err
		}
	}

	return true, nil
}
