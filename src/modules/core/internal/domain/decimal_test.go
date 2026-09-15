package domain

import (
	"strings"
	"testing"
)

func TestParseMoney(t *testing.T) {
	tests := []struct {
		in      string
		want    Money
		wantErr bool
	}{
		{"15900000", 1590000000, false},
		{"15 900 000", 1590000000, false},
		{"15 900 000,5", 1590000050, false},
		{"15900000.05", 1590000005, false},
		{"0.99", 99, false},
		{"-100", -10000, false},
		{"", 0, true},
		{"12,345", 0, true},
		{"12.", 0, true},
		{"abc", 0, true},
		{"1.2.3", 0, true},
		{"99999999999999", 0, true}, // 14 integer digits > numeric(15,2)
		{"9999999999999.99", 999999999999999, false},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseMoney(tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseMoney(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseMoney(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseAreaAndFormat(t *testing.T) {
	a, err := ParseArea("67,4")
	if err != nil {
		t.Fatal(err)
	}
	if a != 6740 || a.String() != "67.40" {
		t.Errorf("area = %d (%s), want 6740 (67.40)", a, a)
	}
	if _, err := ParseArea("123456789"); err == nil {
		t.Error("expected error for area wider than numeric(10,2)")
	}
	if got := Money(-5).String(); got != "-0.05" {
		t.Errorf("Money(-5) = %s", got)
	}
}

func TestPricePerSqm(t *testing.T) {
	tests := []struct {
		name   string
		price  Money
		area   Area
		want   Money
		wantOK bool
	}{
		{"exact", 1000000, 10000, 10000, true},        // 10 000 ₽ / 100 m² = 100 ₽
		{"lot 145", 1590000000, 6740, 23590504, true}, // 15 900 000 / 67.4 = 235 905.04
		{"rounds half up", 1, 300, 0, true},           // 0.01 / 3 = 0.0033 → 0.00
		{"zero area", 100, 0, 0, false},
		{"negative area", 100, -1, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := PricePerSqm(tt.price, tt.area)
			if ok != tt.wantOK || (ok && got != tt.want) {
				t.Errorf("PricePerSqm(%d, %d) = %d, %v; want %d, %v", tt.price, tt.area, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestIDRoundTrip(t *testing.T) {
	id := NewID()
	if id.IsZero() {
		t.Fatal("NewID returned zero id")
	}
	s := id.String()
	if len(s) != 36 || s[14] != '7' {
		t.Errorf("id %s is not a UUIDv7", s)
	}
	parsed, err := ParseID(strings.ToUpper(s))
	if err != nil || parsed != id {
		t.Errorf("ParseID(%s) = %v, %v", s, parsed, err)
	}
	for _, bad := range []string{"", "not-an-id", strings.Repeat("z", 36), s[:35] + "-"} {
		if _, err := ParseID(bad); err == nil {
			t.Errorf("ParseID(%q) expected error", bad)
		}
	}
}
