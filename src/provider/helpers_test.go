package provider

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// fakeRepo answers ListProviders with a copy of rows, or with err. When block is not nil, each call
// waits on it first (a load held in its query). It counts the calls. Safe for concurrent use.
type fakeRepo struct {
	mu    sync.Mutex
	rows  []Provider
	err   error
	block chan struct{}
	calls int
}

func (f *fakeRepo) set(rows []Provider, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rows, f.err = append([]Provider(nil), rows...), err
}

func (f *fakeRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

func (f *fakeRepo) ListProviders(ctx context.Context) ([]Provider, error) {
	f.mu.Lock()
	block := f.block
	f.calls++
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	return append([]Provider(nil), f.rows...), nil
}

// logLine is one recorded log call.
type logLine struct {
	level string
	msg   string
	kv    map[string]any
}

// fakeLog records every line. Safe for concurrent use.
type fakeLog struct {
	mu    sync.Mutex
	lines []logLine
}

func (l *fakeLog) add(level, msg string, kv []any) {
	m := map[string]any{}
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			m[k] = kv[i+1]
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, logLine{level: level, msg: msg, kv: m})
}

func (l *fakeLog) Debug(msg string, kv ...any) { l.add("debug", msg, kv) }
func (l *fakeLog) Warn(msg string, kv ...any)  { l.add("warn", msg, kv) }
func (l *fakeLog) Error(msg string, kv ...any) { l.add("error", msg, kv) }

// byLevel returns the lines of one level.
func (l *fakeLog) byLevel(level string) []logLine {
	l.mu.Lock()
	defer l.mu.Unlock()
	var out []logLine
	for _, x := range l.lines {
		if x.level == level {
			out = append(out, x)
		}
	}
	return out
}

func (l *fakeLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = nil
}

// newTestService returns a service on repo with a 1-hour interval. It stops the test on an error.
func newTestService(t testing.TB, repo Repository, log Logger) *ProvidersService {
	t.Helper()
	s, err := NewProvidersService(repo, Options{Interval: func() time.Duration { return time.Hour }, Logger: log})
	if !assert.NoError(t, err) || !assert.NotNil(t, s) {
		t.FailNow()
	}
	return s
}

// loaded returns a service that has loaded rows. It stops the test on an error.
func loaded(t testing.TB, rows []Provider) (*ProvidersService, *fakeRepo, *fakeLog) {
	t.Helper()
	repo, log := &fakeRepo{}, &fakeLog{}
	repo.set(rows, nil)
	s := newTestService(t, repo, log)
	if !assert.NoError(t, s.Load(context.Background())) {
		t.FailNow()
	}
	return s, repo, log
}

// seedTuple matches one row of seed_providers.sql: (id, 'name', 'subject', 'bill_type', 'action',
// 'type' or NULL, priority, timeout, 'status', 'updated_at', use_nats).
var seedTuple = regexp.MustCompile(`^\((\d+), '([^']*)', '([^']*)', '([^']*)', '([^']*)', (NULL|'[^']*'), (-?\d+), (\d+), '([^']*)', '([^']*)', (true|false)\),?$`)

// seedRows reads the 45 seed rows of the dev database (task 015) from the devdb seed SQL, so the
// test uses the same rows as `devdb seed`. It stops the test if the file does not parse.
func seedRows(t testing.TB) []Provider {
	t.Helper()
	data, err := os.ReadFile("../system/postgres/devdb/seed_providers.sql")
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	var out []Provider
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "(") {
			continue
		}
		m := seedTuple.FindStringSubmatch(line)
		if !assert.Len(t, m, 12, line) {
			t.FailNow()
		}
		id, err1 := strconv.ParseInt(m[1], 10, 64)
		prio, err2 := strconv.Atoi(m[7])
		timeout, err3 := strconv.Atoi(m[8])
		updated, err4 := time.Parse(time.RFC3339Nano, m[10])
		if !assert.NoError(t, err1) || !assert.NoError(t, err2) || !assert.NoError(t, err3) || !assert.NoError(t, err4) {
			t.FailNow()
		}
		typ := ""
		if m[6] != "NULL" {
			typ = strings.Trim(m[6], "'")
		}
		out = append(out, Provider{ID: id, Name: m[2], Subject: m[3], BillType: m[4], Action: m[5], Type: typ,
			Priority: prio, Timeout: timeout, Status: m[9], UpdatedAt: updated, UseNats: m[11] == "true", InquiryStep: 1})
	}
	if !assert.Len(t, out, 45, "the 45 seed rows") {
		t.FailNow()
	}
	return out
}

