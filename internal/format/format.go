package format

import (
	"strings"
	"time"
	"unicode"
)

func Slug(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = false
		} else if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "task"
	}
	if len(out) > 80 {
		return out[:80]
	}
	return out
}

func Now() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func Age(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "?"
	}
	return age(time.Since(t))
}

func AgeUnix(value int64) string {
	return age(time.Since(time.Unix(value, 0)))
}

func age(d time.Duration) string {
	if d < time.Minute {
		return "0m"
	}
	if d < time.Hour {
		return plural(int(d.Minutes()), "m")
	}
	if d < 24*time.Hour {
		return plural(int(d.Hours()), "h")
	}
	return plural(int(d.Hours()/24), "d")
}

func plural(n int, suffix string) string {
	return strings.TrimSpace(strings.Join([]string{itoa(n), suffix}, ""))
}

func itoa(n int) string {
	return strconvItoa(n)
}

func Trunc(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 1 {
		return value[:width]
	}
	return value[:width-1] + "…"
}

func strconvItoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
