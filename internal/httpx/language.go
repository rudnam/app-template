package httpx

import (
	"net/http"
	"strconv"
	"strings"
)

// Language is the language the client prefers, from Accept-Language: "ja"
// when Japanese ranks highest, otherwise "en". The app sends the language
// it shows; browsers send the device's.
func Language(r *http.Request) string {
	best, bestQ := "en", -1.0
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		tag, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		q := 1.0
		if v, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				q = f
			}
		}
		lang, _, _ := strings.Cut(strings.ToLower(tag), "-")
		if lang == "" || lang == "*" || q <= bestQ {
			continue
		}
		bestQ = q
		if lang == "ja" {
			best = "ja"
		} else {
			best = "en"
		}
	}
	return best
}