// manyRows returns at least 200 providers in at least 10 groups: equal priorities, gaps, negative
// and large values, disabled providers, NULL status, and a group with no enabled provider
// ("deadtype"/"order"). Ids are unique and not in id order of the groups.
func manyRows() []Provider {
	r := rand.New(rand.NewPCG(20, 2026))
	types := []string{"gas", "water", "electricity", "tel", "telmidterm", "mobile", "mobilemidterm", "tax", "raw", "deadtype", "irancell"}
	actions := []string{"order", "inquiry"}
	prios := []int{10, 10, 10, 9, 1, 0, -5, -32768, 32767, 100, 10, 4, 4}
	var out []Provider
	id := int64(1000)
	for _, bt := range types {
		for _, ac := range actions {
			n := 6 + r.IntN(10)
			for range n {
				id += int64(1 + r.IntN(7))
				// Ids go up and down, so the id order is not the input order.
				pid := id
				if r.IntN(2) == 0 {
					pid = 1_000_000 - id
				}
				status := StatusEnable
				switch r.IntN(5) {
				case 0:
					status = StatusDisable
				case 1:
					if r.IntN(3) == 0 {
						status = ""
					}
				}
				if bt == "deadtype" && ac == "order" {
					status = StatusDisable
				}
				out = append(out, Provider{ID: pid, Name: fmt.Sprintf("p%d", pid), BillType: bt, Action: ac,
					Priority: prios[r.IntN(len(prios))], Status: status})
			}
		}
	}
	return out
}

// refLess is the reference order of the test, written apart from the code: bill type, action,
// then priority (the stored one; after a load it equals TmpPriority), then id.
func refLess(a, b Provider) bool {
	if a.BillType != b.BillType {
		return a.BillType < b.BillType
	}
	if a.Action != b.Action {
		return a.Action < b.Action
	}
	if a.Priority != b.Priority {
		return a.Priority < b.Priority
	}
	return a.ID < b.ID
}

// refSort returns rows in the reference order (insertion sort), with TmpPriority = Priority.
func refSort(rows []Provider) []Provider {
	out := make([]Provider, 0, len(rows))
	for _, p := range rows {
		p.TmpPriority = p.Priority
		i := len(out)
		out = append(out, p)
		for i > 0 && refLess(out[i], out[i-1]) {
			out[i], out[i-1] = out[i-1], out[i]
			i--
		}
	}
	return out
}

// refGroup returns the providers of one group in a reference-sorted list.
func refGroup(sorted []Provider, billType, action string) []Provider {
	out := []Provider{}
	for _, p := range sorted {
		if p.BillType == billType && p.Action == action {
			out = append(out, p)
		}
	}
	return out
}

// refPick returns the first enabled provider of a reference group.
func refPick(g []Provider) (Provider, bool) {
	for _, p := range g {
		if p.Status == StatusEnable {
			return p, true
		}
	}
	return Provider{}, false
}

// keysOf returns the (bill type, action) pairs of rows, each once.
func keysOf(rows []Provider) [][2]string {
	seen := map[[2]string]bool{}
	var out [][2]string
	for _, p := range rows {
		k := [2]string{p.BillType, p.Action}
		if !seen[k] {
			seen[k] = true
			out = append(out, k)
		}
	}
	return out
}

// ids returns the ids of ps in order.
func ids(ps []Provider) []int64 {
	out := make([]int64, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

// isPureSort reports whether every group of list is sorted by (TmpPriority, ID) and the groups
// come in (bill type, action) order, each group in one block.
func isPureSort(list []Provider) bool {
	for i := 1; i < len(list); i++ {
		a, b := list[i-1], list[i]
		if a.BillType != b.BillType || a.Action != b.Action {
			if a.BillType > b.BillType || (a.BillType == b.BillType && a.Action > b.Action) {
				return false
			}
			continue
		}
		if a.TmpPriority > b.TmpPriority || (a.TmpPriority == b.TmpPriority && a.ID >= b.ID) {
			return false
		}
	}
	return true
}
