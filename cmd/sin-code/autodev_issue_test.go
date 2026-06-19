// SPDX-License-Identifier: MIT
// Purpose: tests for autodev_issue_cmd.go — focused on the `gh auth
// status` parsing path that detectRepo relies on (issue #418 follow-up).
//
// Two surfaces are exercised:
//  1. parseGhAuthStatus                — pure-string helper, hermetic.
//  2. detectRepo end-to-end             — using the newGhBridgeForDetect
//     hook to inject a fake ghbridge.Bridge with a deterministic Runner
//     so we cover both the "auth OK" and the "auth not OK" branches
//     without a real `gh` install, network, or auth credentials.
//
// All cases for parseGhAuthStatus are static fixtures (the public gh
// docs + hand-collected samples from gh 2.40 through 2.50); no real
// `gh` is invoked. Tests are race-clean by construction (no shared
// state, no goroutines).
// Docs: autodev.doc.md
package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/ghbridge"
)

// Static `gh auth status` sample outputs, transcribed verbatim from
// `gh auth status` runs against gh 2.40 ≥ 2.50. Kept as package-level
// consts so test failures point at the exact byte-sequence under
// scrutiny — not at an interpolated format string that masks drift.
const (
	// sampleOldFormat is the pre-gh-2.40 ASCII layout: a single
	// "Logged in to <host> ..." line, no leading marker, no host
	// header. gh still emits this on Linux/Wine builds pinned at
	// 2.39 and earlier.
	sampleOldFormat = "Logged in to github.com as jeremyblow (oauth_token)\n"

	// sampleNewFormatUnicode is the gh 2.40+ layout: each host has a
	// header line, then a marker-prefixed status line. "✓" is
	// U+2713 (CHECK MARK).
	sampleNewFormatUnicode = "github.com\n" +
		"  ✓ Logged in to github.com account Delqhi (keyring)\n" +
		"  - Active account: true\n" +
		"  - Git operations protocol: https\n" +
		"  - Token: gho_************************************\n" +
		"  - Token scopes: 'gist', 'read:org', 'repo', 'workflow'\n"

	// sampleNewFormatHeavyUnicode uses U+2714 (HEAVY CHECK MARK).
	// Emitted by some terminal/font fallbacks. Must match.
	sampleNewFormatHeavyUnicode = "github.com\n" +
		"  ✔ Logged in to github.com account user (keyring)\n"

	// sampleNewFormatASCIICheckmark uses "[OK]" — emitted by some
	// CI / Windows builds where the unicode glyph is missing.
	sampleNewFormatASCIICheckmark = "github.com\n" +
		"  [OK] Logged in to github.com account user (oauth_token)\n"

	// sampleNotLoggedInPre240 is the pre-2.40 unauthenticated
	// message. Sent on stdout or stderr depending on gh version;
	// either way the substring check sees it on stdout in some
	// installs, so we treat it as a negative case.
	sampleNotLoggedInPre240 = "You are not logged into any GitHub hosts. Run gh auth login to authenticate.\n"

	// sampleNotLoggedIn240 is the gh 2.40+ per-host failure line for
	// github.com itself.
	sampleNotLoggedIn240 = "github.com\n" +
		"  ✗ You are not logged in\n"

	// sampleGithubFailed is the gh 2.40+ per-host token-rejection
	// line — different verb ("Failed to log in") so it must NOT pass
	// even though the host name "github.com" is on the line.
	sampleGithubFailed = "github.com\n" +
		"  X Failed to log in to github.com using token (default)\n" +
		"  - The token in default is invalid.\n"

	// sampleMultiHostPartialOK is the multi-host case where one
	// non-github host has a stale token but github.com is OK. Must
	// return true because the user IS authenticated to github.com.
	sampleMultiHostPartialOK = "example.com\n" +
		"  X Failed to log in to example.com using token (default)\n" +
		"github.com\n" +
		"  ✓ Logged in to github.com account jeremyblow (keyring)\n"

	// sampleNonGithubHost uses a gitlab host. The phrase structure
	// is identical to the GitHub positive case — the host segment is
	// the only discriminator. Must return false.
	sampleNonGithubHost = "gitlab.example.com\n" +
		"  ✓ Logged in to gitlab.example.com account user (oauth_token)\n"
)

