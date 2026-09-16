package domain

import (
	"sync"
	"time"
	_ "time/tzdata" // the business time zone must be available on any OS
)

// businessTimeZone is the time zone of business dates (A-07). The rule belongs
// to the domain: every caller of Core must get the same business date for the
// same moment, whatever zone the moment is expressed in.
const businessTimeZone = "Europe/Moscow"

var businessLocation = sync.OnceValue(func() *time.Location {
	loc, err := time.LoadLocation(businessTimeZone)
	if err != nil {
		// Moscow has had a fixed UTC+3 offset since 2014; the fallback keeps
		// business dates correct if the zone database is unavailable.
		return time.FixedZone("MSK", 3*60*60)
	}
	return loc
})

// BusinessLocation returns the time zone of business dates and UI times.
func BusinessLocation() *time.Location { return businessLocation() }

// BusinessDateOf returns the business calendar date of moment t: the date in
// the business time zone, as UTC midnight. A moment already reduced to a date
// (UTC midnight) keeps its date.
func BusinessDateOf(t time.Time) time.Time { return DateOf(t.In(businessLocation())) }
