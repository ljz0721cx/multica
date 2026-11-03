package handler

import (
	"testing"
)

func TestLLMModelsEndpoint(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"https://api.example.com/v1", "https://api.example.com/v1/models"},
		{"https://api.example.com/v1/", "https://api.example.com/v1/models"},
		{"https://api.example.com", "https://api.example.com/v1/models"},
		{"https://api.example.com/v1/models", "https://api.example.com/v1/models"},
	}
	for _, tc := range cases {
		if got := llmModelsEndpoint(tc.in); got != tc.want {
			t.Fatalf("llmModelsEndpoint(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseGatewayModelIDs(t *testing.T) {
	ids, err := parseGatewayModelIDs([]byte(`{"data":[{"id":"b"},{"id":"a"},{"id":"a"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("unexpected ids: %#v", ids)
	}

	ids, err = parseGatewayModelIDs([]byte(`{"models":[{"id":"m1"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "m1" {
		t.Fatalf("unexpected ids: %#v", ids)
	}

	ids, err = parseGatewayModelIDs([]byte(`[{"id":"x"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "x" {
		t.Fatalf("unexpected ids: %#v", ids)
	}
}
