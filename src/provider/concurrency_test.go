package provider

import (
	"context"
	"math/rand/v2"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// waitGroupOrFail waits for wg; after timeout the test fails (a deadlock).
func waitGroupOrFail(t *testing.T, wg *sync.WaitGroup, timeout time.Duration) bool {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return assert.Fail(t, "deadlock: the goroutines did not end")
	}
}

// sortedIDs returns the ids of ps, sorted.
func sortedIDs(ps []Provider) []int64 {
	out := ids(ps)
	slices.Sort(out)
	return out
}

// TestConcurrentReadersAndWriters (run with -race): many goroutines call PickProvider, List and
// Group while others load (two row sets in turn), move and toggle. Each reader sees a whole list
// (the ids of one of the two row sets) that is a pure sort; no race, no panic, no deadlock.
func TestConcurrentReadersAndWriters(t *testing.T) {
	rowsA := manyRows()
	keys := keysOf(rowsA)
	if !assert.Greater(t, len(rowsA), 40) || !assert.NotEmpty(t, keys) {
		return
	}
	rowsB := slices.Clone(rowsA[:len(rowsA)-40])
	for i := range rowsB {
		if i%7 == 0 {
			rowsB[i].Priority = -rowsB[i].Priority
		}
	}
	rowsB = append(rowsB, Provider{ID: 1, BillType: "gas", Action: "order", Priority: -1, Status: StatusEnable})
	wholeA, wholeB := sortedIDs(rowsA), sortedIDs(rowsB)
	allIDs := ids(rowsA)
	if !assert.NotEmpty(t, allIDs) {
		return
	}

	s, repo, _ := loaded(t, rowsA)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var bad sync.Map // reason -> true

	for r := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; ; i++ {
				select {
				case <-stop:
					return
				default:
				}
				k := keys[(i+r)%len(keys)]
				if p, err := s.PickProvider(k[0], k[1]); err == nil && (!p.Enabled() || p.BillType != k[0] || p.Action != k[1]) {
					bad.Store("pick returned a wrong provider", true)
				}
				g := s.Group(k[0], k[1])
				if !isPureSort(g) {
					bad.Store("group not sorted", true)
				}
				if i%5 == 0 {
					list := s.List()
					if !isPureSort(list) {
						bad.Store("list not sorted", true)
					}
					whole := sortedIDs(list)
					if !slices.Equal(whole, wholeA) && !slices.Equal(whole, wholeB) {
						bad.Store("list is a mix of two loads", true)
					}
				}
			}
		}()
	}
	wg.Add(1)
	go func() { // loader
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			if i%2 == 0 {
				repo.set(rowsB, nil)
			} else {
				repo.set(rowsA, nil)
			}
			_ = s.Load(context.Background())
		}
	}()
	for w := range 4 {
		wg.Add(1)
		go func() { // movers and togglers
			defer wg.Done()
			r := rand.New(rand.NewPCG(uint64(w), 5))
			for {
				select {
				case <-stop:
					return
				default:
				}
				id := allIDs[r.IntN(len(allIDs))]
				switch r.IntN(4) {
				case 0:
					_ = s.MoveUp(id)
				case 1:
					_ = s.MoveDown(id)
				case 2:
					_ = s.Enable(id)
				default:
					_ = s.Disable(id)
				}
			}
		}()
	}
	time.Sleep(300 * time.Millisecond)
	close(stop)
	if !waitGroupOrFail(t, &wg, 10*time.Second) {
		return
	}
	bad.Range(func(k, _ any) bool {
		reason, _ := k.(string)
		assert.Fail(t, reason)
		return true
	})
}

