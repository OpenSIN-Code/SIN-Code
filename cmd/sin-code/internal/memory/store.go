// SPDX-License-Identifier: MIT
// Purpose: bbolt-backed Memory store with embeddings, links, and
// append-only audit log. Embeddings are cached in-memory by text hash.
package memory

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	bucketMems       = "memories"
	bucketLinks      = "links"
	bucketEmbeddings = "embeddings"
	bucketAudit      = "audit"
	bucketMeta       = "meta"

	ttlDefault = 365 * 24 * time.Hour
)

var (
	ErrNotFound = errors.New("memory: not found")
)

type Store struct {
	mu   sync.RWMutex
	db   *bolt.DB
	path string

	embedCache sync.Map
}

func Open(path string) (*Store, error) {
	if path == "" {
		dir, err := defaultDir()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(dir, "memory.db")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o644, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open memory db: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, b := range []string{bucketMems, bucketLinks, bucketEmbeddings, bucketAudit, bucketMeta} {
			if _, err := tx.CreateBucketIfNotExists([]byte(b)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db, path: path}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) Path() string { return s.path }

func defaultDir() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg, "sin-code"), nil
}

func memKey(id string) []byte { return []byte(id) }

func linkKey(from, to string) []byte {
	return []byte(from + "\x00" + to)
}

func embKey(textHash string) []byte { return []byte(textHash) }

func textHash(text string) string {
	h := sha256.Sum256([]byte(text))
	return hex.EncodeToString(h[:8])
}

func encodeEmbedding(v []float32) []byte {
	buf := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(f))
	}
	return buf
}

func decodeEmbedding(b []byte) []float32 {
	if len(b) == 0 || len(b)%4 != 0 {
		return nil
	}
	v := make([]float32, len(b)/4)
	for i := range v {
		v[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[i*4:]))
	}
	return v
}

func (s *Store) Add(m *Memory) error {
	if m == nil {
		return fmt.Errorf("nil memory")
	}
	if strings.TrimSpace(m.Insight) == "" {
		return fmt.Errorf("insight required")
	}
	if m.ID == "" {
		m.ID = GenerateID(m.Insight)
	}
	now := time.Now().UTC()
	if m.Created.IsZero() {
		m.Created = now
	}
	m.Updated = now
	m.Tags = NormalizeTags(m.Tags)
	if len(m.Embedding) == 0 {
		emb, err := s.computeEmbedding(m.Insight)
		if err == nil {
			m.Embedding = emb
		}
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(m)
		if err != nil {
			return err
		}
		if err := tx.Bucket([]byte(bucketMems)).Put(memKey(m.ID), raw); err != nil {
			return err
		}
		if len(m.Embedding) > 0 {
			emb := encodeEmbedding(m.Embedding)
			hash := textHash(m.Insight)
			if err := tx.Bucket([]byte(bucketEmbeddings)).Put(embKey(hash), emb); err != nil {
				return err
			}
		}
		return s.appendAudit(tx, m.ID, "create", "", m.Insight)
	})
}

func (s *Store) Get(id string) (*Memory, error) {
	var m *Memory
	err := s.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket([]byte(bucketMems)).Get(memKey(id))
		if raw == nil {
			return ErrNotFound
		}
		var x Memory
		if err := json.Unmarshal(raw, &x); err != nil {
			return err
		}
		x.Embedding = s.loadEmbedding(tx, x.Insight)
		m = &x
		return nil
	})
	return m, err
}

func (s *Store) loadEmbedding(tx *bolt.Tx, insight string) []float32 {
	hash := textHash(insight)
	raw := tx.Bucket([]byte(bucketEmbeddings)).Get(embKey(hash))
	if raw == nil {
		return nil
	}
	return decodeEmbedding(raw)
}

func (s *Store) computeEmbedding(text string) ([]float32, error) {
	hash := textHash(text)
	if cached, ok := s.embedCache.Load(hash); ok {
		return cached.([]float32), nil
	}
	fn, _ := GetEmbedder()
	if fn == nil {
		return nil, nil
	}
	v, err := fn(text)
	if err != nil {
		return nil, err
	}
	if len(v) > 0 {
		s.embedCache.Store(hash, v)
	}
	return v, nil
}

type ListFilter struct {
	Project  string
	Tag      string
	TagsAny  []string
	TagsAll  []string
	Actor    string
	Search   string
	Limit    int
}

