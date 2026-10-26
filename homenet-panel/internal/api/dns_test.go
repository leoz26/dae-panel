package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"homenet-panel/internal/model"
)

func TestDNSHandlersRoundTrip(t *testing.T) {
	s := &Server{}
	body := `{"text":"    upstream {\n        cn: &#39;udp://223.5.5.5:53&#39;\n    }\n    routing {\n        request {\n            fallback: cn\n        }\n    }"}`
	body = strings.ReplaceAll(body, "&#39;", "\u0027")
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/dns/parse", strings.NewReader(body))
	s.handleDNSParse(rec, req)
	if rec.Code != 200 {
		t.Fatalf("parse status %d: %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Spec     model.DNSSpec `json:"spec"`
		Warnings []string      `json:"warnings"`
		Default  bool          `json:"defaulted"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Spec.Upstreams) != 1 || out.Spec.RequestFallback != "cn" {
		t.Fatalf("parsed wrong: %+v", out.Spec)
	}
	if len(out.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", out.Warnings)
	}

	// render back
	payload, _ := json.Marshal(map[string]any{"spec": out.Spec})
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/api/dns/render", strings.NewReader(string(payload)))
	s.handleDNSRender(rec2, req2)
	if rec2.Code != 200 {
		t.Fatalf("render status %d: %s", rec2.Code, rec2.Body.String())
	}
	var out2 struct {
		Text string `json:"text"`
	}
	json.Unmarshal(rec2.Body.Bytes(), &out2)
	if !strings.Contains(out2.Text, "cn: \u0027udp://223.5.5.5:53\u0027") {
		t.Fatalf("render lost upstream: %s", out2.Text)
	}
	if !strings.Contains(out2.Text, "fallback: cn") {
		t.Fatalf("render lost fallback: %s", out2.Text)
	}
}

func TestDNSParseDefaultsWhenEmpty(t *testing.T) {
	s := &Server{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/dns/parse", strings.NewReader(`{"text":""}`))
	s.handleDNSParse(rec, req)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var out struct {
		Spec    model.DNSSpec `json:"spec"`
		Default bool          `json:"defaulted"`
	}
	json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.Default {
		t.Fatal("empty text should fall back to defaults")
	}
	if len(out.Spec.Upstreams) != 1 || out.Spec.ResponseFallback != "accept" {
		t.Fatalf("defaults wrong: %+v", out.Spec)
	}
}
