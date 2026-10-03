// Package orgchart manages department hierarchy assignments effective by date.
package orgchart

import "time"

// Date is a calendar day (proleptic Gregorian), time-of-day ignored.
type Date = time.Time

// Adjustment describes one reassignment of a department's parent,
// effective on [From, To). A zero To means "forever".
type Adjustment struct {
	Dept   string
	Parent string
	From   Date
	To     Date // zero value means open-ended
}

// Interval is a stored assignment of Parent to a department over [From, To).
type Interval struct {
	Parent string
	From   Date
	To     Date // zero value means open-ended
}

// ErrInvalidBatch is returned when a batch would leave the hierarchy invalid.
type ErrInvalidBatch struct {
	Reason string
}

func (e *ErrInvalidBatch) Error() string { return "orgchart: invalid batch: " + e.Reason }
