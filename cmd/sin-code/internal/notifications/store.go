// SPDX-License-Identifier: MIT
// Purpose: notification model and store for todo events. Backed by bbolt.
package notifications

import (
	"crypto/sha1"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	bucketNotifs     = "notifications"
	bucketIdxUnread  = "idx_unread"
	bucketIdxType    = "idx_type"
	bucketIdxTodo    = "idx_todo"
	ttlDefault       = 7 * 24 * time.Hour
)

var (
	ErrNotFound = fmt.Errorf("notifications: not found")
)

type Type string

const (
	TypeTodoCreated   Type = "todo_created"
	TypeTodoAssigned  Type = "todo_assigned"
	TypeTodoClaimed   Type = "todo_claimed"
	TypeTodoCompleted Type = "todo_completed"
	TypeTodoCancelled Type = "todo_cancelled"
	TypeTodoBlocked   Type = "todo_blocked"
	TypeTodoUnblocked Type = "todo_unblocked"
	TypeTodoDeleted   Type = "todo_deleted"
	TypeTodoDepAdd    Type = "todo_dep_add"
	TypeTodoStale     Type = "todo_stale"
	TypeTodoOverdue   Type = "todo_overdue"
)

type Notification struct {
	ID        string    `json:"id"`
	Type      Type      `json:"type"`
	TodoID    string    `json:"todo_id"`
	Title     string    `json:"title"`
	Message   string    `json:"message"`
	Actor     string    `json:"actor,omitempty"`
	Created   time.Time `json:"created"`
	Read      bool      `json:"read"`
	Dismissed bool      `json:"dismissed"`
}

type Store struct {
	mu   sync.Mutex
	db   *bolt.DB
	path string
}

func Open(path string) (*Store, error) {
	if path == "" {
		cfg, err := defaultConfigDir()
		if err != nil {
			return nil, err
		}
		path = cfg + "/notifications.db"
	}
	if err := mkdirAll(dirOf(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bolt.Open(path, 0o644, &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open notifications db: %w", err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, b := range []string{bucketNotifs, bucketIdxUnread, bucketIdxType, bucketIdxTodo} {
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

func (s *Store) Add(n *Notification) error {
	if n == nil {
		return fmt.Errorf("nil notification")
	}
	if n.Type == "" {
		return fmt.Errorf("type required")
	}
	if n.ID == "" {
		n.ID = generateID(n)
	}
	if n.Created.IsZero() {
		n.Created = time.Now().UTC()
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		raw, err := json.Marshal(n)
		if err != nil {
			return err
		}
		b := tx.Bucket([]byte(bucketNotifs))
		if err := b.Put(key(n.Created, n.ID), raw); err != nil {
			return err
		}
		writeIndex(tx, bucketIdxType, string(n.Type), n.ID)
		writeIndex(tx, bucketIdxTodo, n.TodoID, n.ID)
		if !n.Read {
			writeIndex(tx, bucketIdxUnread, "1", n.ID)
		}
		return nil
	})
}

func (s *Store) Get(id string) (*Notification, error) {
	var n *Notification
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(bucketNotifs)).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var x Notification
			if err := json.Unmarshal(v, &x); err != nil {
				continue
			}
			if x.ID == id {
				n = &x
				return nil
			}
		}
		return ErrNotFound
	})
	return n, err
}

type ListFilter struct {
	Type      Type
	TodoID    string
	Unread    bool
	NotDismissed bool
}

func (s *Store) List(f ListFilter, limit int) ([]*Notification, error) {
	var out []*Notification
	err := s.db.View(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(bucketNotifs)).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var n Notification
			if err := json.Unmarshal(v, &n); err != nil {
				continue
			}
			if f.Type != "" && n.Type != f.Type {
				continue
			}
			if f.TodoID != "" && n.TodoID != f.TodoID {
				continue
			}
			if f.Unread && n.Read {
				continue
			}
			if f.NotDismissed && n.Dismissed {
				continue
			}
			out = append(out, &n)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, err
}

func (s *Store) MarkRead(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return updateNotifInTx(tx, id, func(n *Notification) bool {
			if n.Read {
				return false
			}
			n.Read = true
			return true
		}, true)
	})
}

