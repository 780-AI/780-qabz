package provider

import (
	"context"
	"math/rand/v2"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewProvidersServiceRules(t *testing.T) {
	interval := func() time.Duration { return time.Minute }
	_, err := NewProvidersService(nil, Options{Interval: interval, Logger: &fakeLog{}})
	assert.ErrorContains(t, err, "repository is required")
	_, err = NewProvidersService(&fakeRepo{}, Options{Logger: &fakeLog{}})
	assert.ErrorContains(t, err, "load interval is required")
	_, err = NewProvidersService(&fakeRepo{}, Options{Interval: interval})
	assert.ErrorContains(t, err, "logger is required")

	s := newTestService(t, &fakeRepo{}, &fakeLog{})
	assert.Empty(t, s.List(), "empty before the first load")
	_, err = s.PickProvider("gas", "order")
	assert.ErrorIs(t, err, ErrNoProvider)
}

// TestZeroServiceIsSafe: a service not built with NewProvidersService does not panic.
func TestZeroServiceIsSafe(t *testing.T) {
	var s ProvidersService
	_, err := s.PickProvider("gas", "order")
	assert.ErrorIs(t, err, ErrNoProvider)
	assert.Empty(t, s.List())
	assert.Empty(t, s.Group("gas", "order"))
	assert.ErrorIs(t, s.MoveUp(1), ErrUnknownProvider)
	assert.ErrorIs(t, s.Enable(1), ErrUnknownProvider)
	assert.ErrorIs(t, s.Load(context.Background()), errNotBuilt)
	assert.ErrorIs(t, s.Start(context.Background()), errNotBuilt)
	assert.NoError(t, s.Stop(context.Background()))
}

// TestSortIsDeterministic: the same rows in many shuffled input orders give one identical sorted
// result, equal to the reference sort of the test (many fake rows and the 45 seed rows).
func TestSortIsDeterministic(t *testing.T) {
	for name, rows := range map[string][]Provider{"many": manyRows(), "seed": seedRows(t)} {
		t.Run(name, func(t *testing.T) {
			if name == "many" {
				assert.GreaterOrEqual(t, len(rows), 200)
				assert.GreaterOrEqual(t, len(keysOf(rows)), 10)
			}
			want := refSort(rows)
			r := rand.New(rand.NewPCG(7, 11))
			repo, log := &fakeRepo{}, &fakeLog{}
			s := newTestService(t, repo, log)
			for i := range 60 {
				shuffled := slices.Clone(rows)
				r.Shuffle(len(shuffled), func(a, b int) { shuffled[a], shuffled[b] = shuffled[b], shuffled[a] })
				if i == 1 {
					slices.Reverse(shuffled)
				}
				repo.set(shuffled, nil)
				if !assert.NoError(t, s.Load(context.Background())) {
					return
				}
				got := s.List()
				if !assert.Equal(t, want, got, "shuffle %d", i) {
					return
				}
				for _, k := range keysOf(rows) {
					assert.Equal(t, refGroup(want, k[0], k[1]), s.Group(k[0], k[1]))
				}
			}
		})
	}
}

// TestSeedOrder pins the order of the seed groups with more than one provider, as the reference
// sort and the task 020 Verification expect.
func TestSeedOrder(t *testing.T) {
	s, _, _ := loaded(t, seedRows(t))
	for _, c := range []struct {
		billType, action string
		order            []int64
		pick             int64
	}{
		{"gas", "inquiry", []int64{43, 99, 35}, 43},
		{"telmidterm", "order", []int64{51, 20, 600}, 51},
		{"telmidterm", "inquiry", []int64{123456, 42, 53}, 123456},
		{"tel", "inquiry", []int64{40, 124325, 52}, 124325},
		{"mobile", "order", []int64{48, 30}, 0},
	} {
		assert.Equal(t, c.order, ids(s.Group(c.billType, c.action)), c.billType+"/"+c.action)
		p, err := s.PickProvider(c.billType, c.action)
		if c.pick == 0 {
			assert.ErrorIs(t, err, ErrNoProvider, "every provider of the group is disabled")
			continue
		}
		assert.NoError(t, err)
		assert.Equal(t, c.pick, p.ID)
	}
}

// TestPickProvider: for each group the first enabled provider, the same on every call; the error
// for a group with no enabled provider, an unknown group and an empty group.
func TestPickProvider(t *testing.T) {
	rows := manyRows()
	s, _, _ := loaded(t, rows)
	want := refSort(rows)
	checked := 0
	for _, k := range keysOf(rows) {
		ref, ok := refPick(refGroup(want, k[0], k[1]))
		for range 2000 {
			p, err := s.PickProvider(k[0], k[1])
			if !ok {
				if !assert.ErrorIs(t, err, ErrNoProvider, k) {
					return
				}
				continue
			}
			if !assert.NoError(t, err) || !assert.Equal(t, ref, p, k) {
				return
			}
		}
		checked++
	}
	assert.Equal(t, len(keysOf(rows)), checked)

	_, err := s.PickProvider("deadtype", "order")
	assert.ErrorIs(t, err, ErrNoProvider, "a group with no enabled provider")
	assert.NotEmpty(t, s.Group("deadtype", "order"), "the group has providers")
	_, err = s.PickProvider("nosuchtype", "order")
	assert.ErrorIs(t, err, ErrNoProvider, "an unknown group")
	_, err = s.PickProvider("gas", "")
	assert.ErrorIs(t, err, ErrNoProvider, "an empty action")
	assert.Equal(t, []Provider{}, s.Group("nosuchtype", "order"), "an empty group is an empty slice")
}

func TestPickProviderSkipsDisabledAndNullStatus(t *testing.T) {
	s, _, _ := loaded(t, []Provider{
		{ID: 1, BillType: "gas", Action: "order", Priority: 1, Status: StatusDisable},
		{ID: 2, BillType: "gas", Action: "order", Priority: 2, Status: ""},
		{ID: 3, BillType: "gas", Action: "order", Priority: 3, Status: "Enable"},
		{ID: 4, BillType: "gas", Action: "order", Priority: 4, Status: StatusEnable},
		{ID: 5, BillType: "gas", Action: "order", Priority: 5, Status: StatusEnable},
	})
	p, err := s.PickProvider("gas", "order")
	assert.NoError(t, err)
	assert.Equal(t, int64(4), p.ID, "only the exact value 'enable' is enabled")
	assert.True(t, p.Enabled())
	assert.False(t, Provider{Status: "Enable"}.Enabled())
}

func TestPickProviderDoesNotAllocate(t *testing.T) {
	s, _, _ := loaded(t, seedRows(t))
	allocs := testing.AllocsPerRun(1000, func() {
		_, _ = s.PickProvider("gas", "inquiry")
		_, _ = s.PickProvider("mobile", "order")
		_, _ = s.PickProvider("nosuchtype", "order")
	})
	assert.Zero(t, allocs)
}

// TestReturnedValuesAreCopies: a caller that changes a returned list or provider does not change
// the service.
func TestReturnedValuesAreCopies(t *testing.T) {
	s, _, _ := loaded(t, seedRows(t))
	before := s.List()
	list := s.List()
	for i := range list {
		list[i].Status, list[i].TmpPriority, list[i].ID = "x", -1, 0
	}
	slices.Reverse(list)
	g := s.Group("gas", "inquiry")
	if !assert.NotEmpty(t, g) {
		return
	}
	g[0].Status, g[0].TmpPriority = StatusDisable, 99
	g = append(g[:0], Provider{ID: 7})
	_ = g
	p, err := s.PickProvider("gas", "inquiry")
	if !assert.NoError(t, err) {
		return
	}
	p.Status = StatusDisable
	assert.Equal(t, before, s.List())
	p2, err := s.PickProvider("gas", "inquiry")
	assert.NoError(t, err)
	assert.Equal(t, int64(43), p2.ID)
	assert.True(t, p2.Enabled())
}

// moveRows: one group with ties (most priorities 10), one group of one provider.
func moveRows() []Provider {
	return []Provider{
		{ID: 50, BillType: "gas", Action: "order", Priority: 10, Status: StatusEnable},
		{ID: 20, BillType: "gas", Action: "order", Priority: 10, Status: StatusEnable},
		{ID: 30, BillType: "gas", Action: "order", Priority: 4, Status: StatusDisable},
		{ID: 40, BillType: "gas", Action: "order", Priority: 10, Status: StatusEnable},
		{ID: 10, BillType: "gas", Action: "order", Priority: 99, Status: StatusEnable},
		{ID: 9, BillType: "water", Action: "order", Priority: 1, Status: StatusEnable},
	}
}

// TestMoveEachPlace: MoveUp and MoveDown from each place of a group with ties. The provider moves
// exactly one place; the group's TmpPriority values are 0..n-1 in the old order with the two
// neighbors swapped (operator decision 5); the result is a pure sort by (TmpPriority, ID); the
// first up and the last down change nothing; other groups do not change.
func TestMoveEachPlace(t *testing.T) {
	start := []int64{30, 20, 40, 50, 10} // priority 4, then the three 10s by id, then 99
	for i := range start {
		for _, step := range []int{-1, 1} {
			s, _, _ := loaded(t, moveRows())
			if !assert.Equal(t, start, ids(s.Group("gas", "order"))) {
				return
			}
			water := s.Group("water", "order")
			var err error
			if step < 0 {
				err = s.MoveUp(start[i])
			} else {
				err = s.MoveDown(start[i])
			}
			if !assert.NoError(t, err) {
				return
			}
			got := s.Group("gas", "order")
			j := i + step
			if j < 0 || j >= len(start) {
				assert.Equal(t, start, ids(got), "place %d step %d: no change", i, step)
				assert.Equal(t, []int{4, 10, 10, 10, 99}, tmps(got), "no change keeps the loaded values")
				continue
			}
			want := slices.Clone(start)
			want[i], want[j] = want[j], want[i]
			assert.Equal(t, want, ids(got), "place %d step %d", i, step)
			assert.Equal(t, []int{0, 1, 2, 3, 4}, tmps(got), "values 0..n-1, the two neighbors swapped")
			assert.True(t, isPureSort(s.List()))
			assert.Equal(t, water, s.Group("water", "order"))
			for _, p := range got {
				assert.Contains(t, []int{4, 10, 99}, p.Priority, "the stored priority does not change")
			}
		}
	}
}

func tmps(ps []Provider) []int {
	out := make([]int, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.TmpPriority)
	}
	return out
}

