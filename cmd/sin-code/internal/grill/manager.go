// SPDX-License-Identifier: MIT
// Purpose: session manager — create, mutate, persist, and render
// grilling sessions. The Session is the unit of state; the manager
// owns the on-disk representation.
package grill

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"
)

// hook variables make filesystem and JSON error paths testable
// without changing public behavior.
var (
	osMkdirAllHook    = os.MkdirAll
	osWriteFileHook   = os.WriteFile
	osReadFileHook    = os.ReadFile
	osRenameHook      = os.Rename
	jsonMarshalHook   = json.MarshalIndent
	jsonUnmarshalHook = json.Unmarshal
)

// Manager is a process-wide registry of grilling sessions. It is
// safe for concurrent use (the operator may run `grill next` and
// `grill answer` in parallel from a TUI shell, for example).
type Manager struct {
	mu       sync.Mutex
	dir      string              // JSON file directory (e.g. ~/.local/share/sin-code/grill)
	sessions map[string]*Session // keyed by ID
}

// NewManager opens (or creates) the grill directory. Sessions are
// loaded lazily on first access; the manager is cheap to construct.
func NewManager(dir string) (*Manager, error) {
	if err := osMkdirAllHook(dir, 0o755); err != nil {
		return nil, fmt.Errorf("grill: create dir: %w", err)
	}
	return &Manager{dir: dir, sessions: map[string]*Session{}}, nil
}

// Dir returns the manager's directory. Used by the CLI to print
// "sessions stored at <dir>" on startup.
func (m *Manager) Dir() string { return m.dir }

// Start creates a new session for the given topic. The first
// question is auto-generated from the catalog (round-robin based
// on a hash of the topic so two operators get different openings).
func (m *Manager) Start(topic string) (*Session, error) {
	if topic == "" {
		return nil, fmt.Errorf("grill: topic is required")
	}
	now := time.Now().UTC()
	id := newSessionID(topic, now)
	s := &Session{
		ID:        id,
		Topic:     topic,
		StartedAt: now,
		UpdatedAt: now,
		Decisions: []Decision{},
	}
	// Seed the first decision (the root question).
	q := openingQuestion(topic)
	s.Decisions = append(s.Decisions, Decision{
		ID:       "d0",
		Question: q,
		Status:   "open",
		AskedAt:  now,
	})
	s.recomputeOpen()
	if err := m.save(s); err != nil {
		return nil, err
	}
	// Cache the session in memory so subsequent Get/Next/Answer
	// calls don't round-trip through disk. The load-on-miss path
	// in Get is a fallback for managers that crash and restart.
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s, nil
}

// Get returns the session with the given id, loading from disk if
// it is not in memory.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.Lock()
	if s, ok := m.sessions[id]; ok {
		m.mu.Unlock()
		return s, nil
	}
	m.mu.Unlock()
	s, err := m.load(id)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.sessions[id] = s
	m.mu.Unlock()
	return s, nil
}

