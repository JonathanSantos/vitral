package mock

import (
	"strings"
	"testing"

	"github.com/JonathanSantos/vitral/internal/security"
)

// FuzzMatches: o matcher nunca entra em pânico e respeita as regras básicas de glob e método.
func FuzzMatches(f *testing.F) {
	f.Add("*/api/orders*", "http://x/api/orders?id=1", "GET", "")
	f.Add("/api", "http://x/api/v1", "post", "POST")
	f.Add("http://x/v?/users", "http://x/v2/users", "GET", "GET")
	f.Add("[a-", "http://x/[a-", "GET", "")
	f.Add("***", "", "", "DELETE")
	f.Fuzz(func(t *testing.T, pattern, url, method, ruleMethod string) {
		r := Rule{URLPattern: pattern, Method: ruleMethod}
		got := Matches(r, url, method)
		if ruleMethod != "" && !strings.EqualFold(ruleMethod, method) && got {
			t.Fatalf("method %q must not match rule method %q", method, ruleMethod)
		}
		if ruleMethod == "" && !strings.ContainsAny(pattern, "*?") && strings.Contains(url, pattern) && !got {
			t.Fatalf("substring pattern %q must match %q", pattern, url)
		}
		if ruleMethod == "" && pattern == "*" && !got {
			t.Fatal("* must match everything")
		}
		_ = security.MatchGlob(pattern, url)
		_ = FetchPattern(r)
	})
}
