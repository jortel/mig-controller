package web

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/auth"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/container"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	"github.com/konveyor/mig-controller/pkg/logging"
	"github.com/konveyor/mig-controller/pkg/settings"
	"net/http"
	"regexp"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Application settings.
var Settings = &settings.Settings

// Shared logger.
var Log *logging.Logger

// Root - all routes.
const (
	Root = "/namespaces/:ns1"
)

//
// Web server
type WebServer struct {
	// Allowed CORS origins.
	allowedOrigins []*regexp.Regexp
	// Reference to the container.
	Container *container.Container
}

//
// Start the web-server.
// Initializes `gin` with routes and CORS origins.
func (w *WebServer) Start() {
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowMethods:     []string{"GET"},
		AllowHeaders:     []string{"Authorization", "Origin"},
		AllowOriginFunc:  w.allow,
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))
	w.buildOrigins()
	w.addRoutes(r)
	go r.Run()
}

//
// Build a REGEX for each CORS origin.
func (w *WebServer) buildOrigins() {
	w.allowedOrigins = []*regexp.Regexp{}
	for _, r := range Settings.Discovery.CORS.AllowedOrigins {
		expr, err := regexp.Compile(r)
		if err != nil {
			Log.Error(
				err,
				"origin not valid",
				"expr",
				r)
			continue
		}
		w.allowedOrigins = append(w.allowedOrigins, expr)
		Log.Info(
			"Added allowed origin.",
			"expr",
			r)
	}
}

//
// Add the routes.
func (w *WebServer) addRoutes(r *gin.Engine) {
	handlers := []RequestHandler{
		SchemaHandler{
			router: r,
		},
		RootNsHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
		ClusterHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
		ReviewHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
		NsHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
		PodHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
		LogHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
		PvHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
		PvcHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
		ServiceHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
		PlanHandler{
			BaseHandler: BaseHandler{
				container: w.Container,
			},
		},
	}
	for _, h := range handlers {
		h.AddRoutes(r)
	}
}

//
// Called by `gin` to perform CORS authorization.
func (w *WebServer) allow(origin string) bool {
	for _, expr := range w.allowedOrigins {
		if expr.MatchString(origin) {
			return true
		}
	}

	Log.Info("Denied.", "origin", origin)

	return false
}

//
// Web request handler.
type RequestHandler interface {
	// Add routes to the `gin` router.
	AddRoutes(*gin.Engine)
	// List resources in a REST collection.
	List(*gin.Context)
	// Get a specific REST resource.
	Get(*gin.Context)
}

//
// Base web request handler.
type BaseHandler struct {
	// Reference to the container used to fulfil requests.
	container *container.Container
	// The bearer token passed in the `Authorization` header.
	token string
	// The `page` parameter passed in the request.
	page model.Page
	// The cluster specified in the request.
	cluster model.Cluster
	// The DataSource.
	ds *container.DataSource
	// RBAC
	rbac auth.RBAC
}

//
// Prepare the handler to fulfil the request.
// Set the `token` and `page` fields using passed parameters.
func (h *BaseHandler) Prepare(ctx *gin.Context) int {
	Log.Reset()
	h.setCluster(ctx)
	status := h.setToken(ctx)
	if status != http.StatusOK {
		return status
	}
	status = h.setPage(ctx)
	if status != http.StatusOK {
		return status
	}
	status = h.setDs()
	if status != http.StatusOK {
		return status
	}
	h.cluster = h.ds.Cluster
	h.rbac = auth.RBAC{
		Client:  h.ds.Client,
		Cluster: &h.cluster,
		Token:   h.token,
		Db:      h.container.Db,
	}

	return http.StatusOK
}

//
// Find and set the `DataSource`.
func (h *BaseHandler) setDs() int {
	wait := time.Second * 30
	poll := time.Microsecond * 100
	for {
		mark := time.Now()
		if ds, found := h.container.GetDs(&h.cluster); found {
			if ds.IsReady() {
				h.ds = ds
				return http.StatusOK
			}
		}
		hasCluster, err := h.container.HasCluster(&h.cluster)
		if err != nil {
			return http.StatusInternalServerError
		}
		if !hasCluster {
			return http.StatusNotFound
		}
		if wait > 0 {
			time.Sleep(poll)
			wait -= time.Since(mark)
		} else {
			break
		}
	}

	return http.StatusPartialContent
}

