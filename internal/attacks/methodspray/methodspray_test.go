package methodspray

import (
	"testing"
)

func TestMethodSprayValidate(t *testing.T) {
	m := &MethodSpray{methods: "GET,POST,DELETE"}
	if err := m.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.methods != "GET,POST,DELETE" {
		t.Fatalf("methods: got %q", m.methods)
	}
}

func TestMethodSprayValidateEmpty(t *testing.T) {
	m := &MethodSpray{methods: ",,"}
	if err := m.Validate(); err == nil {
		t.Fatal("expected error for empty methods")
	}
}

func TestMethodSprayValidateNormalizes(t *testing.T) {
	m := &MethodSpray{methods: "get, post"}
	if err := m.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.methods != "GET,POST" {
		t.Fatalf("methods not uppercased: %q", m.methods)
	}
}
