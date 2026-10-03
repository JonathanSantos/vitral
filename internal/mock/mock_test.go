package mock

import (
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/proto"
)

func TestMatches(t *testing.T) {
	tests := []struct {
		name   string
		rule   Rule
		url    string
		method string
		want   bool
	}{
		{"glob", Rule{URLPattern: "*/api/orders*"}, "http://x/api/orders?page=2", "GET", true},
		{"glob miss", Rule{URLPattern: "*/api/orders"}, "http://x/api/orders/1", "GET", false},
		{"substring", Rule{URLPattern: "/api/orders"}, "http://x/api/orders/1", "GET", true},
		{"method match", Rule{URLPattern: "/api", Method: "POST"}, "http://x/api", "post", true},
		{"method miss", Rule{URLPattern: "/api", Method: "POST"}, "http://x/api", "GET", false},
		{"question mark", Rule{URLPattern: "http://x/v?/users"}, "http://x/v2/users", "GET", true},
	}
	for _, tt := range tests {
		if got := Matches(tt.rule, tt.url, tt.method); got != tt.want {
			t.Errorf("%s: got %v want %v", tt.name, got, tt.want)
		}
	}
}

func TestValidate(t *testing.T) {
	r, err := Validate(Rule{URLPattern: " /api ", Method: "post", Fail: "Timeout"})
	if err != nil || r.Status != 200 || r.Method != "POST" || r.Fail != "timeout" || r.URLPattern != "/api" {
		t.Fatalf("unexpected normalize: %+v %v", r, err)
	}
	bad := []Rule{
		{},
		{URLPattern: "/x", Status: 700},
		{URLPattern: "/x", Fail: "explode"},
		{URLPattern: "/x", DelayMs: -1},
		{URLPattern: "/x", Times: -2},
	}
	for _, b := range bad {
		if _, err := Validate(b); err == nil {
			t.Errorf("expected error for %+v", b)
		}
	}
	if _, err := FailReason("explode"); err == nil || !strings.Contains(err.Error(), "connection_refused") {
		t.Errorf("fail error should list options: %v", err)
	}
}

func TestRegistryNewestWinsAndTimes(t *testing.T) {
	reg := NewRegistry(nil)
	changes := 0
	reg.OnChange(func() { changes++ })

	old, _ := reg.Add(Rule{URLPattern: "/api", Status: 200})
	newer, _ := reg.Add(Rule{URLPattern: "/api", Status: 500, Times: 2})

	for i := 0; i < 2; i++ {
		got, ok := reg.Take("http://x/api", "GET")
		if !ok || got.ID != newer.ID {
			t.Fatalf("take %d: newest rule should win, got %+v", i, got)
		}
	}
	got, ok := reg.Take("http://x/api", "GET")
	if !ok || got.ID != old.ID {
		t.Fatalf("after times, older rule should apply, got %+v", got)
	}
	if list := reg.List(); len(list) != 1 || list[0].Used != 1 {
		t.Fatalf("expired rule should be gone: %+v", list)
	}
	if _, ok := reg.Take("http://x/other", "GET"); ok {
		t.Fatal("unrelated url should not match")
	}
	if changes != 2 { // só os dois Add: a expiração é avisada pelo interceptador depois de responder
		t.Fatalf("changes = %d", changes)
	}
}

func TestRegistryClearAndPatterns(t *testing.T) {
	reg := NewRegistry(nil)
	a, _ := reg.Add(Rule{URLPattern: "/api/a"})
	reg.Add(Rule{URLPattern: "*/b*"})
	reg.Add(Rule{URLPattern: "/api/a", Method: "POST"})
	if got := reg.Patterns(); len(got) != 2 || got[0] != "*/api/a*" || got[1] != "*/b*" {
		t.Fatalf("patterns = %v", got)
	}
	if n, err := reg.Clear(a.ID); err != nil || n != 1 {
		t.Fatalf("clear one: %d %v", n, err)
	}
	if _, err := reg.Clear("m99"); err == nil {
		t.Fatal("unknown id should fail")
	}
	if n, _ := reg.Clear(""); n != 2 || len(reg.List()) != 0 {
		t.Fatalf("clear all removed %d", n)
	}
}

func TestRegistryAllowedHosts(t *testing.T) {
	reg := NewRegistry([]string{"portal.local"})
	reg.Add(Rule{URLPattern: "/api"})
	if _, ok := reg.Take("http://evil.com/api", "GET"); ok {
		t.Fatal("mock must not apply outside allowed hosts")
	}
	if _, ok := reg.Take("http://portal.local/api", "GET"); !ok {
		t.Fatal("mock should apply on allowed host")
	}
}

func TestResponseHeadersAndSummary(t *testing.T) {
	h := ResponseHeaders(Rule{Body: `{"a":1}`, Headers: map[string]string{"X-Test": "1"}})
	if len(h) != 2 || h[1].Name != "Content-Type" || h[1].Value != "application/json" {
		t.Fatalf("headers = %+v", h)
	}
	h = ResponseHeaders(Rule{Body: "oi", Headers: map[string]string{"content-type": "text/html"}})
	if len(h) != 1 {
		t.Fatalf("explicit content-type must be kept: %+v", h)
	}
	if h := ResponseHeaders(Rule{Body: "oi"}); h[0].Value != "text/plain; charset=utf-8" {
		t.Fatalf("plain text type = %+v", h)
	}

	tests := []struct {
		rule Rule
		want string
	}{
		{Rule{ID: "m1", URLPattern: "/api", Status: 500}, "m1 ANY /api → 500 (used 0)"},
		{Rule{ID: "m2", URLPattern: "/api", Method: "GET", Fail: "timeout", Times: 3, Used: 1, DelayMs: 200}, "m2 GET /api → fail timeout (used 1/3, delay 200ms)"},
	}
	for _, tt := range tests {
		if got := Summary(tt.rule); got != tt.want {
			t.Errorf("Summary = %q want %q", got, tt.want)
		}
	}
	if _, err := FailReason("connection_refused"); err != nil {
		t.Fatal(err)
	}
	if r, _ := FailReason("offline"); r != proto.NetworkErrorReasonInternetDisconnected {
		t.Fatal("offline mapping")
	}
}
