package domain

import (
	"testing"
	"time"
)

func TestCheckNewLotPlacementKeyK2(t *testing.T) {
	project := NewID()
	buildingOne, buildingTwo := NewID(), NewID()
	sectionOne, sectionTwo := NewID(), NewID()
	now := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)

	lot := func(building, section *ID, number string, lotType LotType) Lot {
		return Lot{ID: NewID(), CreatedAt: now, Placement: Placement{
			ProjectID: project, BuildingID: building, SectionID: section, Type: lotType, Number: number,
		}}
	}
	placement := func(building, section *ID, number string, lotType LotType) Placement {
		return Placement{ProjectID: project, BuildingID: building, SectionID: section, Type: lotType, Number: number}
	}

	inBuildingOneSectionOne := lot(&buildingOne, &sectionOne, "145", LotTypeApartment)
	inBuildingOneNoSection := lot(&buildingOne, nil, "146", LotTypeApartment)
	withoutBuilding := lot(nil, nil, "147", LotTypeApartment)

	tests := []struct {
		name  string
		p     Placement
		lots  []Lot
		code  string // "" means the placement is accepted
		about string
	}{
		{
			name: "same building and number, section not stated",
			p:    placement(&buildingOne, nil, "145", LotTypeApartment),
			lots: []Lot{inBuildingOneSectionOne},
			code: CodeLotAlreadyExists,
			// The section is not part of K2: the document simply does not state it.
		},
		{
			name: "same building and number, another section",
			p:    placement(&buildingOne, &sectionTwo, "145", LotTypeApartment),
			lots: []Lot{inBuildingOneSectionOne},
			code: CodeLotAlreadyExists,
		},
		{
			name: "same number in another building",
			p:    placement(&buildingTwo, nil, "145", LotTypeApartment),
			lots: []Lot{inBuildingOneSectionOne},
			code: "",
		},
		{
			name: "building unknown, one candidate with a building",
			p:    placement(nil, nil, "145", LotTypeApartment),
			lots: []Lot{inBuildingOneSectionOne},
			code: CodeMatchConflict,
		},
		{
			name: "building unknown, candidate without a building",
			p:    placement(nil, nil, "147", LotTypeApartment),
			lots: []Lot{withoutBuilding},
			code: CodeLotAlreadyExists,
		},
		{
			name: "building stated, stored lot without a building",
			p:    placement(&buildingOne, nil, "147", LotTypeApartment),
			lots: []Lot{withoutBuilding},
			code: CodeMatchConflict,
		},
		{
			name: "building unknown, candidates in two buildings",
			p:    placement(nil, nil, "145", LotTypeApartment),
			lots: []Lot{inBuildingOneSectionOne, lot(&buildingTwo, nil, "145", LotTypeApartment)},
			code: CodeMatchConflict,
		},
		{
			name: "exact match wins over a possible one",
			p:    placement(&buildingOne, nil, "146", LotTypeApartment),
			lots: []Lot{inBuildingOneNoSection, lot(nil, nil, "146", LotTypeApartment)},
			code: CodeLotAlreadyExists,
		},
		{
			name: "same number, another lot type",
			p:    placement(&buildingOne, nil, "145", LotTypeParking),
			lots: []Lot{inBuildingOneSectionOne},
			code: "",
		},
		{
			name: "no candidates",
			p:    placement(&buildingOne, nil, "900", LotTypeApartment),
			lots: nil,
			code: "",
		},
		{
			name: "similar numbers are not matched",
			p:    placement(&buildingOne, nil, "145", LotTypeApartment),
			lots: []Lot{lot(&buildingOne, nil, "145А", LotTypeApartment), lot(&buildingOne, nil, " 145", LotTypeApartment)},
			code: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckNewLotPlacement(tt.p, tt.lots)
			if tt.code == "" {
				if err != nil {
					t.Fatalf("placement rejected: %v", err)
				}
				return
			}
			assertRuleCode(t, err, tt.code)
		})
	}
}

func TestMatchPlacementIgnoresOtherProjects(t *testing.T) {
	project, other := NewID(), NewID()
	p := Placement{ProjectID: project, Type: LotTypeApartment, Number: "1"}
	foreign := Lot{ID: NewID(), Placement: Placement{ProjectID: other, Type: LotTypeApartment, Number: "1"}}
	if got := MatchPlacement(p, []Lot{foreign}); len(got) != 0 {
		t.Errorf("lots of another project must not be candidates: %+v", got)
	}
	own := Lot{ID: NewID(), Placement: p}
	got := MatchPlacement(p, []Lot{own})
	if len(got) != 1 || got[0].Kind != MatchExact || got[0].Lot.ID != own.ID {
		t.Errorf("candidates = %+v", got)
	}
}
