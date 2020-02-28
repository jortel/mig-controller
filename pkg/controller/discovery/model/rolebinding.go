package model

import (
	"database/sql"
	"encoding/json"
	rbac "k8s.io/api/rbac/v1beta1"
)

var RoleBindingBySubjectSQL = `
SELECT
  a.pk,
  a.uid,
  a.version,
  a.namespace,
  a.name,
  a.object,
  a.cluster,
  a.role
FROM RoleBinding a,
     Subject b
WHERE
  a.cluster = :cluster AND
  b.parent = a.pk AND
  b.kind = :kind AND
  b.namespace = :namespace AND
  b.name = :name;
`

//
// Subject referenced in a RoleBinding.
type Subject struct {
	// RoleBinding.
	Parent string `sql:"unique(a),fk:RoleBinding(pk)"`
	// Subject kind.
	Kind string `sql:"unique(a),index(a)"`
	// Subject namespace.
	Namespace string `sql:"unique(a),index(a)"`
	// Subject name.
	Name string `sql:"unique(a),index(a)"`
}

//
// Update the model `with` a k8s Subject.
func (m *Subject) With(object *rbac.Subject) {
	m.Kind = object.Kind
	m.Namespace = object.Namespace
	m.Name = object.Name
}

//
// RoleBinding model.
type RoleBinding struct {
	Base
	// Role json-encoded k8s rbac.RoleRef.
	Role string `sql:""`
	// Subjects
	subjects []rbac.Subject
}

//
// Update the model `with` a k8s RoleBinding.
func (m *RoleBinding) With(object *rbac.RoleBinding) {
	m.UID = string(object.UID)
	m.Version = object.ResourceVersion
	m.Namespace = object.Namespace
	m.Name = object.Name
	m.subjects = object.Subjects
	m.EncodeRole(&object.RoleRef)
}

//
// Update the model `with` a k8s ClusterRoleBinding.
func (m *RoleBinding) With2(object *rbac.ClusterRoleBinding) {
	m.UID = string(object.UID)
	m.Version = object.ResourceVersion
	m.Namespace = object.Namespace
	m.Name = object.Name
	m.subjects = object.Subjects
	m.EncodeRole(&object.RoleRef)
}

//
// Encode roleRef
func (m *RoleBinding) EncodeRole(r *rbac.RoleRef) {
	ref, _ := json.Marshal(r)
	m.Role = string(ref)
}

//
// Decode roleRef
func (m *RoleBinding) DecodeRole() *rbac.RoleRef {
	ref := &rbac.RoleRef{}
	json.Unmarshal([]byte(m.Role), ref)
	return ref
}

//
// Count in the DB.
func (m RoleBinding) Count(db DB, options ListOptions) (int64, error) {
	return Table{db}.Count(&m, options)
}

//
// List role-bindings by user.
func (m RoleBinding) ListBySubject(db DB, subject Subject) ([]*RoleBinding, error) {
	list := []*RoleBinding{}
	cursor, err := db.Query(
		RoleBindingBySubjectSQL,
		sql.Named("cluster", m.Cluster),
		sql.Named("kind", subject.Kind),
		sql.Named("namespace", subject.Namespace),
		sql.Named("name", subject.Name),
	)
	if err != nil {
		Log.Trace(err)
		return nil, err
	}
	defer cursor.Close()
	for cursor.Next() {
		rb := &RoleBinding{}
		err := Table{db}.Scan(cursor, rb)
		if err != nil {
			Log.Trace(err)
			return nil, err
		}
		list = append(list, rb)
	}

	return list, nil
}

//
// Fetch the from in the DB.
func (m RoleBinding) List(db DB, options ListOptions) ([]*RoleBinding, error) {
	list := []*RoleBinding{}
	listed, err := Table{db}.List(&m, options)
	if err != nil {
		Log.Trace(err)
		return nil, err
	}
	for _, intPtr := range listed {
		list = append(list, intPtr.(*RoleBinding))
	}

	return list, err
}

//
// Fetch the from in the DB.
func (m *RoleBinding) Get(db DB) error {
	return Table{db}.Get(m)
}

//
// Insert the model into the DB.
func (m *RoleBinding) Insert(db DB) error {
	m.SetPk()
	err := Table{db}.Insert(m)
	if err != nil {
		Log.Trace(err)
		return err
	}
	err = m.addSubjects(db)
	if err != nil {
		Log.Trace(err)
		return err
	}

	return nil
}

//
// Update the model in the DB.
func (m *RoleBinding) Update(db DB) error {
	m.SetPk()
	err := Table{db}.Update(m)
	if err != nil {
		Log.Trace(err)
		return err
	}
	err = m.updateSubjects(db)
	if err != nil {
		Log.Trace(err)
		return err
	}

	return nil
}

//
// Delete the model in the DB.
func (m *RoleBinding) Delete(db DB) error {
	m.SetPk()
	m.SetPk()
	err := Table{db}.Delete(m)
	if err != nil {
		Log.Trace(err)
		return err
	}
	err = m.deleteSubjects(db)
	if err != nil {
		Log.Trace(err)
		return err
	}

	return nil
}

//
// Insert role-binding/subject in the DB.
func (m *RoleBinding) addSubjects(db DB) error {
	for _, subject := range m.subjects {
		m := &Subject{
			Parent: m.PK,
		}
		m.With(&subject)
		err := Table{db}.Insert(m)
		if err != nil {
			Log.Trace(err)
			return err
		}
	}

	return nil
}

//
// Delete role-binding/users in the DB.
func (m *RoleBinding) deleteSubjects(db DB) error {
	err := Table{db}.Delete(&Subject{Parent: m.PK})
	if err != nil {
		Log.Trace(err)
		return err
	}

	return nil
}

//
// Update role-binding/subjects in the DB.
// Delete & Insert.
func (m *RoleBinding) updateSubjects(db DB) error {
	err := m.deleteSubjects(db)
	if err != nil {
		Log.Trace(err)
		return err
	}
	err = m.addSubjects(db)
	if err != nil {
		Log.Trace(err)
		return err
	}

	return nil
}
