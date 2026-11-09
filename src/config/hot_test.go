package config

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type hotCfg struct {
	LogLevel     string        `hot_reload:"true"`
	Workers      int           `hot_reload:"true"`
	Verbose      bool          `hot_reload:"true"`
	Timeout      time.Duration `hot_reload:"true"`
	OTLPEndpoint string        `hot_reload:"true"`
	Frozen       string        // no tag: never reloaded
	Off          string        `hot_reload:"false"`
}

type fakeRepo struct {
	mu   sync.Mutex
	rows []Row
	err  error
	n    atomic.Int32
}

func (f *fakeRepo) Load(context.Context) ([]Row, error) {
	f.n.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Row(nil), f.rows...), f.err
}

func (f *fakeRepo) set(rows []Row, err error) { f.mu.Lock(); f.rows, f.err = rows, err; f.mu.Unlock() }

type fakeLog struct {
	mu   sync.Mutex
	msgs []string
}

func (l *fakeLog) add(m string)             { l.mu.Lock(); l.msgs = append(l.msgs, m); l.mu.Unlock() }
func (l *fakeLog) Info(m string, _ ...any)  { l.add("I " + m) }
func (l *fakeLog) Warn(m string, _ ...any)  { l.add("W " + m) }
func (l *fakeLog) Error(m string, _ ...any) { l.add("E " + m) }
func (l *fakeLog) has(m string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, x := range l.msgs {
		if x == m {
			return true
		}
	}
	return false
}

func newRel(t *testing.T, repo Repository, interval time.Duration) (*Reloader[hotCfg], *Holder[hotCfg], *fakeLog) {
	t.Helper()
	h := NewHolder(hotCfg{LogLevel: "info", Workers: 1, Frozen: "keep"})
	l := &fakeLog{}
	r, err := NewReloader[hotCfg](repo, h, interval, l)
	if !assert.NoError(t, err) || !assert.NotNil(t, r) {
		t.FailNow()
	}
	return r, h, l
}

func TestSnakeCase(t *testing.T) {
	assert.Equal(t, "log_level", snakeCase("LogLevel"))
	assert.Equal(t, "otlp_endpoint", snakeCase("OTLPEndpoint"))
	assert.Equal(t, "postgres_db", snakeCase("PostgresDB"))
	assert.Equal(t, "workers", snakeCase("Workers"))
}

func TestHotFieldsErrors(t *testing.T) {
	_, err := hotFields[struct {
		X float64 `hot_reload:"true"`
	}]()
	assert.ErrorContains(t, err, "cannot be hot-reloaded")
	_, err = hotFields[struct {
		X string `hot_reload:"yes"`
	}]()
	assert.ErrorContains(t, err, `must be "true" or "false"`)
	_, err = hotFields[struct {
		FooBar  string `hot_reload:"true"`
		Foo_Bar string `hot_reload:"true"`
	}]()
	assert.ErrorContains(t, err, "used twice")
	_, err = hotFields[int]()
	assert.ErrorContains(t, err, "not a struct")
	_, err = NewReloader[hotCfg](&fakeRepo{}, NewHolder(hotCfg{}), 0, &fakeLog{})
	assert.ErrorContains(t, err, "must be positive")
	for name, args := range map[string]struct {
		repo Repository
		h    *Holder[hotCfg]
		log  Logger
	}{
		"no repository": {nil, NewHolder(hotCfg{}), &fakeLog{}},
		"no holder":     {&fakeRepo{}, nil, &fakeLog{}},
		"no logger":     {&fakeRepo{}, NewHolder(hotCfg{}), nil},
	} {
		_, err = NewReloader[hotCfg](args.repo, args.h, time.Second, args.log)
		assert.ErrorContains(t, err, "needs a repository, a holder and a logger", name)
	}
}

