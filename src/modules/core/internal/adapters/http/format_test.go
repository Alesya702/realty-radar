package httpadapter

import (
	"slices"
	"strings"
	"testing"

	"realtyradar/src/modules/core/internal/domain"
)

func TestFormatMoneyAndArea(t *testing.T) {
	m := func(v domain.Money) *domain.Money { return &v }
	a := func(v domain.Area) *domain.Area { return &v }
	nb := func(s string) string { return strings.ReplaceAll(s, " ", " ") }

	tests := []struct {
		got, want string
	}{
		{formatMoney(m(1590000000)), nb("15 900 000") + " ₽"},
		{formatMoney(m(23590504)), nb("235 905") + ",04 ₽"},
		{formatMoney(m(99)), "0,99 ₽"},
		{formatMoney(m(100000)), nb("1 000") + " ₽"},
		{formatMoney(nil), "—"},
		{formatArea(a(6740)), "67,4 м²"},
		{formatArea(a(6700)), "67 м²"},
		{inputMoney(m(1590000050)), "15900000.5"},
		{inputArea(a(6740)), "67.4"},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("got %q, want %q", tt.got, tt.want)
		}
	}
}

func TestRuleMessage(t *testing.T) {
	msg := ruleMessage(&domain.RuleError{Code: domain.CodeRequired, Field: "number"})
	if msg != "Заполните поле «Номер»." {
		t.Errorf("message = %q", msg)
	}
	if msg := ruleMessage(&domain.RuleError{Code: domain.CodeDiscountNotGreater}); !strings.Contains(msg, "Цена до скидки должна быть больше цены") {
		t.Errorf("message = %q", msg)
	}
	if msg := ruleMessage(&domain.RuleError{Code: "something_new", Detail: "x"}); !strings.HasPrefix(msg, "Некорректные данные") {
		t.Errorf("unknown code message = %q", msg)
	}
	// Every rule code the domain can produce has a user message. The list
	// comes from the domain (domain.RuleCodes), not from this test, so that a
	// new code without a message fails the build instead of reaching the user
	// as "Некорректные данные: <code>".
	for _, code := range domain.RuleCodes {
		if _, ok := codeMessages[code]; !ok {
			t.Errorf("no user message for rule code %q", code)
		}
		msg := ruleMessage(&domain.RuleError{Code: code, Field: "number", Detail: "x"})
		if strings.HasPrefix(msg, "Некорректные данные") || strings.Contains(msg, "%!") {
			t.Errorf("rule code %q renders as %q", code, msg)
		}
	}
	for code := range codeMessages {
		if !slices.Contains(domain.RuleCodes, code) {
			t.Errorf("message for unknown rule code %q", code)
		}
	}
}
