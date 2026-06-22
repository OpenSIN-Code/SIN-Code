package todo

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/OpenSIN-Code/SIN-Code/cmd/sin-code/internal/filemode"

	bolt "go.etcd.io/bbolt"
)

var (
	osUserConfigDirStore      = os.UserConfigDir
	osMkdirAllStore           = os.MkdirAll
	bboltOpenStore            = bolt.Open
	dbUpdateInit              = func(db *bolt.DB, fn func(*bolt.Tx) error) error { return db.Update(fn) }
	jsonMarshalStore          = json.Marshal
	jsonUnmarshalStore        = json.Unmarshal
	txBucketStore             = func(tx *bolt.Tx, name []byte) *bolt.Bucket { return tx.Bucket(name) }
	bucketPutFn               = func(b *bolt.Bucket, k, v []byte) error { return b.Put(k, v) }
	createBucketIfNotExistsFn = func(tx *bolt.Tx, name []byte) (*bolt.Bucket, error) { return tx.CreateBucketIfNotExists(name) }
)

const (
	bucketTodos = "todos"
	bucketDeps  = "deps"
	bucketAudit = "audit"
	bucketMems  = "memories"
	bucketMeta  = "meta"
	bucketIdxSt = "idx_status"
	bucketIdxPr = "idx_priority"
	bucketIdxAs = "idx_assignee"
	bucketIdxPj = "idx_project"
	bucketIdxTg = "idx_tag"
)

var (
	ErrNotFound = errors.New("todo: not found")
	ErrInvalid  = errors.New("todo: invalid argument")
)

type Store struct {
	mu   sync.Mutex
	db   *bolt.DB
	path string
}