func TestReloadAssignsChangedValues(t *testing.T) {
	repo := &fakeRepo{rows: []Row{
		{"log_level", "string", "debug"}, {"workers", "int", "8"}, {"verbose", "bool", "true"},
		{"timeout", "duration", "5s"}, {"otlp_endpoint", "string", "jaeger:4317"},
	}}
	r, h, l := newRel(t, repo, time.Hour)
	changed := r.Reload(context.Background())
	assert.ElementsMatch(t, []string{"log_level", "workers", "verbose", "timeout", "otlp_endpoint"}, changed)
	got := h.Get()
	assert.Equal(t, hotCfg{LogLevel: "debug", Workers: 8, Verbose: true, Timeout: 5 * time.Second,
		OTLPEndpoint: "jaeger:4317", Frozen: "keep"}, got)
	assert.True(t, l.has("I config reloaded"))

	l.msgs = nil
	assert.Empty(t, r.Reload(context.Background()), "same values: no change")
	assert.False(t, l.has("I config reloaded"))
}

func TestReloadIgnoresUntaggedAndBadRows(t *testing.T) {
	repo := &fakeRepo{rows: []Row{
		{"frozen", "string", "hacked"},  // no tag
		{"off", "string", "hacked"},     // tag false
		{"nope", "string", "x"},         // unknown key
		{"workers", "string", "9"},      // wrong type
		{"verbose", "bool", "maybe"},    // bad value
		{"log_level", "string", "warn"}, // good row still applies
	}}
	r, h, l := newRel(t, repo, time.Hour)
	assert.Equal(t, []string{"log_level"}, r.Reload(context.Background()))
	got := h.Get()
	assert.Equal(t, "keep", got.Frozen)
	assert.Equal(t, "", got.Off)
	assert.Equal(t, 1, got.Workers)
	assert.False(t, got.Verbose)
	assert.Equal(t, "warn", got.LogLevel)
	assert.True(t, l.has("W config row ignored: key is not hot-reloadable"))
	assert.True(t, l.has("W config row ignored: wrong type"))
	assert.True(t, l.has("W config row ignored: bad value"))
}

func TestFailedReloadKeepsLastValues(t *testing.T) {
	repo := &fakeRepo{rows: []Row{{"workers", "int", "4"}}}
	r, h, l := newRel(t, repo, time.Hour)
	r.Reload(context.Background())
	repo.set(nil, errors.New("db down"))
	assert.Empty(t, r.Reload(context.Background()))
	assert.Equal(t, 4, h.Get().Workers)
	assert.True(t, l.has("E config reload failed; keeping the last values"))
}

func TestLogNeverHasValues(t *testing.T) {
	// The logger interface gets keys only; this guards the call sites by checking the message set.
	repo := &fakeRepo{rows: []Row{{"log_level", "string", "s3cret-value"}}}
	r, _, l := newRel(t, repo, time.Hour)
	r.Reload(context.Background())
	for _, m := range l.msgs {
		assert.NotContains(t, m, "s3cret-value")
	}
}

func TestStartReloadsAtEachInterval(t *testing.T) {
	repo := &fakeRepo{rows: []Row{{"workers", "int", "2"}}}
	r, h, _ := newRel(t, repo, 20*time.Millisecond)
	assert.NoError(t, r.Start(context.Background()))
	assert.Equal(t, 2, h.Get().Workers, "first load is synchronous")
	assert.ErrorContains(t, r.Start(context.Background()), "already started")

	repo.set([]Row{{"workers", "int", "7"}}, nil)
	assert.Eventually(t, func() bool { return h.Get().Workers == 7 }, 2*time.Second, 5*time.Millisecond)

	assert.NoError(t, r.Stop(context.Background()))
	n := repo.n.Load()
	time.Sleep(80 * time.Millisecond)
	assert.Equal(t, n, repo.n.Load(), "no load after Stop")
	assert.NoError(t, r.Stop(context.Background()), "Stop twice is fine")
}

