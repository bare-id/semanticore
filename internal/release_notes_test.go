package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReleaseNotesConfigGenerateOpenAI(t *testing.T) {
	testserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/v1/chat/completions", r.URL.Path)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))

		var body map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "gpt-test", body["model"])
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"## Release notes\n\n- improved experience"}}]}`)
	}))
	defer testserver.Close()

	cfg := NewReleaseNotesConfig(true, "openai", testserver.URL+"/v1/chat/completions", "gpt-test", "test-key", "", "")
	notes, err := cfg.Generate("## Version 1.2.3\n\n- feat: add search", []int{42})
	assert.NoError(t, err)
	assert.Contains(t, notes, "Release notes")
	assert.Contains(t, notes, "experience")
	assert.Contains(t, cfg.BuildPrompt("## Version 1.2.3", []int{42}), "#42")
}

func TestReleaseNotesConfigGenerateOllama(t *testing.T) {
	testserver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/generate", r.URL.Path)

		var body map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "llama3.1", body["model"])
		_, _ = fmt.Fprint(w, `{"response":"## Release notes\n\n- shipping faster"}`)
	}))
	defer testserver.Close()

	cfg := NewReleaseNotesConfig(true, "ollama", testserver.URL, "llama3.1", "", "", "")
	notes, err := cfg.Generate("## Version 1.2.3\n\n- docs: refresh README", nil)
	assert.NoError(t, err)
	assert.Contains(t, notes, "shipping")
}

func TestReleaseNotesConfigPromptFile(t *testing.T) {
	path := t.TempDir() + "/release-notes-prompt.txt"
	assert.NoError(t, os.WriteFile(path, []byte("Summarize from {{CHANGELOG}} and {{ISSUES}}"), 0644))

	cfg := NewReleaseNotesConfig(true, "openai", "https://api.openai.com/v1", "gpt-4o-mini", "token", "", path)
	assert.Contains(t, cfg.Prompt, "{{CHANGELOG}}")
	assert.Contains(t, cfg.Prompt, "{{ISSUES}}")
}

func TestReleaseNotesConfigDisabled(t *testing.T) {
	cfg := NewReleaseNotesConfig(false, "openai", "https://api.openai.com/v1", "gpt-4o-mini", "token", "", "")
	notes, err := cfg.Generate("## Version 1.2.3", nil)
	assert.NoError(t, err)
	assert.Empty(t, notes)
}