func (s *Store) MarkUnread(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return updateNotifInTx(tx, id, func(n *Notification) bool {
			if !n.Read {
				return false
			}
			n.Read = false
			return true
		}, false)
	})
}

func (s *Store) Dismiss(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return updateNotifInTx(tx, id, func(n *Notification) bool {
			n.Dismissed = true
			if !n.Read {
				n.Read = true
				return true
			}
			return false
		}, true)
	})
}

func updateNotifInTx(tx *bolt.Tx, id string, fn func(*Notification) bool, removeUnread bool) error {
	c := tx.Bucket([]byte(bucketNotifs)).Cursor()
	for k, v := c.First(); k != nil; k, v = c.Next() {
		var n Notification
		if err := json.Unmarshal(v, &n); err != nil {
			continue
		}
		if n.ID != id {
			continue
		}
		changed := fn(&n)
		raw, err := json.Marshal(&n)
		if err != nil {
			return err
		}
		if err := tx.Bucket([]byte(bucketNotifs)).Put(k, raw); err != nil {
			return err
		}
		if changed && removeUnread {
			if err := tx.Bucket([]byte(bucketIdxUnread)).Delete([]byte("1\x00" + id)); err != nil {
				return err
			}
		}
		return nil
	}
	return ErrNotFound
}

func (s *Store) Clear() error {
	return s.db.Update(func(tx *bolt.Tx) error {
		for _, b := range []string{bucketNotifs, bucketIdxUnread, bucketIdxType, bucketIdxTodo} {
			if err := tx.Bucket([]byte(b)).ForEach(func(k, _ []byte) error {
				return tx.Bucket([]byte(b)).Delete(k)
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Prune(ttl time.Duration) (int, error) {
	if ttl <= 0 {
		ttl = ttlDefault
	}
	cutoff := time.Now().Add(-ttl)
	pruned := 0
	err := s.db.Update(func(tx *bolt.Tx) error {
		c := tx.Bucket([]byte(bucketNotifs)).Cursor()
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var n Notification
			if err := json.Unmarshal(v, &n); err != nil {
				continue
			}
			if n.Created.Before(cutoff) || n.Dismissed {
				if err := c.Delete(); err != nil {
					return err
				}
				pruned++
			}
		}
		return nil
	})
	return pruned, err
}

func (s *Store) Count() (int, error) {
	c := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		c = tx.Bucket([]byte(bucketNotifs)).Stats().KeyN
		return nil
	})
	return c, err
}

func (s *Store) CountUnread() (int, error) {
	c := 0
	err := s.db.View(func(tx *bolt.Tx) error {
		c = tx.Bucket([]byte(bucketIdxUnread)).Stats().KeyN
		return nil
	})
	return c, err
}

type Stats struct {
	Total  int            `json:"total"`
	Unread int            `json:"unread"`
	ByType map[Type]int   `json:"by_type"`
}

func (s *Store) ComputeStats() (*Stats, error) {
	st := &Stats{ByType: map[Type]int{}}
	all, err := s.List(ListFilter{}, 0)
	if err != nil {
		return nil, err
	}
	st.Total = len(all)
	for _, n := range all {
		st.ByType[n.Type]++
		if !n.Read {
			st.Unread++
		}
	}
	return st, nil
}

func generateID(n *Notification) string {
	h := sha1.Sum([]byte(fmt.Sprintf("%d-%s-%s-%s", time.Now().UnixNano(), n.Type, n.TodoID, n.Title)))
	return fmt.Sprintf("nt-%x", h[:6])
}

func key(ts time.Time, id string) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(ts.UnixNano()))
	return append(b, append([]byte{0}, []byte(id)...)...)
}

func writeIndex(tx *bolt.Tx, bucket, key, id string) {
	if key == "" {
		return
	}
	b := tx.Bucket([]byte(bucket))
	if b == nil {
		return
	}
	_ = b.Put([]byte(key+"\x00"+id), []byte{})
}

func (s *Store) DB() *bolt.DB { return s.db }

// Notification interface methods (so *Notification satisfies tui.NotificationSource).

func (n *Notification) GetID() string      { return n.ID }
func (n *Notification) GetTitle() string   { return n.Title }
func (n *Notification) GetMessage() string { return n.Message }
func (n *Notification) GetType() string    { return string(n.Type) }
