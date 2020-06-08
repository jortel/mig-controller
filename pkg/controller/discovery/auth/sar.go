package auth

import (
	"context"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	"github.com/konveyor/mig-controller/pkg/logging"
	project "github.com/openshift/api/project/v1"
	auth "k8s.io/api/authentication/v1beta1"
	"k8s.io/api/authorization/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/utils/pointer"
	"net/http"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"time"
)

//
// Shared logger.
var Log *logging.Logger

//
// k8s Resources.
const (
	ALL       = "*"
	Namespace = "namespaces"
	PV        = "persistentvolumes"
	PVC       = "persistentvolumeclaims"
	Service   = "services"
	Pod       = "pods"
	PodLog    = "pods/log"
)

//
// Verbs
const (
	ANY    = "*"
	LIST   = "list"
	GET    = "get"
	CREATE = "create"
	DELETE = "delete"
	PATCH  = "patch"
	UPDATE = "update"
)

//
// SAR request.
type Review = v1.ResourceAttributes

//
// Provides RBAC authorization.
type RBAC struct {
	client.Client
	// Cluster
	Cluster *model.Cluster
	// The subject bearer token.
	Token string
	// Token review status.
	tokenStatus auth.TokenReviewStatus
	// Token authenticated.
	authenticated *bool
	// Allowed project list.
	allowedProject map[string]bool
}

//
// Allow/deny the subject review request.
// Special (optimized) steps for namespaces:
//   1. Populate `allowedProject`.
//   2. Match namespace in `allowedProject`.
//   3. Match `*` in `allowedProject`.
//   4. SAR
// The SAR for each namespace is last resort. For large
// clusters this will be very slow.
func (r *RBAC) Allow(request *Review) (bool, error) {
	mark := time.Now()
	defer Log.Info("Allow", "duration", time.Since(mark))
	authenticated, err := r.authenticate()
	if err != nil {
		Log.Trace(err)
		return false, err
	}
	if !authenticated {
		return false, nil
	}
	switch request.Resource {
	case Namespace:
		switch request.Verb {
		case LIST, GET:
			if r.allowProject(request.Name) {
				return true, nil
			}
		}
		fallthrough
	default:
		return r.sar(request)
	}
}

//
// Get whether authenticated
func (r *RBAC) Authenticated() bool {
	return r != nil && *r.authenticated
}

//
// Get the user name for the token.
func (r *RBAC) User() string {
	if r.Authenticated() {
		return r.tokenStatus.User.Username
	}

	return ""
}

//
// Get the user ID for the token.
func (r *RBAC) UID() string {
	if r.Authenticated() {
		return r.tokenStatus.User.UID
	}

	return ""
}

//
// Get the user's groups.
func (r *RBAC) Groups() []string {
	if r.Authenticated() {
		return r.tokenStatus.User.Groups
	}

	return []string{}
}

//
// Get the extra fields for the token.
func (r *RBAC) Extra() map[string]v1.ExtraValue {
	extra := map[string]v1.ExtraValue{}
	if r.Authenticated() {
		for k, v := range r.tokenStatus.User.Extra {
			extra[k] = append(
				v1.ExtraValue{},
				v...)
		}
	}

	return extra
}

//
// Do subject access review.
func (r *RBAC) sar(request *Review) (bool, error) {
	sar := v1.SubjectAccessReview{
		Spec: v1.SubjectAccessReviewSpec{
			ResourceAttributes: request,
			Groups:             r.Groups(),
			User:               r.User(),
			UID:                r.UID(),
			Extra:              r.Extra(),
		},
	}
	err := r.Client.Create(context.TODO(), &sar)
	if err != nil {
		Log.Trace(err)
		return false, err
	}

	return sar.Status.Allowed, nil
}

//
// Authenticate the token.
// Build the `subjectClient`.
func (r *RBAC) authenticate() (bool, error) {
	if r.authenticated != nil {
		return *r.authenticated, nil
	}
	r.authenticated = pointer.BoolPtr(false)
	tr := &auth.TokenReview{
		Spec: auth.TokenReviewSpec{
			Token: r.Token,
		},
	}
	err := r.Client.Create(context.TODO(), tr)
	if err != nil {
		Log.Trace(err)
		return false, err
	}
	if !tr.Status.Authenticated {
		return false, err
	}
	r.tokenStatus = tr.Status
	r.authenticated = pointer.BoolPtr(true)

	return true, nil
}

//
// Load project list (as needed).
// When project API not supported, it will fall-back to
// a SAR to determine if the subject has GET on all namespaces and
// add the `ALL: true` entry.
func (r *RBAC) loadProjects() {
	if r.allowedProject != nil {
		return
	}
	r.allowedProject = map[string]bool{}
	subjectClient, err := r.subjectClient()
	if err != nil {
		return
	}
	list := &project.ProjectList{}
	err = subjectClient.List(context.TODO(), nil, list)
	if err != nil {
		allowed, err := r.sar(&Review{
			Resource: Namespace,
			Verb:     GET,
			Name:     ALL,
		})
		if err == nil && allowed {
			r.allowedProject[ALL] = true
		} else {
			Log.Trace(err)
		}
		return
	}
	for _, p := range list.Items {
		r.allowedProject[p.Name] = true
	}
}

//
// Build the subject client using the REST
// configuration and token.
func (r *RBAC) subjectClient() (client.Client, error) {
	update := func(cfg *rest.Config) {
		cfg.BearerToken = r.Token
		cfg.Burst = 1000
		cfg.QPS = 100
	}
	var err error
	var restCfg *rest.Config
	cluster := r.Cluster.DecodeObject()
	if cluster.Spec.IsHostCluster {
		restCfg, err = config.GetConfig()
		if err != nil {
			Log.Trace(err)
			return nil, err
		}
	} else {
		restCfg = &rest.Config{
			Host: cluster.Spec.URL,
			TLSClientConfig: rest.TLSClientConfig{
				Insecure: cluster.Spec.Insecure,
				CAData:   cluster.Spec.CABundle,
			},
		}
	}
	update(restCfg)
	subjectClient, err := client.New(
		restCfg,
		client.Options{
			Scheme: scheme.Scheme,
		})
	if err != nil {
		if stErr, cast := err.(*errors.StatusError); cast {
			switch stErr.Status().Code {
			case http.StatusUnauthorized,
				http.StatusForbidden:
				return nil, nil
			}
		}
		Log.Trace(err)
		return nil, err
	}

	return subjectClient, nil
}

//
// Determine if the subject has access to the specified project.
func (r *RBAC) allowProject(name string) bool {
	r.loadProjects()
	_, all := r.allowedProject[ALL]
	_, allowed := r.allowedProject[name]
	return all || allowed
}
