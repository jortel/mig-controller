package v1alpha1

// Validation status.
type ValidationStatus struct {
	Invalid bool     `json:"invalid"`
	Errors  []string `json:"reasons"`
}

func (r ValidationStatus) Reset() {
	r.Invalid = false
	r.Errors = []string{}
}

func (r ValidationStatus) Failed(error string) {
	r.Errors = append(r.Errors, error)
	r.Invalid = true
}
