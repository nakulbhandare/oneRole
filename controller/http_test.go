package controller

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONRequiresSingleBoundedObject(t *testing.T) {
	type request struct {
		Name string `json:"name"`
	}
	cases := []struct {
		name string
		body string
		want string
		bad  bool
	}{
		{name: "valid", body: `{"name":"OneRole"}`, want: "OneRole"},
		{name: "unknown field", body: `{"name":"OneRole","tenant_id":"untrusted"}`, bad: true},
		{name: "trailing object", body: `{"name":"OneRole"}{}`, bad: true},
		{name: "oversized body", body: `{"name":"` + strings.Repeat("x", maxJSONRequestBytes) + `"}`, bad: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/api/v1", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			var got request
			err := decodeJSON(r, &got)
			if tc.bad && err == nil {
				t.Fatal("expected invalid request")
			}
			if !tc.bad && err != nil {
				t.Fatalf("valid request rejected: %v", err)
			}
			if !tc.bad && got.Name != tc.want {
				t.Fatalf("got name %q, want %q", got.Name, tc.want)
			}
		})
	}
}

func TestDecodeJSONRequiresApplicationJSON(t *testing.T) {
	r := httptest.NewRequest("POST", "/api/v1", strings.NewReader(`{"name":"OneRole"}`))
	r.Header.Set("Content-Type", "text/plain")
	var got struct {
		Name string `json:"name"`
	}
	if err := decodeJSON(r, &got); err == nil {
		t.Fatal("expected invalid content type to be rejected")
	}
}
