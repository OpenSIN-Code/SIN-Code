package todo

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

func auditKey(ts time.Time, id string) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(ts.UnixNano()))
	combined := append(b, []byte("\x00")...)
	combined = append(combined, []byte(id)...)
	return combined
}

func auditPrefix() []byte {
	return []byte{}
}

func (s *Store) AppendAudit(e AuditEntry) error {
	if e.ID == "" {
		h := sha1.Sum([]byte(fmt.Sprintf("%d-%s-%s", time.Now().UnixNano(), e.TodoID, e.Action)))
		e.ID = fmt.Sprintf("au-%x", h[:6])
	}
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.Actor == "" {
		e.Actor = "system"
	}
	return s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketAudit))
		data, err := json.Marshal(&e)
		if err != nil {
			return err
		}
		return b.Put(auditKey(e.Timestamp, e.ID), data)
	})
}

func (s *Store) ListAudit(todoID string) ([]*AuditEntry, error) {
	var out []*AuditEntry
	err := s.view(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketAudit))
		c := b.Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var e AuditEntry
			if uerr := json.Unmarshal(v, &e); uerr != nil {
				continue
			}
			if todoID == "" || e.TodoID == todoID {
				out = append(out, &e)
			}
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.Before(out[j].Timestamp) })
	return out, err
}

func (s *Store) CountAudit() (int, error) {
	c := 0
	err := s.view(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketAudit))
		c = b.Stats().KeyN
		return nil
	})
	return c, err
}
