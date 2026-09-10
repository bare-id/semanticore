package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseChangeLabels(t *testing.T) {
	// standard 4-label config
	labels, ok := parseChangeLabels("change::emergency,change::major,change::normal,change::standard")
	assert.True(t, ok)
	assert.Equal(t, []string{"change::emergency", "change::major", "change::normal", "change::standard"}, labels)

	// 3 labels
	labels, ok = parseChangeLabels("change::emergency,change::normal,change::standard")
	assert.True(t, ok)
	assert.Equal(t, []string{"change::emergency", "change::normal", "change::standard"}, labels)

	// 5 labels
	labels, ok = parseChangeLabels("change::emergency,change::major,change::normal,change::minor,change::standard")
	assert.True(t, ok)
	assert.Len(t, labels, 5)

	// minimum: exactly 2 labels
	labels, ok = parseChangeLabels("change::important,change::standard")
	assert.True(t, ok)
	assert.Equal(t, []string{"change::important", "change::standard"}, labels)

	// too few: only 1 label
	_, ok = parseChangeLabels("change::emergency")
	assert.False(t, ok)

	// empty
	_, ok = parseChangeLabels("")
	assert.False(t, ok)

	// duplicates
	_, ok = parseChangeLabels("change::emergency,change::major,change::normal,change::normal")
	assert.False(t, ok)

	// labels without :: separator
	_, ok = parseChangeLabels("emergency,major,normal,standard")
	assert.False(t, ok)
}

func TestParseChangeLabelDefault(t *testing.T) {
	priority := []string{"change::emergency", "change::major", "change::normal", "change::standard"}

	dl, ok := parseChangeLabelDefault("change::standard", priority)
	assert.True(t, ok)
	assert.Equal(t, "change::standard", dl)

	// empty string is valid (no fallback)
	dl, ok = parseChangeLabelDefault("", priority)
	assert.True(t, ok)
	assert.Equal(t, "", dl)

	// label not in priority list
	_, ok = parseChangeLabelDefault("change::unknown", priority)
	assert.False(t, ok)
}

func TestParseChangeLabelMap(t *testing.T) {
	priority := []string{"change::emergency", "change::major", "change::normal", "change::standard"}

	mapping, ok := parseChangeLabelMap("feat=change::normal,chore=change::standard,ops=change::standard", priority)
	assert.True(t, ok)
	assert.Equal(t, map[string]string{
		"feat":  "change::normal",
		"chore": "change::standard",
		"ops":   "change::standard",
	}, mapping)

	mapping, ok = parseChangeLabelMap("", priority)
	assert.True(t, ok)
	assert.Empty(t, mapping)

	_, ok = parseChangeLabelMap("feat=change::normal,feat=change::standard", priority)
	assert.False(t, ok)

	_, ok = parseChangeLabelMap("feat=change::critical", priority)
	assert.False(t, ok)

	_, ok = parseChangeLabelMap("feat-change::normal", priority)
	assert.False(t, ok)
}

func TestParseChangeLabelPolicy(t *testing.T) {
	policy, ok := parseChangeLabelPolicy("upgrade-only")
	assert.True(t, ok)
	assert.Equal(t, "upgrade-only", policy)

	// case-insensitive, trims whitespace
	policy, ok = parseChangeLabelPolicy("  Overwrite  ")
	assert.True(t, ok)
	assert.Equal(t, "overwrite", policy)

	_, ok = parseChangeLabelPolicy("")
	assert.False(t, ok)

	_, ok = parseChangeLabelPolicy("downgrade")
	assert.False(t, ok)
}

func TestParseEmergencyTrailer(t *testing.T) {
	key, value, ok := parseEmergencyTrailer("Change-Type: emergency")
	assert.True(t, ok)
	assert.Equal(t, "Change-Type", key)
	assert.Equal(t, "emergency", value)

	// extra whitespace is trimmed
	key, value, ok = parseEmergencyTrailer("  Change-Type :  emergency  ")
	assert.True(t, ok)
	assert.Equal(t, "Change-Type", key)
	assert.Equal(t, "emergency", value)

	// missing colon
	_, _, ok = parseEmergencyTrailer("Change-Type emergency")
	assert.False(t, ok)

	// missing key or value
	_, _, ok = parseEmergencyTrailer(": emergency")
	assert.False(t, ok)
	_, _, ok = parseEmergencyTrailer("Change-Type:")
	assert.False(t, ok)

	_, _, ok = parseEmergencyTrailer("")
	assert.False(t, ok)
}

func TestParseCSVList(t *testing.T) {
	assert.Equal(t, []string{"hotfix/*"}, parseCSVList("hotfix/*"))
	assert.Equal(t, []string{"hotfix/*", "urgent/*"}, parseCSVList("hotfix/*,urgent/*"))
	assert.Equal(t, []string{"hotfix/*", "urgent/*"}, parseCSVList(" hotfix/* , urgent/* "))
	assert.Empty(t, parseCSVList(""))
	assert.Empty(t, parseCSVList(" , , "))
}

func TestDetectCurrentTag(t *testing.T) {
	t.Setenv("CI_COMMIT_TAG", "")
	t.Setenv("GITHUB_REF_TYPE", "")
	t.Setenv("GITHUB_REF_NAME", "")
	assert.Equal(t, "", detectCurrentTag())

	t.Setenv("GITHUB_REF_TYPE", "branch")
	t.Setenv("GITHUB_REF_NAME", "main")
	assert.Equal(t, "", detectCurrentTag())

	t.Setenv("GITHUB_REF_TYPE", "tag")
	t.Setenv("GITHUB_REF_NAME", "v1.2.3")
	assert.Equal(t, "v1.2.3", detectCurrentTag())

	t.Setenv("CI_COMMIT_TAG", "v1.2.4")
	assert.Equal(t, "v1.2.4", detectCurrentTag())
}

func TestReadChangelogSectionForTag(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	assert.NoError(t, err)
	defer func() { assert.NoError(t, os.Chdir(cwd)) }()
	assert.NoError(t, os.Chdir(dir))

	content := "# Changelog\n\n## Version v1.2.0 (2026-01-02)\n\nfeatures\n"
	assert.NoError(t, os.WriteFile(filepath.Join(dir, "Changelog.md"), []byte(content), 0644))

	section, err := readChangelogSectionForTag("Changelog.md", "v1.2.0")
	assert.NoError(t, err)
	assert.Equal(t, "## Version v1.2.0 (2026-01-02)\n\nfeatures", section)

	// case-insensitive filename lookup, like the release commit logic
	section, err = readChangelogSectionForTag("changelog.md", "v1.2.0")
	assert.NoError(t, err)
	assert.Equal(t, "## Version v1.2.0 (2026-01-02)\n\nfeatures", section)

	_, err = readChangelogSectionForTag("missing.md", "v1.2.0")
	assert.Error(t, err)
}