// TestDetectRepo drives the parseGhAuthStatus table. Subtests under
// `TestDetectRepo` so `go test -run TestDetectRepo` matches the
// entire matrix in one run.
func TestDetectRepo(t *testing.T) {
	t.Run("OldFormat_PlainASCII", func(t *testing.T) {
		if !parseGhAuthStatus(sampleOldFormat) {
			t.Errorf("old ASCII format must parse positive:\n%s", sampleOldFormat)
		}
	})
	t.Run("NewFormat_UnicodeCheckmark", func(t *testing.T) {
		if !parseGhAuthStatus(sampleNewFormatUnicode) {
			t.Errorf("2.40+ unicode '✓' format must parse positive:\n%s",
				sampleNewFormatUnicode)
		}
		// Lock the formatting assumption: if the fixture ever loses
		// the unicode checkmark, fail loudly so reviewers see the
		// drift instead of relying on a coincidental regex match.
		if !strings.Contains(sampleNewFormatUnicode, "✓") {
			t.Fatalf("sampleNewFormatUnicode lost its '✓' (U+2713); fixture drift")
		}
	})
	t.Run("NewFormat_HeavyUnicodeCheckmark", func(t *testing.T) {
		if !parseGhAuthStatus(sampleNewFormatHeavyUnicode) {
			t.Errorf("2.40+ heavy unicode '✔' (U+2714) must parse positive:\n%s",
				sampleNewFormatHeavyUnicode)
		}
	})
	t.Run("NewFormat_ASCIIBracket", func(t *testing.T) {
		if !parseGhAuthStatus(sampleNewFormatASCIICheckmark) {
			t.Errorf("[OK] bracket marker must parse positive:\n%s",
				sampleNewFormatASCIICheckmark)
		}
	})
	t.Run("MultiHost_PartialFailure_GitHubOK", func(t *testing.T) {
		if !parseGhAuthStatus(sampleMultiHostPartialOK) {
			t.Errorf("multi-host partial-failure with github.com OK must parse positive:\n%s",
				sampleMultiHostPartialOK)
		}
	})

	// Negative cases — each must NOT parse positive.
	t.Run("NotLoggedIn_Pre240", func(t *testing.T) {
		if parseGhAuthStatus(sampleNotLoggedInPre240) {
			t.Errorf("pre-2.40 not-logged-in must parse negative:\n%s",
				sampleNotLoggedInPre240)
		}
	})
	t.Run("NotLoggedIn_240_GitHubHost", func(t *testing.T) {
		if parseGhAuthStatus(sampleNotLoggedIn240) {
			t.Errorf("2.40+ github.com not-logged-in must parse negative:\n%s",
				sampleNotLoggedIn240)
		}
	})
	t.Run("GitHubFailedLogin", func(t *testing.T) {
		if parseGhAuthStatus(sampleGithubFailed) {
			t.Errorf("github.com token-rejected must parse negative:\n%s",
				sampleGithubFailed)
		}
	})
	t.Run("NonGithubHost_GitLab", func(t *testing.T) {
		if parseGhAuthStatus(sampleNonGithubHost) {
			t.Errorf("non-github host must parse negative:\n%s",
				sampleNonGithubHost)
		}
	})
	t.Run("EmptyOutput", func(t *testing.T) {
		if parseGhAuthStatus("") {
			t.Errorf("empty output must parse negative")
		}
	})
	t.Run("WhitespaceOnly", func(t *testing.T) {
		if parseGhAuthStatus("   \n\t\n") {
			t.Errorf("whitespace-only output must parse negative")
		}
	})

	// Case insensitivity — future-proofs gh whose message casing may
	// shift across locales or versions.
	t.Run("CaseInsensitive_LowercaseVerb", func(t *testing.T) {
		lower := strings.ToLower(sampleOldFormat)
		if !parseGhAuthStatus(lower) {
			t.Errorf("lowercase verb must still parse positive:\n%s", lower)
		}
	})
	t.Run("CaseInsensitive_AllUpper", func(t *testing.T) {
		upper := strings.ToUpper(sampleOldFormat)
		if !parseGhAuthStatus(upper) {
			t.Errorf("uppercase verb must still parse positive:\n%s", upper)
		}
	})
}

