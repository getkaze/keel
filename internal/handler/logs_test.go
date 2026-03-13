package handler

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/getkaze/keel/internal/model"
)

func TestParseLines_Default(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: ""}}
	if got := parseLines(r); got != defaultLogLines {
		t.Errorf("expected default %d, got %d", defaultLogLines, got)
	}
}

func TestParseLines_Valid(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: "lines=250"}}
	if got := parseLines(r); got != 250 {
		t.Errorf("expected 250, got %d", got)
	}
}

func TestParseLines_ExceedsMax(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: "lines=99999"}}
	if got := parseLines(r); got != maxLogLines {
		t.Errorf("expected cap at %d, got %d", maxLogLines, got)
	}
}

func TestParseLines_Invalid(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: "lines=abc"}}
	if got := parseLines(r); got != defaultLogLines {
		t.Errorf("expected default %d for invalid input, got %d", defaultLogLines, got)
	}
}

func TestParseLines_Zero(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: "lines=0"}}
	if got := parseLines(r); got != defaultLogLines {
		t.Errorf("expected default %d for zero, got %d", defaultLogLines, got)
	}
}

func TestParseLines_Negative(t *testing.T) {
	r := &http.Request{URL: &url.URL{RawQuery: "lines=-5"}}
	if got := parseLines(r); got != defaultLogLines {
		t.Errorf("expected default %d for negative, got %d", defaultLogLines, got)
	}
}

func TestFindLogSource_Found(t *testing.T) {
	sources := []model.LogSource{
		{Name: "container", Type: "docker"},
		{Name: "app", Type: "file", Path: "/var/log/app.log"},
	}
	got := findLogSource(sources, "app")
	if got == nil {
		t.Fatal("expected to find 'app' source, got nil")
	}
	if got.Path != "/var/log/app.log" {
		t.Errorf("expected path '/var/log/app.log', got %q", got.Path)
	}
}

func TestFindLogSource_NotFound(t *testing.T) {
	sources := []model.LogSource{
		{Name: "container", Type: "docker"},
	}
	got := findLogSource(sources, "nonexistent")
	if got != nil {
		t.Errorf("expected nil for missing source, got %+v", got)
	}
}

func TestFindLogSource_Empty(t *testing.T) {
	got := findLogSource(nil, "app")
	if got != nil {
		t.Errorf("expected nil for empty sources, got %+v", got)
	}
}