// TestMoveTiesAndUnknown: a swap of two equal neighbors really moves the provider; a single
// provider and an unknown id.
func TestMoveTiesAndUnknown(t *testing.T) {
	s, _, _ := loaded(t, moveRows())
	assert.NoError(t, s.MoveUp(40)) // 40 and 20 both have 10
	assert.Equal(t, []int64{30, 40, 20, 50, 10}, ids(s.Group("gas", "order")))
	assert.NoError(t, s.MoveUp(40))
	assert.NoError(t, s.MoveUp(40))
	assert.Equal(t, []int64{40, 30, 20, 50, 10}, ids(s.Group("gas", "order")))
	p, err := s.PickProvider("gas", "order")
	assert.NoError(t, err)
	assert.Equal(t, int64(40), p.ID)

	assert.NoError(t, s.MoveUp(9), "the only provider of its group")
	assert.NoError(t, s.MoveDown(9))
	assert.Equal(t, []int64{9}, ids(s.Group("water", "order")))

	before := s.List()
	assert.ErrorIs(t, s.MoveUp(12345), ErrUnknownProvider)
	assert.ErrorIs(t, s.MoveDown(-1), ErrUnknownProvider)
	assert.ErrorIs(t, s.Enable(12345), ErrUnknownProvider)
	assert.ErrorIs(t, s.Disable(0), ErrUnknownProvider)
	assert.Equal(t, before, s.List())
}

