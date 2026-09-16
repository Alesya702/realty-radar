package domain

import (
	"testing"
	"time"
)

func TestBusinessDateOfAroundTheDateBoundary(t *testing.T) {
	// Moscow is UTC+3: the business date turns over at 21:00 UTC.
	tests := []struct {
		name string
		at   time.Time
		want string
	}{
		{"just before the Moscow midnight", time.Date(2026, 9, 16, 20, 59, 59, 0, time.UTC), "2026-09-16"},
		{"at the Moscow midnight", time.Date(2026, 9, 16, 21, 0, 0, 0, time.UTC), "2026-09-17"},
		{"late evening UTC", time.Date(2026, 9, 16, 23, 30, 0, 0, time.UTC), "2026-09-17"},
		{"midnight UTC", time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC), "2026-09-17"},
		{"the same moment in another zone", time.Date(2026, 9, 16, 17, 30, 0, 0, time.FixedZone("EDT", -4*60*60)), "2026-09-17"},
		{"a date already reduced to UTC midnight keeps its date", time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC), "2026-09-16"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BusinessDateOf(tt.at)
			if got.Format("2006-01-02") != tt.want {
				t.Errorf("BusinessDateOf(%s) = %s, want %s", tt.at, got.Format("2006-01-02"), tt.want)
			}
			if got.Location() != time.UTC || got.Hour() != 0 || got.Minute() != 0 {
				t.Errorf("business date must be UTC midnight, got %s", got)
			}
		})
	}
}

func TestBusinessDateDoesNotDependOnTheCallerZone(t *testing.T) {
	// The same moment expressed in three zones must give one business date:
	// a module passing time.Now().UTC() must not get yesterday (review M6).
	at := time.Date(2026, 9, 16, 22, 15, 0, 0, time.UTC)
	zones := []*time.Location{time.UTC, time.FixedZone("UTC-8", -8*60*60), BusinessLocation()}
	want := BusinessDateOf(at)
	for _, loc := range zones {
		if got := BusinessDateOf(at.In(loc)); !got.Equal(want) {
			t.Errorf("business date in %s = %s, want %s", loc, got, want)
		}
	}
	if want.Format("2006-01-02") != "2026-09-17" {
		t.Errorf("business date = %s, want 2026-09-17", want.Format("2006-01-02"))
	}
}

func TestBusinessLocationIsMoscow(t *testing.T) {
	_, offset := time.Date(2026, 9, 16, 12, 0, 0, 0, BusinessLocation()).Zone()
	if offset != 3*60*60 {
		t.Errorf("business zone offset = %d s, want 10800 s (A-07, Europe/Moscow)", offset)
	}
}
