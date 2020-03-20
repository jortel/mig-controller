package auth

import (
	"context"
	"database/sql"
	"github.com/konveyor/mig-controller/pkg/controller/discovery/model"
	"github.com/konveyor/mig-controller/pkg/logging"
	"github.com/konveyor/mig-controller/pkg/settings"
	auth "k8s.io/api/authentication/v1beta1"
	rbac "k8s.io/api/rbac/v1beta1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"strings"
	"time"
)

// Application settings.
var Settings = &settings.Settings

// Shared logger.
var Log *logging.Logger

//
// Special Users.
const (
	KubeAdmin = "kube:admin"
)

var (
	AllowUsers = map[string]bool{
		KubeAdmin: true,
	}
)

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
// RBAC rule review.
type Review struct {
	// The k8s API group.
	Group string
	// Resources.
	Resources []string
	// Namespace (optional).
	Namespace string
	// Verbs
	Verbs []string
	// Matrix of expanded Groups, Resources and Verbs
	matrix Matrix
}

//
// Expand the Resources and Verbs into the `matrix`.
func (r *Review) expand() {
	r.matrix = Matrix{}
	for _, resource := range r.Resources {
		for _, verb := range r.Verbs {
			r.matrix = append(
				r.matrix,
				MxItem{
					group:    r.Group,
					resource: resource,
					verb:     verb,
				})
		}
	}
}

//
// Apply the rule to the matrix.
func (r *Review) apply(rule *rbac.PolicyRule) {
	ruleMatrix := Matrix{}
	for _, resource := range rule.Resources {
		for _, verb := range rule.Verbs {
			ruleMatrix = append(
				ruleMatrix,
				MxItem{
					group:    r.Group,
					resource: resource,
					verb:     verb,
				})
		}
	}
	for i := range r.matrix {
		needed := &r.matrix[i]
		for _, m := range ruleMatrix {
			if !needed.matched {
				needed.match(&m)
			}
		}
	}
}

//
// Return `true` when all of the matrix items have been matched.
func (r *Review) satisfied() bool {
	for _, m := range r.matrix {
		if !m.matched {
			return false
		}
	}

	return true
}

//
// The matrix is a de-normalized set of Resources and verbs.
type Matrix = []MxItem

//
// A matrix item.
type MxItem struct {
	// API group.
	group string
	// Resource kind.
	resource string
	// Verb
	verb string
	// Has been matched.
	matched bool
}

//
// Match another matrix item.
func (needed *MxItem) match(rule *MxItem) {
	matchVerb := func() {
		if needed.verb == ALL {
			if rule.verb == ALL {
				needed.matched = true
			}
			return
		}
		if needed.verb == rule.verb || rule.verb == ANY {
			needed.matched = true
			return
		}
	}
	matchResource := func() {
		if needed.resource == ALL {
			if rule.resource == ALL {
				matchVerb()
			}
			return
		}
		if needed.resource == rule.resource || rule.resource == ANY {
			matchVerb()
			return
		}
	}
	matchGroup := func() {
		if needed.group == ALL {
			if rule.group == ALL {
				matchResource()
			}
			return
		}
		if needed.group == rule.group || rule.group == ANY {
			matchResource()
			return
		}
	}

	matchGroup()
}

//
// RBAC
type RBAC struct {
	Client client.Client
	// Database
	Db *sql.DB
	// Cluster
	Cluster *model.Cluster
	// A Bearer token.
	Token string
	// The ServiceAccount for the token.
	sa types.NamespacedName
	// The User for the token.
	user string
	// The user group membership.
	groups []string
	// RoleBindings for token.
	roleBindings []*model.RoleBinding
	// Roles for role-bindings.
	roles map[string]*model.Role
	// The token has been authenticated.
	authenticated bool
	// The role-bindings have been loaded.
	loaded bool
}

//
// Allow request.
func (r *RBAC) Allow(request *Review) (bool, error) {
	if r.Token == "" && Settings.Discovery.AuthOptional {
		return true, nil
	}
	err := r.load()
	if err != nil {
		return false, nil
	}
	if !r.authenticated {
		return false, nil
	}
	if _, found := AllowUsers[r.user]; found {
		return true, nil
	}
	request.expand()
	if len(request.matrix) == 0 {
		return false, nil
	}
	for _, rb := range r.roleBindings {
		role, found := r.roles[rb.Pk()]
		if !found {
			continue
		}
		if rb.Namespace == "" || rb.Namespace == request.Namespace {
			if r.matchRules(request, role) {
				return true, nil
			}
		}
	}

	return false, nil
}

