package container

import (
	"database/sql"
	"errors"
	migapi "github.com/konveyor/mig-controller/pkg/apis/migration/v1alpha1"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	"k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"strings"
	"time"
)

type Collections []Collection

//
// A DataSource corresponds to a MigCluster and is
// responsible for maintaining a k8s:
//   - Manager/controller
//   - REST configuration
//   - Client
// Each contains a set of `Collection`.
type DataSource struct {
	// The associated (owner) container.
	Container *Container
	// Collections.
	Collections Collections
	// The REST configuration for the cluster.
	RestCfg *rest.Config
	// The k8s client for the cluster.
	Client client.Client
	// The corresponding cluster in the DB.
	Cluster model.Cluster
	// The k8s manager.
	manager controllerruntime.Manager
	// The k8s manager/controller `stop` channel.
	stopChannel chan struct{}
	// Model event channel.
	eventChannel chan ModelEvent
	// The model version threshold used to determine if a
	// model event is obsolete. An event (model) with a version
	// lower than the threshold is redundant to changes made
	// during collection reconciliation.
	versionThreshold uint64
	// Heartbeat monitor.
	// Monitor health of watches.
	heartbeat HeartbeatMonitor
}

//
// The DataSource name.
func (r *DataSource) Name() string {
	return strings.Join([]string{r.Cluster.Namespace, r.Cluster.Name}, "/")
}

//
// Determine if the DataSource is `ready`.
// The DataSource is `ready` when all of the collections are `ready`.
func (r *DataSource) IsReady() bool {
	if !r.heartbeat.Healthy() {
		return false
	}
	for _, collection := range r.Collections {
		if !collection.IsReady() {
			return false
		}
	}

	return true
}

//
// The k8s reconcile loop.
// Implements the k8s Reconciler interface. The DataSource is the reconciler
// for the container k8s manager/controller but is should never be called. The design
// is for watches added by each collection reference a predicate that handles the change
// rather than queuing a reconcile event.
func (r *DataSource) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}

//
// Start the DataSource.
//   - Create the cluster in the DB.
//   - Create a k8s client.
//   - Reconcile each collection.
func (r *DataSource) Start(cluster *migapi.MigCluster) error {
	var err error
	r.versionThreshold = 0
	r.eventChannel = make(chan ModelEvent, 100)
	r.stopChannel = make(chan struct{})
	for _, collection := range r.Collections {
		collection.Bind(r)
	}
	r.Cluster = model.Cluster{}
	r.Cluster.With(cluster)
	err = r.Cluster.Insert(r.Container.Db)
	if err != nil {
		Log.Trace(err)
		return err
	}
	mark := time.Now()
	err = r.buildClient(cluster)
	if err != nil {
		Log.Trace(err)
		return err
	}
	connectDuration := time.Since(mark)
	mark = time.Now()
	err = r.buildManager()
	if err != nil {
		Log.Trace(err)
		return err
	}
	go r.manager.Start(r.stopChannel)
	for _, collection := range r.Collections {
		err = collection.Reconcile()
		if err != nil {
			Log.Trace(err)
			return err
		}
	}
	go r.applyEvents()

	startDuration := time.Since(mark)

	Log.Info(
		"DataSource Started.",
		"name",
		r.Name(),
		"connected",
		connectDuration,
		"reconciled",
		startDuration)

	return nil
}

//
// Stop the DataSource.
// Stop the associated k8s manager/controller and delete all
// of the associated data in the DB. The data should be deleted
// when the DataSource is not being restarted.
func (r *DataSource) Stop(purge bool) {
	close(r.stopChannel)
	close(r.eventChannel)
	for _, collection := range r.Collections {
		collection.Reset()
	}
	if purge {
		r.Cluster.Delete(r.Container.Db)
	}

	Log.Info(
		"DataSource Stopped.",
		"ns",
		r.Cluster.Namespace,
		"name",
		r.Cluster.Name)
}

// The specified model has been discovered.
// The `versionThreshold` will be updated as needed.
func (r *DataSource) HasDiscovered(m model.Model) {
	version := m.Meta().Version
	if version > r.versionThreshold {
		r.versionThreshold = version
	}
}

//
// Enqueue create model event.
// Used by watch predicates.
// Swallow panic: send on closed channel.
func (r *DataSource) Create(m model.Model) {
	defer func() {
		if p := recover(); p != nil {
			Log.Info("channel send failed")
		}
	}()
	r.heartbeat.Received()
	r.eventChannel <- ModelEvent{}.Create(m)
}

//
// Enqueue update model event.
// Used by watch predicates.
// Swallow panic: send on closed channel.
func (r *DataSource) Update(m model.Model) {
	defer func() {
		if p := recover(); p != nil {
			Log.Info("channel send failed")
		}
	}()
	r.heartbeat.Received()
	r.eventChannel <- ModelEvent{}.Update(m)
}

