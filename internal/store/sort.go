package store

import (
	"sort"
	"strings"
)

func joinModels(models map[string]bool) string {
	if len(models) == 0 {
		return ""
	}
	values := make([]string, 0, len(models))
	for model := range models {
		values = append(values, model)
	}
	sort.Strings(values)
	return strings.Join(values, ", ")
}

func sortDailyDesc(values []DailySpend) {
	sort.Slice(values, func(i, j int) bool {
		return values[i].Day > values[j].Day
	})
}
