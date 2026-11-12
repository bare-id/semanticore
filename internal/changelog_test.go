package internal

import (
	_ "embed"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

//go:embed test/Changelog.md
var cl []byte

func TestTrimChangelog(t *testing.T) {
	cases := []struct {
		input    []byte
		trim     int
		expected int
	}{
		{cl, 50, 44},                    // default trim
		{cl, 0, 195},                    // do not trim if nothing is found
		{cl, 200, 195},                  // do not trim if less than expected
		{nil, 200, 1},                   // edge case
		{[]byte(``), 100, 1},            // edge case
		{[]byte(`# Changelog`), 100, 1}, // edge case
	}

	for _, c := range cases {
		cl := TrimChangelog(c.input, c.trim)
		t.Log(string(cl))
		got := len(strings.Split(string(cl), "\n"))
		if got != c.expected {
			t.Logf("got %d, expected %d", got, c.expected)
			t.Fail()
		}
	}
}

func TestExtractChangelogSection(t *testing.T) {
	content := []byte("# Changelog\n\n## Version v1.2.0 (2026-01-02)\n\nfeatures\n\n## Version v1.1.0 (2026-01-01)\n\nfixes\n")

	section, err := ExtractChangelogSection(content, "v1.2.0")
	assert.NoError(t, err)
	assert.Equal(t, "## Version v1.2.0 (2026-01-02)\n\nfeatures", section)

	// version without the "v" prefix still matches
	section, err = ExtractChangelogSection(content, "1.1.0")
	assert.NoError(t, err)
	assert.Equal(t, "## Version v1.1.0 (2026-01-01)\n\nfixes", section)

	// unknown version falls back to the most recent (topmost) section
	section, err = ExtractChangelogSection(content, "v9.9.9")
	assert.NoError(t, err)
	assert.Equal(t, "## Version v1.2.0 (2026-01-02)\n\nfeatures", section)

	// no "## Version" sections at all
	_, err = ExtractChangelogSection([]byte("# Changelog\n"), "v1.2.0")
	assert.Error(t, err)
}
