package model

import (
	"reflect"
	"testing"
)

func TestRewriteSubtags_Basic(t *testing.T) {
	got := RewriteSubtags(
		[]string{"subtag(dual) && name(keyword: '香港')", "subtag(CF) && name(keyword: '香港')"},
		[]string{"CF", "dual"},
		"merged",
	)
	want := []string{"subtag(merged) && name(keyword: '香港')"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRewriteSubtags_MultiLineDedupe(t *testing.T) {
	got := RewriteSubtags(
		[]string{"subtag(CF)", "subtag(dual)"},
		[]string{"CF", "dual"},
		"merged",
	)
	want := []string{"subtag(merged)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRewriteSubtags_KeepUnrelated(t *testing.T) {
	got := RewriteSubtags(
		[]string{"subtag(other) && name(keyword: 'x')"},
		[]string{"CF", "dual"},
		"merged",
	)
	want := []string{"subtag(other) && name(keyword: 'x')"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestRewriteSubtags_QuotedArg(t *testing.T) {
	got := RewriteSubtags(
		[]string{"subtag('dual')"},
		[]string{"dual"},
		"merged",
	)
	want := []string{"subtag(merged)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
