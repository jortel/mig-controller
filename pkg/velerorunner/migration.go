package velerorunner

import (
	"context"
	migapi "github.com/fusor/mig-controller/pkg/apis/migration/v1alpha1"
	"github.com/go-logr/logr"
	velero "github.com/heptio/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"reflect"
	k8sclient "sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

var VeleroNamespace = "velero"


type Task struct {
	Log *logr.Logger
	Client k8sclient.Client
	Owner migapi.MigResource
	PlanResources *migapi.PlanRefResources
	BackupResources []string
	Namespace string
	BackupName string
	RestoreName string
}

// task := velerorunner.Task{
//             Log: log,
//             Client: r,
//             Owner: migration,
//             PlanResources: plan.GetPlanResources(),
//         }
// err := task.EnsureBackup()
//
// err := task.EnsureRestore()

func (t Task) EnsureBackup() error {
	newBackup := t.BuildBackup()
	foundBackup, err := t.GetBackup()
	if err != nil {
		return err
	}
	if foundBackup == nil {
		t.BackupName = newBackup.Name
		err := t.Client.Create(context.TODO(), newBackup)
		if err != nil {
			return err
		}
	}
	if !t.EqualsBackup(newBackup, foundBackup) {
		t.BackupName = foundBackup.Name
		t.UpdateBackup(foundBackup)
		err := t.Client.Update(context.TODO(), foundBackup)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t Task) EqualsBackup(a, b *velero.Backup) bool {
	return reflect.DeepEqual(a.Spec, b.Spec)
}

func (t Task) GetBackup() (*velero.Backup, error) {
	cluster := t.PlanResources.SrcMigCluster
	client, err  := cluster.GetClient(t.Client)
	labels := t.Owner.GetCorrelationLabels()
	list := velero.BackupList{}
	err = client.List(
		context.TODO(),
		k8sclient.MatchingLabels(labels),
		&list)
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 0 {
		return &list.Items[0], nil
	}

	return nil, nil
}

func (t Task) GetBSL() (*velero.BackupStorageLocation, error) {
	storage := t.PlanResources.MigStorage
	cluster := t.PlanResources.SrcMigCluster
	client, err  := cluster.GetClient(t.Client)
	if err != nil {
		return nil, err
	}
	location, err := storage.GetBSL(client)
	if err != nil {
		return nil, err
	}

	return location, nil
}

func (t Task) GetVSL() (*velero.VolumeSnapshotLocation, error) {
	storage := t.PlanResources.MigStorage
	cluster := t.PlanResources.SrcMigCluster
	client, err  := cluster.GetClient(t.Client)
	if err != nil {
		return nil, err
	}
	location, err := storage.GetVSL(client)
	if err != nil {
		return nil, err
	}

	return location, nil
}

func (t Task) BuildBackup() *velero.Backup {
	backup := &velero.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Labels: t.Owner.GetCorrelationLabels(),
			GenerateName: t.Owner.GetName()+"-",
			Namespace:    t.Namespace,
		},
	}
	t.UpdateBackup(backup)
	return backup
}

func (t Task) UpdateBackup(backup *velero.Backup) error {
	namespaces := t.PlanResources.MigAssets.Spec.Namespaces
	backupLocation, err := t.GetBSL()
	if err != nil {
		return err
	}
	backup.Spec = velero.BackupSpec{
		StorageLocation:    backupLocation.Name,
		VolumeSnapshotLocations: []string{"default-aws"},
		TTL:                metav1.Duration{Duration: 720 * time.Hour},
		IncludedNamespaces: namespaces,
		ExcludedNamespaces: []string{},
		IncludedResources:  t.BackupResources,
		ExcludedResources:  []string{},
		Hooks:              velero.BackupHooks{
			                    Resources: []velero.BackupResourceHookSpec{},
	                        },
	}
	return nil
}


func (t Task) EnsureRestore() error {
	newRestore := t.BuildRestore()
	foundRestore, err := t.GetRestore()
	if err != nil {
		return err
	}
	if foundRestore == nil {
		t.RestoreName = newRestore.Name
		err := t.Client.Create(context.TODO(), newRestore)
		if err != nil {
			return err
		}
	}
	if !t.EqualsRestore(newRestore, foundRestore) {
		t.BackupName = foundRestore.Name
		t.UpdateRestore(foundRestore)
		err := t.Client.Update(context.TODO(), foundRestore)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t Task) EqualsRestore(a, b *velero.Restore) bool {
	return reflect.DeepEqual(a.Spec, b.Spec)
}

func (t Task) GetRestore() (*velero.Restore, error) {
	cluster := t.PlanResources.DestMigCluster
	client, err  := cluster.GetClient(t.Client)
	labels := t.Owner.GetCorrelationLabels()
	list := velero.RestoreList{}
	err = client.List(
		context.TODO(),
		k8sclient.MatchingLabels(labels),
		&list)
	if err != nil {
		return nil, err
	}
	if len(list.Items) > 0 {
		return &list.Items[0], nil
	}

	return nil, nil
}

func (t Task) BuildRestore() *velero.Restore {
	restore := &velero.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Labels: t.Owner.GetCorrelationLabels(),
			GenerateName: t.Owner.GetName()+"-",
			Namespace:    t.Namespace,
		},
	}
	t.UpdateRestore(restore)
	return restore
}

func (t Task) UpdateRestore(restore *velero.Restore) {
	restorePVs := true
	restore.Spec = velero.RestoreSpec{
		BackupName: t.BackupName,
		RestorePVs: &restorePVs,
	}
}
