package envdiff

import "sort"

type Result struct {
	OnlyLeft  []string
	OnlyRight []string
	Changed   []string
	SameCount int
}

func Compare(left map[string]string, right map[string]string) Result {
	result := Result{}

	for key, leftValue := range left {
		rightValue, ok := right[key]
		if !ok {
			result.OnlyLeft = append(result.OnlyLeft, key)
			continue
		}
		if leftValue != rightValue {
			result.Changed = append(result.Changed, key)
			continue
		}
		result.SameCount++
	}

	for key := range right {
		if _, ok := left[key]; !ok {
			result.OnlyRight = append(result.OnlyRight, key)
		}
	}

	sort.Strings(result.OnlyLeft)
	sort.Strings(result.OnlyRight)
	sort.Strings(result.Changed)

	return result
}

func (r Result) DifferenceCount() int {
	return len(r.OnlyLeft) + len(r.OnlyRight) + len(r.Changed)
}

func (r Result) HasDifferences() bool {
	return r.DifferenceCount() > 0
}