func TestHasHotFields(t *testing.T) {
	r, _, _ := newRel(t, &fakeRepo{}, time.Second)
	assert.True(t, r.HasHotFields())
	r2, err := NewReloader[Config](&fakeRepo{}, NewHolder(Config{}), time.Second, &fakeLog{})
	if assert.NoError(t, err) && assert.NotNil(t, r2) {
		assert.True(t, r2.HasHotFields(), "Config has providers.general.load_interval (task 020)")
	}
}

// Run with -race: readers call Get while the reloader swaps the value.
func TestConcurrentReadsDuringReload(t *testing.T) {
	repo := &fakeRepo{}
	r, h, _ := newRel(t, repo, time.Hour)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					c := h.Get()
					assert.Equal(t, "keep", c.Frozen)
					assert.Equal(t, c.Workers, c.Workers)
				}
			}
		}()
	}
	for i := 1; i <= 300; i++ {
		repo.set([]Row{{"workers", "int", string(rune('0' + i%10))}, {"log_level", "string", "l"}}, nil)
		r.Reload(context.Background())
	}
	close(stop)
	wg.Wait()
}

// closed reports whether ch is closed now.
func closed(ch <-chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

func TestHolderChangedOnSet(t *testing.T) {
	h := NewHolder(hotCfg{Workers: 1})
	c1 := h.Changed()
	assert.False(t, closed(c1), "no Set yet")
	assert.Equal(t, c1, h.Changed(), "the same channel until the next Set")
	h.Set(hotCfg{Workers: 2})
	assert.True(t, closed(c1), "Set closes the channel")
	assert.Equal(t, 2, h.Get().Workers)
	c2 := h.Changed()
	assert.False(t, closed(c2), "a new channel after the Set")
	h.Set(hotCfg{Workers: 2})
	assert.True(t, closed(c2), "a Set of the same value also notifies; the subscriber compares")
	h.Set(hotCfg{Workers: 3}) // no subscriber: nothing to close, no panic
	assert.Equal(t, 3, h.Get().Workers)
}

func TestHolderChangedByReload(t *testing.T) {
	repo := &fakeRepo{rows: []Row{{"workers", "int", "4"}}}
	r, h, _ := newRel(t, repo, time.Hour)
	ch := h.Changed()
	r.Reload(context.Background())
	assert.True(t, closed(ch), "a reload that changed a value notifies")

	ch = h.Changed()
	r.Reload(context.Background())
	assert.False(t, closed(ch), "a reload with no change does not notify")
	repo.set(nil, errors.New("db down"))
	r.Reload(context.Background())
	assert.False(t, closed(ch), "a failed reload does not notify")
	repo.set([]Row{{"workers", "int", "bad"}}, nil)
	r.Reload(context.Background())
	assert.False(t, closed(ch), "a reload with only a bad row does not notify")
}

// Run with -race: many subscribers wait while many goroutines Set; every subscriber ends on the
// last value.
func TestHolderChangedConcurrent(t *testing.T) {
	h := NewHolder(hotCfg{Workers: 0})
	const subs, setters, sets = 8, 4, 200
	final := setters * sets
	stop := make(chan struct{})
	var wg sync.WaitGroup
	seen := make([]int, subs)
	for i := 0; i < subs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for {
				ch := h.Changed()
				if v := h.Get().Workers; v == final {
					seen[i] = v
					return
				}
				select {
				case <-ch:
				case <-stop:
					return
				}
			}
		}(i)
	}
	var sw sync.WaitGroup
	var next atomic.Int32
	for i := 0; i < setters; i++ {
		sw.Add(1)
		go func() {
			defer sw.Done()
			for j := 0; j < sets; j++ {
				if n := int(next.Add(1)); n < final {
					h.Set(hotCfg{Workers: n})
				}
			}
		}()
	}
	sw.Wait()
	h.Set(hotCfg{Workers: final})
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		close(stop)
		<-done
		assert.Fail(t, "a subscriber missed the last Set")
	}
	for i, v := range seen {
		assert.Equal(t, final, v, "subscriber %d", i)
	}
}
