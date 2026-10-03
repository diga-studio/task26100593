# orgchart

按生效日期管理部门归属的 Go 库，组织历史保存在进程内存中，由上层人事系统直接调用。

## 语义

- 部门集合与唯一根部门固定；自 2000-01-01（`Epoch`）起每个非根部门每天恰有一个上级，根部门无上级且不可调整。
- 每条调整为 `[起日, 止日)` 区间（含起日、不含止日，止日为零值表示永久），仅替换区间内归属，区间外安排原样保留；允许追溯调整。
- 每批调整按共同生效后的结果整体判定：任意日期出现无法到达根（成环/悬空）则整批拒绝、历史不变；输入顺序不影响结果。
- `Chain` 返回指定日期从部门到根的完整上级链。

## 用法

```go
c, _ := orgchart.NewChart("R", []string{"R", "A", "B"}, []orgchart.Assignment{
    {Dept: "A", Parent: "R", Start: orgchart.Epoch},
    {Dept: "B", Parent: "A", Start: orgchart.Epoch},
})
err := c.Apply([]orgchart.Adjustment{
    {Dept: "A", Parent: "B", Start: orgchart.Day(2024, time.March, 1)},
    {Dept: "B", Parent: "R", Start: orgchart.Day(2024, time.March, 1)},
})
chain, _ := c.Chain("A", orgchart.Day(2024, time.March, 1)) // [A B R]
```

## 测试

```sh
go test ./...
```
