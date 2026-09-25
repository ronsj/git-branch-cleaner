package tui

import (
	"fmt"
	"time"
)

// relativeTime describes how long before now t was, the way git does for
// --date=relative ("3 weeks ago", "1 year, 2 months ago"). It follows git's
// rounding (show_date_relative in git's date.c) so the column reads the same
// as git's own output.
func relativeTime(t, now time.Time) string {
	diff := int64(now.Sub(t) / time.Second)
	if diff < 0 {
		return "in the future"
	}
	if diff < 90 {
		return ago(diff, "second")
	}
	diff = (diff + 30) / 60 // minutes
	if diff < 90 {
		return ago(diff, "minute")
	}
	diff = (diff + 30) / 60 // hours
	if diff < 36 {
		return ago(diff, "hour")
	}
	diff = (diff + 12) / 24 // days
	switch {
	case diff < 14:
		return ago(diff, "day")
	case diff < 70:
		return ago((diff+3)/7, "week")
	case diff < 365:
		return ago((diff+15)/30, "month")
	case diff < 1825:
		totalMonths := (diff*12*2 + 365) / (365 * 2)
		years, months := totalMonths/12, totalMonths%12
		if months == 0 {
			return ago(years, "year")
		}
		return plural(years, "year") + ", " + ago(months, "month")
	default:
		return ago((diff+183)/365, "year")
	}
}

// ago returns "1 day ago", "3 days ago", and so on.
func ago(n int64, unit string) string {
	return plural(n, unit) + " ago"
}

func plural(n int64, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
