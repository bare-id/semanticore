package internal

import (
	"fmt"
	"regexp"
	"strings"
)

var versionHeading = regexp.MustCompile(`(?m)^## Version `)

func ExtractChangelogSection(content []byte, version string) (string, error) {
	trimmedVersion := strings.TrimPrefix(strings.TrimSpace(version), "v")

	for _, block := range versionHeading.Split(string(content), -1)[1:] {
		fields := strings.Fields(strings.SplitN(block, "\n", 2)[0])
		if len(fields) > 0 && strings.TrimPrefix(fields[0], "v") == trimmedVersion {
			return strings.TrimSpace("## Version " + block), nil
		}
	}

	return "", fmt.Errorf("no changelog section found for %s", version)
}

func TrimChangelog(cl []byte, changelogMaxLines int) []byte {
	clLines := strings.Split(string(cl), "\n")

	if len(clLines) < changelogMaxLines {
		return cl
	}

	for i := changelogMaxLines - 1; i > 0; i-- {
		var l = strings.ReplaceAll(clLines[i], " ", "")
		l = strings.ToLower(l)
		if strings.HasPrefix(l, "##version") {
			return []byte(strings.Join(clLines[:i], "\n"))
		}
	}

	return cl
}
