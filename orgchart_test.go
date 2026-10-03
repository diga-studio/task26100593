package orgchart

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func mustChart(t *testing.T, root string, depts []string, initial []Assignment) *Chart {
	t.Helper()
	c, err := NewChart(root, depts, initial)
	if err != nil {
		t.Fatalf("NewChart: %v", err)
	}
	return c
}

func mustChain(t *testing.T, c *Chart, dept string, date Date) []string {
	t.Helper()
	chain, err := c.Chain(dept, date)
	if err != nil {
		t.Fatalf("Chain(%q, %v): %v", dept, date.Format("2006-01-02"), err)
	}
	return chain
}

func assertChain(t *testing.T, c *Chart, dept string, date Date, want []string) {
	t.Helper()
	got := mustChain(t, c, dept, date)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Chain(%q, %v) = %v, want %v", dept, date.Format("2006-01-02"), got, want)
	}
}

// 同日整批上下级换位：单独应用会暂时成环，整批共同生效后合法，必须接受。
func TestBatchSwapSameDay(t *testing.T) {
	swap := Day(2024, time.March, 1)
	c := mustChart(t, "R", []string{"R", "A", "B"}, []Assignment{
		{Dept: "A", Parent: "R", Start: Epoch},
		{Dept: "B", Parent: "A", Start: Epoch},
	})

	err := c.Apply([]Adjustment{
		{Dept: "A", Parent: "B", Start: swap}, // 先换 A 会暂时成环 A->B->A
		{Dept: "B", Parent: "R", Start: swap},
	})
	if err != nil {
		t.Fatalf("Apply swap batch: %v", err)
	}

	// 边界前一天：旧安排保留。
	before := Day(2024, time.February, 29)
	assertChain(t, c, "A", before, []string{"A", "R"})
	assertChain(t, c, "B", before, []string{"B", "A", "R"})
	// 生效当天起：新安排。
	assertChain(t, c, "A", swap, []string{"A", "B", "R"})
	assertChain(t, c, "B", swap, []string{"B", "R"})
	assertChain(t, c, "A", Day(2030, time.June, 15), []string{"A", "B", "R"})
}

// 已有未来安排使新调整仅在区间中间成环：整批必须拒绝，且涉及部门历史不变。
func TestRejectBatchWithMidIntervalCycle(t *testing.T) {
	t1 := Day(2024, time.January, 1)
	t2 := Day(2024, time.February, 1)
	t3 := Day(2024, time.March, 1)
	t4 := Day(2024, time.April, 1)

	c := mustChart(t, "R", []string{"R", "A", "B"}, []Assignment{
		{Dept: "A", Parent: "R", Start: Epoch},
		{Dept: "B", Parent: "R", Start: Epoch},
	})

	// 先提交一个合法的未来安排：B 在 [t2, t3) 挂到 A 下。
	if err := c.Apply([]Adjustment{{Dept: "B", Parent: "A", Start: t2, End: t3}}); err != nil {
		t.Fatalf("Apply future arrangement: %v", err)
	}

	// 新调整 A->B 覆盖 [t1, t4)：边界 [t1,t2) 与 [t3,t4) 均合法，
	// 但中间 [t2,t3) 与已有安排形成 A->B->A 环，整批必须拒绝。
	err := c.Apply([]Adjustment{{Dept: "A", Parent: "B", Start: t1, End: t4}})
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("Apply cyclic batch = %v, want ErrCycle", err)
	}

	// 拒绝后历史保持不变。
	assertChain(t, c, "A", t1, []string{"A", "R"})
	assertChain(t, c, "A", t2, []string{"A", "R"})
	assertChain(t, c, "A", t3, []string{"A", "R"})
	assertChain(t, c, "B", t1, []string{"B", "R"})
	assertChain(t, c, "B", t2, []string{"B", "A", "R"})
	assertChain(t, c, "B", t3, []string{"B", "R"})
}

