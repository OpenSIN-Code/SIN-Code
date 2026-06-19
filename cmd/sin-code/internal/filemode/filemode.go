// Package filemode centralises the file-mode policy for write paths that
// default to world-readable (0o644). Honours SIN_CODE_FILE_MODE env var
// (validated octal string) with fallback 0o644.
package filemode

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Default is the file mode used by callers that previously hard-coded 0o644.
// Reads SIN_CODE_FILE_MODE on every call so an operator can flip the knob
// without restarting the binary.
func Default() os.FileMode {
	mode, err := Resolve(os.Getenv("SIN_CODE_FILE_MODE"), 0o644)
	if err != nil || mode == 0 {
		return 0o644
	}
	return mode
}

// Resolve parses envValue as an octal string (with optional "0o" / "0O"
// prefix) and returns the corresponding os.FileMode. When envValue is
// empty, fallback is returned. The result is rejected when any bit grants
// write access to group or other — that would relax, not tighten, the
// 0o644 baseline the security knob is meant to dial down.
func Resolve(envValue string, fallback os.FileMode) (os.FileMode, error) {
	envValue = strings.TrimSpace(envValue)
	if envValue == "" {
		if fallback == 0 {
			return 0, fmt.Errorf("filemode: empty env value and zero fallback")
		}
		if err := validate(fallback); err != nil {
			return 0, err
		}
		return fallback, nil
	}

	raw, err := parseOctal(envValue)
	if err != nil {
		return 0, fmt.Errorf("filemode: invalid SIN_CODE_FILE_MODE %q: %w", envValue, err)
	}
	if raw > 0o7777 {
		return 0, fmt.Errorf("filemode: SIN_CODE_FILE_MODE %q out of range (max 0o7777)", envValue)
	}
	mode := os.FileMode(raw)
	if err := validate(mode); err != nil {
		return 0, err
	}
	return mode, nil
}

func parseOctal(s string) (uint64, error) {
	stripped := strings.TrimPrefix(s, "0o")
	stripped = strings.TrimPrefix(stripped, "0O")
	if stripped == s {
		// require a valid digit so empty / signed strings fall through to ParseUint
		if len(stripped) == 0 || stripped[0] < '0' || stripped[0] > '7' {
			return 0, fmt.Errorf("not an octal digit")
		}
	}
	return strconv.ParseUint(stripped, 8, 32)
}

// validate refuses modes whose group or other bits grant write access —
// the 0o644 baseline must not be loosened by the knob.
func validate(mode os.FileMode) error {
	if mode&0o022 != 0 {
		return fmt.Errorf("filemode: refusing non-tight mode 0o%o (group/other must not have write)", uint32(mode))
	}
	return nil
}
