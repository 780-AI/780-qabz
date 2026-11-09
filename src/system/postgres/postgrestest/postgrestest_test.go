package postgrestest

import (
	"context"
	"io"
	"net/url"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"qabz/system/postgres"
)

// TestServicesSchema needs a real Postgres (QABZ_TEST_RUN_DSN, make test). It checks that the helper makes
// an empty services table in its own schema, and that the schema is gone after the test.
func TestServicesSchema(t *testing.T) {
	dsn := RunDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var schema string
	t.Run("create", func(t *testing.T) {
		schemaDSN := ServicesSchema(t, ctx, dsn)
		u, err := url.Parse(schemaDSN)
		if !assert.NoError(t, err) || !assert.NotNil(t, u) {
			return
		}
		schema = u.Query().Get("search_path")
		assert.True(t, strings.HasPrefix(schema, "qabz_test_"), schema)

		c, err := postgres.New(ctx, schemaDSN)
		if !assert.NoError(t, err) || !assert.NotNil(t, c) {
			return
		}
		defer c.Close()
		var current string
		var rows int
		assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT current_schema()`).Scan(&current))
		assert.Equal(t, schema, current)
		assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM services`).Scan(&rows))
		assert.Equal(t, 0, rows)
		_, err = c.Pool().Exec(ctx, `INSERT INTO services (id, type) VALUES (1, 'water')`) // the defaults fill the rest
		assert.NoError(t, err)
	})
	if schema == "" {
		return
	}

	// The cleanup of the sub-test dropped the schema.
	c, err := postgres.New(ctx, dsn)
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	defer c.Close()
	var n int
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM information_schema.schemata WHERE schema_name = $1`, schema).Scan(&n))
	assert.Equal(t, 0, n)
}

// TestBillsSchema needs a real Postgres (QABZ_TEST_RUN_DSN, make test). It checks that the helper makes an
// empty bills table with bill's defaults and its UNIQUE (user_id, type, bill_id) key.
func TestBillsSchema(t *testing.T) {
	dsn := RunDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := postgres.New(ctx, BillsSchema(t, ctx, dsn))
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		t.FailNow()
	}
	defer c.Close()

	var rows int
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT count(*) FROM bills`).Scan(&rows))
	assert.Equal(t, 0, rows)
	const insert = `INSERT INTO bills (id, user_id, type, bill_id) VALUES (gen_random_uuid(), '6f1c1e0a-3b7e-4c2b-9a51-2f0f6d1d8c11', 'water', '1234567')`
	_, err = c.Pool().Exec(ctx, insert)
	if !assert.NoError(t, err) {
		return
	}
	var status string
	var hasCreated bool
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT status, created_at IS NOT NULL FROM bills`).Scan(&status, &hasCreated))
	assert.Equal(t, "enable", status)
	assert.True(t, hasCreated)
	_, err = c.Pool().Exec(ctx, insert) // the same user, type and bill_id
	assert.ErrorContains(t, err, "23505")
}

// TestSeededSchema needs a real Postgres (QABZ_TEST_RUN_DSN, make test). It checks that the helper makes
// the devdb bill tables with the 45 seed providers in its own schema.
func TestSeededSchema(t *testing.T) {
	dsn := RunDSN(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	schemaDSN := SeededSchema(t, ctx, dsn)
	c, err := postgres.New(ctx, schemaDSN)
	if !assert.NoError(t, err) || !assert.NotNil(t, c) {
		return
	}
	defer c.Close()
	var current string
	var providers, services int
	assert.NoError(t, c.Pool().QueryRow(ctx, `SELECT current_schema(), (SELECT count(*) FROM providers), (SELECT count(*) FROM services)`).Scan(&current, &providers, &services))
	assert.True(t, strings.HasPrefix(current, "qabz_test_"), current)
	assert.Equal(t, 45, providers)
	assert.Equal(t, 5, services)
}

func TestRunDBName(t *testing.T) {
	t.Setenv("PGDATABASE", "") // a DSN with no database name must not take one from the environment
	good := map[string]string{
		"postgres://qabz:pw@127.0.0.1:55432/qabz_test_0123abcd":                     "qabz_test_0123abcd",
		"postgresql://qabz@127.0.0.1/qabz_test_x?sslmode=disable&search_path=s":     "qabz_test_x",
		"postgres://qabz:p%40ss@127.0.0.1:55432/qabz_test_0123abcd?search_path=abc": "qabz_test_0123abcd",
	}
	for dsn, want := range good {
		got, err := RunDBName(dsn)
		assert.NoError(t, err, dsn)
		assert.Equal(t, want, got, dsn)
	}
	for _, dsn := range []string{
		"postgres://qabz:secret@127.0.0.1:55432/qabz", // the development database
		"postgres://qabz:secret@127.0.0.1:55432/qabz_test_",
		"postgres://qabz:secret@127.0.0.1:55432/",
		"postgres://qabz:secret@127.0.0.1:55432/postgres",
		"postgres://qabz:secret@127.0.0.1:55432/my_qabz_test_1",
		"mysql://qabz:secret@127.0.0.1:3306/qabz_test_1",
		"host=127.0.0.1 user=qabz password=secret dbname=qabz_test_1",
		"postgres://qabz:secret@127.0.0.1:bad port/qabz_test_1",
		"",
		// The driver connects to the database of a dbname= or database= parameter, not of the path (D1).
		"postgres://qabz:secret@127.0.0.1:55432/qabz_test_abc?dbname=qabz",
		"postgres://qabz:secret@127.0.0.1:55432/qabz_test_abc?database=qabz",
		"postgresql://qabz:secret@127.0.0.1:55432/qabz_test_abc?sslmode=disable&dbname=qabz",
		"host=127.0.0.1 user=qabz password=secret dbname=qabz",
		"postgres://qabz:secret@127.0.0.1:55432",
		"postgres://qabz:secret@127.0.0.1:55432/?dbname=",
	} {
		_, err := RunDBName(dsn)
		if assert.Error(t, err, dsn) {
			assert.NotContains(t, err.Error(), "secret", dsn)
		}
	}
	// The name the driver uses is the one checked: the override names the development database.
	_, err := RunDBName("postgres://qabz:secret@127.0.0.1:55432/qabz_test_abc?dbname=qabz")
	assert.EqualError(t, err, `database "qabz" is not a test run database (name must start with qabz_test_)`)
	// No database in the DSN: the driver takes PGDATABASE, so the guard checks PGDATABASE.
	t.Setenv("PGDATABASE", "qabz")
	_, err = RunDBName("postgres://qabz:secret@127.0.0.1:55432")
	assert.EqualError(t, err, `database "qabz" is not a test run database (name must start with qabz_test_)`)
}

// fakeTB records the failure of a helper. FailNow ends the goroutine, as testing.T does.
//
// The nil testing.TB embed is there only for the unexported method of the interface (it cannot be
// implemented outside package testing, and the testing package never calls it on a fake). fakeTB
// implements EVERY exported method of testing.TB itself, so no call reaches the nil embed;
// TestFakeTBImplementsTB calls each method of the interface and fails when a newer Go adds one that
// fakeTB does not implement. Methods that would change the process or the disk (Setenv, Chdir,
// TempDir, ArtifactDir) record an error and stop the goroutine instead.
type fakeTB struct {
	testing.TB
	mu      sync.Mutex
	errs    []string
	failed  bool
	stopped bool
	skipped bool
}

func (f *fakeTB) record(format string, args ...any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failed = true
	f.errs = append(f.errs, format)
	for _, a := range args {
		if s, ok := a.(string); ok {
			f.errs = append(f.errs, s)
		}
	}
}

func (f *fakeTB) stop() {
	f.mu.Lock()
	f.stopped = true
	f.failed = true
	f.mu.Unlock()
	runtime.Goexit()
}

func (f *fakeTB) skip() {
	f.mu.Lock()
	f.skipped = true
	f.mu.Unlock()
	runtime.Goexit()
}

func (f *fakeTB) Helper()                           {}
func (f *fakeTB) Name() string                      { return "fake" }
func (f *fakeTB) Errorf(format string, args ...any) { f.record(format, args...) }
func (f *fakeTB) Error(args ...any)                 { f.record("", args...) }
func (f *fakeTB) Fail()                             { f.mu.Lock(); f.failed = true; f.mu.Unlock() }
func (f *fakeTB) FailNow()                          { f.stop() }
func (f *fakeTB) Fatal(args ...any)                 { f.record("", args...); f.stop() }
func (f *fakeTB) Fatalf(format string, args ...any) { f.record(format, args...); f.stop() }
func (f *fakeTB) Failed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.failed
}
func (f *fakeTB) Skip(...any)          { f.skip() }
func (f *fakeTB) SkipNow()             { f.skip() }
func (f *fakeTB) Skipf(string, ...any) { f.skip() }
func (f *fakeTB) Skipped() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.skipped
}
func (f *fakeTB) Log(...any)               {}
func (f *fakeTB) Logf(string, ...any)      {}
func (f *fakeTB) Cleanup(func())           {}
func (f *fakeTB) Attr(string, string)      {}
func (f *fakeTB) Context() context.Context { return context.Background() }
func (f *fakeTB) Output() io.Writer        { return io.Discard }
func (f *fakeTB) Setenv(key, _ string)     { f.record("fakeTB: Setenv not supported", key); f.stop() }
func (f *fakeTB) Chdir(dir string)         { f.record("fakeTB: Chdir not supported", dir); f.stop() }
func (f *fakeTB) TempDir() string          { f.record("fakeTB: TempDir not supported"); f.stop(); return "" }
func (f *fakeTB) ArtifactDir() string {
	f.record("fakeTB: ArtifactDir not supported")
	f.stop()
	return ""
}

// TestFakeTBImplementsTB calls every exported method of testing.TB on a fakeTB (zero arguments) in
// its own goroutine. A method that fakeTB does not implement reaches the nil embed and panics; the
// test then fails with the method name.
func TestFakeTBImplementsTB(t *testing.T) {
	tbType := reflect.TypeOf((*testing.TB)(nil)).Elem()
	n := 0
	for i := 0; i < tbType.NumMethod(); i++ {
		m := tbType.Method(i)
		if !m.IsExported() {
			continue
		}
		n++
		f := &fakeTB{}
		fn := reflect.ValueOf(testing.TB(f)).MethodByName(m.Name)
		if !assert.True(t, fn.IsValid(), m.Name) {
			continue
		}
		args := make([]reflect.Value, fn.Type().NumIn())
		for j := range args {
			if fn.Type().IsVariadic() && j == len(args)-1 {
				args[j] = reflect.MakeSlice(fn.Type().In(j), 0, 0)
				continue
			}
			args[j] = reflect.Zero(fn.Type().In(j))
		}
		var panicked any
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { panicked = recover() }()
			if fn.Type().IsVariadic() {
				fn.CallSlice(args)
				return
			}
			fn.Call(args)
		}()
		<-done
		assert.Nil(t, panicked, "fakeTB.%s reaches the nil testing.TB embed", m.Name)
	}
	assert.Greater(t, n, 20, "testing.TB has fewer methods than expected")
}

// callStops runs fn with a fake TB in its own goroutine and returns the TB after fn ended.
func callStops(fn func(tb testing.TB)) *fakeTB {
	f := &fakeTB{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		fn(f)
	}()
	<-done
	return f
}

// TestHelpersRefuseDevelopmentDatabase is the positive control of the guard (task 020b): every
// schema helper, given the DSN of a database that is not a run database, stops the test before it
// connects. The host does not exist; a connection attempt would give a different error.
func TestHelpersRefuseDevelopmentDatabase(t *testing.T) {
	t.Setenv("PGDATABASE", "")
	ctx := context.Background()
	for _, devDSN := range []string{
		"postgres://qabz:secret@127.0.0.1:1/qabz",
		// The path names a run database, but the driver would connect to qabz (D1).
		"postgres://qabz:secret@127.0.0.1:1/qabz_test_abc?dbname=qabz",
		"postgres://qabz:secret@127.0.0.1:1/qabz_test_abc?database=qabz",
		"host=127.0.0.1 port=1 user=qabz password=secret dbname=qabz",
		"postgres://qabz:secret@127.0.0.1:1/", // no database name
	} {
		helpers := map[string]func(testing.TB){
			"ServicesSchema": func(tb testing.TB) { ServicesSchema(tb, ctx, devDSN) },
			"BillsSchema":    func(tb testing.TB) { BillsSchema(tb, ctx, devDSN) },
			"EmptySchema":    func(tb testing.TB) { EmptySchema(tb, ctx, devDSN) },
			"SeededSchema":   func(tb testing.TB) { SeededSchema(tb, ctx, devDSN) },
		}
		for name, h := range helpers {
			f := callStops(h)
			errs := strings.Join(f.errs, " ")
			assert.True(t, f.stopped, name+" "+devDSN)
			assert.Contains(t, errs, "is not a test run database", name+" "+devDSN)
			assert.NotContains(t, errs, "connect", name+" "+devDSN)
			assert.NotContains(t, errs, "secret", name+" "+devDSN)
		}
	}
}

func TestRunDSN(t *testing.T) {
	t.Run("unset skips", func(t *testing.T) {
		t.Setenv(RunDSNEnv, "")
		f := callStops(func(tb testing.TB) { RunDSN(tb) })
		assert.True(t, f.skipped)
		assert.False(t, f.stopped)
	})
	t.Run("development database stops", func(t *testing.T) {
		t.Setenv(RunDSNEnv, "postgres://qabz:secret@127.0.0.1:55432/qabz")
		f := callStops(func(tb testing.TB) { RunDSN(tb) })
		assert.True(t, f.stopped)
		assert.False(t, f.skipped)
		assert.NotContains(t, strings.Join(f.errs, " "), "secret")
	})
	t.Run("run database passes", func(t *testing.T) {
		const dsn = "postgres://qabz:pw@127.0.0.1:55432/qabz_test_0123abcd"
		t.Setenv(RunDSNEnv, dsn)
		var got string
		f := callStops(func(tb testing.TB) { got = RunDSN(tb) })
		assert.False(t, f.stopped)
		assert.False(t, f.skipped)
		assert.Equal(t, dsn, got)
	})
}
