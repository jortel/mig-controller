package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MigPlan Conditions.
type Condition struct {
	Type string `json:"type"`
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
	Message string `json:"message,omitempty"`
	LastTransitionTime *metav1.Time `json:"lastTransitionTime"`
}

func (r *Condition) Update(other Condition) {
	r.Type = other.Type
	r.Status = other.Status
	r.Reason = other.Reason
	r.Message = other.Message
	r.LastTransitionTime = &metav1.Time{}
}

type ConditionContainer struct {
	Conditions []Condition `json:"conditions"`
}

func (r ConditionContainer) FindCondition(name string) (int, *Condition) {
	if r.Conditions == nil {
		r.Conditions = []Condition{}
	}
	for index, condition := range r.Conditions {
		if condition.Type == name {
			return index, &condition
		}
	}
	return 0, nil
}

func (r ConditionContainer) SetCondition(condition Condition) {
	if r.Conditions == nil {
		r.Conditions = []Condition{}
	}
	condition.LastTransitionTime = &metav1.Time{}
	_, found := r.FindCondition(condition.Type)
	if found != nil {
		found.Update(condition)
	} else {
		r.Conditions = append(r.Conditions, condition)
	}
}

func (r ConditionContainer) ClearConditions(names ...string) {
	if r.Conditions == nil {
		r.Conditions = []Condition{}
	}
	for _, name := range names {
		index, condition := r.FindCondition(name)
		if condition != nil {
			r.Conditions = append(r.Conditions[:index], r.Conditions[index+1:]...)
		}
	}
}