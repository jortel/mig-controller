package web

import (
	"github.com/gin-gonic/gin"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/auth"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	"k8s.io/api/core/v1"
	"net/http"
	"time"
)

const (
	NamespacesRoot = ClusterRoot + "/namespaces"
	NamespaceRoot  = NamespacesRoot + "/:ns2"
)

//
// Namespaces (route) handler.
type NsHandler struct {
	// Base
	BaseHandler
}

//
// Add routes.
func (h NsHandler) AddRoutes(r *gin.Engine) {
	r.GET(NamespacesRoot, h.List)
	r.GET(NamespacesRoot+"/", h.List)
	r.GET(NamespaceRoot, h.Get)
}

//
// List namespaces on a cluster.
func (h NsHandler) List(ctx *gin.Context) {
	status := h.Prepare(ctx)
	if status != http.StatusOK {
		ctx.Status(status)
		return
	}
	db := h.container.Db
	collection := model.Namespace{
		Base: model.Base{
			Cluster: h.cluster.PK,
		},
	}
	count, err := collection.Count(db, model.ListOptions{})
	if err != nil {
		Log.Trace(err)
		ctx.Status(http.StatusInternalServerError)
		return
	}
	list, err := collection.List(
		db,
		model.ListOptions{
			Page: &h.page,
			Sort: []int{5},
		})
	if err != nil {
		Log.Trace(err)
		ctx.Status(http.StatusInternalServerError)
		return
	}
	review := &auth.Review{
		Resources: []string{auth.Pod},
		Verbs: []string{
			auth.LIST,
			auth.GET,
			auth.CREATE,
			auth.DELETE,
			auth.PATCH,
			auth.UPDATE,
		},
	}
	content := NamespaceList{
		Count: count,
	}
	duration := time.Duration(0)
	for _, m := range list {
		review.Namespace = m.Name
		mark := time.Now()
		allow, err := h.rbac.Allow(review)
		if err != nil {
			Log.Trace(err)
			ctx.Status(http.StatusInternalServerError)
			return
		}
		duration += time.Since(mark)
		if !allow {
			continue
		}
		podCount, err := model.Pod{
			Base: model.Base{
				Cluster:   h.cluster.PK,
				Namespace: m.Name,
			},
		}.Count(
			h.container.Db,
			model.ListOptions{})
		if err != nil {
			Log.Trace(err)
			ctx.Status(http.StatusInternalServerError)
			return
		}
		pvcCount, err := model.PVC{
			Base: model.Base{
				Cluster:   h.cluster.PK,
				Namespace: m.Name,
			},
		}.Count(
			h.container.Db,
			model.ListOptions{})
		if err != nil {
			Log.Trace(err)
			ctx.Status(http.StatusInternalServerError)
			return
		}
		SrvCount, err := model.Service{
			Base: model.Base{
				Cluster:   h.cluster.PK,
				Namespace: m.Name,
			},
		}.Count(
			h.container.Db,
			model.ListOptions{})
		if err != nil {
			Log.Trace(err)
			ctx.Status(http.StatusInternalServerError)
			return
		}
		content.Items = append(
			content.Items,
			Namespace{
				Name:         m.Name,
				ServiceCount: SrvCount,
				PvcCount:     pvcCount,
				PodCount:     podCount,
			})
	}

	Log.Info("NS List", "auth", duration)

	ctx.JSON(http.StatusOK, content)
}

//
// Get a specific namespace on a cluster.
func (h NsHandler) Get(ctx *gin.Context) {
	ctx.Status(http.StatusMethodNotAllowed)
}

// Namespace REST resource
type Namespace struct {
	// Cluster k8s namespace.
	Namespace string `json:"namespace,omitempty"`
	// Cluster k8s name.
	Name string `json:"name"`
	// Raw k8s object.
	Object *v1.Namespace `json:"object,omitempty"`
	// Number of services.
	ServiceCount int64 `json:"serviceCount"`
	// Number of pods.
	PodCount int64 `json:"podCount"`
	// Number of PVCs.
	PvcCount int64 `json:"pvcCount"`
}

func (n *Namespace) Path() string {
	return NamespaceRoot
}

//
// NS collection REST resource.
type NamespaceList struct {
	// Total number in the collection.
	Count int64 `json:"count"`
	// List of resources.
	Items []Namespace `json:"resources"`
}

func (n *NamespaceList) Path() string {
	return NamespacesRoot
}
