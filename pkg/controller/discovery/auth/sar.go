package auth

import (
	"context"
	"k8s.io/api/authorization/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"
)

type SAR struct {
	Client client.Client
}

func (a *SAR) Benchmark(n int) {
	max := time.Duration(0)
	mark := time.Now()
	for i := 0; i < n; i++ {
		mark2 := time.Now()
		sar := v1.SelfSubjectAccessReview{
			Spec: v1.SelfSubjectAccessReviewSpec{
				ResourceAttributes: &v1.ResourceAttributes{
					Group:     "*",
					Resource:  "*",
					Namespace: "elmer",
					Verb:      "*",
				},
			},
		}
		err := a.Client.Create(context.TODO(), &sar)
		if err != nil {
			Log.Trace(err)
			return
		}

		d := time.Since(mark2)
		if d > max {
			max = d
		}

		Log.Info("SAR:", "n", i, "allowed", sar.Status.Allowed, "duration", time.Since(mark2))
	}

	Log.Info("SAR: Benchmark", "total", time.Since(mark), "max", max)
}
