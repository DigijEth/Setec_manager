package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronExpr represents a parsed 5-field cron expression.
// Each field is expanded into a sorted slice of valid integer values.
type CronExpr struct {
	Minutes     []int // 0-59
	Hours       []int // 0-23
	DaysOfMonth []int // 1-31
	Months      []int // 1-12
	DaysOfWeek  []int // 0-6 (0 = Sunday)
}

// fieldBounds defines the min/max for each cron field.
var fieldBounds = [5][2]int{
	{0, 59}, // minute
	{0, 23}, // hour
	{1, 31}, // day of month
	{1, 12}, // month
	{0, 6},  // day of week
}

// ParseCron parses a standard 5-field cron expression into a CronExpr.
//
// Supported syntax per field:
//   - *        all values in range
//   - N        single number
//   - N-M      range from N to M inclusive
//   - N-M/S    range with step S
//   - */S      full range with step S
//   - N,M,O    list of values (each element can be a number or range)
func ParseCron(expr string) (*CronExpr, error) {
	fields := strings.Fields(strings.TrimSpace(expr))
	if len(fields) != 5 {
		return nil, fmt.Errorf("cron: expected 5 fields, got %d in %q", len(fields), expr)
	}

	ce := &CronExpr{}
	targets := []*[]int{&ce.Minutes, &ce.Hours, &ce.DaysOfMonth, &ce.Months, &ce.DaysOfWeek}

	for i, field := range fields {
		vals, err := parseField(field, fieldBounds[i][0], fieldBounds[i][1])
		if err != nil {
			return nil, fmt.Errorf("cron field %d (%q): %w", i+1, field, err)
		}
		if len(vals) == 0 {
			return nil, fmt.Errorf("cron field %d (%q): produced no values", i+1, field)
		}
		*targets[i] = vals
	}

	return ce, nil
}

// parseField parses a single cron field into a sorted slice of ints.
func parseField(field string, min, max int) ([]int, error) {
	// Handle lists: "1,3,5" or "1-3,7,10-12"
	parts := strings.Split(field, ",")
	seen := make(map[int]bool)

	for _, part := range parts {
		vals, err := parsePart(part, min, max)
		if err != nil {
			return nil, err
		}
		for _, v := range vals {
			seen[v] = true
		}
	}

	// Collect and sort.
	result := make([]int, 0, len(seen))
	for v := range seen {
		result = append(result, v)
	}
	sortInts(result)
	return result, nil
}

// parsePart parses a single element that may be *, a number, a range, or have a step.
func parsePart(part string, min, max int) ([]int, error) {
	// Split on "/" for step.
	var stepStr string
	base := part
	if idx := strings.Index(part, "/"); idx >= 0 {
		base = part[:idx]
		stepStr = part[idx+1:]
	}

	// Determine the range.
	var lo, hi int
	if base == "*" {
		lo, hi = min, max
	} else if idx := strings.Index(base, "-"); idx >= 0 {
		var err error
		lo, err = strconv.Atoi(base[:idx])
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", base[:idx], err)
		}
		hi, err = strconv.Atoi(base[idx+1:])
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", base[idx+1:], err)
		}
	} else {
		n, err := strconv.Atoi(base)
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", base, err)
		}
		if stepStr == "" {
			// Single value, no step.
			if n < min || n > max {
				return nil, fmt.Errorf("value %d out of range [%d, %d]", n, min, max)
			}
			return []int{n}, nil
		}
		// e.g., "5/10" means starting at 5, step 10, up to max.
		lo, hi = n, max
	}

	// Validate bounds.
	if lo < min || lo > max {
		return nil, fmt.Errorf("value %d out of range [%d, %d]", lo, min, max)
	}
	if hi < min || hi > max {
		return nil, fmt.Errorf("value %d out of range [%d, %d]", hi, min, max)
	}
	if lo > hi {
		return nil, fmt.Errorf("range start %d > end %d", lo, hi)
	}

	step := 1
	if stepStr != "" {
		var err error
		step, err = strconv.Atoi(stepStr)
		if err != nil {
			return nil, fmt.Errorf("invalid step %q: %w", stepStr, err)
		}
		if step < 1 {
			return nil, fmt.Errorf("step must be >= 1, got %d", step)
		}
	}

	var vals []int
	for v := lo; v <= hi; v += step {
		vals = append(vals, v)
	}
	return vals, nil
}

