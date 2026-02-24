package p9

import (
	"strings"
	"testing"
)

// goodDesc is a well-formed description that should pass all lint rules.
// It contains: file path, backtick identifier, Acme address, acceptance
// criterion, how signal, imperative start, and a cross-reference.
const goodDesc = "Add `lintDescription()` check in `internal/p9/beads.go:615,663` following the pattern of existing checks; the function must return a non-empty warnings slice when the description violates any rule. See bd-9y4 for full rule list."

func warnCount(desc, substr string) bool {
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

// TestGoodDescPassesAll verifies that a high-quality description triggers no warnings.
func TestGoodDescPassesAll(t *testing.T) {
	warnings := lintDescription(goodDesc)
	if len(warnings) != 0 {
		t.Errorf("expected no warnings for good description, got: %v", warnings)
	}
}

// --- Rule 1: File path ---

func TestMissingFilePath(t *testing.T) {
	desc := "Add a helper function following the existing pattern; it must return an error when the input is empty."
	if !warnCount(desc, "missing file path") {
		t.Error("expected 'missing file path' warning")
	}
}

func TestFilePathPresent(t *testing.T) {
	desc := "Add `helper()` in `internal/util.go:42` following the existing pattern; it must return an error when input is empty. See bd-abc."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "missing file path") {
			t.Errorf("unexpected 'missing file path' warning for desc with .go extension")
		}
	}
}

// --- Rule 2: Function name or location ---

func TestMissingFuncOrLine(t *testing.T) {
	desc := "Update internal/p9/beads.go following the existing pattern; the file must compile without errors after the change. See bd-abc for context."
	if !warnCount(desc, "missing function name") {
		t.Error("expected 'missing function name or location' warning")
	}
}

func TestFuncNamePresent(t *testing.T) {
	desc := "Fix `lintDescription()` in `internal/p9/beads.go` following the existing pattern; it must return warnings for vague descriptions. See bd-9y4."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "missing function name") {
			t.Errorf("unexpected location warning when func name present")
		}
	}
}

func TestAcmeLineAddrRecognised(t *testing.T) {
	desc := "Fix `foo()` in `cmd/server.go:385` following the existing error-handling pattern; it must return a wrapped error. See bd-abc."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "missing function name") {
			t.Errorf("unexpected location warning when Acme line address present")
		}
	}
}

func TestAcmeRegexAddrRecognised(t *testing.T) {
	desc := "Update `internal/p9/beads.go:/lintDescription/` following the existing pattern; it must emit a warning for vague language. See bd-abc."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "missing function name") {
			t.Errorf("unexpected location warning when Acme /regex/ address present")
		}
	}
}

// --- Rule 3: Minimum length ---

func TestTooShort(t *testing.T) {
	if !warnCount("Fix bug.", "too short") {
		t.Error("expected 'too short' warning for very short description")
	}
}

// --- Rule 4: Acceptance criterion ---

func TestMissingAcceptance(t *testing.T) {
	desc := "Add `helper()` to `internal/util.go:42` following the existing pattern; the function handles empty input gracefully with a cross-reference to bd-abc."
	if !warnCount(desc, "acceptance criterion") {
		t.Error("expected 'acceptance criterion' warning")
	}
}

func TestAcceptancePresent(t *testing.T) {
	desc := "Add `helper()` to `internal/util.go:42` following the existing pattern; it must return an error on empty input. See bd-abc."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "acceptance criterion") {
			t.Errorf("unexpected acceptance warning when 'must' keyword present")
		}
	}
}

// --- Rule 5: Acme address format ---

func TestHyphenRangeWarning(t *testing.T) {
	desc := "Fix `lintDescription()` in `internal/p9/beads.go:611-663` following the existing pattern; it must return warnings correctly. See bd-abc for context."
	if !warnCount(desc, "invalid Acme address") {
		t.Error("expected 'invalid Acme address' warning for hyphen range")
	}
}

func TestCommaRangeNoWarning(t *testing.T) {
	desc := "Fix `lintDescription()` in `internal/p9/beads.go:611,663` following the existing pattern; it must return warnings correctly. See bd-abc for context."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "invalid Acme address") {
			t.Errorf("unexpected Acme address warning for correct comma range")
		}
	}
}

// --- Rule 6: Imperative verb start ---

func TestNonImperativeStart(t *testing.T) {
	desc := "Need to add `helper()` in `internal/util.go:42` following existing pattern; it must return error on empty input. See bd-abc."
	if !warnCount(desc, "imperative verb") {
		t.Error("expected imperative verb warning for 'Need to' start")
	}
}

func TestImperativeStart(t *testing.T) {
	desc := "Add `helper()` in `internal/util.go:42` following existing pattern; it must return error on empty input. See bd-abc."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "imperative verb") {
			t.Errorf("unexpected imperative verb warning for description starting with 'Add'")
		}
	}
}

// --- Rule 7: Vague language ---

func TestVagueLanguage(t *testing.T) {
	desc := "Add `helper()` in `internal/util.go:42` following existing pattern; it must somehow return an error etc. See bd-abc."
	if !warnCount(desc, "vague language") {
		t.Error("expected vague language warning for 'somehow'")
	}
}

func TestNoVagueLanguage(t *testing.T) {
	desc := "Add `helper()` in `internal/util.go:42` following existing pattern; it must return a wrapped error on empty input. See bd-abc."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "vague language") {
			t.Errorf("unexpected vague language warning for precise description")
		}
	}
}

// --- Rule 8: "How" signal ---

