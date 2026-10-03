package network

import (
	"strings"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/ysmood/gson"
)

func headers(kv ...string) proto.NetworkHeaders {
	h := proto.NetworkHeaders{}
	for i := 0; i+1 < len(kv); i += 2 {
		h[kv[i]] = gson.New(kv[i+1])
	}
	return h
}

func request(id, url string, typ proto.NetworkResourceType, ts float64) *proto.NetworkRequestWillBeSent {
	return &proto.NetworkRequestWillBeSent{
		RequestID: proto.NetworkRequestID(id),
		LoaderID:  "loader",
		Request:   &proto.NetworkRequest{URL: url, Method: "GET", Headers: headers("Authorization", "Bearer x")},
		Timestamp: proto.MonotonicTime(ts),
		Type:      typ,
		FrameID:   "main",
	}
}

func newTestTracker(capacity int) (*Tracker, *time.Time) {
	now := time.Unix(1000, 0)
	tr := NewTracker(capacity, false)
	tr.now = func() time.Time { return now }
	tr.mainFrame = "main"
	return tr, &now
}

func TestTrackerLifecycle(t *testing.T) {
	tr, now := newTestTracker(10)
	tr.OnRequest(request("1", "http://x/api", proto.NetworkResourceTypeFetch, 1.0))
	if pending, _ := tr.IdleSince(*now); pending != 1 {
		t.Fatalf("want 1 pending, got %d", pending)
	}
	tr.OnResponse(&proto.NetworkResponseReceived{RequestID: "1", Response: &proto.NetworkResponse{
		Status: 200, StatusText: "OK", MIMEType: "application/json", Headers: headers("Set-Cookie", "s=1"),
	}})
	tr.OnFinished(&proto.NetworkLoadingFinished{RequestID: "1", Timestamp: 1.25, EncodedDataLength: 2048})
	*now = now.Add(time.Second)

	pending, idle := tr.IdleSince(time.Unix(0, 0))
	if pending != 0 || idle != time.Second {
		t.Fatalf("pending=%d idle=%v", pending, idle)
	}
	e, ok := tr.Get(1)
	if !ok || e.Status != 200 || e.DurationMS() != 250 || e.Size != 2048 || !e.Done {
		t.Fatalf("unexpected entry %+v", e)
	}
	if got := Line(e); got != "1 GET 200 http://x/api (250ms, 2.0KB)" {
		t.Fatalf("line = %q", got)
	}
}

func TestTrackerFailureMockAndRedirect(t *testing.T) {
	tr, _ := newTestTracker(10)
	tr.MarkMocked("2") // o Fetch pode avisar antes do requestWillBeSent
	tr.OnRequest(request("2", "http://x/mock", proto.NetworkResourceTypeXHR, 1))
	tr.OnFailed(&proto.NetworkLoadingFailed{RequestID: "2", Timestamp: 2, ErrorText: "net::ERR_CONNECTION_REFUSED"})

	tr.OnRequest(request("3", "http://x/old", proto.NetworkResourceTypeFetch, 3))
	redirect := request("3", "http://x/new", proto.NetworkResourceTypeFetch, 4)
	redirect.RedirectResponse = &proto.NetworkResponse{Status: 302}
	tr.OnRequest(redirect)

	entries := tr.Entries()
	if len(entries) != 3 {
		t.Fatalf("want 3 entries, got %d", len(entries))
	}
	if !entries[0].Mocked || entries[0].Failed == "" || !IsProblem(entries[0]) {
		t.Fatalf("mocked failure not recorded: %+v", entries[0])
	}
	if entries[1].Status != 302 || !entries[1].Done || entries[2].URL != "http://x/new" {
		t.Fatalf("redirect not split: %+v / %+v", entries[1], entries[2])
	}
	if got := Line(entries[0]); !strings.Contains(got, "failed") || !strings.Contains(got, "mocked") {
		t.Fatalf("line = %q", got)
	}
}

func TestTrackerNavigationAndEviction(t *testing.T) {
	tr, _ := newTestTracker(2)
	tr.OnRequest(request("a", "http://x/1", proto.NetworkResourceTypeFetch, 1))
	tr.OnRequest(request("b", "http://x/2", proto.NetworkResourceTypeFetch, 1))
	tr.OnRequest(request("c", "http://x/3", proto.NetworkResourceTypeFetch, 1))
	if got := tr.Entries(); len(got) != 2 || got[0].URL != "http://x/2" {
		t.Fatalf("eviction failed: %+v", got)
	}

	nav := request("loader", "http://x/page", proto.NetworkResourceTypeDocument, 2)
	tr.OnRequest(nav)
	if got := tr.Entries(); len(got) != 1 || got[0].URL != "http://x/page" {
		t.Fatalf("navigation should clear history: %+v", got)
	}
	// Documento de iframe não limpa o histórico.
	iframe := request("loader", "http://x/frame", proto.NetworkResourceTypeDocument, 3)
	iframe.FrameID = "child"
	tr.OnRequest(iframe)
	if len(tr.Entries()) != 2 {
		t.Fatal("iframe navigation must not clear history")
	}
}

