// Package orgchart 管理按生效日期划分的部门归属历史。
// 组织历史保存在进程内存中，由上层人事系统直接调用。
package orgchart

import "time"

// Date 表示公历自然日（仅年月日有效，时区固定为 UTC）。
type Date = time.Time

// Day 由年月日构造一个 Date。
func Day(year int, month time.Month, day int) Date {
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

// 历史起始日：自该日起每个非根部门每天恰有一个上级。
var Epoch = Day(2000, time.January, 1)

// Assignment 描述某部门在区间 [Start, End) 内的上级。
// End 为零值表示永久（无终止日）。
type Assignment struct {
	Dept   string
	Parent string
	Start  Date
	End    Date // 零值表示永久
}

// Adjustment 是一条归属调整，语义与 Assignment 相同：
// 仅替换区间内的归属，区间外已有安排原样保留。
type Adjustment = Assignment

// segment 是内部使用的归属片段，end 为零值表示永久。
type segment struct {
	parent string
	start  Date
	end    Date
}

// Chart 保存全部部门的历史归属。部门集合与唯一根部门固定。
type Chart struct {
	root     string
	depts    map[string]bool
	segments map[string][]segment // 每个非根部门按 start 升序、互不重叠、覆盖 [Epoch, ∞)
}

// timeFromUnix 由 Unix 秒构造 UTC 日期。
func timeFromUnix(sec int64) Date {
	return time.Unix(sec, 0).UTC()
}
