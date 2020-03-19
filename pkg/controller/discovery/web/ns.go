package web

import (
	"github.com/gin-gonic/gin"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/auth"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	"k8s.io/api/core/v1"
	"net/http"
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
	request := &auth.Request{
		Resources: []string{auth.ANY},
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
	for _, m := range list {
		allow, err := h.rbac.Allow(request)
		if err != nil {
			Log.Trace(err)
			ctx.Status(http.StatusInternalServerError)
			return
		}
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

	ctx.JSON(http.StatusOK, content)
}

//
// Get a specific namespace on a cluster.
func (h NsHandler) Get(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, h.cluster.Namespace)
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

//
// NS collection REST resource.
type NamespaceList struct {
	// Total number in the collection.
	Count int64 `json:"count"`
	// List of resources.
	Items []Namespace `json:"resources"`
}
