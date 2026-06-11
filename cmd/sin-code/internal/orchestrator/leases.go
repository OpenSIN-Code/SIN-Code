// SPDX-License-Identifier: MIT
// Purpose: Lease Coordinator — collision-free multi-agent work via path
// leases with intent broadcast. Acquisition is all-or-nothing (deadlock
// impossible). Conflicts carry the holder's intent so the requesting
// agent can decide: wait, renegotiate, or work elsewhere.
package orchestrator

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type Lease struct {
	ID        int64
	AgentID   string
	TaskID    string
	Globs     []string
	Intent    string
	ExpiresAt time.Time
}

type LeaseTable struct {
	mu         sync.Mutex
	nextID     int64
	leases     map[int64]*Lease
	DefaultTTL time.Duration
}

func NewLeaseTable() *LeaseTable {
	return &LeaseTable{leases: map[int64]*Lease{}, DefaultTTL: 15 * time.Minute}
}

type Conflict struct {
	HeldBy  string
	TaskID  string
	Intent  string
	Overlap string
}

func (lt *LeaseTable) Acquire(agentID, taskID, intent string, globs []string) (*Lease, []Conflict, error) {
	if len(globs) == 0 {
		return nil, nil, fmt.Errorf("lease: empty glob set")
	}
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.expireLocked()

	var conflicts []Conflict
	for _, held := range lt.leases {
		if held.AgentID == agentID {
			continue
		}
		for _, hg := range held.Globs {
			for _, rg := range globs {
				if globsOverlap(hg, rg) {
					conflicts = append(conflicts, Conflict{
						HeldBy: held.AgentID, TaskID: held.TaskID,
						Intent: held.Intent, Overlap: hg + " ~ " + rg,
					})
				}
			}
		}
	}
	if len(conflicts) > 0 {
		return nil, conflicts, nil
	}

	lt.nextID++
	l := &Lease{
		ID: lt.nextID, AgentID: agentID, TaskID: taskID,
		Globs: normalizeGlobs(globs), Intent: intent,
		ExpiresAt: timeNow().Add(lt.DefaultTTL),
	}
	lt.leases[l.ID] = l
	return l, nil, nil
}

func (lt *LeaseTable) Renew(id int64, agentID string) error {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	l, ok := lt.leases[id]
	if !ok || l.AgentID != agentID {
		return fmt.Errorf("lease %d: not held by %s", id, agentID)
	}
	l.ExpiresAt = timeNow().Add(lt.DefaultTTL)
	return nil
}

func (lt *LeaseTable) Release(id int64, agentID string) error {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	l, ok := lt.leases[id]
	if !ok {
		return nil
	}
	if l.AgentID != agentID {
		return fmt.Errorf("lease %d: held by %s, not %s", id, l.AgentID, agentID)
	}
	delete(lt.leases, id)
	return nil
}

func (lt *LeaseTable) Board() string {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	lt.expireLocked()

	if len(lt.leases) == 0 {
		return ""
	}
	ids := make([]int64, 0, len(lt.leases))
	for id := range lt.leases {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	var b strings.Builder
	b.WriteString("## Active work board (other agents)\n")
	for _, id := range ids {
		l := lt.leases[id]
		fmt.Fprintf(&b, "- %s [task %s] holds %v — %s\n", l.AgentID, l.TaskID, l.Globs, l.Intent)
	}
	b.WriteString("Do not plan edits inside paths held by other agents; coordinate via lease acquisition.\n")
	return b.String()
}

func (lt *LeaseTable) Count() int {
	lt.mu.Lock()
	defer lt.mu.Unlock()
	return len(lt.leases)
}

func (lt *LeaseTable) expireLocked() {
	now := timeNow()
	for id, l := range lt.leases {
		if now.After(l.ExpiresAt) {
			delete(lt.leases, id)
		}
	}
}

func globsOverlap(a, b string) bool {
	if a == b {
		return true
	}
	ap, bp := strings.TrimSuffix(a, "/*"), strings.TrimSuffix(b, "/*")
	if ap != a && (strings.HasPrefix(bp, ap+"/") || bp == ap) {
		return true
	}
	if bp != b && (strings.HasPrefix(ap, bp+"/") || ap == bp) {
		return true
	}
	if m, _ := filepath.Match(a, b); m {
		return true
	}
	if m, _ := filepath.Match(b, a); m {
		return true
	}
	return false
}

func normalizeGlobs(gs []string) []string {
	out := make([]string, len(gs))
	copy(out, gs)
	sort.Strings(out)
	return out
}