//
// Set the cluster.
func (h *BaseHandler) setCluster(ctx *gin.Context) {
	namespace := ctx.Param("ns1")
	cluster := ctx.Param("cluster")
	if cluster == "" {
		h.cluster = model.Cluster{
			CR: model.CR{
				Namespace: "",
				Name:      "",
			},
		}
	} else {
		h.cluster = model.Cluster{
			CR: model.CR{
				Namespace: namespace,
				Name:      cluster,
			},
		}
	}
}

//
// Set the `token` field.
func (h *BaseHandler) setToken(ctx *gin.Context) int {
	header := ctx.GetHeader("Authorization")
	fields := strings.Fields(header)
	if len(fields) == 2 && fields[0] == "Bearer" {
		h.token = fields[1]
		return http.StatusOK
	}
	if Settings.Discovery.AuthOptional {
		restCfg, _ := config.GetConfig()
		h.token = restCfg.BearerToken
		return http.StatusOK
	}

	Log.Info("`Authorization: Bearer <token>` header required but not found.")

	return http.StatusBadRequest
}

//
// Set the `token` field.
func (h *BaseHandler) setPage(ctx *gin.Context) int {
	q := ctx.Request.URL.Query()
	page := model.Page{
		Limit:  int(^uint(0) >> 1),
		Offset: 0,
	}
	pLimit := q.Get("limit")
	if len(pLimit) != 0 {
		nLimit, err := strconv.Atoi(pLimit)
		if err != nil || nLimit < 0 {
			return http.StatusBadRequest
		}
		page.Limit = nLimit
	}
	pOffset := q.Get("offset")
	if len(pOffset) != 0 {
		nOffset, err := strconv.Atoi(pOffset)
		if err != nil || nOffset < 0 {
			return http.StatusBadRequest
		}
		page.Offset = nOffset
	}

	h.page = page
	return http.StatusOK
}

//
// Schema (route) handler.
type SchemaHandler struct {
	// The `gin` router.
	router *gin.Engine
}

func (h SchemaHandler) AddRoutes(r *gin.Engine) {
	r.GET("/schema", h.List)
}

//
// List schema.
func (h SchemaHandler) List(ctx *gin.Context) {
	type Schema struct {
		Version string
		Release int
		Paths   []string
	}
	schema := Schema{
		Version: "beta1",
		Release: 2,
		Paths:   []string{},
	}
	for _, rte := range h.router.Routes() {
		schema.Paths = append(schema.Paths, rte.Path)
	}

	ctx.JSON(http.StatusOK, schema)
}

//
// Not supported.
func (h SchemaHandler) Get(ctx *gin.Context) {
	ctx.Status(http.StatusMethodNotAllowed)
}

//
// Root namespace (route) handler.
type RootNsHandler struct {
	// Base
	BaseHandler
}

func (h RootNsHandler) AddRoutes(r *gin.Engine) {
	r.GET(strings.Split(Root, "/")[1], h.List)
	r.GET(strings.Split(Root, "/")[1]+"/", h.List)
}

//
// List all root level namespaces.
func (h RootNsHandler) List(ctx *gin.Context) {
	status := h.Prepare(ctx)
	if status != http.StatusOK {
		ctx.Status(status)
		return
	}
	list := []string{}
	set := map[string]bool{}
	// Cluster
	clusters, err := model.Cluster{}.List(
		h.container.Db,
		model.ListOptions{
			Page: &h.page,
		})
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return
	}
	for _, m := range clusters {
		if m.Namespace == "" {
			continue
		}
		if _, found := set[m.Namespace]; !found {
			list = append(list, m.Namespace)
			set[m.Namespace] = true
		}

	}
	// Plan
	plans, err := model.Plan{}.List(
		h.container.Db,
		model.ListOptions{
			Page: &h.page,
		})
	if err != nil {
		ctx.Status(http.StatusInternalServerError)
		return
	}
	for _, m := range plans {
		if _, found := set[m.Namespace]; !found {
			list = append(list, m.Namespace)
			set[m.Namespace] = true
		}

	}
	sort.Strings(list)
	h.page.Slice(&list)
	ctx.JSON(http.StatusOK, list)
}

//
// Not supported.
func (h RootNsHandler) Get(ctx *gin.Context) {
	ctx.Status(http.StatusMethodNotAllowed)
}
