package fakeagent

import (
	"strings"
	"testing"
)

func TestMaskTranscript(t *testing.T) {
	in := []Exchange{
		{Tool: "mock_add", Args: map[string]any{"urlPattern": "/api", "headers": map[string]any{"Authorization": "Bearer s3cr3t", "X-Ok": "1"}}},
		{Tool: "network_get", Text: "request headers:\n  Authorization: Bearer leak\n  Cookie: sid=9\n  Accept: */*"},
	}
	out := MaskTranscript(in)
	all := ""
	for _, e := range out {
		all += e.Text + " " + strings.Join(flatten(e.Args), " ")
	}
	for _, secret := range []string{"s3cr3t", "leak", "sid=9"} {
		if strings.Contains(all, secret) {
			t.Fatalf("secret %q not masked: %s", secret, all)
		}
	}
	if !strings.Contains(all, "Accept: */*") || !strings.Contains(all, "1") {
		t.Fatal("non-sensitive values must stay")
	}
	if in[0].Args["headers"].(map[string]any)["Authorization"] != "Bearer s3cr3t" {
		t.Fatal("input must not be modified")
	}
}

func flatten(m map[string]any) []string {
	var out []string
	for k, v := range m {
		switch x := v.(type) {
		case map[string]any:
			out = append(out, flatten(x)...)
		default:
			out = append(out, k+"="+toString(x))
		}
	}
	return out
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
