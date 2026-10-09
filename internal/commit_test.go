package internal

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseCommit(t *testing.T) {
	var cases = []struct {
		commit             string
		typ                CommitType
		scope, description string
		major              bool
	}{
		{`feat(something): test`, TypeFeat, `something`, `test`, false},
		{`bug(something): test`, TypeFix, `something`, `test`, false},
		{`bugfix(something): test`, TypeFix, `something`, `test`, false},
		{`bugfixes(something): test`, TypeFix, `something`, `test`, false},
		{`fix(something): test`, TypeFix, `something`, `test`, false},
		{`fix(something) test`, TypeFix, `something`, `fix(something) test`, false},
		{`fixes(something) test`, TypeFix, `something`, `fixes(something) test`, false},
		{`feat: test`, TypeFeat, ``, `test`, false},
		{`feat`, TypeFeat, ``, `feat`, false},
		{`feat:`, TypeOther, ``, `feat:`, false},
		{`feat:   test   `, TypeFeat, ``, `test`, false},
		{`Feat:   test   `, TypeFeat, ``, `test`, false},
		{`Feat   test   `, TypeFeat, ``, `Feat   test`, false},
		{`Feat[ someScope ]   test   `, TypeFeat, `somescope`, `Feat[ someScope ]   test`, false},
		{`Feat[ someScope ]:   test   `, TypeFeat, `somescope`, `test`, false},
		{`Feature[ someScope ]:   test   `, TypeFeat, `somescope`, `test`, false},
		{`test: test`, TypeTest, ``, `test`, false},
		{`testing: test`, TypeTest, ``, `test`, false},
		{"testing:\n\ttest\n", TypeTest, ``, `test`, false},
		// prefixes or ticket numbers
		{"#123 fix: something", TypeFix, ``, `something`, false},
		{"[fix] something", TypeFix, ``, `[fix] something`, false},
		{"#12345 [fix] something", TypeFix, ``, `#12345 [fix] something`, false},
		{"#12345 fix(test): something", TypeFix, `test`, `something`, false},
		// all possible values
		{`fix(something): test`, TypeFix, `something`, `test`, false},
		{`bug(something): test`, TypeFix, `something`, `test`, false},
		{`feat(something): test`, TypeFeat, `something`, `test`, false},
		{`test(something): test`, TypeTest, `something`, `test`, false},
		{`chore(something): test`, TypeChore, `something`, `test`, false},
		{`update(something): test`, TypeChore, `something`, `test`, false},
		{`ops(something): test`, TypeOps, `something`, `test`, false},
		{`ci(something): test`, TypeOps, `something`, `test`, false},
		{`cd(something): test`, TypeOps, `something`, `test`, false},
		{`build(something): test`, TypeOps, `something`, `test`, false},
		{`doc(something): test`, TypeDocs, `something`, `test`, false},
		{`perf(something): test`, TypePerf, `something`, `test`, false},
		{`refactor(something): test`, TypeRefactor, `something`, `test`, false},
		{`rework(something): test`, TypeRefactor, `something`, `test`, false},
		{`security(something): test`, TypeSecurity, `something`, `test`, false},
		{`sec(something): test`, TypeSecurity, `something`, `test`, false},
		{`invalid(something): test`, TypeOther, ``, `invalid(something): test`, false},
		// major commits
		{"testing:\n\ttest\nBREAKING CHANGE: major commit", TypeTest, ``, `test`, true},
		{"testing!:\n\ttest\n", TypeTest, ``, `test`, true},
		{"testing(scope)!:\n\ttest\n", TypeTest, `scope`, `test`, true},
		// special chars
		{"test(<&>): fix <foo> & bar tags", TypeTest, `&lt;&amp;&gt;`, `fix &lt;foo&gt; &amp; bar tags`, false},
	}

	for _, c := range cases {
		typ, scope, description, major := ParseCommitMessage(c.commit)
		if typ != c.typ || scope != c.scope || description != c.description || major != c.major {
			t.Errorf("commit %q not parsed: typ: %q != %q, scope: %q != %q, description: %q != %q, major %v != %v", c.commit, c.typ, typ, c.scope, scope, c.description, description, c.major, major)
		}
	}
}

