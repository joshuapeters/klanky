package config

import (
	"fmt"
	"regexp"
)

// slugRe is the canonical slug grammar from the locked spec: `[a-z0-9-]+`.
var slugRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// ValidateSlug returns nil iff s is a valid klanky slug.
func ValidateSlug(s string) error {
	if s == "" {
		return fmt.Errorf("slug is empty")
	}
	if !slugRe.MatchString(s) {
		return fmt.Errorf("slug %q must match [a-z0-9-]+ (lowercase letters, digits, hyphens)", s)
	}
	return nil
}

// DeriveSlug turns a freeform project title into a klanky slug. Lowercases,
// replaces non-alphanumeric runs with single hyphens, trims leading/trailing
// hyphens. Returns "" when the title contains no slug-eligible characters.
func DeriveSlug(title string) string {
	out := make([]byte, 0, len(title))
	prevHyphen := true // suppresses leading hyphens
	for i := 0; i < len(title); i++ {
		c := title[i]
		switch {
		case c >= 'A' && c <= 'Z':
			out = append(out, c+('a'-'A'))
			prevHyphen = false
		case (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9'):
			out = append(out, c)
			prevHyphen = false
		default:
			if !prevHyphen {
				out = append(out, '-')
				prevHyphen = true
			}
		}
	}
	for len(out) > 0 && out[len(out)-1] == '-' {
		out = out[:len(out)-1]
	}
	return string(out)
}

// ProjectFieldsRaw mirrors GitHub's project field shape for schema validation
// at link time, before we've narrowed down to the single Status field we care
// about. Keep separate from the on-disk ProjectFields type.
type ProjectFieldsRaw struct {
	Fields []RawField `json:"fields"`
}

// RawField is one field returned by `gh project field-list`. We only consume
// Status (single-select); other types are ignored at validation time.
type RawField struct {
	ID      string             `json:"id"`
	Name    string             `json:"name"`
	Type    string             `json:"type"`
	Options []RawFieldOption   `json:"options,omitempty"`
}

// RawFieldOption is one option of a single-select field.
type RawFieldOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// FindField returns the named field or nil.
func FindField(fs []RawField, name string) *RawField {
	for i := range fs {
		if fs[i].Name == name {
			return &fs[i]
		}
	}
	return nil
}

// ValidateProjectSchema returns a list of human-readable problems with the
// project's field configuration. Empty slice means conforming. Used by
// `klanky project link` to refuse a non-conforming project.
func ValidateProjectSchema(fields []RawField) []string {
	var errs []string
	status := FindField(fields, StatusFieldName)
	const wantType = "ProjectV2SingleSelectField"
	switch {
	case status == nil:
		errs = append(errs, fmt.Sprintf("missing required field %q (type %s)", StatusFieldName, wantType))
	case status.Type != wantType:
		errs = append(errs, fmt.Sprintf("field %q has type %q, want %q", StatusFieldName, status.Type, wantType))
	default:
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