func TestEnableDisable(t *testing.T) {
	s, _, _ := loaded(t, moveRows())
	p, _ := s.PickProvider("gas", "order")
	assert.Equal(t, int64(20), p.ID, "30 is first but disabled")

	assert.NoError(t, s.Enable(30))
	p, _ = s.PickProvider("gas", "order")
	assert.Equal(t, int64(30), p.ID)
	assert.NoError(t, s.Enable(30), "repeatable")
	assert.Equal(t, []int64{30, 20, 40, 50, 10}, ids(s.Group("gas", "order")), "a status change does not move")

	for _, id := range []int64{30, 20, 40, 50} {
		assert.NoError(t, s.Disable(id))
		assert.NoError(t, s.Disable(id), "repeatable")
	}
	p, _ = s.PickProvider("gas", "order")
	assert.Equal(t, int64(10), p.ID)
	assert.NoError(t, s.Disable(10))
	_, err := s.PickProvider("gas", "order")
	assert.ErrorIs(t, err, ErrNoProvider)
	assert.Len(t, s.Group("gas", "order"), 5, "disabled providers stay in the list")
}

func TestChangeCallback(t *testing.T) {
	var got [][2]string
	repo := &fakeRepo{}
	repo.set(moveRows(), nil)
	s, err := NewProvidersService(repo, Options{Interval: func() time.Duration { return time.Hour }, Logger: &fakeLog{},
		OnChange: func(change, result string) { got = append(got, [2]string{change, result}) }})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) || !assert.NoError(t, s.Load(context.Background())) {
		return
	}
	_ = s.MoveUp(20)
	_ = s.MoveUp(20)
	_ = s.MoveDown(10)
	_ = s.MoveDown(77)
	_ = s.Enable(30)
	_ = s.Enable(30)
	_ = s.Disable(30)
	_ = s.Disable(77)
	assert.Equal(t, [][2]string{
		{ChangeMoveUp, ResultChanged}, {ChangeMoveUp, ResultUnchanged}, {ChangeMoveDown, ResultUnchanged},
		{ChangeMoveDown, ResultUnknownID}, {ChangeEnable, ResultChanged}, {ChangeEnable, ResultUnchanged},
		{ChangeDisable, ResultChanged}, {ChangeDisable, ResultUnknownID},
	}, got)
}

