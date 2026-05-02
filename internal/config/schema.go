package config

import "fmt"

const (
	SchemaVersion    = 1
	FieldNamePhase   = "Phase"
	FieldNameStatus  = "Status"
	FieldTypeNumber  = "ProjectV2Field"
	FieldTypeSelect  = "ProjectV2SingleSelectField"
	LabelFeatureName = "klanky:feature"
)

// StatusOptions lists the required Status options in the order they should
// appear on the kanban (left-to-right). Klanky validates exact name match.
var StatusOptions = []string{"Todo", "In Progress", "In Review", "Needs Attention", "Done"}

// ProjectFields mirrors `gh project field-list --format json` output.
type ProjectFields struct {
	Fields []ProjectField `json:"fields"`
}

type ProjectField struct {
	ID      string               `json:"id"`
	Name    string               `json:"name"`
	Type    string               `json:"type"`
	Options []ProjectFieldOption `json:"options,omitempty"`
}

type ProjectFieldOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ValidateProject returns a list of human-readable error messages describing
// every way the given project's field configuration deviates from the schema.
// Empty slice means conforming.
func ValidateProject(pf ProjectFields) []string {
	var errs []string

	phase := FindField(pf.Fields, FieldNamePhase)
	if phase == nil {
		errs = append(errs, fmt.Sprintf("missing required field %q (expected type %s)", FieldNamePhase, FieldTypeNumber))
	} else if phase.Type != FieldTypeNumber {
		errs = append(errs, fmt.Sprintf("field %q has type %q, want %q", FieldNamePhase, phase.Type, FieldTypeNumber))
	}

	status := FindField(pf.Fields, FieldNameStatus)
	if status == nil {
		errs = append(errs, fmt.Sprintf("missing required field %q (expected type %s)", FieldNameStatus, FieldTypeSelect))
	} else {
		if status.Type != FieldTypeSelect {
			errs = append(errs, fmt.Sprintf("field %q has type %q, want %q", FieldNameStatus, status.Type, FieldTypeSelect))
		}
		present := make(map[string]bool, len(status.Options))
		for _, o := range status.Options {
			present[o.Name] = true
		}
		for _, want := range StatusOptions {
			if !present[want] {
				errs = append(errs, fmt.Sprintf("Status field is missing option %q (case-sensitive)", want))
			}
		}
	}

	return errs
}

func FindField(fs []ProjectField, name string) *ProjectField {
	for i := range fs {
		if fs[i].Name == name {
			return &fs[i]
		}
	}
	return nil
}
