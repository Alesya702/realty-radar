package app

import (
	"context"
	"fmt"
	"time"

	"realtyradar/src/modules/core/internal/domain"
)

// ListLots returns the lots of a project with their canonical current state.
func (s *Service) ListLots(ctx context.Context, projectID domain.ID) ([]LotRow, error) {
	rows, err := s.repo.ListLotRows(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("list lots of project %s: %w", projectID, err)
	}
	return rows, nil
}

// GetLot returns a lot.
func (s *Service) GetLot(ctx context.Context, id domain.ID) (domain.Lot, error) {
	return s.repo.GetLot(ctx, id)
}

// LotHistory is the canonical history of a lot: versions of its primary
// listing, oldest first.
type LotHistory struct {
	Lot      domain.Lot
	Listings []ListingView
	Primary  *ListingView
	Versions []domain.Version
}

// Current returns the current canonical state.
func (h LotHistory) Current() *domain.Version {
	for i := len(h.Versions) - 1; i >= 0; i-- {
		if h.Versions[i].IsCurrent() {
			return &h.Versions[i]
		}
	}
	return nil
}

// At returns the canonical state at moment t.
func (h LotHistory) At(t time.Time) *domain.Version {
	v, ok := domain.StateAt(h.Versions, t)
	if !ok {
		return nil
	}
	return &v
}

// GetLotHistory returns a lot with its listings and the history of the primary listing.
func (s *Service) GetLotHistory(ctx context.Context, lotID domain.ID) (LotHistory, error) {
	lot, err := s.repo.GetLot(ctx, lotID)
	if err != nil {
		return LotHistory{}, fmt.Errorf("lot %s: %w", lotID, err)
	}
	listings, err := s.repo.ListListings(ctx, lotID)
	if err != nil {
		return LotHistory{}, fmt.Errorf("listings of lot %s: %w", lotID, err)
	}
	h := LotHistory{Lot: lot, Listings: listings}
	for i := range listings {
		if lot.PrimaryListingID != nil && listings[i].Listing.ID == *lot.PrimaryListingID {
			h.Primary = &listings[i]
		}
	}
	if h.Primary != nil {
		h.Versions, err = s.repo.ListVersions(ctx, h.Primary.Listing.ID)
		if err != nil {
			return LotHistory{}, fmt.Errorf("history of lot %s: %w", lotID, err)
		}
		domain.SortVersions(h.Versions)
	}
	return h, nil
}

// GetLotState returns the canonical state of a lot at moment t (nil when the
// lot had no state then).
func (s *Service) GetLotState(ctx context.Context, lotID domain.ID, t time.Time) (*domain.Version, error) {
	h, err := s.GetLotHistory(ctx, lotID)
	if err != nil {
		return nil, err
	}
	return h.At(t), nil
}