func TestPreserveKeepsHistory(t *testing.T) {
	tr := NewTracker(10, true)
	tr.OnRequest(request("a", "http://x/1", proto.NetworkResourceTypeFetch, 1))
	tr.OnRequest(request("loader", "http://x/page", proto.NetworkResourceTypeDocument, 2))
	if len(tr.Entries()) != 2 {
		t.Fatal("preserve should keep history across navigations")
	}
}

func TestApplyAndMatchStatus(t *testing.T) {
	entries := []Entry{
		{ID: 1, Method: "GET", URL: "http://x/api/orders", Type: "Fetch", Status: 200, Done: true},
		{ID: 2, Method: "POST", URL: "http://x/api/orders", Type: "Fetch", Status: 500, Done: true},
		{ID: 3, Method: "GET", URL: "http://x/app.js", Type: "Script", Status: 304, Done: true},
		{ID: 4, Method: "GET", URL: "http://x/api/users", Type: "XHR", Failed: "net::ERR_FAILED", Done: true},
		{ID: 5, Method: "GET", URL: "http://x/api/slow", Type: "Fetch"},
	}
	tests := []struct {
		name string
		f    Filter
		want []int
	}{
		{"url substring", Filter{URL: "API/ORDERS"}, []int{1, 2}},
		{"method", Filter{Method: "post"}, []int{2}},
		{"type", Filter{Type: "xhr"}, []int{4}},
		{"2xx", Filter{Status: "2xx"}, []int{1}},
		{"exact", Filter{Status: "304"}, []int{3}},
		{"failed", Filter{Status: "failed"}, []int{4}},
		{"error", Filter{Status: "error"}, []int{2, 4}},
		{"pending", Filter{Status: "pending"}, []int{5}},
		{"limit keeps newest", Filter{Limit: 2}, []int{4, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var ids []int
			for _, e := range Apply(entries, tt.f) {
				ids = append(ids, e.ID)
			}
			if len(ids) != len(tt.want) {
				t.Fatalf("got %v want %v", ids, tt.want)
			}
			for i := range ids {
				if ids[i] != tt.want[i] {
					t.Fatalf("got %v want %v", ids, tt.want)
				}
			}
		})
	}
}

func TestFormatBody(t *testing.T) {
	if got := FormatBody([]byte(`{"a":1,"b":[2]}`), "application/json"); got != "{\n  \"a\": 1,\n  \"b\": [\n    2\n  ]\n}" {
		t.Fatalf("pretty json = %q", got)
	}
	if got := FormatBody(nil, ""); got != "(empty)" {
		t.Fatalf("empty = %q", got)
	}
	if got := FormatBody([]byte{0xff, 0xfe, 0x00}, "image/png"); !strings.HasPrefix(got, "(binary image/png") {
		t.Fatalf("binary = %q", got)
	}
	big := strings.Repeat("á", BodyLimit)
	got := FormatBody([]byte(big), "text/plain")
	if !strings.Contains(got, "… (truncated, 16.0KB total)") || len(got) > BodyLimit+100 {
		t.Fatalf("truncation failed, len=%d tail=%q", len(got), got[len(got)-40:])
	}
}

func TestDetailMasksHeaders(t *testing.T) {
	e := Entry{
		ID: 7, Method: "POST", URL: "http://x/api", Type: "Fetch", Status: 201, Done: true,
		ReqHeaders:  map[string]string{"Authorization": "Bearer secret", "X-Tenant": "acme", "Content-Type": "application/json"},
		RespHeaders: map[string]string{"Set-Cookie": "sid=1", "Content-Type": "application/json"},
		PostData:    `{"id":1}`,
		MIMEType:    "application/json",
	}
	got := Detail(e, "{}", []string{"x-tenant"})
	for _, want := range []string{"POST http://x/api", "status: 201", "Authorization: ***", "X-Tenant: ***",
		"Set-Cookie: ***", "request body:\n{\n  \"id\": 1\n}", "response body (application/json):\n{}"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "secret") || strings.Contains(got, "sid=1") {
		t.Fatalf("sensitive value leaked:\n%s", got)
	}
}

func TestIsNoise(t *testing.T) {
	if !IsNoise("http://x/favicon.ico?v=2") || IsNoise("http://x/api") {
		t.Fatal("favicon detection failed")
	}
}