func TestDetectReleaseCommit(t *testing.T) {
	var cases = []struct {
		commit              string
		merge               bool
		vPrefix             string
		major, minor, patch int
	}{
		{"Release v1.2.3", false, "v", 1, 2, 3},
		{"Merge a into b\n\nRelease v1.2.3\n\nFoo bar", true, "v", 1, 2, 3},
		{"multi line\n\nRelease v1.2.3\n\nFoo bar", false, "v", 0, 0, 0},
		{"Release v1.2.3\nfoo", false, "v", 0, 0, 0},
		{"Release v1.2.3\n\nfoo", false, "v", 1, 2, 3},
		{"Fixed Release v1.2.3", false, "v", 0, 0, 0},
		{"Release v1.2.3 was totally broken", false, "v", 0, 0, 0},
		{"Release v1.2.3 (#15)", false, "v", 1, 2, 3},
		{"Release v1.2.3 (#15)", true, "v", 1, 2, 3},
		{"Release v1.2.3 (#15)\n\nCo-authored-by: test", false, "v", 1, 2, 3},
		{"Release 1.2.3 (#15)\n\nCo-authored-by: test", false, "", 1, 2, 3},
		{"Release 1.2.3 (#15)", true, "", 1, 2, 3},
		{"Merge a into b\n\nRelease 1.2.3\n\nFoo bar", true, "", 1, 2, 3},
	}
	for _, c := range cases {
		vPrefix, major, minor, patch := DetectReleaseCommit(c.commit, c.merge)
		if vPrefix != c.vPrefix || major != c.major || minor != c.minor || patch != c.patch {
			t.Errorf("detectReleaseCommit %q failed with %q != %q, %d != %d, %d != %d, %d != %d", c.commit, c.vPrefix, vPrefix, c.major, major, c.minor, minor, c.patch, patch)
		}
	}
}

func TestExtractPrefixedLabels(t *testing.T) {
	labels := ExtractPrefixedLabels("feat: x\n\nchange::major and change::major\nCHANGE::EMERGENCY", "change::")
	assert.Equal(t, []string{"change::major", "change::emergency"}, labels)

	assert.Empty(t, ExtractPrefixedLabels("feat: x", ""))
}

func TestParseTrailers(t *testing.T) {
	// simple trailer in the last paragraph
	trailers := ParseTrailers("fix: something\n\nChange-Type: emergency")
	assert.Equal(t, map[string]string{"change-type": "emergency"}, trailers)

	// case-insensitive key and value are normalized/preserved as-is
	trailers = ParseTrailers("fix: something\n\nCHANGE-TYPE: EMERGENCY")
	assert.Equal(t, map[string]string{"change-type": "EMERGENCY"}, trailers)

	// multiple trailers in the same block
	trailers = ParseTrailers("fix: something\n\nChange-Type: emergency\nReviewed-by: someone")
	assert.Equal(t, map[string]string{"change-type": "emergency", "reviewed-by": "someone"}, trailers)

	// trailer-looking line is NOT in the last paragraph -> no trailer block at all
	trailers = ParseTrailers("fix: something\n\nChange-Type: emergency\n\nSome more prose after it")
	assert.Nil(t, trailers)

	// last paragraph has a mix of trailer-like and free-text lines -> not a trailer block
	trailers = ParseTrailers("fix: something\n\nChange-Type: emergency\nthis is not a trailer")
	assert.Nil(t, trailers)

	// no body at all
	trailers = ParseTrailers("fix: something")
	assert.Nil(t, trailers)
}

func TestHasTrailer(t *testing.T) {
	assert.True(t, HasTrailer("fix: something\n\nChange-Type: emergency", "Change-Type", "emergency"))
	assert.True(t, HasTrailer("fix: something\n\nchange-type: EMERGENCY", "Change-Type", "emergency"))
	assert.False(t, HasTrailer("fix: something\n\nChange-Type: normal", "Change-Type", "emergency"))
	assert.False(t, HasTrailer("fix: something\n\nChange-Type: emergency\n\nmore prose", "Change-Type", "emergency"))
	assert.False(t, HasTrailer("fix: something", "Change-Type", "emergency"))
}

func TestExtractIssueRefs(t *testing.T) {
	refs := ExtractIssueRefs("fix: something\n\nCloses #42, related to #123\nalso see #42")
	assert.Equal(t, []int{42, 123}, refs)

	refs = ExtractIssueRefs("#1 fix: something")
	assert.Equal(t, []int{1}, refs)

	assert.Empty(t, ExtractIssueRefs("fix: no issue ref here"))
}