func TestMissingHowSignal(t *testing.T) {
	desc := "Add `helper()` in `internal/util.go:42`; it must return an error on empty input. Cross-reference: bd-abc."
	if !warnCount(desc, "how' signal") {
		t.Error("expected 'how' signal warning")
	}
}

func TestHowSignalPresent(t *testing.T) {
	desc := "Add `helper()` in `internal/util.go:42` following the existing error-handling pattern; it must return an error on empty input. See bd-abc."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "how' signal") {
			t.Errorf("unexpected 'how' warning when 'following' keyword present")
		}
	}
}

// --- Rule 9: No first-person ---

func TestFirstPersonWarning(t *testing.T) {
	desc := "I need to add `helper()` in `internal/util.go:42` following existing pattern; it must return error on empty input. See bd-abc."
	if !warnCount(desc, "first-person") {
		t.Error("expected first-person warning for 'I need'")
	}
}

func TestNoFirstPerson(t *testing.T) {
	desc := "Add `helper()` in `internal/util.go:42` following existing pattern; it must return error on empty input. See bd-abc."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "first-person") {
			t.Errorf("unexpected first-person warning for imperative description")
		}
	}
}

// --- Rule 10: Forbidden vague phrases ---

func TestForbiddenPhrase(t *testing.T) {
	desc := "Fix this in `internal/util.go:42` following existing pattern; it must return error on empty input. See bd-abc."
	if !warnCount(desc, "forbidden vague phrase") {
		t.Error("expected forbidden phrase warning for 'fix this'")
	}
}

func TestNoForbiddenPhrase(t *testing.T) {
	desc := "Fix `lintDescription()` in `internal/p9/beads.go:615` following the existing pattern; it must return warnings for vague descriptions. See bd-9y4."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "forbidden vague phrase") {
			t.Errorf("unexpected forbidden phrase warning for well-formed description")
		}
	}
}

// --- Rule 11: Inline code (backticks) ---

func TestMissingBacktick(t *testing.T) {
	desc := "Add helper() in internal/util.go:42 following existing pattern; it must return error on empty input. See bd-abc."
	if !warnCount(desc, "inline code") {
		t.Error("expected inline code warning when file path present but no backticks")
	}
}

func TestBacktickPresent(t *testing.T) {
	desc := "Add `helper()` in `internal/util.go:42` following existing pattern; it must return error on empty input. See bd-abc."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "inline code") {
			t.Errorf("unexpected inline code warning when backtick identifier present")
		}
	}
}

// --- Rule 12: Cross-reference on long descriptions ---

func TestLongDescMissingCrossRef(t *testing.T) {
	// >150 chars, has file path and backtick, but no bd-XXX or URL
	desc := "Add `lintDescription()` check in `internal/p9/beads.go:615` following the pattern of existing checks; the function must return a non-empty warnings slice when the description violates any rule and emit all applicable warnings."
	if !warnCount(desc, "cross-reference") {
		t.Error("expected cross-reference warning for long description without bd- ref")
	}
}

func TestLongDescWithCrossRef(t *testing.T) {
	desc := "Add `lintDescription()` check in `internal/p9/beads.go:615` following the pattern of existing checks; the function must return a non-empty warnings slice when the description violates any rule and emit all applicable warnings. See bd-9y4."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "cross-reference") {
			t.Errorf("unexpected cross-reference warning when bd- ref present")
		}
	}
}

func TestShortDescNoCrossRefWarning(t *testing.T) {
	// <=150 chars should not trigger the cross-reference rule
	desc := "Add `helper()` in `internal/util.go:42` following existing pattern; it must return error on empty input."
	for _, w := range lintDescription(desc) {
		if strings.Contains(w, "cross-reference") {
			t.Errorf("unexpected cross-reference warning for short description")
		}
	}
}

// --- Helper function unit tests ---

func TestContainsLineRef(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"beads.go:385", true},
		{"see L385 for details", true},
		{"char pos #4096", true},
		{"no address here", false},
	}
	for _, tc := range cases {
		if got := containsLineRef(tc.s); got != tc.want {
			t.Errorf("containsLineRef(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestContainsAcmeRegexAddr(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"beads.go:/lintDescription/", true},
		{"see /funcName/ in the file", true},
		{"no slashes", false},
		{"a/b/c directory path", false},          // single-char tokens, below 4-char minimum
		{"internal/p9/beads.go", false},           // short path components (/p9/ = 2 chars)
		{"internal/pkg/beads.go", false},          // /pkg/ = 3 chars, below minimum
		{"see /handleNewCommand/ here", true},     // 16 chars >= 4
	}
	for _, tc := range cases {
		if got := containsAcmeRegexAddr(tc.s); got != tc.want {
			t.Errorf("containsAcmeRegexAddr(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestContainsHyphenRange(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"beads.go:123-125", true},
		{"beads.go:123,125", false},
		{"beads.go:123", false},
		{"version 1-2", false}, // no colon before
	}
	for _, tc := range cases {
		if got := containsHyphenRange(tc.s); got != tc.want {
			t.Errorf("containsHyphenRange(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}

func TestContainsBdRef(t *testing.T) {
	cases := []struct {
		s    string
		want bool
	}{
		{"see bd-abc for context", true},
		{"bd-9y4 and bd-iti", true},
		{"bd- no alphanumeric", false},
		{"no reference here", false},
	}
	for _, tc := range cases {
		if got := containsBdRef(tc.s); got != tc.want {
			t.Errorf("containsBdRef(%q) = %v, want %v", tc.s, got, tc.want)
		}
	}
}