// Next asks the next adversarial question. It picks an open
// decision (in order), then appends a new sub-question from the
// catalog. Returns the new decision and the parent id.
func (m *Manager) Next(id string) (decision Decision, parentID string, err error) {
	s, err := m.Get(id)
	if err != nil {
		return Decision{}, "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// Find the first open decision. There is always at least one
	// (the seed) until the operator resolves every decision.
	for _, d := range s.Decisions {
		if d.Status == "open" {
			// Append a sub-question from the catalog.
			q := catalogQuestion(s.Topic, len(s.Decisions))
			child := Decision{
				ID:       fmt.Sprintf("d%d", len(s.Decisions)),
				ParentID: d.ID,
				Question: q,
				Status:   "open",
				AskedAt:  time.Now().UTC(),
			}
			s.Decisions = append(s.Decisions, child)
			s.UpdatedAt = child.AskedAt
			s.recomputeOpen()
			if err := m.save(s); err != nil {
				return Decision{}, "", err
			}
			return child, d.ID, nil
		}
	}
	return Decision{}, "", fmt.Errorf("grill: no open decisions in session %s", id)
}

// Answer records the operator's response to a decision. The
// decision's status moves to "answered" (or "resolved" if the
// operator said "done").
func (m *Manager) Answer(id, decisionID, answer string) error {
	s, err := m.Get(id)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range s.Decisions {
		if s.Decisions[i].ID == decisionID {
			s.Decisions[i].Answer = answer
			now := time.Now().UTC()
			if answer == "done" || answer == "skip" {
				s.Decisions[i].Status = "resolved"
				s.Decisions[i].ResolvedAt = now
			} else {
				s.Decisions[i].Status = "answered"
			}
			s.UpdatedAt = now
			s.recomputeOpen()
			return m.save(s)
		}
	}
	return fmt.Errorf("grill: decision %s not found in session %s", decisionID, id)
}

// Status returns a summary of the session: how many decisions, how
// many open, how many resolved.
func (m *Manager) Status(id string) (resolved, open, answered, deferred int, err error) {
	s, err := m.Get(id)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, d := range s.Decisions {
		switch d.Status {
		case "resolved":
			resolved++
		case "answered":
			answered++
		case "deferred":
			deferred++
		default:
			open++
		}
	}
	return
}

// Synthesize produces a structured summary of the session: the
// resolved decisions, the open questions, and the assumptions
// surfaced during the interview. The CLI emits this in both human
// text and JSON.
func (m *Manager) Synthesize(id string) (Synthesize, error) {
	s, err := m.Get(id)
	if err != nil {
		return Synthesize{}, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := Synthesize{
		Resolved:    []string{},
		Open:        []string{},
		Assumptions: []string{},
	}
	for _, d := range s.Decisions {
		switch d.Status {
		case "resolved":
			out.Resolved = append(out.Resolved, d.Question+" — "+d.Answer)
		case "answered", "open":
			out.Open = append(out.Open, d.Question)
		}
		// Heuristic: an "answered" decision with "I assume" or
		// "probably" in the answer is flagged as an assumption.
		if d.Status == "answered" {
			low := d.Answer
			if contains(low, "assume") || contains(low, "probably") || contains(low, "guess") {
				out.Assumptions = append(out.Assumptions, d.Question+" (assumed: "+d.Answer+")")
			}
		}
	}
	return out, nil
}

// recomputeOpen rebuilds the OpenQuestions count from the
// decisions. Called on every mutation.
func (s *Session) recomputeOpen() {
	n := 0
	for _, d := range s.Decisions {
		if d.Status == "open" {
			n++
		}
	}
	s.OpenQuestions = n
}

// save writes the session to its JSON file. Atomic write: temp +
// rename, so a crash mid-write never leaves a half-written file.
func (m *Manager) save(s *Session) error {
	path := m.path(s.ID)
	b, err := jsonMarshalHook(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := osWriteFileHook(tmp, b, filemode.Default()); err != nil {
		return err
	}
	return osRenameHook(tmp, path)
}

func (m *Manager) load(id string) (*Session, error) {
	path := m.path(id)
	b, err := osReadFileHook(path)
	if err != nil {
		return nil, fmt.Errorf("grill: load %s: %w", id, err)
	}
	var s Session
	if err := jsonUnmarshalHook(b, &s); err != nil {
		return nil, fmt.Errorf("grill: parse %s: %w", id, err)
	}
	// Sort decisions by ID for stable rendering. The IDs are
	// "d0", "d1", ... so lexical == chronological.
	sort.SliceStable(s.Decisions, func(i, j int) bool {
		return s.Decisions[i].ID < s.Decisions[j].ID
	})
	return &s, nil
}

func (m *Manager) path(id string) string {
	return filepath.Join(m.dir, id+".json")
}

func contains(s, sub string) bool {
	if sub == "" {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// openingQuestion returns the seed question for a new session. The
// question is templated on the topic so the operator can see the
// interview is grounded in their plan, not generic boilerplate.
func openingQuestion(topic string) string {
	return "What is the core decision you want this grilling to resolve about: " + topic + "?"
}

// catalogQuestion returns a question from the catalog, picked by
// index. The index is hash-derived so the same topic yields the
// same opening question across sessions, but different topics open
// with different questions. Round-robin from there.
func catalogQuestion(topic string, decisionIdx int) string {
	n := len(Catalog)
	if n == 0 {
		return "What is the assumption behind this step?"
	}
	// Hash the topic to seed the round-robin.
	seed := 0
	for _, c := range topic {
		seed = seed*131 + int(c)
	}
	if seed < 0 {
		seed = -seed
	}
	idx := (seed + decisionIdx) % n
	p := Catalog[idx]
	if len(p.Questions) == 0 {
		return p.Description
	}
	q := p.Questions[decisionIdx%len(p.Questions)]
	return "[" + p.Name + "] " + q
}
