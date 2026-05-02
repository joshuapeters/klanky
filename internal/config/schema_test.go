package config

import (
	"strings"
	"testing"
)

func TestValidateSlug_Accepts(t *testing.T) {
	for _, s := range []string{"a", "auth", "auth-2", "billing-v2", "x1", "12-thing"} {
		if err := ValidateSlug(s); err != nil {
			t.Errorf("ValidateSlug(%q) = %v, want nil", s, err)
		}
	}
}

func TestValidateSlug_Rejects(t *testing.T) {
	for _, s := range []string{"", "Auth", "auth_system", "auth/payment", "auth space"} {
		if err := ValidateSlug(s); err == nil {
			t.Errorf("ValidateSlug(%q) = nil, want error", s)
		}
	}
}

func TestDeriveSlug(t *testing.T) {
	cases := map[string]string{
		"Auth System":       "auth-system",
		"  Auth - System ":  "auth-system",
		"Auth/Payment v2":   "auth-payment-v2",
		"!!!":               "",
		"foo___bar":         "foo-bar",
		"Already-A-Slug":    "already-a-slug",
	}
	for in, want := range cases {
		if got := DeriveSlug(in); got != want {
			t.Errorf("DeriveSlug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateProjectSchema_OK(t *testing.T) {
	fields := []RawField{
		{Name: "Title", Type: "ProjectV2Field"},
		{
			Name: "Status", Type: "ProjectV2SingleSelectField",
			Options: []RawFieldOption{
				{ID: "1", Name: "Todo"},
				{ID: "2", Name: "In Progress"},
				{ID: "3", Name: "In Review"},
				{ID: "4", Name: "Needs Attention"},
				{ID: "5", Name: "Done"},
			},
		},
	}
	if errs := ValidateProjectSchema(fields); len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
}

func TestValidateProjectSchema_MissingOption(t *testing.T) {
	fields := []RawField{
		{
			Name: "Status", Type: "ProjectV2SingleSelectField",
			Options: []RawFieldOption{
				{ID: "1", Name: "Todo"},
				{ID: "2", Name: "In Progress"},
				{ID: "3", Name: "Done"},
			},
		},
	}
	errs := ValidateProjectSchema(fields)
	joined := strings.Join(errs, "\n")
	for _, want := range []string{"In Review", "Needs Attention"} {
		if !strings.Contains(joined, want) {
			t.Errorf("expected validation to mention %q; got %v", want, errs)
		}
	}
}

func TestValidateProjectSchema_NoStatusField(t *testing.T) {
	errs := ValidateProjectSchema([]RawField{{Name: "Title", Type: "ProjectV2Field"}})
	if len(errs) == 0 || !strings.Contains(errs[0], "Status") {
		t.Errorf("expected missing-Status error; got %v", errs)
	}
}