// NextRun computes the next run time for a cron expression after the given time.
// It searches up to 2 years ahead before giving up.
func NextRun(schedule string, from time.Time) (time.Time, error) {
	ce, err := ParseCron(schedule)
	if err != nil {
		return time.Time{}, err
	}
	return ce.Next(from)
}

// Next finds the earliest time after "from" that matches the cron expression.
func (ce *CronExpr) Next(from time.Time) (time.Time, error) {
	// Start from the next whole minute.
	t := from.Truncate(time.Minute).Add(time.Minute)

	// Search limit: 2 years of minutes (~1,051,200). We iterate by
	// advancing fields intelligently rather than minute-by-minute.
	deadline := t.Add(2 * 365 * 24 * time.Hour)

	for t.Before(deadline) {
		// Check month.
		if !contains(ce.Months, int(t.Month())) {
			// Advance to next valid month.
			t = advanceMonth(t, ce.Months)
			continue
		}

		// Check day of month.
		dom := t.Day()
		domOk := contains(ce.DaysOfMonth, dom)
		dowOk := contains(ce.DaysOfWeek, int(t.Weekday()))
		if !domOk || !dowOk {
			// Advance one day.
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		// Check hour.
		if !contains(ce.Hours, t.Hour()) {
			// Advance to next valid hour today.
			nextH := nextVal(ce.Hours, t.Hour())
			if nextH == -1 {
				// No more valid hours today, go to next day.
				t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			} else {
				t = time.Date(t.Year(), t.Month(), t.Day(), nextH, 0, 0, 0, t.Location())
			}
			continue
		}

		// Check minute.
		if !contains(ce.Minutes, t.Minute()) {
			nextM := nextVal(ce.Minutes, t.Minute())
			if nextM == -1 {
				// No more valid minutes this hour, advance hour.
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			} else {
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), nextM, 0, 0, t.Location())
			}
			continue
		}

		// All fields match.
		return t, nil
	}

	return time.Time{}, fmt.Errorf("cron: no matching time found within 2 years for %q", ce.String())
}

// String reconstructs a human-readable representation of the cron expression.
func (ce *CronExpr) String() string {
	return fmt.Sprintf("%v %v %v %v %v",
		ce.Minutes, ce.Hours, ce.DaysOfMonth, ce.Months, ce.DaysOfWeek)
}

// contains checks if val is in the sorted slice.
func contains(vals []int, val int) bool {
	for _, v := range vals {
		if v == val {
			return true
		}
		if v > val {
			return false
		}
	}
	return false
}

// nextVal returns the smallest value in vals that is > current, or -1.
func nextVal(vals []int, current int) int {
	for _, v := range vals {
		if v > current {
			return v
		}
	}
	return -1
}

// advanceMonth jumps to day 1, hour 0, minute 0 of the next valid month.
func advanceMonth(t time.Time, months []int) time.Time {
	cur := int(t.Month())
	year := t.Year()

	// Find next valid month in this year.
	for _, m := range months {
		if m > cur {
			return time.Date(year, time.Month(m), 1, 0, 0, 0, 0, t.Location())
		}
	}
	// Wrap to first valid month of next year.
	return time.Date(year+1, time.Month(months[0]), 1, 0, 0, 0, 0, t.Location())
}

// sortInts performs an insertion sort on a small slice.
func sortInts(a []int) {
	for i := 1; i < len(a); i++ {
		key := a[i]
		j := i - 1
		for j >= 0 && a[j] > key {
			a[j+1] = a[j]
			j--
		}
		a[j+1] = key
	}
}
