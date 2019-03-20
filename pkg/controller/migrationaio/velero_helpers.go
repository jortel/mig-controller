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

package migrationaio

import (
	"time"

	velerov1 "github.com/heptio/velero/pkg/apis/velero/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func getControllerRuntimeClient(clusterURL string, bearerToken string) (c client.Client, err error) {
	clusterConfig := &rest.Config{
		Host:        clusterURL,
		BearerToken: bearerToken,
	}
	clusterConfig.Insecure = true

	c, err = client.New(clusterConfig, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return nil, err
	}

	return c, nil
}

func getVeleroBackup(ns string, name string, backupNamespaces []string) *velerov1.Backup {
	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: velerov1.BackupSpec{
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "nginx"},
			},
			StorageLocation:    "default",
			TTL:                metav1.Duration{Duration: 720 * time.Hour},
			IncludedNamespaces: backupNamespaces,
		},
	}
	return backup
}

func getVeleroRestore(ns string, name string, backupUniqueName string) *velerov1.Restore {
	restorePVs := true
	restore := &velerov1.Restore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
		},
		Spec: velerov1.RestoreSpec{
			BackupName: backupUniqueName,
			RestorePVs: &restorePVs,
		},
	}

	return restore
}