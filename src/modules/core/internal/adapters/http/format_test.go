package httpadapter

import (
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
	// Every rule code produced by the domain has a user message.
	for _, code := range []string{
		domain.CodeRequired, domain.CodeTooLong, domain.CodeInvalidValue, domain.CodeInvalidNumber,
		domain.CodeMustBePositive, domain.CodeMustNotBeNegative, domain.CodeOutOfRange,
		domain.CodeDiscountWithoutPrice, domain.CodeDiscountNotGreater, domain.CodeSectionWithoutBldg,
		domain.CodePlacementMismatch, domain.CodeLotAlreadyExists, domain.CodeNameTaken, domain.CodeCodeTaken,
		domain.CodeSelfRelation, domain.CodePeriodOrder, domain.CodePeriodOverlap, domain.CodeObservationInFuture,
		domain.CodeObservationOutOfOrdr, domain.CodeHistoryConflict, domain.CodeSourceInactive,
		domain.CodeSourceKeyConflict, domain.CodeWrongProject,
	} {
		if _, ok := codeMessages[code]; !ok {
			t.Errorf("no user message for rule code %q", code)
		}
	}
}
