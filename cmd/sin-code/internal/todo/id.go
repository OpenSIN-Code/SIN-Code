package todo

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

var (
	idMu          sync.Mutex
	seenIDs       = make(map[string]struct{})
	idSalt        uint64
	idNowUnixNano = func() int64 { return time.Now().UnixNano() }
	idSha256Sum   = sha256.Sum256
)

const idPrefix = "st-"
const idBodyLen = 4
const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func init() {
	// UnixNano is non-negative until the year 2262 and fits safely into uint64.
	idSalt = uint64(time.Now().UnixNano()) // #nosec G115
}

func encodeBase36(n uint64, w int) string {
	if n == 0 {
		return string(idAlphabet[0:1])
	}
	b := make([]byte, 0, w)
	for n > 0 && len(b) < w {
		b = append([]byte{idAlphabet[n%36]}, b...)
		n /= 36
	}
	for len(b) < w {
		b = append([]byte{idAlphabet[0]}, b...)
	}
	return string(b)
}

func GenerateID() string {
	idMu.Lock()
	defer idMu.Unlock()
	for i := 0; i < 32; i++ {
		h := idSha256Sum([]byte(fmt.Sprintf("%d-%d-%d", idNowUnixNano(), idSalt, i)))
		body := encodeBase36(uint64(h[0])<<24|uint64(h[1])<<16|uint64(h[2])<<8|uint64(h[3]), idBodyLen)
		id := idPrefix + body
		if _, exists := seenIDs[id]; !exists {
			seenIDs[id] = struct{}{}
			return id
		}
	}
	return fmt.Sprintf("%s%s", idPrefix, encodeBase36(uint64(idNowUnixNano()), 8))
}

func IsValidID(id string) bool {
	if len(id) != len(idPrefix)+idBodyLen {
		return false
	}
	if id[:len(idPrefix)] != idPrefix {
		return false
	}
	body := id[len(idPrefix):]
	for _, c := range body {
		found := false
		for _, a := range idAlphabet {
			if byte(a) == byte(c) { // #nosec G115
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func resetIDState() {
	idMu.Lock()
	defer idMu.Unlock()
	seenIDs = make(map[string]struct{})
	idSalt = uint64(time.Now().UnixNano())
}