func (s *Store) List(f ListFilter) ([]*Memory, error) {
	var all []*Memory
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketMems)).ForEach(func(_, v []byte) error {
			var m Memory
			if err := json.Unmarshal(v, &m); err != nil {
				return nil
			}
			m.Embedding = s.loadEmbedding(tx, m.Insight)
			all = append(all, &m)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	out := all[:0]
	for _, m := range all {
		if f.Project != "" && m.Project != f.Project {
			continue
		}
		if f.Tag != "" {
			found := false
			for _, t := range m.Tags {
				if t == f.Tag {
					found = true
					break
				}
			}
			if !found {
				continue
			}
		}
		if len(f.TagsAll) > 0 {
			must := map[string]bool{}
			for _, t := range m.Tags {
				must[t] = true
			}
			ok := true
			for _, t := range f.TagsAll {
				if !must[t] {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}
		}
		if len(f.TagsAny) > 0 {
			anyFound := false
			for _, t := range f.TagsAny {
				for _, mt := range m.Tags {
					if mt == t {
						anyFound = true
						break
					}
				}
				if anyFound {
					break
				}
			}
			if !anyFound {
				continue
			}
		}
		if f.Actor != "" && m.Actor != f.Actor {
			continue
		}
		if f.Search != "" {
			needle := strings.ToLower(f.Search)
			if !strings.Contains(strings.ToLower(m.Insight), needle) {
				continue
			}
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Updated.After(out[j].Updated) })
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func (s *Store) Delete(id string, hard bool) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMems))
		raw := b.Get(memKey(id))
		if raw == nil {
			return ErrNotFound
		}
		if !hard {
			var m Memory
			if err := json.Unmarshal(raw, &m); err != nil {
				return err
			}
			m.Insight = "[forgotten] " + m.Insight
			m.Updated = time.Now().UTC()
			buf, _ := json.Marshal(&m)
			_ = b.Put(memKey(id), buf)
			return s.appendAudit(tx, id, "forget", "", m.Insight)
		}
		_ = b.Delete(memKey(id))
		_ = s.appendAudit(tx, id, "delete", "", "")
		c := tx.Bucket([]byte(bucketLinks)).Cursor()
		for k, _ := c.First(); k != nil; {
			if strings.HasPrefix(string(k), id+"\x00") {
				_ = c.Delete()
			}
			k, _ = c.Next()
		}
		return nil
	})
}

func (s *Store) AddLink(l Link) error {
	if l.From == "" || l.To == "" {
		return fmt.Errorf("from and to required")
	}
	if l.From == l.To {
		return fmt.Errorf("self-link not allowed")
	}
	if !LinkType(l.Rel).Valid() {
		return fmt.Errorf("invalid link type: %q", l.Rel)
	}
	if l.Created.IsZero() {
		l.Created = time.Now().UTC()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketLinks))
		return b.Put(linkKey(l.From, l.To), []byte(l.Rel))
	})
}

func (s *Store) GetLinks(id string) ([]Link, error) {
	seen := map[string]bool{}
	var out []Link
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(bucketLinks)).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			parts := strings.SplitN(string(k), "\x00", 2)
			if len(parts) != 2 {
				continue
			}
			if parts[0] == id || parts[1] == id {
				ek := string(k)
				if seen[ek] {
					continue
				}
				seen[ek] = true
				out = append(out, Link{From: parts[0], To: parts[1], Rel: string(v)})
			}
		}
		return nil
	})
	return out, err
}

func (s *Store) RemoveLink(from, to string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketLinks)).Delete(linkKey(from, to))
	})
}

func (s *Store) Stats() (map[string]int, error) {
	out := map[string]int{
		"total":      0,
		"links":      0,
		"embeddings": 0,
	}
	err := s.db.View(func(tx *bolt.Tx) error {
		_ = tx.Bucket([]byte(bucketMems)).ForEach(func(_, _ []byte) error {
			out["total"]++
			return nil
		})
		_ = tx.Bucket([]byte(bucketLinks)).ForEach(func(_, _ []byte) error {
			out["links"]++
			return nil
		})
		_ = tx.Bucket([]byte(bucketEmbeddings)).ForEach(func(_, _ []byte) error {
			out["embeddings"]++
			return nil
		})
		return nil
	})
	return out, err
}

func (s *Store) appendAudit(tx *bolt.Tx, memID, action, from, to string) error {
	entry := map[string]any{
		"id":        memID,
		"action":    action,
		"timestamp": time.Now().UTC(),
	}
	if from != "" {
		entry["from"] = from
	}
	if to != "" {
		entry["to"] = to
	}
	raw, _ := json.Marshal(entry)
	b := tx.Bucket([]byte(bucketAudit))
	seq, _ := b.NextSequence()
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, seq)
	return b.Put(key, raw)
}

func (s *Store) EmbeddingStatus() (bool, int) {
	fn, dim := GetEmbedder()
	if fn == nil {
		return false, 0
	}
	return true, dim
}
