package orgchart

import (
	"errors"
	"fmt"
	"sort"
)

// ErrCycle 表示整批调整生效后会导致某天的上级关系无法到达根（成环或悬空）。
var ErrCycle = errors.New("orgchart: batch rejected, hierarchy would not reach root")

// ErrRoot 表示试图调整根部门或查询根部门的上级。
var ErrRoot = errors.New("orgchart: root department has no parent and cannot be adjusted")

// NewChart 创建组织历史并载入初始归属。
// 调用方保证初始历史合法：自 Epoch 起每个非根部门每天恰有一个上级，
// 且任意日期沿上级关系都能到达 root。
func NewChart(root string, depts []string, initial []Assignment) (*Chart, error) {
	c := &Chart{
		root:     root,
		depts:    make(map[string]bool, len(depts)),
		segments: make(map[string][]segment),
	}
	for _, d := range depts {
		c.depts[d] = true
	}
	if !c.depts[root] {
		return nil, fmt.Errorf("orgchart: root %q not in department set", root)
	}
	for _, a := range initial {
		if a.Dept == root {
			return nil, ErrRoot
		}
		if !c.depts[a.Dept] {
			return nil, fmt.Errorf("orgchart: unknown department %q", a.Dept)
		}
		c.segments[a.Dept] = append(c.segments[a.Dept], segment{parent: a.Parent, start: a.Start, end: a.End})
	}
	for dept := range c.segments {
		c.segments[dept] = normalize(c.segments[dept])
	}
	if !validate(c.segments, c.root, c.depts) {
		return nil, ErrCycle
	}
	return c, nil
}

// Apply 提交一批归属调整。整批共同生效后判定：
// 若任意日期存在部门无法沿上级链到达根，则整批拒绝，历史保持不变；
// 否则整批接受。输入顺序不影响结果。
func (c *Chart) Apply(batch []Adjustment) error {
	next := make(map[string][]segment, len(c.segments))
	for dept, segs := range c.segments {
		cp := make([]segment, len(segs))
		copy(cp, segs)
		next[dept] = cp
	}
	for _, adj := range batch {
		if adj.Dept == c.root {
			return ErrRoot
		}
		if !c.depts[adj.Dept] {
			return fmt.Errorf("orgchart: unknown department %q", adj.Dept)
		}
		next[adj.Dept] = splice(next[adj.Dept], adj.Parent, adj.Start, adj.End)
	}
	for dept := range next {
		next[dept] = normalize(next[dept])
	}
	if !validate(next, c.root, c.depts) {
		return ErrCycle
	}
	c.segments = next
	return nil
}

// Chain 返回 date 当天从 dept 到根的完整上级链（含 dept 自身与根）。
func (c *Chart) Chain(dept string, date Date) ([]string, error) {
	if !c.depts[dept] {
		return nil, fmt.Errorf("orgchart: unknown department %q", dept)
	}
	if date.Before(Epoch) {
		return nil, fmt.Errorf("orgchart: date %v before epoch %v", date, Epoch)
	}
	chain := []string{dept}
	seen := map[string]bool{dept: true}
	for cur := dept; cur != c.root; {
		parent := c.parentAt(cur, date)
		if parent == "" {
			return nil, fmt.Errorf("orgchart: %q has no parent on %v", cur, date)
		}
		if seen[parent] {
			return nil, ErrCycle
		}
		seen[parent] = true
		chain = append(chain, parent)
		cur = parent
	}
	return chain, nil
}

// parentAt 返回 dept 在 date 当天的直接上级；无安排时返回空串。
func (c *Chart) parentAt(dept string, date Date) string {
	for _, s := range c.segments[dept] {
		if covers(s, date) {
			return s.parent
		}
	}
	return ""
}

// covers 判断片段是否覆盖 date：区间含起日、不含止日，止日为零值表示永久。
func covers(s segment, date Date) bool {
	if date.Before(s.start) {
		return false
	}
	return s.end.IsZero() || date.Before(s.end)
}

