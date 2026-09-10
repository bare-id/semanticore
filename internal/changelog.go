package internal

import (
	"fmt"
	"log"
	"strings"
)

// ExtractChangelogSection returns the "## Version <version>" block from a changelog
// file's contents, up to (but not including) the next "## Version" heading or EOF.
// version is matched with or without a leading "v" prefix. If no block matches
// version exactly, the first "## Version" block in the file is returned instead
// (with a logged warning), since a changelog written by a prior Semanticore run is
// expected to have the relevant version at the top.
func ExtractChangelogSection(content []byte, version string) (string, error) {
	blocks := strings.Split(string(content), "## Version ")
	if len(blocks) < 2 {
		return "", fmt.Errorf("no changelog sections found")
	}

	trimmedVersion := strings.TrimPrefix(strings.TrimSpace(version), "v")

	var fallback string
	for i, block := range blocks[1:] {
		i++
		heading := strings.SplitN(block, "\n", 2)[0]
		headingVersion := strings.TrimPrefix(strings.TrimSpace(strings.Fields(heading)[0]), "v")
		if i == 1 {
			fallback = strings.TrimSpace("## Version " + block)
		}
		if headingVersion == trimmedVersion {
			return strings.TrimSpace("## Version " + block), nil
		}
	}

	log.Printf("[semanticore] no changelog section found for %s, using the most recent entry instead", version)
	return fallback, nil
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
