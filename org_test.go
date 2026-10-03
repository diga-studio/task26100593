package orgchart

import (
	"reflect"
	"testing"
	"time"
)

func day(s string) Date {
	t, err := time.ParseInLocation("2006-01-02", s, time.UTC)
	if err != nil {
		panic(err)
	}
	return t
}

// newChart builds R -> A -> B and R -> C from 2000-01-01.
func newChart() *Chart {
	c := New("R", []string{"A", "B", "C"})
	c.LoadInitial("A", []Interval{{Parent: "R", From: day("2000-01-01")}})
	c.LoadInitial("B", []Interval{{Parent: "A", From: day("2000-01-01")}})
	c.LoadInitial("C", []Interval{{Parent: "R", From: day("2000-01-01")}})
	return c
}

func checkChain(t *testing.T, c *Chart, dept, date string, want []string) {
	t.Helper()
	got := c.Chain(dept, day(date))
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Chain(%s, %s) = %v, want %v", dept, date, got, want)
	}
}

func TestInitialChain(t *testing.T) {
	c := newChart()
	checkChain(t, c, "B", "2000-01-01", []string{"B", "A", "R"})
	checkChain(t, c, "B", "2024-05-06", []string{"B", "A", "R"})
	checkChain(t, c, "R", "2024-05-06", []string{"R"})
}

// A and B swap positions in one batch: R->A->B becomes R->B->A.
// Applying either adjustment alone would create a cycle; the batch is
// judged by its combined result and must succeed.
func TestBatchSwapSameDay(t *testing.T) {
	c := newChart()
	err := c.ApplyBatch([]Adjustment{
		{Dept: "A", Parent: "B", From: day("2024-03-01")},
		{Dept: "B", Parent: "R", From: day("2024-03-01")},
	})
	if err != nil {
		t.Fatalf("swap batch rejected: %v", err)
	}
	// Day before the swap: old hierarchy intact.
	checkChain(t, c, "A", "2024-02-29", []string{"A", "R"})
	checkChain(t, c, "B", "2024-02-29", []string{"B", "A", "R"})
	// Effective day: swapped hierarchy.
	checkChain(t, c, "A", "2024-03-01", []string{"A", "B", "R"})
	checkChain(t, c, "B", "2024-03-01", []string{"B", "R"})
	// Later days keep the new hierarchy.
	checkChain(t, c, "A", "2030-12-31", []string{"A", "B", "R"})
}

// Input order within a batch must not matter.
func TestBatchOrderIrrelevant(t *testing.T) {
	c1 := newChart()
	c2 := newChart()
	fwd := []Adjustment{
		{Dept: "A", Parent: "B", From: day("2024-03-01")},
		{Dept: "B", Parent: "R", From: day("2024-03-01")},
	}
	rev := []Adjustment{fwd[1], fwd[0]}
	if err := c1.ApplyBatch(fwd); err != nil {
		t.Fatalf("forward order rejected: %v", err)
	}
	if err := c2.ApplyBatch(rev); err != nil {
		t.Fatalf("reverse order rejected: %v", err)
	}
	for _, d := range []string{"2024-02-29", "2024-03-01", "2025-01-01"} {
		if got, want := c1.Chain("A", day(d)), c2.Chain("A", day(d)); !reflect.DeepEqual(got, want) {
			t.Errorf("order changed result on %s: %v vs %v", d, got, want)
		}
	}
}

// A bounded adjustment replaces only its range; the old assignment
// resumes at the (exclusive) end date.
func TestBoundedAdjustmentBoundaries(t *testing.T) {
	c := newChart()
	err := c.ApplyBatch([]Adjustment{
		{Dept: "C", Parent: "A", From: day("2024-01-10"), To: day("2024-01-20")},
	})
	if err != nil {
		t.Fatalf("bounded adjustment rejected: %v", err)
	}
	checkChain(t, c, "C", "2024-01-09", []string{"C", "R"})      // before
	checkChain(t, c, "C", "2024-01-10", []string{"C", "A", "R"}) // From inclusive
	checkChain(t, c, "C", "2024-01-19", []string{"C", "A", "R"}) // last day inside
	checkChain(t, c, "C", "2024-01-20", []string{"C", "R"})      // To exclusive
}

// Retroactive adjustments rewrite the past but keep surrounding days.
func TestRetroactiveAdjustment(t *testing.T) {
	c := newChart()
	err := c.ApplyBatch([]Adjustment{
		{Dept: "B", Parent: "C", From: day("2010-06-01"), To: day("2010-07-01")},
	})
	if err != nil {
		t.Fatalf("retroactive adjustment rejected: %v", err)
	}
	checkChain(t, c, "B", "2010-05-31", []string{"B", "A", "R"})
	checkChain(t, c, "B", "2010-06-15", []string{"B", "C", "R"})
	checkChain(t, c, "B", "2010-07-01", []string{"B", "A", "R"})
	checkChain(t, c, "B", "2000-01-01", []string{"B", "A", "R"})
}