// splice 将 [start, end) 区间内的归属替换为 parent，区间外原样保留。
// end 为零值表示永久。区间内未被既有片段覆盖的空档同样填入 parent。
func splice(segs []segment, parent string, start, end Date) []segment {
	var out []segment
	cursor := start
	for _, s := range segs {
		if !end.IsZero() && !s.start.Before(end) {
			out = append(out, s)
			continue
		}
		if !s.end.IsZero() && !s.end.After(start) {
			out = append(out, s)
			continue
		}
		if s.start.Before(start) {
			out = append(out, segment{parent: s.parent, start: s.start, end: start})
		}
		ovStart := s.start
		if ovStart.Before(start) {
			ovStart = start
		}
		if !cursor.IsZero() && cursor.Before(ovStart) {
			out = append(out, segment{parent: parent, start: cursor, end: ovStart})
		}
		ovEnd := s.end
		if !end.IsZero() && (ovEnd.IsZero() || end.Before(ovEnd)) {
			ovEnd = end
		}
		out = append(out, segment{parent: parent, start: ovStart, end: ovEnd})
		cursor = ovEnd
		if !end.IsZero() && (s.end.IsZero() || end.Before(s.end)) {
			out = append(out, segment{parent: s.parent, start: end, end: s.end})
		}
	}
	if !cursor.IsZero() && (end.IsZero() || cursor.Before(end)) {
		out = append(out, segment{parent: parent, start: cursor, end: end})
	}
	return out
}

// normalize 按起日排序并合并相邻且上级相同的片段。
func normalize(segs []segment) []segment {
	sort.Slice(segs, func(i, j int) bool { return segs[i].start.Before(segs[j].start) })
	out := segs[:0]
	for _, s := range segs {
		if n := len(out); n > 0 && out[n-1].parent == s.parent &&
			(out[n-1].end.IsZero() || !out[n-1].end.Before(s.start)) {
			if s.end.IsZero() {
				out[n-1].end = s.end
			} else if !out[n-1].end.IsZero() && out[n-1].end.Before(s.end) {
				out[n-1].end = s.end
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

// validate 检查自 Epoch 起每个非根部门每天恰有一个上级，
// 且任意日期沿上级关系都能到达根。
// 归属只在片段边界变化，故只需检查每个原子区间的代表日。
func validate(segments map[string][]segment, root string, depts map[string]bool) bool {
	boundaries := map[int64]bool{Epoch.Unix(): true}
	for dept, segs := range segments {
		if dept == root {
			continue
		}
		for _, s := range segs {
			boundaries[s.start.Unix()] = true
			if !s.end.IsZero() {
				boundaries[s.end.Unix()] = true
			}
		}
	}
	days := make([]int64, 0, len(boundaries))
	for b := range boundaries {
		days = append(days, b)
	}
	sort.Slice(days, func(i, j int) bool { return days[i] < days[j] })

	for _, b := range days {
		date := timeFromUnix(b)
		if date.Before(Epoch) {
			continue
		}
		if !reachableAt(segments, root, depts, date) {
			return false
		}
	}
	return true
}

// reachableAt 检查 date 当天每个非根部门是否都能沿上级链到达根。
func reachableAt(segments map[string][]segment, root string, depts map[string]bool, date Date) bool {
	parents := make(map[string]string, len(depts))
	for dept := range depts {
		if dept == root {
			continue
		}
		p := ""
		for _, s := range segments[dept] {
			if covers(s, date) {
				p = s.parent
				break
			}
		}
		if p == "" {
			return false
		}
		parents[dept] = p
	}
	for dept := range parents {
		seen := map[string]bool{dept: true}
		for cur := dept; cur != root; {
			p, ok := parents[cur]
			if !ok {
				return false
			}
			if seen[p] {
				return false
			}
			seen[p] = true
			cur = p
		}
	}
	return true
}
