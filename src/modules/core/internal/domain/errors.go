package domain

import (
	"errors"
	"fmt"
)

// ErrNotFound is returned when a requested object does not exist.
var ErrNotFound = errors.New("not found")

// RuleError reports a violated domain rule or invalid input. Code is a stable
// machine-readable identifier (UI adapters translate it into user messages);
// Field optionally names the offending input.
type RuleError struct {
	Code   string
	Field  string
	Detail string
}

func (e *RuleError) Error() string {
	if e.Field != "" {
		return fmt.Sprintf("%s (%s): %s", e.Code, e.Field, e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Detail)
}

func ruleErr(code, field, detail string) error {
	return &RuleError{Code: code, Field: field, Detail: detail}
}

// AsRuleError returns the RuleError wrapped in err, if any.
func AsRuleError(err error) (*RuleError, bool) {
	var re *RuleError
	if errors.As(err, &re) {
		return re, true
	}
	return nil, false
}

// Rule error codes.
const (
	CodeRequired             = "required"
	CodeTooLong              = "too_long"
	CodeInvalidValue         = "invalid_value"
	CodeInvalidNumber        = "invalid_number"
	CodeMustBePositive       = "must_be_positive"
	CodeMustNotBeNegative    = "must_not_be_negative"
	CodeOutOfRange           = "out_of_range"
	CodeDiscountWithoutPrice = "price_before_discount_without_price"
	CodeDiscountNotGreater   = "price_before_discount_not_greater"
	CodePriceOnRequestPrice  = "price_on_request_with_price"
	CodeSectionWithoutBldg   = "section_without_building"
	CodePlacementMismatch    = "placement_mismatch"
	CodeLotAlreadyExists     = "lot_already_exists"
	CodeNameTaken            = "name_taken"
	CodeCodeTaken            = "code_taken"
	CodeSelfRelation         = "self_relation"
	CodePeriodOrder          = "period_order"
	CodePeriodOverlap        = "period_overlap"
	CodeObservationInFuture  = "observation_in_future"
	CodeObservationOutOfOrdr = "observation_out_of_order"
	CodeHistoryConflict      = "history_conflict"
	CodeSourceInactive       = "data_source_inactive"
	CodeSourceKeyConflict    = "source_key_conflict"
	CodeWrongProject         = "wrong_project"
	CodeDuplicateInBatch     = "duplicate_in_batch"
	CodeUnknownRef           = "unknown_ref"
	CodeInvalidTarget        = "invalid_target"
	CodeMatchConflict        = "match_conflict"
	CodeOriginConflict       = "origin_conflict"
)

// RuleCodes lists every rule code the domain can produce. Adapters use it to
// check that each code has a user message; a domain test checks that the list
// covers all Code* constants of the package.
var RuleCodes = []string{
	CodeRequired,
	CodeTooLong,
	CodeInvalidValue,
	CodeInvalidNumber,
	CodeMustBePositive,
	CodeMustNotBeNegative,
	CodeOutOfRange,
	CodeDiscountWithoutPrice,
	CodeDiscountNotGreater,
	CodePriceOnRequestPrice,
	CodeSectionWithoutBldg,
	CodePlacementMismatch,
	CodeLotAlreadyExists,
	CodeNameTaken,
	CodeCodeTaken,
	CodeSelfRelation,
	CodePeriodOrder,
	CodePeriodOverlap,
	CodeObservationInFuture,
	CodeObservationOutOfOrdr,
	CodeHistoryConflict,
	CodeSourceInactive,
	CodeSourceKeyConflict,
	CodeWrongProject,
	CodeDuplicateInBatch,
	CodeUnknownRef,
	CodeInvalidTarget,
	CodeMatchConflict,
	CodeOriginConflict,
}