// model is the reference model of the random sequence test: the order of ids of each group and the
// status of each id.
type model struct {
	order  map[[2]string][]int64
	status map[int64]string
	group  map[int64][2]string
}

func newModel(rows []Provider) *model {
	m := &model{order: map[[2]string][]int64{}, status: map[int64]string{}, group: map[int64][2]string{}}
	for _, p := range refSort(rows) {
		k := [2]string{p.BillType, p.Action}
		m.order[k] = append(m.order[k], p.ID)
		m.status[p.ID] = p.Status
		m.group[p.ID] = k
	}
	return m
}

// move swaps id with its neighbor; it reports false for an unknown id. It stops the test if the
// model's group of id does not hold id (a broken model), so it never indexes with -1.
func (m *model) move(t testing.TB, id int64, step int) bool {
	t.Helper()
	k, ok := m.group[id]
	if !ok {
		return false
	}
	o := m.order[k]
	i := slices.Index(o, id)
	if !assert.GreaterOrEqual(t, i, 0, "model: id %d is in its group", id) {
		t.FailNow()
	}
	if j := i + step; j >= 0 && j < len(o) {
		o[i], o[j] = o[j], o[i]
	}
	return true
}

func (m *model) set(id int64, status string) bool {
	if _, ok := m.group[id]; !ok {
		return false
	}
	m.status[id] = status
	return true
}

// check compares the service with the model: each group's ids and status, the pick, a pure sort.
func (m *model) check(t *testing.T, s *ProvidersService, step int) bool {
	t.Helper()
	if !assert.True(t, isPureSort(s.List()), "step %d: pure sort", step) {
		return false
	}
	for k, o := range m.order {
		g := s.Group(k[0], k[1])
		if !assert.Equal(t, o, ids(g), "step %d group %v", step, k) {
			return false
		}
		var want int64
		for _, p := range g {
			if !assert.Equal(t, m.status[p.ID], p.Status, "step %d id %d", step, p.ID) {
				return false
			}
			if want == 0 && m.status[p.ID] == StatusEnable {
				want = p.ID
			}
		}
		p, err := s.PickProvider(k[0], k[1])
		if want == 0 {
			if !assert.ErrorIs(t, err, ErrNoProvider, "step %d group %v", step, k) {
				return false
			}
		} else if !assert.NoError(t, err) || !assert.Equal(t, want, p.ID, "step %d group %v", step, k) {
			return false
		}
	}
	return true
}

// TestRandomMovesAndToggles: long random sequences (fixed seeds) of moves and toggles, compared step
// by step with the reference model.
func TestRandomMovesAndToggles(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3} {
		rows := manyRows()
		s, _, _ := loaded(t, rows)
		m := newModel(rows)
		all := ids(rows)
		if !assert.NotEmpty(t, all) {
			return
		}
		r := rand.New(rand.NewPCG(seed, 99))
		for step := range 3000 {
			id := all[r.IntN(len(all))]
			if r.IntN(20) == 0 {
				id = -id // unknown
			}
			var err error
			var known bool
			switch r.IntN(4) {
			case 0:
				err, known = s.MoveUp(id), m.move(t, id, -1)
			case 1:
				err, known = s.MoveDown(id), m.move(t, id, 1)
			case 2:
				err, known = s.Enable(id), m.set(id, StatusEnable)
			default:
				err, known = s.Disable(id), m.set(id, StatusDisable)
			}
			if known {
				if !assert.NoError(t, err, "step %d", step) {
					return
				}
			} else if !assert.ErrorIs(t, err, ErrUnknownProvider, "step %d", step) {
				return
			}
			if step%10 == 0 || step > 2900 {
				if !m.check(t, s, step) {
					return
				}
			}
		}
	}
}