// A previously committed future arrangement makes the new batch cyclic
// only in the middle of its range. Both boundary days are individually
// legal, yet the batch must be rejected as a whole and history preserved.
func TestRejectMidRangeCycle(t *testing.T) {
	c := New("R", []string{"X", "Y"})
	c.LoadInitial("X", []Interval{{Parent: "R", From: day("2000-01-01")}})
	c.LoadInitial("Y", []Interval{{Parent: "R", From: day("2000-01-01")}})

	// Committed future plan: X reports to Y from 2030-06-01 onward.
	if err := c.ApplyBatch([]Adjustment{
		{Dept: "X", Parent: "Y", From: day("2030-06-01")},
	}); err != nil {
		t.Fatalf("future plan rejected: %v", err)
	}

	// New batch: Y reports to X on [2030-01-01, 2040-01-01).
	// On [2030-01-01, 2030-06-01) this is fine (Y->X->R), and after
	// 2040-01-01 it is also fine, but on [2030-06-01, 2040-01-01)
	// X->Y and Y->X form a cycle.
	err := c.ApplyBatch([]Adjustment{
		{Dept: "Y", Parent: "X", From: day("2030-01-01"), To: day("2040-01-01")},
	})
	if err == nil {
		t.Fatal("batch with mid-range cycle was accepted")
	}

	// History of all involved departments must be unchanged.
	checkChain(t, c, "Y", "2030-01-01", []string{"Y", "R"})
	checkChain(t, c, "Y", "2035-01-01", []string{"Y", "R"})
	checkChain(t, c, "X", "2030-05-31", []string{"X", "R"})
	checkChain(t, c, "X", "2030-06-01", []string{"X", "Y", "R"})
	checkChain(t, c, "X", "2035-01-01", []string{"X", "Y", "R"})
}

// A batch that is cyclic as a whole is rejected and changes nothing.
func TestRejectWholeBatchCycle(t *testing.T) {
	c := newChart()
	err := c.ApplyBatch([]Adjustment{
		{Dept: "A", Parent: "B", From: day("2024-01-01")},
		{Dept: "C", Parent: "A", From: day("2024-01-01")}, // legal alone, must roll back too
	})
	if err == nil {
		t.Fatal("cyclic batch was accepted")
	}
	checkChain(t, c, "A", "2024-06-01", []string{"A", "R"})
	checkChain(t, c, "C", "2024-06-01", []string{"C", "R"})
}

// A self-loop is rejected.
func TestRejectSelfLoop(t *testing.T) {
	c := newChart()
	err := c.ApplyBatch([]Adjustment{
		{Dept: "A", Parent: "A", From: day("2024-01-01")},
	})
	if err == nil {
		t.Fatal("self-loop batch was accepted")
	}
	checkChain(t, c, "A", "2024-06-01", []string{"A", "R"})
}

// The root cannot be reassigned.
func TestRejectRootAdjustment(t *testing.T) {
	c := newChart()
	err := c.ApplyBatch([]Adjustment{
		{Dept: "R", Parent: "A", From: day("2024-01-01")},
	})
	if err == nil {
		t.Fatal("root adjustment was accepted")
	}
	checkChain(t, c, "R", "2024-06-01", []string{"R"})
}

// Multiple disjoint adjustments to the same department in one batch.
func TestMultipleIntervalsSameDept(t *testing.T) {
	c := newChart()
	err := c.ApplyBatch([]Adjustment{
		{Dept: "C", Parent: "A", From: day("2024-01-01"), To: day("2024-02-01")},
		{Dept: "C", Parent: "B", From: day("2024-03-01"), To: day("2024-04-01")},
	})
	if err != nil {
		t.Fatalf("multi-interval batch rejected: %v", err)
	}
	checkChain(t, c, "C", "2024-01-15", []string{"C", "A", "R"})
	checkChain(t, c, "C", "2024-02-15", []string{"C", "R"})
	checkChain(t, c, "C", "2024-03-15", []string{"C", "B", "A", "R"})
	checkChain(t, c, "C", "2024-04-15", []string{"C", "R"})
}

// An adjustment nested inside a wider existing interval splits it and
// preserves the outer parts.
func TestNestedAdjustmentPreservesOuter(t *testing.T) {
	c := newChart()
	if err := c.ApplyBatch([]Adjustment{
		{Dept: "C", Parent: "A", From: day("2024-01-01"), To: day("2025-01-01")},
	}); err != nil {
		t.Fatalf("outer adjustment rejected: %v", err)
	}
	if err := c.ApplyBatch([]Adjustment{
		{Dept: "C", Parent: "B", From: day("2024-06-01"), To: day("2024-07-01")},
	}); err != nil {
		t.Fatalf("nested adjustment rejected: %v", err)
	}
	checkChain(t, c, "C", "2024-05-31", []string{"C", "A", "R"})
	checkChain(t, c, "C", "2024-06-15", []string{"C", "B", "A", "R"})
	checkChain(t, c, "C", "2024-07-01", []string{"C", "A", "R"})
	checkChain(t, c, "C", "2025-01-01", []string{"C", "R"})
}
