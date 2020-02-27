package model

import (
	"encoding/json"
	rbac "k8s.io/api/rbac/v1beta1"
)

//
// Role model.
type Role struct {
	Base
	// json encoded rules.
	Rules string
}

//
// Update the model `with` a k8s Role.
func (m *Role) With(object *rbac.Role) {
	m.UID = string(object.UID)
	m.Version = object.ResourceVersion
	m.Namespace = object.Namespace
	m.Name = object.Name
	m.EncodeRules(object.Rules)
}

//
// Update the model `with` a k8s ClusterRole.
func (m *Role) With2(object *rbac.ClusterRole) {
	m.UID = string(object.UID)
	m.Version = object.ResourceVersion
	m.Namespace = object.Namespace
	m.Name = object.Name
	m.EncodeRules(object.Rules)
}

//
// Encode rules.
func (m *Role) EncodeRules(r []rbac.PolicyRule) {
	rules, _ := json.Marshal(r)
	m.Rules = string(rules)
}

//
// Decode rules.
func (m *Role) DecodeRules() []rbac.PolicyRule {
	rules := []rbac.PolicyRule{}
	json.Unmarshal([]byte(m.Rules), &rules)
	return rules
}

//
// Count in the DB.
func (m Role) Count(db DB, options ListOptions) (int64, error) {
	return Table{db}.Count(&m, options)
}

//
// Fetch the from in the DB.
func (m Role) List(db DB, options ListOptions) ([]*Role, error) {
	list := []*Role{}
	listed, err := Table{db}.List(&m, options)
	if err != nil {
		Log.Trace(err)
		return nil, err
	}
	for _, intPtr := range listed {
		list = append(list, intPtr.(*Role))
	}

	return list, err
}

//
// Fetch the from in the DB.
func (m *Role) Get(db DB) error {
	return Table{db}.Get(m)
}

//
// Insert the model into the DB.
func (m *Role) Insert(db DB) error {
	m.SetPk()
	return Table{db}.Insert(m)
}

//
// Update the model in the DB.
func (m *Role) Update(db DB) error {
	m.SetPk()
	return Table{db}.Update(m)
}

//
// Delete the model in the DB.
func (m *Role) Delete(db DB) error {
	m.SetPk()
	return Table{db}.Delete(m)
}
