// SPDX-License-Identifier: MIT
// Purpose: high-level API for the rest of SIN-Code. Owns the project
// detection, store, and (optional) memory mirror. The agent loop and
// the learning subsystem talk to this — never to the Store directly.
// Docs: manager.doc.md
package instinct

import (
	"context"
	"time"
)

// MemorySink is the optional bridge into SIN-Code's existing memory
// subsystem. nil is fine — the manager just skips the mirror.
type MemorySink interface {
	RecordInstinct(ctx context.Context, trigger, action, domain string, confidence float64) error
}

// Manager is the high-level entry point.
type Manager struct {
	store   *Store
	project Project
	sink    MemorySink
}

// NewManager detects the current project, applies tuning config, and
// prepares the store.
func NewManager(workdir string, sink MemorySink) (*Manager, error) {
	ApplyConfig(LoadConfig())
	store := NewStore("")
	proj := DetectProject(workdir)
	if err := store.SaveProjectMeta(proj); err != nil {
		return nil, err
	}
	return &Manager{store: store, project: proj, sink: sink}, nil
}

// NewManagerWithStore is for tests and advanced wiring.
func NewManagerWithStore(store *Store, project Project, sink MemorySink) *Manager {
	return &Manager{store: store, project: project, sink: sink}
}

func (m *Manager) Project() Project   { return m.project }
func (m *Manager) Store() *Store      { return m.store }

// Active returns instincts that should influence behavior right now
// (this project + global), strongest first, active-only.
func (m *Manager) Active() ([]*Instinct, error) {
	all, err := m.store.LoadEffective(m.project.ID)
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, i := range all {
		if i.IsActive() {
			out = append(out, i)
		}
	}
	return out, nil
}

// Observe folds a candidate into the store: reinforces a matching
// instinct or creates a new pending one. Returns true if a new
// instinct was created.
func (m *Manager) Observe(c Candidate) (bool, error) {
	existing, err := m.store.LoadProject(m.project.ID)
	if err != nil {
		return false, err
	}
	probe := NewInstinct(c.Trigger, c.Domain, c.Action, "session-observation", ScopeProject)
	for _, e := range existing {
		if e.SignatureKey() == probe.SignatureKey() {
			e.Reinforce()
			if len(e.Evidence) < 8 {
				e.Evidence = append(e.Evidence, c.Evidence...)
			}
			if err := m.store.Save(e); err != nil {
				return false, err
			}
			m.store.Append(AuditEvent{InstinctID: e.ID, Kind: "reinforced", Confidence: e.Confidence, Detail: c.Action})
			m.mirror(e)
			return false, nil
		}
	}
	probe.ProjectID = m.project.ID
	probe.ProjectName = m.project.Name
	probe.SourceRepo = m.project.Remote
	probe.Evidence = c.Evidence
	if err := m.store.Save(probe); err != nil {
		return false, err
	}
	m.store.Append(AuditEvent{InstinctID: probe.ID, Kind: "created", Confidence: probe.Confidence, Detail: c.Action})
	m.mirror(probe)
	return true, nil
}

// Contradict records a conflicting signal against a matching instinct
// (e.g. an action that was later reverted). No-op if none matches.
func (m *Manager) Contradict(c Candidate) error {
	existing, err := m.store.LoadProject(m.project.ID)
	if err != nil {
		return err
	}
	probe := NewInstinct(c.Trigger, c.Domain, c.Action, "contradiction", ScopeProject)
	for _, e := range existing {
		if e.SignatureKey() == probe.SignatureKey() {
			e.Contradict()
			if err := m.store.Save(e); err != nil {
				return err
			}
			m.store.Append(AuditEvent{InstinctID: e.ID, Kind: "contradicted", Confidence: e.Confidence})
			return nil
		}
	}
	return nil
}

// EvolveAll returns evolution proposals across the effective set.
func (m *Manager) EvolveAll() ([]Proposal, error) {
	all, err := m.store.LoadEffective(m.project.ID)
	if err != nil {
		return nil, err
	}
	return Evolve(all), nil
}

// Prune deletes pending instincts past their TTL and decays the rest.
func (m *Manager) Prune(ttlDays int) (deleted int, err error) {
	if ttlDays <= 0 {
		ttlDays = 30
	}
	all, err := m.store.LoadAll()
	if err != nil {
		return 0, err
	}
	cutoff := time.Now().UTC().Add(-time.Duration(ttlDays) * 24 * time.Hour)
	for _, i := range all {
		idleDays := time.Since(i.UpdatedAt).Hours() / 24
		if i.Status == StatusPending && i.UpdatedAt.Before(cutoff) {
			if err := m.store.Delete(i); err != nil {
				return deleted, err
			}
			m.store.Append(AuditEvent{InstinctID: i.ID, Kind: "pruned", Detail: "ttl"})
			deleted++
			continue
		}
		i.Decay(idleDays)
		if err := m.store.Save(i); err != nil {
			return deleted, err
		}
	}
	return deleted, nil
}

func (m *Manager) mirror(i *Instinct) {
	if m.sink == nil {
		return
	}
	_ = m.sink.RecordInstinct(context.Background(), i.Trigger, i.Action, i.Domain, i.Confidence)
}