//
// Enqueue delete model event.
// Used by watch predicates.
// Swallow panic: send on closed channel.
func (r *DataSource) Delete(m model.Model) {
	defer func() {
		if p := recover(); p != nil {
			Log.Info("channel send failed")
		}
	}()
	r.heartbeat.Received()
	r.eventChannel <- ModelEvent{}.Delete(m)
}

//
// Build k8s client.
func (r *DataSource) buildClient(cluster *migapi.MigCluster) error {
	var err error
	r.RestCfg, err = cluster.BuildRestConfig(r.Container.Client)
	if err != nil {
		Log.Trace(err)
		return err
	}
	r.Client, err = client.New(
		r.RestCfg,
		client.Options{
			Scheme: scheme.Scheme,
		})
	if err != nil {
		Log.Trace(err)
		return err
	}

	return nil
}

//
// Build the k8s manager.
func (r *DataSource) buildManager() error {
	var err error
	r.manager, err = manager.New(r.RestCfg, manager.Options{})
	if err != nil {
		Log.Trace(err)
		return err
	}
	dsController, err := controller.New(
		"DataSource",
		r.manager,
		controller.Options{
			Reconciler: r,
		})
	if err != nil {
		Log.Trace(err)
		return err
	}
	r.heartbeat = HeartbeatMonitor{
		threshold: time.Second * 10,
	}
	err = r.heartbeat.AddWatch(dsController)
	if err != nil {
		Log.Trace(err)
		return err
	}
	for _, collection := range r.Collections {
		err := collection.AddWatch(dsController)
		if err != nil {
			Log.Trace(err)
			return err
		}
	}

	return nil
}

//
// Apply model events.
func (r *DataSource) applyEvents() {
	for event := range r.eventChannel {
		err := event.Apply(r.Container.Db, r.versionThreshold)
		if err != nil {
			Log.Trace(err)
		}
	}
}

//
// Model event.
// Used with `eventChannel`.
type ModelEvent struct {
	// Model the changed.
	model model.Model
	// Action performed on the model:
	//   0x01 Create.
	//   0x02 Update.
	//   0x04 Delete.
	action byte
}

//
// Apply the change to the DB.
func (r *ModelEvent) Apply(db *sql.DB, versionThreshold uint64) error {
	tx, err := db.Begin()
	if err != nil {
		Log.Trace(err)
		return err
	}
	version := r.model.Meta().Version
	switch r.action {
	case 0x01: // Create
		if version > versionThreshold {
			err := r.model.Insert(tx)
			if err != nil {
				Log.Trace(err)
				return err
			}
		}
	case 0x02: // Update
		if version > versionThreshold {
			err := r.model.Update(tx)
			if err != nil {
				Log.Trace(err)
				return err
			}
		}
	case 0x04: // Delete
		err := r.model.Delete(tx)
		if err != nil {
			Log.Trace(err)
			return err
		}
	default:
		return errors.New("unknown action")
	}
	err = tx.Commit()
	if err != nil {
		Log.Trace(err)
		return err
	}

	return nil
}

//
// Set the event model and action.
func (r ModelEvent) Create(m model.Model) ModelEvent {
	r.model = m
	r.action = 0x01
	return r
}

//
// Set the event model and action.
func (r ModelEvent) Update(m model.Model) ModelEvent {
	r.model = m
	r.action = 0x02
	return r
}

//
// Set the event model and action.
func (r ModelEvent) Delete(m model.Model) ModelEvent {
	r.model = m
	r.action = 0x04
	return r
}

//
// Heartbeat
type HeartbeatMonitor struct {
	threshold time.Duration
	// Last observed heartbeat.
	lastHeartbeat time.Time
}

//
// Add required watches.
func (r *HeartbeatMonitor) AddWatch(dsController controller.Controller) error {
	return dsController.Watch(
		&source.Kind{
			Type: &v1.Node{},
		},
		&handler.EnqueueRequestForObject{},
		r)
}

//
// Heartbeat is healthy.
func (r *HeartbeatMonitor) Healthy() bool {
	if time.Since(r.lastHeartbeat) > r.threshold {
		return false
	}

	return true
}

func (r *HeartbeatMonitor) Received() {
	r.lastHeartbeat = time.Now()
}

func (r *HeartbeatMonitor) Create(e event.CreateEvent) bool {
	r.Received()
	return false
}

func (r *HeartbeatMonitor) Update(e event.UpdateEvent) bool {
	r.Received()
	return false
}

func (r *HeartbeatMonitor) Delete(e event.DeleteEvent) bool {
	r.Received()
	return false
}

func (r *HeartbeatMonitor) Generic(e event.GenericEvent) bool {
	r.Received()
	return false
}
