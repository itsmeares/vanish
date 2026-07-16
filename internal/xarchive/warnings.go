package xarchive

const maxWarningGroups = 128

type warningCollector struct {
	total  int
	groups []WarningGroup
	index  map[string]int
}

func (c *warningCollector) add(source, category, reason string, unit WarningUnit, count int) {
	if count <= 0 {
		return
	}
	c.total += count
	if c.index == nil {
		c.index = make(map[string]int)
	}
	key := source + "\x00" + category + "\x00" + reason + "\x00" + string(unit)
	if index, ok := c.index[key]; ok {
		c.groups[index].Count += count
		return
	}
	if len(c.groups) >= maxWarningGroups-1 {
		key = "archive\x00other\x00additional warnings omitted\x00record"
		if index, ok := c.index[key]; ok {
			c.groups[index].Count += count
			return
		}
		source, category, reason, unit = "archive", "other", "additional warnings omitted", WarningRecord
	}
	c.index[key] = len(c.groups)
	c.groups = append(c.groups, WarningGroup{Source: source, Category: category, Reason: reason, Unit: unit, Count: count})
}

func (c *warningCollector) finish() WarningSummary {
	groups := append([]WarningGroup(nil), c.groups...)
	return WarningSummary{Total: c.total, Groups: groups}
}
