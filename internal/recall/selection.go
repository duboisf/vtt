package recall

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ParseSelection turns a user-provided selection string into the set of
// segment IDs it refers to, in order of first appearance and
// deduplicated.
//
// Syntax:
//   - single id: "3"
//   - closed range (inclusive): "3-5"
//   - open end (id and newer): "3-"
//   - open start (oldest up to id): "-3"
//   - all buffered segments: "all" or "*"
//   - comma-separated mix of the above: "3,5-7,10-"
//
// available is the sorted-ascending snapshot of IDs currently in the
// ring buffer. The function needs it so that open-ended ranges can
// resolve to the actual oldest/newest, and so that bare numeric ids can
// be rejected early if the buffer no longer holds them.
//
// Unknown IDs *inside a range* are silently skipped (segments drop out
// of the ring as it fills, so a range like "3-20" reasonably means
// "whichever of 3..20 are still here"). A bare unknown id is an error
// because the user picked it specifically.
func ParseSelection(input string, available []int64) ([]int64, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return nil, errors.New("empty selection")
	}
	if strings.EqualFold(trimmed, "all") || trimmed == "*" {
		if len(available) == 0 {
			return nil, errors.New("no segments in buffer")
		}
		out := make([]int64, len(available))
		copy(out, available)
		return out, nil
	}

	present := make(map[int64]bool, len(available))
	for _, id := range available {
		present[id] = true
	}

	seen := make(map[int64]bool)
	var out []int64
	for _, raw := range strings.Split(trimmed, ",") {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		if strings.Contains(part, "-") {
			picked, err := resolveRange(part, available)
			if err != nil {
				return nil, err
			}
			for _, id := range picked {
				if !seen[id] {
					seen[id] = true
					out = append(out, id)
				}
			}
			continue
		}
		id, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("selection %q: %w", part, err)
		}
		if !present[id] {
			return nil, fmt.Errorf("id %d is not in the ring buffer", id)
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("selection %q matched no segments", input)
	}
	return out, nil
}

func resolveRange(part string, available []int64) ([]int64, error) {
	bits := strings.SplitN(part, "-", 2)
	startStr := strings.TrimSpace(bits[0])
	endStr := strings.TrimSpace(bits[1])

	var start, end int64
	var err error

	if startStr == "" {
		if len(available) == 0 {
			return nil, fmt.Errorf("range %q: buffer is empty", part)
		}
		start = available[0]
	} else {
		start, err = strconv.ParseInt(startStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("range %q start: %w", part, err)
		}
	}

	if endStr == "" {
		if len(available) == 0 {
			return nil, fmt.Errorf("range %q: buffer is empty", part)
		}
		end = available[len(available)-1]
	} else {
		end, err = strconv.ParseInt(endStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("range %q end: %w", part, err)
		}
	}

	if end < start {
		return nil, fmt.Errorf("range %q: end < start", part)
	}

	var picked []int64
	for _, id := range available {
		if id >= start && id <= end {
			picked = append(picked, id)
		}
	}
	return picked, nil
}
