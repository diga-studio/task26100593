package orgchart

import "time"

// epoch is the first day the hierarchy must be defined for.
var epoch = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// Chart holds the full in-memory history of department parent assignments.
type Chart struct {
	root  string
	depts map[string]bool
	// hist maps a non-root department to its parent intervals,
	// sorted by From, non-overlapping, covering [epoch, forever).
	hist map[string][]Interval
}

// New creates a Chart with the given fixed department set and unique root.
func New(root string, depts []string) *Chart {
	set := make(map[string]bool, len(depts))
	for _, d := range depts {
		set[d] = true
	}
	set[root] = true
	return &Chart{root: root, depts: set, hist: make(map[string][]Interval)}
}

// LoadInitial loads the caller-guaranteed-valid initial history for one
// department. Intervals must be sorted, non-overlapping, and cover
// [2000-01-01, forever).
func (c *Chart) LoadInitial(dept string, intervals []Interval) {
	cp := make([]Interval, len(intervals))
	copy(cp, intervals)
	c.hist[dept] = cp
}

// contains reports whether d falls in [iv.From, iv.To).
func contains(iv Interval, d Date) bool {
	return !d.Before(iv.From) && (iv.To.IsZero() || d.Before(iv.To))
}

// parentAt returns the parent of dept on date d.
func (c *Chart) parentAt(dept string, d Date) (string, bool) {
	if dept == c.root {
		return "", false
	}
	for _, iv := range c.hist[dept] {
		if contains(iv, d) {
			return iv.Parent, true
		}
	}
	return "", false
}

// replaceIntervals returns a new interval list where [from, to) is
// overridden by parent, preserving everything outside that range.
func replaceIntervals(ivs []Interval, parent string, from, to Date) []Interval {
	out := make([]Interval, 0, len(ivs)+2)
	for _, iv := range ivs {
		// Part of iv strictly before from.
		if iv.From.Before(from) {
			end := iv.To
			if end.IsZero() || from.Before(end) {
				end = from
			}
			out = append(out, Interval{Parent: iv.Parent, From: iv.From, To: end})
		}
		// Part of iv strictly after to (only when to is finite).
		if !to.IsZero() && (iv.To.IsZero() || iv.To.After(to)) {
			start := iv.From
			if start.Before(to) {
				start = to
			}
			out = append(out, Interval{Parent: iv.Parent, From: start, To: iv.To})
		}
	}
	// Insert the new interval in sorted position.
	ins := Interval{Parent: parent, From: from, To: to}
	i := 0
	for i < len(out) && out[i].From.Before(from) {
		i++
	}
	out = append(out, Interval{})
	copy(out[i+1:], out[i:])
	out[i] = ins
	return out
}

// ApplyBatch validates and applies a batch of adjustments atomically.
// The batch is judged by the combined result of all adjustments; on any
// violation the whole batch is rejected and prior history is unchanged.
func (c *Chart) ApplyBatch(adj []Adjustment) error {
	// Build the candidate new history on a copy; only affected departments
	// get their slices replaced.
	next := make(map[string][]Interval, len(c.hist))
	for k, v := range c.hist {
		next[k] = v
	}
	for _, a := range adj {
		if a.Dept == c.root {
			return &ErrInvalidBatch{Reason: "root department cannot be adjusted"}
		}
		if !c.depts[a.Dept] || !c.depts[a.Parent] {
			return &ErrInvalidBatch{Reason: "unknown department"}
		}
		if a.From.Before(epoch) {
			return &ErrInvalidBatch{Reason: "adjustment starts before 2000-01-01"}
		}
		if !a.To.IsZero() && !a.To.After(a.From) {
			return &ErrInvalidBatch{Reason: "empty or inverted adjustment range"}
		}
		next[a.Dept] = replaceIntervals(next[a.Dept], a.Parent, a.From, a.To)
	}
	if err := c.validate(next); err != nil {
		return err
	}
	c.hist = next
	return nil
}

// validate checks that on every day from epoch onward, following parents
// from any department reaches the root. The assignment graph only changes
// at interval boundaries, so checking each boundary date is sufficient.
func (c *Chart) validate(hist map[string][]Interval) error {
	dates := map[time.Time]bool{epoch: true}
	for _, ivs := range hist {
		for _, iv := range ivs {
			if !iv.From.Before(epoch) {
				dates[iv.From] = true
			}
			if !iv.To.IsZero() && iv.To.After(epoch) {
				dates[iv.To] = true
			}
		}
	}
	for d := range dates {
		if err := c.validateDay(hist, d); err != nil {
			return err
		}
	}
	return nil
}

// validateDay ensures every department reaches the root on date d.
func (c *Chart) validateDay(hist map[string][]Interval, d Date) error {
	for dept := range c.depts {
		seen := make(map[string]bool, len(c.depts))
		cur := dept
		for cur != c.root {
			if seen[cur] {
				return &ErrInvalidBatch{Reason: "cycle detected on " + d.Format("2006-01-02")}
			}
			seen[cur] = true
			parent, ok := parentAtIn(hist, cur, d)
			if !ok {
				return &ErrInvalidBatch{Reason: "department " + cur + " has no parent on " + d.Format("2006-01-02")}
			}
			cur = parent
		}
	}
	return nil
}

func parentAtIn(hist map[string][]Interval, dept string, d Date) (string, bool) {
	for _, iv := range hist[dept] {
		if contains(iv, d) {
			return iv.Parent, true
		}
	}
	return "", false
}

// Chain returns the chain from dept up to the root on date d,
// starting with dept itself and ending with the root.
func (c *Chart) Chain(dept string, d Date) []string {
	chain := []string{dept}
	cur := dept
	for cur != c.root {
		parent, ok := c.parentAt(cur, d)
		if !ok {
			break
		}
		chain = append(chain, parent)
		cur = parent
	}
	return chain
}