// TestParseGhAuthStatus_NoFalsePositiveOnEmptyHostName:
// ensure that an empty host name with a marker still does not match
// when the host segment is missing.
func TestParseGhAuthStatus_NoFalsePositiveOnEmptyHost(t *testing.T) {
	for _, in := range []string{
		"✓ logged in to \n",       // empty host
		"✓ logged in to ()\n",      // empty host with parens
		"✓ logged into github\n",   // "logged into" — not "logged in to"
		"  logged in to  github.com  \n", // double space — ok, must match
	} {
		got := parseGhAuthStatus(in)
		want := strings.Contains(in, "  github.com  ") // only the last fixture should match
		if got != want {
			t.Errorf("parseGhAuthStatus(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestDetectRepo_HelperInvariants: pin the explicit regex policy so a
// future contributor cannot accidentally weaken the negative-side
// guard. Each invariant below is a guarantee, not a side-effect.
func TestDetectRepo_HelperInvariants(t *testing.T) {
	// 1. The fixture-real (collected today) output must parse positive.
	currentReal := "github.com\n" +
		"  ✓ Logged in to github.com account Delqhi (keyring)\n" +
		"  - Active account: true\n" +
		"  - Git operations protocol: https\n"
	if !parseGhAuthStatus(currentReal) {
		t.Errorf("real-world gh auth status output must parse positive — "+
			"regex regressed. Output was:\n%s", currentReal)
	}
	// 2. Expected behaviour on stale 2024-era fixture (kept verbatim
	//    so this test breaks if the regex over-shrinks in the future).
	if !parseGhAuthStatus(sampleOldFormat) {
		t.Errorf("2024-era ASCII format must still parse positive — "+
			"fixture or regex regressed:\n%s", sampleOldFormat)
	}
}

// TestDetectRepo_EndToEnd_MockedBridge exercises detectRepo with the
// newGhBridgeForDetect hook so the entire flow (LookPath → auth probe
// → repo pick) is verified without requiring `gh` to be installed,
// authenticated, or pointed at a real repo.
//
// The runner approves `auth status` (returning the unicode-checkmark
// fixture) and `repo view` (returning a JSON name); detectRepo must
// return the parsed repo name.
func TestDetectRepo_EndToEnd_MockedBridge(t *testing.T) {
	saved := newGhBridgeForDetect
	t.Cleanup(func() { newGhBridgeForDetect = saved })

	called := []string{}
	runner := ghbridge.Runner(func(_ context.Context, args []string) (string, string, error) {
		called = append(called, strings.Join(args, " "))
		switch {
		case len(args) >= 1 && args[0] == "auth":
			return sampleNewFormatUnicode, "", nil
		case len(args) >= 1 && args[0] == "repo":
			return `{"nameWithOwner":"OpenSIN-Code/SIN-Code"}`, "", nil
		}
		return "", "", errors.New("unexpected args: " + strings.Join(args, " "))
	})
	newGhBridgeForDetect = func() *ghbridge.Bridge {
		return ghbridge.NewWithRunner(runner, time.Second)
	}

	repo, err := detectRepo()
	if err != nil {
		t.Fatalf("detectRepo with mocked OK auth returned err: %v", err)
	}
	if repo != "OpenSIN-Code/SIN-Code" {
		t.Errorf("detectRepo repo = %q, want %q", repo, "OpenSIN-Code/SIN-Code")
	}
	// The auth probe MUST have run before the repo lookup.
	if len(called) < 2 {
		t.Fatalf("expected ≥2 gh calls (auth + repo), got %v", called)
	}
	if called[0] != "auth status" {
		t.Errorf("first call = %q, want %q (auth probe must run first)", called[0], "auth status")
	}
	if called[1] != "repo view --json nameWithOwner" {
		t.Errorf("second call = %q, want repo view --json", called[1])
	}
}

// TestDetectRepo_EndToEnd_MockedBridge_AuthNotLoggedIn verifies the
// failure path: when `gh auth status` succeeds but the output does
// not confirm github.com auth, detectRepo must error WITHOUT
// attempting the repo lookup. This is the regression guard for the
// PR-#418 follow-up — a too-permissive regex would let an
// unauthenticated user through and downstream code would fail
// cryptically when `gh repo view` doesn't have a host.
func TestDetectRepo_EndToEnd_MockedBridge_AuthNotLoggedIn(t *testing.T) {
	saved := newGhBridgeForDetect
	t.Cleanup(func() { newGhBridgeForDetect = saved })

	called := []string{}
	runner := ghbridge.Runner(func(_ context.Context, args []string) (string, string, error) {
		called = append(called, strings.Join(args, " "))
		// Return a non-authenticated stdout. Exit 0 because classify
		// already returns the runner output verbatim; we are testing
		// the parsing step here, not the gh exit semantics.
		return sampleNotLoggedInPre240, "", nil
	})
	newGhBridgeForDetect = func() *ghbridge.Bridge {
		return ghbridge.NewWithRunner(runner, time.Second)
	}

	_, err := detectRepo()
	if err == nil {
		t.Fatalf("detectRepo with non-auth output returned nil err; want 'gh not authenticated'")
	}
	if !strings.Contains(err.Error(), "gh not authenticated") {
		t.Errorf("detectRepo err = %q, want substring 'gh not authenticated'", err.Error())
	}
	// Critical: the repo lookup MUST NOT have run. This is the
	// regression guard for "false-positive auth → confusing downstream
	// error from gh repo view".
	if len(called) != 1 || called[0] != "auth status" {
		t.Errorf("only auth probe should run on failure, got calls: %v", called)
	}
}

// TestDetectRepo_EndToEnd_MockedBridge_GhExecError verifies the
// runner-error path: when `gh auth status` exits non-zero (any
// classifier-passing error, e.g. timeout), detectRepo wraps the error
// and surfaces an actionable message.
func TestDetectRepo_EndToEnd_MockedBridge_GhExecError(t *testing.T) {
	saved := newGhBridgeForDetect
	t.Cleanup(func() { newGhBridgeForDetect = saved })

	runner := ghbridge.Runner(func(_ context.Context, _ []string) (string, string, error) {
		return "", "You are not logged into any GitHub hosts. Run gh auth login to authenticate.",
			errors.New("exit status 1")
	})
	newGhBridgeForDetect = func() *ghbridge.Bridge {
		return ghbridge.NewWithRunner(runner, time.Second)
	}

	_, err := detectRepo()
	if err == nil {
		t.Fatalf("detectRepo with gh-exec-error returned nil err")
	}
	if !strings.Contains(err.Error(), "gh not authenticated") {
		t.Errorf("detectRepo err = %q, want substring 'gh not authenticated'", err.Error())
	}
	if !strings.Contains(err.Error(), "gh auth login") {
		t.Errorf("detectRepo err = %q, want substring 'gh auth login' (actionable hint)", err.Error())
	}
}
