package domain

import (
	"testing"
	"time"
)

func TestValidatePlacement(t *testing.T) {
	project := NewID()
	otherProject := NewID()
	building := Building{ID: NewID(), ProjectID: project, Name: "Корпус 1"}
	foreignBuilding := Building{ID: NewID(), ProjectID: otherProject, Name: "Корпус 9"}
	section := Section{ID: NewID(), BuildingID: building.ID, Name: "1"}
	foreignSection := Section{ID: NewID(), BuildingID: foreignBuilding.ID, Name: "2"}
	floor := 7

	base := func() Placement {
		return Placement{ProjectID: project, Type: LotTypeApartment, Number: " 145 ", Floor: &floor}
	}
	withBuilding := func(p Placement, b Building) Placement { p.BuildingID = &b.ID; return p }
	withSection := func(p Placement, s Section) Placement { p.SectionID = &s.ID; return p }

	tests := []struct {
		name     string
		p        Placement
		building *Building
		section  *Section
		wantCode string
	}{
		{"project only (no building, no section)", base(), nil, nil, ""},
		{"building without section", withBuilding(base(), building), &building, nil, ""},
		{"building and section", withSection(withBuilding(base(), building), section), &building, &section, ""},
		{"missing project", Placement{Type: LotTypeApartment, Number: "1"}, nil, nil, CodeRequired},
		{"unknown type", Placement{ProjectID: project, Type: "villa", Number: "1"}, nil, nil, CodeInvalidValue},
		{"number required", Placement{ProjectID: project, Type: LotTypeParking, Number: "  "}, nil, nil, CodeRequired},
		{"section without building", withSection(base(), section), nil, &section, CodeSectionWithoutBldg},
		{"building of another project", withBuilding(base(), foreignBuilding), &foreignBuilding, nil, CodePlacementMismatch},
		{"section of another building", withSection(withBuilding(base(), building), foreignSection), &building, &foreignSection, CodePlacementMismatch},
		{"building not loaded", withBuilding(base(), building), nil, nil, CodePlacementMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ValidatePlacement(tt.p, tt.building, tt.section)
			assertRuleCode(t, err, tt.wantCode)
			if err == nil && got.Number != "145" {
				t.Errorf("number not normalized: %q", got.Number)
			}
		})
	}
}

func TestNewLotKeepsOptionalLevelsEmpty(t *testing.T) {
	project := NewID()
	lot, err := NewLot(NewID(), Placement{ProjectID: project, Type: LotTypeStorage, Number: "К-12"}, nil, nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if lot.Placement.BuildingID != nil || lot.Placement.SectionID != nil || lot.Placement.Floor != nil {
		t.Errorf("optional placement levels must stay empty: %+v", lot.Placement)
	}
}

func TestNormalizeSourceURL(t *testing.T) {
	ok := "https://example.test/lots/145"
	bad := "ftp://example.test"
	blank := "  "
	if got, err := NormalizeSourceURL(&ok); err != nil || *got != ok {
		t.Errorf("valid URL rejected: %v", err)
	}
	if _, err := NormalizeSourceURL(&bad); err == nil {
		t.Error("non-http URL accepted")
	}
	if got, err := NormalizeSourceURL(&blank); err != nil || got != nil {
		t.Errorf("blank URL must become nil, got %v, %v", got, err)
	}
}

func TestNewDataSource(t *testing.T) {
	if _, err := NewDataSource(NewID(), "price_lists-dev_a", "Прайс-листы", DataSourceDocumentChannel, time.Now()); err != nil {
		t.Errorf("valid data source rejected: %v", err)
	}
	_, err := NewDataSource(NewID(), "Bad Code", "x", DataSourceManual, time.Now())
	assertRuleCode(t, err, CodeInvalidValue)
	_, err = NewDataSource(NewID(), "site", "x", "website", time.Now())
	assertRuleCode(t, err, CodeInvalidValue)
}

func assertRuleCode(t *testing.T, err error, wantCode string) {
	t.Helper()
	if wantCode == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	re, ok := AsRuleError(err)
	if !ok {
		t.Fatalf("error = %v, want rule error %q", err, wantCode)
	}
	if re.Code != wantCode {
		t.Fatalf("error code = %q, want %q (%v)", re.Code, wantCode, err)
	}
}