// 追溯调整只替换区间内归属，区间外已有安排原样保留。
func TestRetroactiveAdjustmentPreservesOutside(t *testing.T) {
	c := mustChart(t, "R", []string{"R", "A"}, []Assignment{
		{Dept: "A", Parent: "R", Start: Epoch},
	})
	start := Day(2010, time.May, 1)
	end := Day(2010, time.August, 1)
	if err := c.Apply([]Adjustment{{Dept: "A", Parent: "R", Start: start, End: end}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// 区间边界：起日包含、止日不包含。
	assertChain(t, c, "A", Day(2010, time.April, 30), []string{"A", "R"})
	assertChain(t, c, "A", start, []string{"A", "R"})
	assertChain(t, c, "A", end, []string{"A", "R"})
}

// 多层级追溯调整：区间内换上级，区间外保留旧安排。
func TestRetroactiveReparent(t *testing.T) {
	c := mustChart(t, "R", []string{"R", "A", "B", "C"}, []Assignment{
		{Dept: "A", Parent: "R", Start: Epoch},
		{Dept: "B", Parent: "R", Start: Epoch},
		{Dept: "C", Parent: "A", Start: Epoch},
	})
	start := Day(2015, time.January, 1)
	end := Day(2016, time.January, 1)
	if err := c.Apply([]Adjustment{{Dept: "C", Parent: "B", Start: start, End: end}}); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	assertChain(t, c, "C", Day(2014, time.December, 31), []string{"C", "A", "R"})
	assertChain(t, c, "C", start, []string{"C", "B", "R"})
	assertChain(t, c, "C", Day(2015, time.June, 30), []string{"C", "B", "R"})
	assertChain(t, c, "C", end, []string{"C", "A", "R"})
}

// 初始历史若成环，NewChart 必须拒绝。
func TestInitialHistoryWithCycleRejected(t *testing.T) {
	_, err := NewChart("R", []string{"R", "A", "B"}, []Assignment{
		{Dept: "A", Parent: "B", Start: Epoch},
		{Dept: "B", Parent: "A", Start: Epoch},
	})
	if !errors.Is(err, ErrCycle) {
		t.Fatalf("NewChart = %v, want ErrCycle", err)
	}
}

// 根部门不可调整。
func TestRootNotAdjustable(t *testing.T) {
	c := mustChart(t, "R", []string{"R", "A"}, []Assignment{
		{Dept: "A", Parent: "R", Start: Epoch},
	})
	if err := c.Apply([]Adjustment{{Dept: "R", Parent: "A", Start: Epoch}}); !errors.Is(err, ErrRoot) {
		t.Fatalf("Apply root adjustment = %v, want ErrRoot", err)
	}
}

// 整批内同部门多段调整与永久调整。
func TestBatchMultipleSegmentsAndPermanent(t *testing.T) {
	c := mustChart(t, "R", []string{"R", "A", "B"}, []Assignment{
		{Dept: "A", Parent: "R", Start: Epoch},
		{Dept: "B", Parent: "R", Start: Epoch},
	})
	d1 := Day(2020, time.January, 1)
	d2 := Day(2021, time.January, 1)
	d3 := Day(2022, time.January, 1)
	err := c.Apply([]Adjustment{
		{Dept: "A", Parent: "B", Start: d1, End: d2},
		{Dept: "A", Parent: "R", Start: d2, End: d3},
		{Dept: "B", Parent: "A", Start: d3}, // 永久
	})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	assertChain(t, c, "A", Day(2019, time.December, 31), []string{"A", "R"})
	assertChain(t, c, "A", d1, []string{"A", "B", "R"})
	assertChain(t, c, "A", d2, []string{"A", "R"})
	assertChain(t, c, "B", d3, []string{"B", "A", "R"})
	assertChain(t, c, "B", Day(2040, time.January, 1), []string{"B", "A", "R"})
}
