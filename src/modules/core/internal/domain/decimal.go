package domain

import (
	"strconv"
	"strings"
)

// Money is an amount in rubles kept in kopecks, without floating point (A-08).
// It maps to numeric(15,2).
type Money int64

// Area is an area in square metres kept in hundredths. It maps to numeric(10,2).
type Area int64

const (
	maxMoneyIntDigits = 13 // numeric(15,2)
	maxAreaIntDigits  = 8  // numeric(10,2)
)

// ParseMoney parses a ruble amount such as "15 900 000", "15900000.50" or "15 900 000,5".
// Spaces (including non-breaking) are ignored; comma and dot are decimal separators.
func ParseMoney(s string) (Money, error) {
	v, err := parseFixed2(s, maxMoneyIntDigits)
	return Money(v), err
}

// ParseArea parses an area in square metres such as "67.4" or "67,4".
func ParseArea(s string) (Area, error) {
	v, err := parseFixed2(s, maxAreaIntDigits)
	return Area(v), err
}

// String returns the canonical decimal form with two fraction digits ("15900000.00").
func (m Money) String() string { return formatFixed2(int64(m)) }

// String returns the canonical decimal form with two fraction digits ("67.40").
func (a Area) String() string { return formatFixed2(int64(a)) }

// PricePerSqm returns price / area rounded half up to kopecks. ok is false when
// the area is not positive. The value is calculated, never stored (core-domain-model.md, 6.3).
func PricePerSqm(price Money, area Area) (Money, bool) {
	if area <= 0 || price < 0 {
		return 0, false
	}
	// price in kopecks / (area in hundredths / 100) = price*100/area kopecks per m².
	num := int64(price) * 100
	return Money((2*num + int64(area)) / (2 * int64(area))), true
}

func parseFixed2(s string, maxIntDigits int) (int64, error) {
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', ' ', ' ', '\t':
			return -1
		case ',':
			return '.'
		}
		return r
	}, s)
	if clean == "" {
		return 0, ruleErr(CodeInvalidNumber, "", "empty number")
	}
	negative := false
	if clean[0] == '-' {
		negative = true
		clean = clean[1:]
	}
	intPart, fracPart, hasFrac := strings.Cut(clean, ".")
	if intPart == "" || !allDigits(intPart) || (hasFrac && (!allDigits(fracPart) || fracPart == "")) {
		return 0, ruleErr(CodeInvalidNumber, "", "not a decimal number: "+s)
	}
	if len(fracPart) > 2 {
		return 0, ruleErr(CodeInvalidNumber, "", "more than two fraction digits: "+s)
	}
	intPart = strings.TrimLeft(intPart, "0")
	if len(intPart) > maxIntDigits {
		return 0, ruleErr(CodeOutOfRange, "", "number is too large: "+s)
	}
	for len(fracPart) < 2 {
		fracPart += "0"
	}
	// fracPart always has two digits, so the concatenation is never empty.
	v, err := strconv.ParseInt(intPart+fracPart, 10, 64)
	if err != nil {
		return 0, ruleErr(CodeOutOfRange, "", "number is too large: "+s)
	}
	if negative {
		v = -v
	}
	return v, nil
}

func formatFixed2(v int64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return sign + strconv.FormatInt(v/100, 10) + "." + leftPad2(v%100)
}

func leftPad2(v int64) string {
	if v < 10 {
		return "0" + strconv.FormatInt(v, 10)
	}
	return strconv.FormatInt(v, 10)
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
