package config

import (
	"strings"
	"testing"
)

func conformingFields() ProjectFields {
	return ProjectFields{Fields: []ProjectField{
		{Name: "Phase", Type: "ProjectV2Field"},
		{Name: "Status", Type: "ProjectV2SingleSelectField", Options: []ProjectFieldOption{
			{Name: "Todo"}, {Name: "In Progress"}, {Name: "In Review"},
			{Name: "Needs Attention"}, {Name: "Done"},
		}},
	}}
}

func TestValidateProject_Conforming_ReturnsNoErrors(t *testing.T) {
	if errs := ValidateProject(conformingFields()); len(errs) != 0 {
		t.Errorf("expected no errors, got: %v", errs)
	}
}

func TestValidateProject_MissingPhaseField(t *testing.T) {
	pf := conformingFields()
	pf.Fields = pf.Fields[1:]
	errs := ValidateProject(pf)
	if len(errs) == 0 {
		t.Fatal("expected error, got none")
	}
	if !strings.Contains(errs[0], "Phase") {
		t.Errorf("error should mention Phase: %q", errs[0])
	}
}

func TestValidateProject_PhaseFieldWrongType(t *testing.T) {
	pf := conformingFields()
	pf.Fields[0].Type = "ProjectV2SingleSelectField"
	errs := ValidateProject(pf)
	if len(errs) == 0 {
		t.Fatal("expected error, got none")
	}
}

func TestValidateProject_StatusMissingOption(t *testing.T) {
	pf := conformingFields()
	pf.Fields[1].Options = pf.Fields[1].Options[:4]
	errs := ValidateProject(pf)
	if len(errs) == 0 {
		t.Fatal("expected error, got none")
	}
	joined := strings.Join(errs, "\n")
	if !strings.Contains(joined, "Done") {
		t.Errorf("error should mention missing Done option: %s", joined)
	}
}

func TestValidateProject_StatusOptionWrongCase(t *testing.T) {
	pf := conformingFields()
	pf.Fields[1].Options[0].Name = "todo"
	errs := ValidateProject(pf)
	if len(errs) == 0 {
		t.Fatal("expected error, got none")
	}
}
