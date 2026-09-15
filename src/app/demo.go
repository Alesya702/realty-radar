package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"realtyradar/src/modules/core"
	"realtyradar/src/platform/database"
)

// DemoProjectName is the name of the demo company project; its presence marks
// the demo dataset as loaded.
const DemoProjectName = "ЖК «Альфа» (демо)"

// SeedDemo loads the demonstration dataset into the configured database.
func SeedDemo(ctx context.Context) error {
	cfg, logger, err := bootstrap(os.Stderr)
	if err != nil {
		return err
	}
	pool, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer pool.Close()

	m, err := core.New(pool, logger, core.Nav())
	if err != nil {
		return err
	}
	loaded, err := LoadDemoData(ctx, m)
	if err != nil {
		return err
	}
	if loaded {
		logger.Info("demo data loaded", "project", DemoProjectName)
	} else {
		logger.Info("demo data already present, nothing changed", "project", DemoProjectName)
	}
	return nil
}

// LoadDemoData creates a small fictional dataset through the Core public
// contract: two developers, a company project and an external competitor,
// buildings, a section, lots and the price history of lot 145. It returns
// false when the dataset is already present.
func LoadDemoData(ctx context.Context, m *core.Module) (bool, error) {
	projects, err := m.ListProjects(ctx, time.Now())
	if err != nil {
		return false, fmt.Errorf("demo: list projects: %w", err)
	}
	for _, p := range projects {
		if p.Project.Name == DemoProjectName {
			return false, nil
		}
	}

	msk := time.FixedZone("MSK", 3*60*60)
	t1 := time.Date(2026, 9, 1, 10, 0, 0, 0, msk)
	t2 := time.Date(2026, 9, 8, 10, 0, 0, 0, msk)
	t3 := time.Date(2026, 9, 12, 10, 0, 0, 0, msk)
	jan1 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	manual, err := m.DataSourceByCode(ctx, core.ManualDataSourceCode)
	if err != nil {
		return false, fmt.Errorf("demo: %w", err)
	}

	devA, err := m.CreateDeveloper(ctx, "Девелопер А (демо)")
	if err != nil {
		return false, fmt.Errorf("demo: %w", err)
	}
	devB, err := m.CreateDeveloper(ctx, "Девелопер Б (демо)")
	if err != nil {
		return false, fmt.Errorf("demo: %w", err)
	}
	alpha, err := m.CreateProject(ctx, core.CreateProjectInput{Name: DemoProjectName, DeveloperID: &devA.ID, City: "Москва"})
	if err != nil {
		return false, fmt.Errorf("demo: %w", err)
	}
	beta, err := m.CreateProject(ctx, core.CreateProjectInput{Name: "ЖК «Бета» (демо)", DeveloperID: &devB.ID, City: "Москва"})
	if err != nil {
		return false, fmt.Errorf("demo: %w", err)
	}
	if _, err := m.AddProjectToPortfolio(ctx, core.AddToPortfolioInput{ProjectID: alpha.ID, ValidFrom: &jan1, Note: "Демонстрационный проект компании"}); err != nil {
		return false, fmt.Errorf("demo: %w", err)
	}
	if _, err := m.EstablishProjectRelation(ctx, core.RelationInput{
		SubjectProjectID: alpha.ID, RelatedProjectID: beta.ID, Type: core.RelationDirectCompetitor,
		Rationale: "Соседняя локация, сопоставимый класс и диапазон цен (демо)", ValidFrom: jan1,
	}); err != nil {
		return false, fmt.Errorf("demo: %w", err)
	}

	rub := func(v int64) core.Money { return core.Money(v * 100) }
	area := func(hundredths int64) core.Area { return core.Area(hundredths) }
	apartment := func(rooms int, a core.Area, price core.Money) core.ObservedState {
		return core.ObservedState{
			Status: core.Value(core.LotStatusAvailable), Rooms: core.Value(rooms),
			AreaTotal: core.Value(a), Price: core.Value(price), PriceBeforeDiscount: core.Absent[core.Money](),
		}
	}
	floor7, floor8, floor3 := 7, 8, 3

	// Observation 1 (1 September): the structure is confirmed together with the first states.
	first, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: manual.ID, OriginKind: core.OriginManualEntry, ObservedAt: t1, ProjectID: alpha.ID,
		Buildings: []core.BuildingDecision{{Ref: "k1", NewName: "Корпус 1"}},
		Sections:  []core.SectionDecision{{Ref: "k1s1", BuildingRef: "k1", NewName: "1"}},
		Items: []core.LotObservation{
			{Ref: "145", Target: core.LotTarget{NewLot: &core.NewLotPlacement{BuildingRef: "k1", Type: core.LotTypeApartment, Number: "145", Floor: &floor7}},
				State: apartment(2, area(6740), rub(15_900_000))},
			{Ref: "146", Target: core.LotTarget{NewLot: &core.NewLotPlacement{BuildingRef: "k1", SectionRef: "k1s1", Type: core.LotTypeApartment, Number: "146", Floor: &floor8}},
				State: func() core.ObservedState {
					s := apartment(1, area(4120), rub(11_200_000))
					s.PriceBeforeDiscount = core.Value(rub(11_800_000))
					return s
				}()},
		},
	})
	if err != nil {
		return false, fmt.Errorf("demo: observation 1: %w", err)
	}
	lot145, lot146 := first.Items[0].LotID, first.Items[1].LotID

	// 8 September: the same values again — no new version, only the last confirmation moves.
	if _, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: manual.ID, OriginKind: core.OriginManualEntry, ObservedAt: t2, ProjectID: alpha.ID,
		Items: []core.LotObservation{{Ref: "145", Target: core.LotTarget{ExistingLotID: &lot145}, State: apartment(2, area(6740), rub(15_900_000))}},
	}); err != nil {
		return false, fmt.Errorf("demo: repeated observation: %w", err)
	}

	// Observation 2 (12 September): lot 145 costs more, lot 146 is reserved.
	reserved := apartment(1, area(4120), rub(11_200_000))
	reserved.Status = core.Value(core.LotStatusReserved)
	reserved.PriceBeforeDiscount = core.Value(rub(11_800_000))
	if _, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: manual.ID, OriginKind: core.OriginManualEntry, ObservedAt: t3, ProjectID: alpha.ID,
		Items: []core.LotObservation{
			{Ref: "145", Target: core.LotTarget{ExistingLotID: &lot145}, State: apartment(2, area(6740), rub(16_400_000))},
			{Ref: "146", Target: core.LotTarget{ExistingLotID: &lot146}, State: reserved},
		},
	}); err != nil {
		return false, fmt.Errorf("demo: observation 2: %w", err)
	}

	// External project: one building and one lot.
	if _, err := m.IngestObservationBatch(ctx, core.ObservationBatch{
		DataSourceID: manual.ID, OriginKind: core.OriginManualEntry, ObservedAt: t1, ProjectID: beta.ID,
		Buildings: []core.BuildingDecision{{Ref: "a", NewName: "Корпус А"}},
		Items: []core.LotObservation{
			{Ref: "12", Target: core.LotTarget{NewLot: &core.NewLotPlacement{BuildingRef: "a", Type: core.LotTypeApartment, Number: "12", Floor: &floor3}},
				State: apartment(2, area(6400), rub(16_100_000))},
		},
	}); err != nil {
		return false, fmt.Errorf("demo: external project: %w", err)
	}
	return true, nil
}
