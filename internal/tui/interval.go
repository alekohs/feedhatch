package tui

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// intervalPart is one number and unit of an interval such as "1h30m".
var intervalPart = regexp.MustCompile(`^(\d+)\s*([a-zåäö]*)\s*`)

// Minutes per unit; a number without a unit is minutes. Swedish letters work too.
var intervalUnits = map[string]int64{
	"": 1, "m": 1, "min": 1,
	"h": 60, "t": 60, "tim": 60,
	"d": 24 * 60,
	"w": 7 * 24 * 60, "v": 7 * 24 * 60,
}

const maxInterval = 52 * 7 * 24 * 60 // a year: longer is surely a typo

// parseInterval reads an update interval like "30", "45m", "2h", "1h30m", "1d" or "1w"
// as minutes; it fails on anything else and on less than a minute.
func parseInterval(s string) (int64, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	var total int64
	for s != "" {
		part := intervalPart.FindStringSubmatch(s)
		if part == nil {
			return 0, false
		}
		n, err := strconv.ParseInt(part[1], 10, 64)
		unit, ok := intervalUnits[part[2]]
		if err != nil || !ok || n > maxInterval {
			return 0, false
		}
		total += n * unit
		s = s[len(part[0]):]
	}
	return total, total >= 1 && total <= maxInterval
}

// formatInterval writes minutes the way parseInterval reads them, e.g. 90 as "1h30m".
func formatInterval(minutes int64) string {
	if minutes > 0 && minutes%(7*24*60) == 0 {
		return fmt.Sprintf("%dw", minutes/(7*24*60))
	}
	var b strings.Builder
	for _, u := range []struct {
		size   int64
		suffix string
	}{{24 * 60, "d"}, {60, "h"}, {1, "m"}} {
		if n := minutes / u.size; n > 0 {
			fmt.Fprintf(&b, "%d%s", n, u.suffix)
			minutes -= n * u.size
		}
	}
	if b.Len() == 0 {
		return "0m"
	}
	return b.String()
}