//
// Match the rule.
func (r *RBAC) matchRules(request *Review, role *model.Role) bool {
	rules := role.DecodeRules()
	for _, rule := range rules {
		request.apply(&rule)
		if request.satisfied() {
			return true
		}
	}

	return false
}

//
// Resolve the token to a User or SA.
// Load the associated `RoleBindings`.
func (r *RBAC) load() error {
	if r.loaded {
		return nil
	}
	err := r.authenticate()
	if err != nil {
		Log.Trace(err)
		return err
	}
	if !r.authenticated {
		r.loaded = true
		return nil
	}
	err = r.buildRoleBindings()
	if err != nil {
		Log.Trace(err)
		return err
	}
	err = r.buildRoles()
	if err != nil {
		Log.Trace(err)
		return err
	}

	r.loaded = true

	return nil
}

//
// Authenticate the bearer token.
// Set the user|sa and groups.
func (r *RBAC) authenticate() error {
	mark := time.Now()
	tr := &auth.TokenReview{
		Spec: auth.TokenReviewSpec{
			Token: r.Token,
		},
	}
	err := r.Client.Create(context.TODO(), tr)
	if err != nil {
		Log.Trace(err)
		return err
	}
	if !tr.Status.Authenticated {
		return nil
	}
	r.authenticated = true
	user := tr.Status.User
	r.groups = user.Groups
	name := strings.Split(user.Username, ":")
	if len(name) == 4 {
		if name[0] == "system" && name[1] == "serviceaccount" {
			r.sa.Namespace = name[2]
			r.sa.Name = name[3]
		}
	} else {
		r.user = user.Username
	}

	Log.Info("RBAC: authenticate.", "duration", time.Since(mark))

	return nil
}

//
// Build the list of granted role-bindings.
func (r *RBAC) buildRoleBindings() error {
	var subject model.Subject
	var err error
	mark := time.Now()
	if r.user != "" {
		if _, found := AllowUsers[r.user]; found {
			return nil
		}
		subject = model.Subject{
			Kind: rbac.UserKind,
			Name: r.user,
		}
	}
	if r.sa.Name != "" {
		subject = model.Subject{
			Kind:      rbac.ServiceAccountKind,
			Namespace: r.sa.Namespace,
			Name:      r.sa.Name,
		}
	}
	r.roleBindings, err = model.RoleBinding{
		Base: model.Base{
			Cluster: r.Cluster.Pk(),
		},
	}.ListBySubject(r.Db, subject)
	if err != nil {
		Log.Trace(err)
		return err
	}
	for _, group := range r.groups {
		subject = model.Subject{
			Kind: rbac.GroupKind,
			Name: group,
		}
		roleBindings, err := model.RoleBinding{
			Base: model.Base{
				Cluster: r.Cluster.Pk(),
			},
		}.ListBySubject(r.Db, subject)
		if err != nil {
			Log.Trace(err)
			return err
		}
		for _, rb := range roleBindings {
			r.roleBindings = append(r.roleBindings, rb)
		}
	}

	Log.Info("RBAC: build role-bindings.", "duration", time.Since(mark))

	return nil
}

//
// Build `roles`.
func (r *RBAC) buildRoles() error {
	mark := time.Now()
	r.roles = map[string]*model.Role{}
	for _, rb := range r.roleBindings {
		ref := rb.DecodeRole()
		role := &model.Role{
			Base: model.Base{
				Cluster:   rb.Cluster,
				Namespace: rb.Namespace,
				Name:      ref.Name,
			},
		}
		err := role.Get(r.Db)
		if err == sql.ErrNoRows {
			role = &model.Role{
				Base: model.Base{
					Cluster: rb.Cluster,
					Name:    ref.Name,
				},
			}
			err = role.Get(r.Db)
		}
		if err != nil {
			role = nil
			if err == sql.ErrNoRows {
				Log.Trace(err)
			}
		}
		if err == nil {
			r.roles[rb.Pk()] = role
		}
	}

	Log.Info("RBAC: build roles.", "duration", time.Since(mark))

	return nil
}

//
// Get the role referenced in the binding.
func (r *RBAC) getRole(rb *model.RoleBinding) (*model.Role, error) {
	ref := rb.DecodeRole()
	role := &model.Role{
		Base: model.Base{
			Cluster:   rb.Cluster,
			Namespace: rb.Namespace,
			Name:      ref.Name,
		},
	}
	err := role.Get(r.Db)
	if err == sql.ErrNoRows {
		role = &model.Role{
			Base: model.Base{
				Cluster: rb.Cluster,
				Name:    ref.Name,
			},
		}
		err = role.Get(r.Db)
	}
	if err != nil {
		role = nil
		if err == sql.ErrNoRows {
			Log.Trace(err)
		}
	}

	return role, err
}