// TestReadersDoNotWaitForWriters: a writer held in the middle of a change (it holds the writer
// lock) does not stop PickProvider, List or Group; a load held in its query does not stop readers
// or a move.
func TestReadersDoNotWaitForWriters(t *testing.T) {
	s, repo, _ := loaded(t, seedRows(t))
	s.writeMu.Lock() // a writer in the middle of a change
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 1000 {
			_, _ = s.PickProvider("gas", "inquiry")
			_ = s.List()
			_ = s.Group("gas", "inquiry")
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		assert.Fail(t, "a reader waited for a writer")
	}
	s.writeMu.Unlock()

	repo.mu.Lock()
	repo.block = make(chan struct{})
	repo.mu.Unlock()
	loadDone := make(chan error, 1)
	go func() { loadDone <- s.Load(context.Background()) }()
	if !waitFor(t, func() bool { return repo.count() >= 2 }, "the load is in its query") {
		close(repo.block) // let the load end, so its goroutine does not leak
		<-loadDone
		return
	}
	moved := make(chan error, 1)
	go func() { moved <- s.MoveDown(43) }()
	select {
	case err := <-moved:
		assert.NoError(t, err)
	case <-time.After(3 * time.Second):
		assert.Fail(t, "a move waited for a load query")
	}
	assert.Equal(t, []int64{99, 43, 35}, ids(s.Group("gas", "inquiry")), "the move applied to the old list")
	close(repo.block)
	assert.NoError(t, <-loadDone)
	assert.Equal(t, []int64{43, 99, 35}, ids(s.Group("gas", "inquiry")), "the load replaced the list (database wins)")
}

// TestNoLostChange (run with -race, also with -count > 1): many writers change the list at the
// same time, each in its own groups; after they stop, the list equals the same changes applied one
// by one (the model). A writer that is not serialized loses changes of the others.
func TestNoLostChange(t *testing.T) {
	rows := manyRows()
	s, _, _ := loaded(t, rows)
	m := newModel(rows)
	keys := keysOf(rows)

	type op struct {
		kind int
		id   int64
	}
	plans := make([][]op, len(keys))
	for i, k := range keys {
		r := rand.New(rand.NewPCG(uint64(i), 77))
		group := m.order[k]
		if !assert.NotEmpty(t, group, "group %v", k) {
			return
		}
		for range 400 {
			plans[i] = append(plans[i], op{kind: r.IntN(4), id: group[r.IntN(len(group))]})
		}
	}
	// The model: each plan applied one by one (the groups are independent, so the order of the
	// plans does not matter).
	for _, plan := range plans {
		for _, o := range plan {
			switch o.kind {
			case 0:
				m.move(t, o.id, -1)
			case 1:
				m.move(t, o.id, 1)
			case 2:
				m.set(o.id, StatusEnable)
			default:
				m.set(o.id, StatusDisable)
			}
		}
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, plan := range plans {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for _, o := range plan {
				switch o.kind {
				case 0:
					_ = s.MoveUp(o.id)
				case 1:
					_ = s.MoveDown(o.id)
				case 2:
					_ = s.Enable(o.id)
				default:
					_ = s.Disable(o.id)
				}
			}
		}()
	}
	close(start)
	if !waitGroupOrFail(t, &wg, 10*time.Second) {
		return
	}
	m.check(t, s, -1)
}

func BenchmarkPickProvider(b *testing.B) {
	s, _, _ := loaded(b, seedRows(b))
	b.ReportAllocs()
	for b.Loop() {
		_, _ = s.PickProvider("gas", "inquiry")
	}
}

func BenchmarkPickProviderParallel(b *testing.B) {
	s, _, _ := loaded(b, seedRows(b))
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = s.PickProvider("gas", "inquiry")
		}
	})
}

// BenchmarkPickProviderDuringWrites: PickProvider while one goroutine moves providers all the time.
func BenchmarkPickProviderDuringWrites(b *testing.B) {
	s, _, _ := loaded(b, seedRows(b))
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.MoveDown(43)
				_ = s.MoveUp(43)
			}
		}
	}()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = s.PickProvider("gas", "inquiry")
	}
	close(stop)
	wg.Wait()
}
