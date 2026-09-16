package domain

// Matching of an observed lot to an existing one (core-domain-model.md, 7.2).
//
// Identity key K2 is (project, building, lot type, number): the section is NOT
// part of the key, because sources state the section inconsistently. A number
// without a building is only a possible match: the same number may exist in
// several buildings of the project.
//
// Matching is deterministic: values are compared as they are, never by
// similarity (no fuzzy matching). The decision to reuse an existing lot is
// always the user's; Core only refuses to create a lot silently when a
// candidate exists.

// MatchCandidateKind classifies how an existing lot relates to a placement.
type MatchCandidateKind uint8

// Candidate kinds.
const (
	// MatchExact is identity by K2: same building (both known and equal, or
	// both unknown), same lot type and number.
	MatchExact MatchCandidateKind = iota + 1
	// MatchPossible is a number that matches while the building is unknown on
	// one of the sides.
	MatchPossible
)

// MatchCandidate is an existing lot that a placement may denote.
type MatchCandidate struct {
	Lot  Lot
	Kind MatchCandidateKind
}

// MatchPlacement classifies lots of the project that share the lot type and
// number of p. It is the single implementation of K2 matching: callers pass
// every lot with the same (project, lot type, number) and get the candidates
// for that placement. Lots that differ in a known building are not candidates.
func MatchPlacement(p Placement, lots []Lot) []MatchCandidate {
	candidates := make([]MatchCandidate, 0, len(lots))
	for _, lot := range lots {
		c := lot.Placement
		if c.ProjectID != p.ProjectID || c.Type != p.Type || c.Number != p.Number {
			continue
		}
		switch {
		case equalIDPtr(c.BuildingID, p.BuildingID):
			candidates = append(candidates, MatchCandidate{Lot: lot, Kind: MatchExact})
		case c.BuildingID == nil || p.BuildingID == nil:
			// One side does not state the building: the lots may be the same.
			candidates = append(candidates, MatchCandidate{Lot: lot, Kind: MatchPossible})
		}
		// Both buildings known and different: a different lot.
	}
	return candidates
}

// CheckNewLotPlacement refuses a confirmed "new lot" decision when the project
// already contains a lot the placement may denote. lots are all lots of the
// project with the same lot type and number (see MatchPlacement).
//
//   - exactly one K2 match: the lot already exists (lot_already_exists), the
//     caller must observe that lot instead of creating a second one;
//   - only possible matches, or several matches: the decision is ambiguous
//     (match_conflict) and the user has to resolve it.
func CheckNewLotPlacement(p Placement, lots []Lot) error {
	var exact, possible []MatchCandidate
	for _, c := range MatchPlacement(p, lots) {
		if c.Kind == MatchExact {
			exact = append(exact, c)
		} else {
			possible = append(possible, c)
		}
	}
	switch {
	case len(exact) == 1:
		return ruleErr(CodeLotAlreadyExists, "number",
			"a lot with the same project, building, type and number already exists: "+exact[0].Lot.ID.String())
	case len(exact) > 1:
		return ruleErr(CodeMatchConflict, "number", "several lots match the placement: "+candidateIDs(exact))
	case len(possible) > 0:
		return ruleErr(CodeMatchConflict, "number",
			"the number matches a lot whose building is stated differently, choose the lot explicitly: "+candidateIDs(possible))
	}
	return nil
}

func candidateIDs(candidates []MatchCandidate) string {
	s := ""
	for i, c := range candidates {
		if i > 0 {
			s += ", "
		}
		s += c.Lot.ID.String()
	}
	return s
}

func equalIDPtr(a, b *ID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