func Open(path string) (*Store, error) {
	if path == "" {
		cfg, err := osUserConfigDirStore()
		if err != nil {
			return nil, err
		}
		path = filepath.Join(cfg, "sin-code", "todo.db")
	}
	if err := osMkdirAllStore(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := bboltOpenStore(path, filemode.Default(), &bolt.Options{Timeout: 2 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open todo db: %w", err)
	}
	if err := dbUpdateInit(db, func(tx *bolt.Tx) error {
		for _, b := range []string{
			bucketTodos, bucketDeps, bucketAudit, bucketMems, bucketMeta,
			bucketIdxSt, bucketIdxPr, bucketIdxAs, bucketIdxPj, bucketIdxTg,
		} {
			if _, err := createBucketIfNotExistsFn(tx, []byte(b)); err != nil {
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

func (s *Store) DB() *bolt.DB { return s.db }

func (s *Store) update(fn func(*bolt.Tx) error) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("todo: store not open")
	}
	return s.db.Update(fn)
}

func (s *Store) view(fn func(*bolt.Tx) error) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("todo: store not open")
	}
	return s.db.View(fn)
}

func todoKey(id string) []byte { return []byte(id) }

func hasPrefix(s, prefix []byte) bool {
	return bytes.HasPrefix(s, prefix)
}

func (s *Store) Add(t *Todo) error {
	if t == nil {
		return fmt.Errorf("%w: nil todo", ErrInvalid)
	}
	if strings.TrimSpace(t.Title) == "" {
		return fmt.Errorf("%w: title required", ErrInvalid)
	}
	if t.Priority == "" {
		t.Priority = PriorityP2
	}
	if !t.Priority.Valid() {
		return fmt.Errorf("%w: priority %q", ErrInvalid, t.Priority)
	}
	if t.Type == "" {
		t.Type = TypeTask
	}
	if !t.Type.Valid() {
		return fmt.Errorf("%w: type %q", ErrInvalid, t.Type)
	}
	if t.Status == "" {
		t.Status = StatusOpen
	}
	if !t.Status.Valid() {
		return fmt.Errorf("%w: status %q", ErrInvalid, t.Status)
	}
	now := time.Now().UTC()
	if t.CreatedAt.IsZero() {
		t.CreatedAt = now
	}
	t.UpdatedAt = now
	if t.ID == "" {
		t.ID = GenerateID()
	}
	t.Tags = normalizeTags(t.Tags)
	return s.update(func(tx *bolt.Tx) error {
		raw, err := jsonMarshalStore(t)
		if err != nil {
			return err
		}
		bT := txBucketStore(tx, []byte(bucketTodos))
		if err := bucketPutFn(bT, todoKey(t.ID), raw); err != nil {
			return err
		}
		writeIndex(tx, bucketIdxSt, string(t.Status), t.ID)
		writeIndex(tx, bucketIdxPr, string(t.Priority), t.ID)
		if t.Assignee != "" {
			writeIndex(tx, bucketIdxAs, t.Assignee, t.ID)
		}
		if t.Project != "" {
			writeIndex(tx, bucketIdxPj, t.Project, t.ID)
		}
		for _, tag := range t.Tags {
			writeIndex(tx, bucketIdxTg, tag, t.ID)
		}
		return nil
	})
}

func (s *Store) Update(t *Todo) error {
	if t == nil || t.ID == "" {
		return fmt.Errorf("%w: missing id", ErrInvalid)
	}
	t.UpdatedAt = time.Now().UTC()
	if !t.Status.Valid() {
		return fmt.Errorf("%w: status %q", ErrInvalid, t.Status)
	}
	if t.IsClosed() && t.ClosedAt == nil {
		now := t.UpdatedAt
		t.ClosedAt = &now
	}
	if !t.IsClosed() && t.ClosedAt != nil {
		t.ClosedAt = nil
	}
	t.Tags = normalizeTags(t.Tags)
	return s.update(func(tx *bolt.Tx) error {
		bT := txBucketStore(tx, []byte(bucketTodos))
		old := bT.Get(todoKey(t.ID))
		if old == nil {
			return ErrNotFound
		}
		var prev Todo
		if err := jsonUnmarshalStore(old, &prev); err != nil {
			return err
		}
		if prev.Status != t.Status {
			removeIndex(tx, bucketIdxSt, string(prev.Status), t.ID)
			writeIndex(tx, bucketIdxSt, string(t.Status), t.ID)
		}
		if prev.Priority != t.Priority {
			removeIndex(tx, bucketIdxPr, string(prev.Priority), t.ID)
			writeIndex(tx, bucketIdxPr, string(t.Priority), t.ID)
		}
		if prev.Assignee != t.Assignee {
			removeIndex(tx, bucketIdxAs, prev.Assignee, t.ID)
			if t.Assignee != "" {
				writeIndex(tx, bucketIdxAs, t.Assignee, t.ID)
			}
		}
		if prev.Project != t.Project {
			removeIndex(tx, bucketIdxPj, prev.Project, t.ID)
			if t.Project != "" {
				writeIndex(tx, bucketIdxPj, t.Project, t.ID)
			}
		}
		oldTags := map[string]struct{}{}
		for _, tg := range prev.Tags {
			oldTags[tg] = struct{}{}
		}
		newTags := map[string]struct{}{}
		for _, tg := range t.Tags {
			newTags[tg] = struct{}{}
		}
		for tg := range oldTags {
			if _, keep := newTags[tg]; !keep {
				removeIndex(tx, bucketIdxTg, tg, t.ID)
			}
		}
		for tg := range newTags {
			if _, had := oldTags[tg]; !had {
				writeIndex(tx, bucketIdxTg, tg, t.ID)
			}
		}
		raw, err := jsonMarshalStore(t)
		if err != nil {
			return err
		}
		return bucketPutFn(bT, todoKey(t.ID), raw)
	})
}

func (s *Store) Get(id string) (*Todo, error) {
	var out *Todo
	err := s.view(func(tx *bolt.Tx) error {
		raw := txBucketStore(tx, []byte(bucketTodos)).Get(todoKey(id))
		if raw == nil {
			return ErrNotFound
		}
		var t Todo
		if err := jsonUnmarshalStore(raw, &t); err != nil {
			return err
		}
		out = &t
		return nil
	})
	return out, err
}

func (s *Store) List() ([]*Todo, error) {
	var out []*Todo
	err := s.view(func(tx *bolt.Tx) error {
		return txBucketStore(tx, []byte(bucketTodos)).ForEach(func(_, v []byte) error {
			var t Todo
			if err := jsonUnmarshalStore(v, &t); err != nil {
				return err
			}
			out = append(out, &t)
			return nil
		})
	})
	return out, err
}

func (s *Store) Delete(id string, hard bool) error {
	return s.update(func(tx *bolt.Tx) error {
		bT := txBucketStore(tx, []byte(bucketTodos))
		raw := bT.Get(todoKey(id))
		if raw == nil {
			return ErrNotFound
		}
		if !hard {
			var t Todo
			if err := jsonUnmarshalStore(raw, &t); err != nil {
				return err
			}
			t.Status = StatusCancelled
			t.UpdatedAt = time.Now().UTC()
			now := t.UpdatedAt
			t.ClosedAt = &now
			buf, err := jsonMarshalStore(&t)
			if err != nil {
				return err
			}
			return bucketPutFn(bT, todoKey(id), buf)
		}
		var t Todo
		if err := jsonUnmarshalStore(raw, &t); err != nil {
			return err
		}
		removeIndex(tx, bucketIdxSt, string(t.Status), t.ID)
		removeIndex(tx, bucketIdxPr, string(t.Priority), t.ID)
		removeIndex(tx, bucketIdxAs, t.Assignee, t.ID)
		removeIndex(tx, bucketIdxPj, t.Project, t.ID)
		for _, tg := range t.Tags {
			removeIndex(tx, bucketIdxTg, tg, t.ID)
		}
		return bucketDeleteFn(bT, todoKey(id))
	})
}

func (s *Store) GetMeta(key string) (string, error) {
	var val string
	err := s.view(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMeta))
		raw := b.Get([]byte(key))
		if raw == nil {
			return nil
		}
		val = string(raw)
		return nil
	})
	return val, err
}

func (s *Store) SetMeta(key, value string) error {
	return s.update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketMeta))
		return b.Put([]byte(key), []byte(value))
	})
}

func (s *Store) IndexKeys(bucketName, key string) ([]string, error) {
	var ids []string
	err := s.view(func(tx *bolt.Tx) error {
		b := txBucketStore(tx, []byte(bucketName))
		if b == nil {
			return nil
		}
		prefix := []byte(key + "\x00")
		c := b.Cursor()
		for k, _ := c.Seek(prefix); k != nil && hasPrefix(k, prefix); k, _ = c.Next() {
			id := string(k[len(prefix):])
			ids = append(ids, id)
		}
		return nil
	})
	sort.Strings(ids)
	return ids, err
}

func writeIndex(tx *bolt.Tx, bucketName, key, id string) {
	if key == "" {
		return
	}
	b := txBucketStore(tx, []byte(bucketName))
	if b == nil {
		return
	}
	_ = b.Put([]byte(key+"\x00"+id), []byte{})
}

func removeIndex(tx *bolt.Tx, bucketName, key, id string) {
	if key == "" {
		return
	}
	b := txBucketStore(tx, []byte(bucketName))
	if b == nil {
		return
	}
	_ = b.Delete([]byte(key + "\x00" + id))
}

func normalizeTags(tags []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
