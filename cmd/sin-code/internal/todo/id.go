package todo

import (
	"crypto/sha1"
	"fmt"
	"sync"
	"time"
)

var (
	idMu     sync.Mutex
	seenIDs  = make(map[string]struct{})
	idSalt   uint64
)

const idPrefix = "st-"
const idBodyLen = 4
const idAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

func init() {
	idSalt = uint64(time.Now().UnixNano())
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
		h := sha1.Sum([]byte(fmt.Sprintf("%d-%d-%d", time.Now().UnixNano(), idSalt, i)))
		body := encodeBase36(uint64(h[0])<<24|uint64(h[1])<<16|uint64(h[2])<<8|uint64(h[3]), idBodyLen)
		id := idPrefix + body
		if _, exists := seenIDs[id]; !exists {
			seenIDs[id] = struct{}{}
			return id
		}
	}
	return fmt.Sprintf("%s%s", idPrefix, encodeBase36(uint64(time.Now().UnixNano()), 8))
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
			if byte(a) == byte(c) {
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
