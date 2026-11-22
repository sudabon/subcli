package agy_test

import (
	"slices"
	"testing"

	"github.com/sudabon/subcli/internal/provider"
	"github.com/sudabon/subcli/internal/provider/agy"
)

func TestInteractiveArgs(t *testing.T) {
	a := agy.New("")
	args := []string{"--continue", "--model", "test-model", "--add-dir=repo"}
	got, err := provider.InteractiveArgs(a, provider.AuthSubscription, args)
	if err != nil || !slices.Equal(got, args) {
		t.Fatalf("subscription args = %q, %v", got, err)
	}
	got, err = provider.InteractiveArgs(a, provider.AuthAPIKey, args)
	want := append([]string{"--settings", `{"modelProvider":"gemini"}`}, args...)
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("api_key args = %q, %v; want %q", got, err, want)
	}

	for _, unsafe := range [][]string{
		{"-p", "hi"}, {"-print=hi"}, {"--prompt=hi"}, {"--output-format", "stream-json"},
		{"acp"}, {"remote-control", "start"}, {"--remote-control"}, {"--remote-agent"},
		{"--model", "ok", "acp"}, {"--", "acp"}, {"--settings", "settings.json"},
	} {
		if _, err := provider.InteractiveArgs(a, provider.AuthSubscription, unsafe); err == nil {
			t.Errorf("subscription accepted %q", unsafe)
		}
	}
	for _, override := range [][]string{
		{"--settings", "file"}, {"-settings={}"}, {"--settings=file"},
	} {
		if _, err := provider.InteractiveArgs(a, provider.AuthAPIKey, override); err == nil {
			t.Errorf("api_key accepted settings override %q", override)
		}
	}
}
