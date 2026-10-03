package security

import "testing"

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern, s string
		want       bool
	}{
		{"*.example.com", "api.example.com", true},
		{"*.example.com", "example.com", false},
		{"*/api/orders*", "http://x/api/orders?id=1", true},
		{"http://x/a?c", "http://x/abc", true},
		{"a.b", "axb", false},
		{"*", "", true},
	}
	for _, tt := range tests {
		if got := MatchGlob(tt.pattern, tt.s); got != tt.want {
			t.Errorf("MatchGlob(%q,%q)=%v want %v", tt.pattern, tt.s, got, tt.want)
		}
	}
}

func TestHostAllowed(t *testing.T) {
	tests := []struct {
		allowed []string
		url     string
		want    bool
	}{
		{nil, "https://anything.com", true},
		{[]string{"portal.local", "*.corp.com"}, "http://portal.local:8080/x", true},
		{[]string{"portal.local", "*.corp.com"}, "https://app.corp.com/", true},
		{[]string{"portal.local"}, "https://evil.com/", false},
		{[]string{"portal.local"}, "about:blank", true},
		{[]string{"127.0.0.1"}, "http://127.0.0.1:5555/", true},
	}
	for _, tt := range tests {
		if got := HostAllowed(tt.allowed, tt.url); got != tt.want {
			t.Errorf("HostAllowed(%v,%q)=%v want %v", tt.allowed, tt.url, got, tt.want)
		}
	}
}

func TestMaskHeaders(t *testing.T) {
	in := map[string]string{"Authorization": "Bearer x", "Content-Type": "json", "X-Secret": "s", "cookie": "a=b"}
	got := MaskHeaders(in, []string{"x-secret"})
	want := map[string]string{"Authorization": Masked, "Content-Type": "json", "X-Secret": Masked, "cookie": Masked}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q want %q", k, got[k], v)
		}
	}
	if in["Authorization"] != "Bearer x" {
		t.Error("input must not be modified")
	}
}

func TestNormalizeURL(t *testing.T) {
	tests := map[string]string{
		"localhost:3000/x":   "http://localhost:3000/x",
		"portal/pedidos":     "https://portal/pedidos",
		"http://a.com":       "http://a.com",
		"about:blank":        "about:blank",
		" 127.0.0.1:8080 ":   "http://127.0.0.1:8080",
		"file:///tmp/a.html": "file:///tmp/a.html",
	}
	for in, want := range tests {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q)=%q want %q", in, got, want)
		}
	}
}
