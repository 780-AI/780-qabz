package config

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
)

// Hot-reload: a field with the tag `hot_reload:"true"` is read again from the backend source
// (Postgres) at HotReloadInterval. The key of a field in the source is its name in snake case
// (LogLevel is log_level). The supported kinds are string, int, bool and time.Duration.
// The reflection here runs at bootstrap and at each reload, never on a hot path.

// Row is one entry of the backend source. Type is one of: string, int, bool, duration.
type Row struct {
	Key   string
	Type  string
	Value string
}

// Repository loads the hot configs from the backend source. The source holds only hot-reloadable
// configs, never all configs.
type Repository interface {
	Load(ctx context.Context) ([]Row, error)
}

// Logger is the logger the reloader needs. It never gets a config value, only keys.
type Logger interface {
	Info(msg string, kv ...any)
	Warn(msg string, kv ...any)
	Error(msg string, kv ...any)
}

// Holder keeps the current value of T. Get returns a copy, so a reader never sees a half-written
// value and needs no lock. Set swaps the whole value with one atomic store.
type Holder[T any] struct{ p atomic.Pointer[T] }

// NewHolder returns a Holder with v as its first value.
func NewHolder[T any](v T) *Holder[T] {
	h := &Holder[T]{}
	h.p.Store(&v)
	return h
}

// Get returns a copy of the current value.
func (h *Holder[T]) Get() T { return *h.p.Load() }

// Set replaces the value.
func (h *Holder[T]) Set(v T) { h.p.Store(&v) }

type hotField struct {
	index int
	key   string
	typ   string
}

func hotFields[T any]() ([]hotField, error) {
	t := reflect.TypeFor[T]()
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("config: %s is not a struct", t)
	}
	durType := reflect.TypeFor[time.Duration]()
	var out []hotField
	seen := map[string]bool{}
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag, ok := f.Tag.Lookup("hot_reload")
		if !ok {
			continue
		}
		if tag != "true" && tag != "false" {
			return nil, fmt.Errorf("config: field %s: hot_reload must be \"true\" or \"false\"", f.Name)
		}
		if tag == "false" {
			continue
		}
		var typ string
		switch {
		case f.Type == durType:
			typ = "duration"
		case f.Type.Kind() == reflect.String:
			typ = "string"
		case f.Type.Kind() == reflect.Int:
			typ = "int"
		case f.Type.Kind() == reflect.Bool:
			typ = "bool"
		default:
			return nil, fmt.Errorf("config: field %s: kind %s cannot be hot-reloaded", f.Name, f.Type)
		}
		k := snakeCase(f.Name)
		if seen[k] {
			return nil, fmt.Errorf("config: key %s is used twice", k)
		}
		seen[k] = true
		out = append(out, hotField{index: i, key: k, typ: typ})
	}
	return out, nil
}

func snakeCase(s string) string {
	r := []rune(s)
	var b strings.Builder
	for i, c := range r {
		if i > 0 && r[i-1] != '_' && unicode.IsUpper(c) && (unicode.IsLower(r[i-1]) || unicode.IsDigit(r[i-1]) ||
			(i+1 < len(r) && unicode.IsLower(r[i+1]))) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(c))
	}
	return b.String()
}

// Reloader reloads the hot fields of T into a Holder at an interval.
type Reloader[T any] struct {
	repo     Repository
	holder   *Holder[T]
	interval time.Duration
	log      Logger
	fields   map[string]hotField

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

// NewReloader checks the hot_reload tags of T. A bad tag is an error at bootstrap.
func NewReloader[T any](repo Repository, holder *Holder[T], interval time.Duration, log Logger) (*Reloader[T], error) {
	if interval <= 0 {
		return nil, fmt.Errorf("config: hot_reload_interval must be positive")
	}
	fs, err := hotFields[T]()
	if err != nil {
		return nil, err
	}
	m := make(map[string]hotField, len(fs))
	for _, f := range fs {
		m[f.key] = f
	}
	return &Reloader[T]{repo: repo, holder: holder, interval: interval, log: log, fields: m}, nil
}

// HasHotFields reports whether T has any field to reload.
func (r *Reloader[T]) HasHotFields() bool { return len(r.fields) > 0 }

// Start does the first reload at once, then reloads at each interval until Stop.
// A failed reload is logged and keeps the last good values, so Start returns no load error.
func (r *Reloader[T]) Start(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil {
		return fmt.Errorf("config: reloader already started")
	}
	r.Reload(ctx)
	loopCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.cancel, r.done = cancel, make(chan struct{})
	go r.loop(loopCtx, r.done)
	return nil
}

// Stop ends the loop and waits for it. It is safe to call more than once.
func (r *Reloader[T]) Stop(ctx context.Context) error {
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.cancel = nil
	r.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *Reloader[T]) loop(ctx context.Context, done chan struct{}) {
	defer close(done)
	t := time.NewTicker(r.interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			r.Reload(ctx)
		}
	}
}

// Reload loads the rows once and assigns the changed values. It returns the keys that changed.
// A row with an unknown key, a wrong type or a bad value is skipped and logged; the other rows apply.
func (r *Reloader[T]) Reload(ctx context.Context) []string {
	rows, err := r.repo.Load(ctx)
	if err != nil {
		r.log.Error("config reload failed; keeping the last values", "error", err)
		return nil
	}
	cur := r.holder.Get()
	next := cur
	v := reflect.ValueOf(&next).Elem()
	var changed []string
	for _, row := range rows {
		f, ok := r.fields[row.Key]
		if !ok {
			r.log.Warn("config row ignored: key is not hot-reloadable", "key", row.Key)
			continue
		}
		if row.Type != f.typ {
			r.log.Warn("config row ignored: wrong type", "key", row.Key, "want", f.typ, "got", row.Type)
			continue
		}
		fv := v.Field(f.index)
		old := fv.Interface()
		if err := assign(fv, f.typ, row.Value); err != nil {
			r.log.Warn("config row ignored: bad value", "key", row.Key, "type", f.typ)
			continue
		}
		if !reflect.DeepEqual(old, fv.Interface()) {
			changed = append(changed, row.Key)
		}
	}
	if len(changed) > 0 {
		r.holder.Set(next)
		r.log.Info("config reloaded", "keys", changed)
	}
	return changed
}

func assign(fv reflect.Value, typ, raw string) error {
	switch typ {
	case "string":
		fv.SetString(raw)
	case "int":
		n, err := strconv.Atoi(raw)
		if err != nil {
			return err
		}
		fv.SetInt(int64(n))
	case "bool":
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}
		fv.SetBool(b)
	case "duration":
		d, err := time.ParseDuration(raw)
		if err != nil {
			return err
		}
		fv.SetInt(int64(d))
	}
	return nil
}
