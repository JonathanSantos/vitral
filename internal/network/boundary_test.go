package network

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/go-rod/rod/lib/proto"
)

// Fronteiras exatas apontadas pela mutação.
func TestFormatBoundaries(t *testing.T) {
	entries := []Entry{{ID: 1}, {ID: 2}}
	if got := Apply(entries, Filter{Limit: 2}); len(got) != 2 {
		t.Fatalf("limit equal to total keeps all: %v", got)
	}
	if !MatchStatus("error", Entry{Status: 400}) || MatchStatus("error", Entry{Status: 399}) {
		t.Fatal("error filter starts at 400")
	}
	if !IsProblem(Entry{Status: 400}) || IsProblem(Entry{Status: 399}) || IsProblem(Entry{Failed: "canceled"}) || !IsProblem(Entry{Failed: "net::ERR_FAILED"}) {
		t.Fatal("IsProblem boundaries")
	}
	if got := Line(Entry{ID: 3, Method: "GET", URL: "http://x"}); got != "3 GET pending http://x" {
		t.Fatalf("pending line without extras: %q", got)
	}
	exact := strings.Repeat("a", 150)
	if got := shorten(exact, 150); got != exact {
		t.Fatal("string at the limit is not shortened")
	}
	body := strings.Repeat("b", BodyLimit)
	if got := FormatBody([]byte(body), "text/plain"); got != body {
		t.Fatal("body at the limit is not truncated")
	}
	d := Detail(Entry{ID: 1, Method: "POST", URL: "http://x", HasPostData: true}, "", nil)
	if strings.Contains(d, "type:") || !strings.Contains(d, "request body: (not captured)") {
		t.Fatalf("detail without type and with uncaptured body:\n%s", d)
	}
}

func TestResponseKeepsRequestHeadersWhenAbsent(t *testing.T) {
	tr, _ := newTestTracker(5)
	tr.OnRequest(request("1", "http://x/api", proto.NetworkResourceTypeFetch, 1))
	tr.OnResponse(&proto.NetworkResponseReceived{RequestID: "1", Response: &proto.NetworkResponse{Status: 200}})
	if e, _ := tr.Get(1); e.ReqHeaders["Authorization"] == "" {
		t.Fatal("request headers must be kept when the response has none")
	}
}

// Um redirect reaproveita o requestId; despejar a entrada antiga não pode apagar a nova do índice.
func TestEvictionKeepsNewerEntryOfSameRequest(t *testing.T) {
	tr, _ := newTestTracker(1)
	tr.OnRequest(request("r", "http://x/old", proto.NetworkResourceTypeFetch, 1))
	redirect := request("r", "http://x/new", proto.NetworkResourceTypeFetch, 2)
	redirect.RedirectResponse = &proto.NetworkResponse{Status: 302}
	tr.OnRequest(redirect)
	tr.OnFinished(&proto.NetworkLoadingFinished{RequestID: "r", Timestamp: 3, EncodedDataLength: 10})
	e, ok := tr.Get(2)
	if !ok || !e.Done || e.Size != 10 {
		t.Fatalf("newer entry lost its updates: %+v %v", e, ok)
	}
}

func TestFormatBodyNeverSplitsARune(t *testing.T) {
	body := strings.Repeat("€", BodyLimit) // 3 bytes por caractere: o limite cai no meio de um
	got := FormatBody([]byte(body), "text/plain")
	head, _, _ := strings.Cut(got, "\n… (truncated")
	if !utf8.ValidString(head) || len(head) > BodyLimit || len(head) < BodyLimit-3 {
		t.Fatalf("cut must stay on a rune boundary: len %d valid %v", len(head), utf8.ValidString(head))
	}
}

func TestEvictionCleansIndex(t *testing.T) {
	tr, _ := newTestTracker(2)
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		tr.OnRequest(request(id, "http://x/"+id, proto.NetworkResourceTypeFetch, 1))
	}
	if len(tr.byReq) != 2 {
		t.Fatalf("index must only keep buffered requests, has %d", len(tr.byReq))
	}
}