// TestReloadDatabaseWins: runtime moves and toggles are gone after a load.
func TestReloadDatabaseWins(t *testing.T) {
	rows := seedRows(t)
	s, _, _ := loaded(t, rows)
	want := s.List()
	assert.NoError(t, s.MoveDown(43))
	assert.NoError(t, s.Enable(35))
	assert.NoError(t, s.Disable(51))
	assert.NoError(t, s.MoveUp(600))
	assert.NotEqual(t, want, s.List())
	assert.NoError(t, s.Load(context.Background()))
	assert.Equal(t, want, s.List())
	assert.Equal(t, refSort(rows), s.List())
}

// TestReloadChanges: a load with no change keeps the order and writes no debug line; a changed
// priority, a changed status, a new row and a removed row each change exactly the expected groups,
// with one debug line for each changed group.
func TestReloadChanges(t *testing.T) {
	base := seedRows(t)
	// Each case edits a copy of base inside its own subtest, so a lookup of a missing id stops
	// only that subtest (indexOf), never the package binary.
	cases := map[string]struct {
		edit    func(t *testing.T, r []Provider) []Provider
		changed [][2]string
	}{
		"no change": {edit: func(_ *testing.T, r []Provider) []Provider { return r }},
		"priority": {edit: func(t *testing.T, r []Provider) []Provider {
			r[indexOf(t, r, 35)].Priority = 1 // qabzino gas inquiry moves to the top
			return r
		}, changed: [][2]string{{"gas", "inquiry"}}},
		"status": {edit: func(t *testing.T, r []Provider) []Provider {
			r[indexOf(t, r, 43)].Status = StatusDisable
			return r
		}, changed: [][2]string{{"gas", "inquiry"}}},
		"new row": {edit: func(_ *testing.T, r []Provider) []Provider {
			return append(r, Provider{ID: 700, BillType: "telmidterm", Action: "order", Priority: 9, Status: StatusEnable},
				Provider{ID: 701, BillType: "newtype", Action: "order", Priority: 1, Status: StatusEnable})
		}, changed: [][2]string{{"telmidterm", "order"}, {"newtype", "order"}}},
		"removed row": {edit: func(t *testing.T, r []Provider) []Provider {
			i := indexOf(t, r, 51)
			r = slices.Delete(r, i, i+1)
			j := indexOf(t, r, 2) // the only electricity inquiry
			return slices.Delete(r, j, j+1)
		}, changed: [][2]string{{"telmidterm", "order"}, {"electricity", "inquiry"}}},
		"priority that keeps the order": {edit: func(t *testing.T, r []Provider) []Provider {
			r[indexOf(t, r, 35)].Priority = 11
			return r
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rows := c.edit(t, slices.Clone(base))
			s, repo, log := loaded(t, base)
			before := map[[2]string][]Provider{}
			for _, k := range keysOf(base) {
				before[k] = s.Group(k[0], k[1])
			}
			log.reset()
			repo.set(rows, nil)
			if !assert.NoError(t, s.Load(context.Background())) {
				return
			}
			assert.Equal(t, refSort(rows), s.List())
			var lines [][2]string
			for _, l := range log.byLevel("debug") {
				assert.Equal(t, "providers: group order", l.msg)
				bt, _ := l.kv["bill_type"].(string)
				ac, _ := l.kv["action"].(string)
				lines = append(lines, [2]string{bt, ac})
			}
			assert.ElementsMatch(t, c.changed, lines, "one debug line per changed group")
			for k, g := range before {
				if slices.Contains(c.changed, k) {
					assert.NotEqual(t, orderOf(g), orderOf(s.Group(k[0], k[1])), "changed group %v", k)
					continue
				}
				assert.Equal(t, orderOf(g), orderOf(s.Group(k[0], k[1])), "unchanged group %v", k)
			}
		})
	}
}

// indexOf returns the index of the provider id in rows. It stops the test when rows has no such
// id, so the caller never indexes with -1 (conventions § 4).
func indexOf(t *testing.T, rows []Provider, id int64) int {
	t.Helper()
	i := slices.IndexFunc(rows, func(p Provider) bool { return p.ID == id })
	if !assert.GreaterOrEqual(t, i, 0, "provider id %d is in the rows", id) {
		t.FailNow()
	}
	return i
}

// orderOf returns "id:status" for each provider of g, in order.
func orderOf(g []Provider) []string {
	out := make([]string, 0, len(g))
	for _, p := range g {
		out = append(out, strconv.FormatInt(p.ID, 10)+":"+p.Status)
	}
	return out
}
