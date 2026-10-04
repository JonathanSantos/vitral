package network

import (
	"strings"
	"testing"
)

func TestBriefDropsQueryAndShortens(t *testing.T) {
	long := "https://rr3---sn-q4fl6ndl.googlevideo.com/videoplayback?expire=1621358378&ei=abc&ip=1.2.3.4&sig=" + strings.Repeat("x", 600)
	got := Brief(Entry{ID: 73, Method: "GET", URL: long, Status: 403, Failed: "canceled", Done: true})
	if got != "73 GET failed https://rr3---sn-q4fl6ndl.googlevideo.com/videoplayback?… (canceled)" {
		t.Fatalf("got %q", got)
	}
	path := "https://example.com/" + strings.Repeat("a", 200)
	if got := Brief(Entry{ID: 1, Method: "POST", URL: path, Status: 500, Done: true}); len([]rune(got)) > 100 || !strings.HasSuffix(got, "…") || !strings.HasPrefix(got, "1 POST 500 https://example.com/") {
		t.Fatalf("long path should be cut: %q", got)
	}
	if got := Brief(Entry{ID: 3, Method: "GET", URL: "https://a.test/api", Status: 500, Done: true, Mocked: true}); got != "3 GET 500 https://a.test/api (mocked)" {
		t.Fatalf("mocked responses must say so: %q", got)
	}
	if got := Brief(Entry{ID: 2, Method: "GET", URL: "https://a.test/x?", Status: 0}); got != "2 GET pending https://a.test/x" {
		t.Fatalf("empty query: %q", got)
	}
}

func TestBriefGroups(t *testing.T) {
	video := "https://rr1.googlevideo.com/videoplayback?sig="
	got := BriefGroups([]Entry{
		{ID: 83, Method: "GET", URL: video + "a", Failed: "canceled", Done: true},
		{ID: 84, Method: "GET", URL: "https://a.test/api", Status: 500, Done: true},
		{ID: 87, Method: "GET", URL: video + "b", Failed: "canceled", Done: true},
		{ID: 88, Method: "GET", URL: video + "c", Failed: "canceled", Done: true},
	})
	want := []string{
		"83 GET failed https://rr1.googlevideo.com/videoplayback?… (canceled) ×3",
		"84 GET 500 https://a.test/api",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
	if len(BriefGroups(nil)) != 0 {
		t.Fatal("no entries, no lines")
	}
}
