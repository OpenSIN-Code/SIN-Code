package todo

import (
	"encoding/json"
	"fmt"
	"time"

	bolt "go.etcd.io/bbolt"
)

type Memory struct {
	ID        string    `json:"id"`
	Insight   string    `json:"insight"`
	CreatedAt time.Time `json:"created_at"`
	Actor     string    `json:"actor"`
}

func memKey(ts time.Time, id string) []byte {
	b := make([]byte, 8)
	v := uint64(ts.UnixNano())
	for i := 7; i >= 0; i-- {
		b[i] = byte(v)
		v >>= 8
	}
	return append(b, append([]byte("\x00"), []byte(id)...)...)
}

func (s *Store) AddMemory(m *Memory) error {
	if m.ID == "" {
		m.ID = GenerateID()
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	if m.Actor == "" {
		m.Actor = "system"
	}
	return s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMems))
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		return b.Put(memKey(m.CreatedAt, m.ID), data)
	})
}

func (s *Store) ListMemories() ([]*Memory, error) {
	var out []*Memory
	err := s.view(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMems))
		return b.ForEach(func(_, v []byte) error {
			var m Memory
			if err := json.Unmarshal(v, &m); err != nil {
				return err
			}
			out = append(out, &m)
			return nil
		})
	})
	return out, err
}

var _ = fmt.Sprintf
