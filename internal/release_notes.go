package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ReleaseNotesConfig struct {
	Enabled    bool
	Provider   string
	Endpoint   string
	Model      string
	APIKey     string
	Prompt     string
	PromptFile string
}

func NewReleaseNotesConfig(enabled bool, provider, endpoint, model, apiKey, prompt, promptFile string) ReleaseNotesConfig {
	cfg := ReleaseNotesConfig{
		Enabled:    enabled,
		Provider:   strings.TrimSpace(provider),
		Endpoint:   strings.TrimSpace(endpoint),
		Model:      strings.TrimSpace(model),
		APIKey:     strings.TrimSpace(apiKey),
		Prompt:     strings.TrimSpace(prompt),
		PromptFile: strings.TrimSpace(promptFile),
	}
	if cfg.Provider == "" && cfg.Endpoint != "" {
		if strings.Contains(strings.ToLower(cfg.Endpoint), "ollama") || strings.Contains(strings.ToLower(cfg.Endpoint), "localhost") || strings.Contains(strings.ToLower(cfg.Endpoint), "127.0.0.1") {
			cfg.Provider = "ollama"
		}
	}
	if cfg.Provider == "" {
		cfg.Provider = "openai"
	}
	if cfg.Provider == "openai" && cfg.Endpoint == "" {
		cfg.Endpoint = "https://api.openai.com/v1"
	}
	if cfg.Provider == "ollama" && cfg.Endpoint == "" {
		cfg.Endpoint = "http://localhost:11434"
	}
	if cfg.Model == "" {
		if cfg.Provider == "ollama" {
			cfg.Model = "llama3.1"
		} else {
			cfg.Model = "gpt-4o-mini"
		}
	}
	if cfg.Prompt == "" {
		cfg.Prompt = resolveReleaseNotesPrompt(
			cfg.PromptFile,
			defaultReleaseNotesPrompt,
		)
	}
	return cfg
}

func NewReleaseNotesConfigFromEnv() ReleaseNotesConfig {
	return NewReleaseNotesConfig(
		strings.EqualFold(os.Getenv("SEMANTICORE_RELEASE_NOTES_ENABLED"), "true"),
		os.Getenv("SEMANTICORE_RELEASE_NOTES_PROVIDER"),
		os.Getenv("SEMANTICORE_RELEASE_NOTES_ENDPOINT"),
		os.Getenv("SEMANTICORE_RELEASE_NOTES_MODEL"),
		os.Getenv("SEMANTICORE_RELEASE_NOTES_API_KEY"),
		os.Getenv("SEMANTICORE_RELEASE_NOTES_PROMPT"),
		os.Getenv("SEMANTICORE_RELEASE_NOTES_PROMPT_FILE"),
	)
}

func resolveReleaseNotesPrompt(promptFile string, fallback func() string) string {
	if strings.TrimSpace(promptFile) != "" {
		if data, err := os.ReadFile(promptFile); err == nil {
			if text := strings.TrimSpace(string(data)); text != "" {
				return text
			}
		}
	}
	for _, candidate := range []string{
		filepath.Join(".gitlab", "release-notes-prompt.txt"),
		filepath.Join(".github", "release-notes-prompt.txt"),
		"release-notes-prompt.txt",
	} {
		if data, err := os.ReadFile(candidate); err == nil {
			if text := strings.TrimSpace(string(data)); text != "" {
				return text
			}
		}
	}
	return fallback()
}

func defaultReleaseNotesPrompt() string {
	return `You write release notes for a software project. Respond with polished Markdown that is short, readable, and suitable for a pull request. Ignore marketing fluff. Use the changelog content as the source of truth, and keep the output concise. Include a short summary and up to 5 bullet points if useful. Do not repeat the raw markdown changelog verbatim.

Use this format:

## Release notes

- Short summary
- Key changes
- Impact / migration notes if relevant

{{CHANGELOG}}

{{ISSUES}}`
}

func (cfg ReleaseNotesConfig) BuildPrompt(changelog string, issueRefs []int) string {
	base := cfg.Prompt
	if strings.TrimSpace(base) == "" {
		base = defaultReleaseNotesPrompt()
	}

	issueSection := "No linked issues were identified."
	if len(issueRefs) > 0 {
		refs := make([]string, 0, len(issueRefs))
		for _, id := range issueRefs {
			refs = append(refs, fmt.Sprintf("#%d", id))
		}
		issueSection = "Linked issues or references: " + strings.Join(refs, ", ")
	}

	prompt := strings.ReplaceAll(base, "{{CHANGELOG}}", changelog)
	prompt = strings.ReplaceAll(prompt, "{{ISSUES}}", issueSection)
	return strings.TrimSpace(prompt)
}

func (cfg ReleaseNotesConfig) Generate(changelog string, issueRefs []int) (string, error) {
	if !cfg.Enabled || strings.TrimSpace(changelog) == "" {
		return "", nil
	}
	provider := strings.ToLower(strings.TrimSpace(cfg.Provider))
	switch provider {
	case "", "openai":
		return cfg.generateOpenAIReleaseNotes(cfg.BuildPrompt(changelog, issueRefs))
	case "ollama":
		return cfg.generateOllamaReleaseNotes(cfg.BuildPrompt(changelog, issueRefs))
	default:
		return "", fmt.Errorf("unsupported release-notes provider %q", cfg.Provider)
	}
}

func (cfg ReleaseNotesConfig) generateOpenAIReleaseNotes(prompt string) (string, error) {
	payload := map[string]any{
		"model": cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a technical release-note writer."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.2,
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("unable to marshal OpenAI request: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, cfg.Endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("unable to create OpenAI request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("unable to call OpenAI endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("unable to decode OpenAI response: %w", err)
	}
	if len(result.Choices) == 0 || strings.TrimSpace(result.Choices[0].Message.Content) == "" {
		return "", nil
	}
	return strings.TrimSpace(result.Choices[0].Message.Content), nil
}

func (cfg ReleaseNotesConfig) generateOllamaReleaseNotes(prompt string) (string, error) {
	payload := map[string]any{
		"model":  cfg.Model,
		"prompt": prompt,
		"stream": false,
	}

	reqBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("unable to marshal Ollama request: %w", err)
	}
	endpoint := strings.TrimRight(cfg.Endpoint, "/") + "/api/generate"
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return "", fmt.Errorf("unable to create Ollama request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("unable to call Ollama endpoint: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("unable to decode Ollama response: %w", err)
	}
	return strings.TrimSpace(result.Response), nil
}
