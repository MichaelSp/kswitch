package util

import "testing"

func TestGetContextForAlias_Found(t *testing.T) {
	mapping := map[string]string{
		"alias1": "context1",
		"alias2": "context2",
	}
	if got := GetContextForAlias("alias1", mapping); got != "context1" {
		t.Errorf("expected context1, got %q", got)
	}
	if got := GetContextForAlias("alias2", mapping); got != "context2" {
		t.Errorf("expected context2, got %q", got)
	}
}

func TestGetContextForAlias_NotFound(t *testing.T) {
	mapping := map[string]string{"alias1": "context1"}
	if got := GetContextForAlias("missing", mapping); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetContextForAlias_EmptyMap(t *testing.T) {
	if got := GetContextForAlias("anything", map[string]string{}); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestGetContextForAlias_NilMap(t *testing.T) {
	if got := GetContextForAlias("anything", nil); got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}
