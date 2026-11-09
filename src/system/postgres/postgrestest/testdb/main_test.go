package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"qabz/system/postgres/postgrestest"
)

// env returns a getenv function that reads m.
func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestRunUsage(t *testing.T) {
	for _, args := range [][]string{
		nil, {"bogus"}, {"drop"}, {"create"}, {"admin-dsn"}, {"name", "x"}, {"clean", "x"},
		{"drop", "qabz_test_ab", "x"}, {"create", "qabz_test_ab", "x"},
	} {
		var out, errOut bytes.Buffer
		code := run(context.Background(), args, env(nil), &out, &errOut)
		assert.Equal(t, 2, code, "%v", args)
		assert.Empty(t, out.String(), "%v", args)
		assert.Contains(t, errOut.String(), "usage: testdb", "%v", args)
	}
}

// specialPassword has every character that broke or could break the old shell sourcing (M3). The
// unit tests parse it only; no role ever has it (the live test uses randomPassword).
const specialPassword = "p'a\"s`x$y@h/z:w q"

// randomPassword returns the characters of specialPassword and 16 random hex digits (crypto/rand):
// a new password for each run, so no usable password is in the repository (decision 7).
func randomPassword(t *testing.T) string {
	t.Helper()
	b := make([]byte, 8)
	if _, err := rand.Read(b); !assert.NoError(t, err) {
		t.FailNow()
	}
	return specialPassword + hex.EncodeToString(b)
}

// writeEnv writes content to a new env file and returns its path.
func writeEnv(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config_generated.env")
	if !assert.NoError(t, os.WriteFile(path, []byte(content), 0o600)) {
		t.FailNow()
	}
	return path
}

// generatorQuote quotes v the way scripts/yml_to_env (quote) writes it: plain, single-quoted, or
// double-quoted with \\, \", \n, \r and \$ escaped when v has ' or a line break.
func generatorQuote(v string) string {
	if regexp.MustCompile(`^[A-Za-z0-9_@%+=:,./-]*$`).MatchString(v) {
		return v
	}
	if strings.ContainsAny(v, "'\n\r") {
		r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`, "\r", `\r`, "$", `\$`)
		return `"` + r.Replace(v) + `"`
	}
	return "'" + v + "'"
}

// envFile returns the POSTGRES_* lines of config_generated.env for the given values.
func envFile(host, port, user, password string) string {
	return "# AUTO-GENERATED\nMODE=dev\nLOGS_OTLP_ENDPOINT=\n" +
		"POSTGRES_HOST=" + generatorQuote(host) + "\n" +
		"POSTGRES_PORT=" + generatorQuote(port) + "\n" +
		"POSTGRES_USER=" + generatorQuote(user) + "\n" +
		"POSTGRES_PASSWORD=" + generatorQuote(password) + "\n" +
		"POSTGRES_DB=qabz\n"
}

func TestGeneratorQuoteFixture(t *testing.T) {
	// The exact lines that scripts/yml_to_env writes for these values (checked by hand, task 020b).
	assert.Equal(t, `"p'a\"s`+"`"+`x\$y@h/z:w q"`, generatorQuote(specialPassword))
	assert.Equal(t, `'a"b`+"`"+`c$d @/:e'`, generatorQuote("a\"b`c$d @/:e"))
	assert.Equal(t, `p@ss/w:rd`, generatorQuote("p@ss/w:rd"))
}

func TestAdminDSN(t *testing.T) {
	for _, password := range []string{specialPassword, "a\"b`c$d @/:e", "p@ss/w:rd", "", "back\\slash'"} {
		path := writeEnv(t, envFile("127.0.0.1", "55432", "qabz", password))
		var out, errOut bytes.Buffer
		code := run(context.Background(), []string{"admin-dsn", path}, env(nil), &out, &errOut)
		if !assert.Equal(t, 0, code, errOut.String()) {
			continue
		}
		assert.Empty(t, errOut.String())
		// The driver reads back the same password; the admin DSN names the maintenance database,
		// not POSTGRES_DB.
		cfg, err := pgconn.ParseConfig(strings.TrimSpace(out.String()))
		if !assert.NoError(t, err, password) || !assert.NotNil(t, cfg) {
			continue
		}
		assert.Equal(t, password, cfg.Password)
		assert.Equal(t, "qabz", cfg.User)
		assert.Equal(t, "127.0.0.1", cfg.Host)
		assert.Equal(t, uint16(55432), cfg.Port)
		assert.Equal(t, "postgres", cfg.Database)
	}
	path := writeEnv(t, envFile("127.0.0.1", "55432", "qabz", "p@ss/w:rd"))
	var out, errOut bytes.Buffer
	assert.Equal(t, 0, run(context.Background(), []string{"admin-dsn", path}, env(nil), &out, &errOut))
	assert.Equal(t, "postgres://qabz:p%40ss%2Fw%3Ard@127.0.0.1:55432/postgres\n", out.String())
}

func TestAdminDSNMissingVariables(t *testing.T) {
	path := writeEnv(t, "POSTGRES_PORT=55432\nPOSTGRES_PASSWORD=secret\n")
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"admin-dsn", path}, env(nil), &out, &errOut)
	assert.Equal(t, 1, code)
	assert.Empty(t, out.String())
	assert.Contains(t, errOut.String(), "POSTGRES_HOST, POSTGRES_USER not set")
	assert.NotContains(t, errOut.String(), "secret")
}

func TestParseEnvFileErrors(t *testing.T) {
	cases := map[string]string{
		"not a KEY=VALUE line":                 "POSTGRES_PASSWORD secret\n",
		"line 1: not a KEY":                    "=secret\n",
		"(POSTGRES_PASSWORD): bad single":      "POSTGRES_PASSWORD='secret\n",
		"line 2 (POSTGRES_PASSWORD): bad sing": "A=1\nPOSTGRES_PASSWORD='sec'ret'\n",
		"bad double-quoted value":              "POSTGRES_PASSWORD=\"secret\n",
		"bad escape":                           "POSTGRES_PASSWORD=\"sec\\tret\"\n",
		"(POSTGRES_PASSWORD): bad double":      "POSTGRES_PASSWORD=\"sec\"ret\"\n",
		"bad double-quoted":                    "POSTGRES_PASSWORD=\"secret\\\"\n",
	}
	for want, content := range cases {
		_, err := parseEnvFile(writeEnv(t, content))
		if assert.Error(t, err, want) {
			assert.Contains(t, err.Error(), want)
			assert.NotContains(t, err.Error(), "secret", want)
		}
	}
	_, err := parseEnvFile(filepath.Join(t.TempDir(), "missing.env"))
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "no such file or directory (run make env)")
	}
	var out, errOut bytes.Buffer
	assert.Equal(t, 1, run(context.Background(), []string{"admin-dsn", filepath.Join(t.TempDir(), "x.env")}, env(nil), &out, &errOut))
	assert.Contains(t, errOut.String(), "testdb: admin-dsn failed: read the env file")
}

func TestParseEnvFileValues(t *testing.T) {
	vals, err := parseEnvFile(writeEnv(t, "# comment\r\n\nA=plain\r\nB='a b\"$`'\nC=\"x\\\\y\\\"z\\$w\\n\\r'\"\nD=\nE=a=b\n"))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, map[string]string{
		"A": "plain", "B": "a b\"$`", "C": "x\\y\"z$w\n\r'", "D": "", "E": "a=b",
	}, vals)
}

func TestRunName(t *testing.T) {
	var out, errOut bytes.Buffer
	assert.Equal(t, 0, run(context.Background(), []string{"name"}, env(nil), &out, &errOut))
	assert.Regexp(t, `^qabz_test_[0-9a-f]{16}\n$`, out.String())
	assert.Empty(t, errOut.String())
}

func TestRunNeedsAdminDSN(t *testing.T) {
	for _, args := range [][]string{{"create", "qabz_test_ab"}, {"drop", "qabz_test_ab"}, {"clean"}} {
		var out, errOut bytes.Buffer
		code := run(context.Background(), args, env(nil), &out, &errOut)
		assert.Equal(t, 1, code, "%v", args)
		assert.Empty(t, out.String(), "%v", args)
		assert.Contains(t, errOut.String(), AdminDSNEnv+" is not set", "%v", args)
	}
}

// TestRefusesNonRunDatabase is the positive control of the guard: create and drop refuse a name that
// is not a run database name before any connection (the admin host does not exist, so a connection
// attempt would give a different error). A DSN given by mistake is refused and never repeated.
func TestRefusesNonRunDatabase(t *testing.T) {
	admin := env(map[string]string{AdminDSNEnv: "postgres://u:p@127.0.0.1:1/postgres"})
	for _, verb := range []string{"create", "drop"} {
		for _, name := range []string{
			"qabz", "qabz_test_", "postgres", "my_qabz_test_1", "",
			"postgres://qabz:secret@127.0.0.1:55432/qabz_test_ab",
			"qabz_test_ab?dbname=qabz", "qabz_test_a b", `qabz_test_a"; DROP DATABASE qabz; --`,
		} {
			var out, errOut bytes.Buffer
			code := run(context.Background(), []string{verb, name}, admin, &out, &errOut)
			assert.Equal(t, 1, code, verb+" "+name)
			assert.Empty(t, out.String(), verb+" "+name)
			assert.Contains(t, errOut.String(), "testdb: "+verb+" failed: ", verb+" "+name)
			assert.NotContains(t, errOut.String(), "connect", verb+" "+name)
			assert.NotContains(t, errOut.String(), "secret", verb+" "+name)
		}
	}
	var errOut bytes.Buffer
	assert.Error(t, drop(context.Background(), "postgres://u:p@127.0.0.1:1/postgres", "qabz", &errOut))
}

func TestCleanRefusesForeignPrefix(t *testing.T) {
	var errOut bytes.Buffer
	kept, err := clean(context.Background(), "postgres://u:p@127.0.0.1:1/postgres", "qabz", &errOut)
	assert.Equal(t, 0, kept)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "does not start with qabz_test_")
	}
	errOut.Reset()
	assert.Equal(t, 1, runClean(context.Background(), "postgres://u:p@127.0.0.1:1/postgres", "qabz", &errOut))
	assert.Contains(t, errOut.String(), "testdb: clean failed: prefix")
}

// TestRoleVerdict is the decision of clean over the pg_shdepend rows of a role: no rows → drop; rows
// → keep, with the places named (a row in a database: that database; a shared row of pg_database:
// the database it is about, never the database of the connection; another shared object: its
// catalog), each once, sorted, each labeled for what it is ("(not a test database)" only next to a
// database whose name does not start with qabz_test_).
func TestRoleVerdict(t *testing.T) {
	drop, places := roleVerdict(nil)
	assert.True(t, drop)
	assert.Empty(t, places)
	drop, places = roleVerdict([]shdep{})
	assert.True(t, drop)
	assert.Empty(t, places)

	drop, places = roleVerdict([]shdep{
		{DB: "shop", Class: "pg_class"},
		{DB: "shop", Class: "pg_namespace"},
		{Class: "pg_database", ObjName: "qabz"},
		{DB: "billing", Class: "pg_class"},
		{Class: "pg_database", ObjName: "shop"},
		{Class: "pg_tablespace"},
		{Class: "pg_database"},
		{DB: "qabz_test_ab", Class: "pg_class"},
		{Class: "pg_database", ObjName: "qabz_test_cd"},
	})
	assert.False(t, drop)
	assert.Equal(t, []string{
		"a shared object (pg_tablespace)", "an unknown database",
		"database billing (not a test database)", "database qabz (not a test database)",
		"database qabz_test_ab", "database qabz_test_cd", "database shop (not a test database)",
	}, places)

	drop, places = roleVerdict([]shdep{{Class: "pg_database", ObjName: "postgres"}})
	assert.False(t, drop)
	assert.Equal(t, []string{"database postgres (not a test database)"}, places)
}

// TestRoleOutcome is the decision of clean for one role over the result of DROP ROLE and the
// pg_shdepend rows: dropped, an error, kept with the places, or (2BP01 with no rows: the dependencies
// went away after DROP ROLE failed) kept with its own line, so that the next clean drops it.
func TestRoleOutcome(t *testing.T) {
	const role = "qabz_test_r"
	line, err := roleOutcome(role, nil, nil)
	assert.NoError(t, err)
	assert.Empty(t, line)
	line, err = roleOutcome(role, nil, []shdep{{DB: "qabz", Class: "pg_class"}})
	assert.NoError(t, err, "DROP ROLE done: the rows are not read")
	assert.Empty(t, line)

	for _, dropErr := range []error{errors.New("conn closed"), &pgconn.PgError{Code: "42501", Message: "permission denied"}} {
		line, err = roleOutcome(role, dropErr, []shdep{{DB: "qabz", Class: "pg_class"}})
		assert.Empty(t, line)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "drop role qabz_test_r: ")
			assert.ErrorIs(t, err, dropErr)
		}
	}

	dependents := &pgconn.PgError{Code: "2BP01", Message: "role cannot be dropped"}
	for _, dropErr := range []error{dependents, fmt.Errorf("exec: %w", dependents)} {
		for _, deps := range [][]shdep{nil, {}} {
			line, err = roleOutcome(role, dropErr, deps)
			assert.NoError(t, err)
			assert.Equal(t, "testdb: kept role qabz_test_r: DROP ROLE reports dependencies but none were found; run make test-db-clean again", line)
		}
		line, err = roleOutcome(role, dropErr, []shdep{
			{Class: "pg_tablespace"}, {Class: "pg_database", ObjName: "qabz_tmp"}, {DB: "qabz_test_x", Class: "pg_class"},
		})
		assert.NoError(t, err)
		assert.Equal(t, "testdb: kept role qabz_test_r: it has objects or privileges in a shared object (pg_tablespace), "+
			"database qabz_test_x, database qabz_tmp (not a test database); remove them by hand", line)
	}
}

func TestDropDBRefusesForeignName(t *testing.T) {
	var errOut bytes.Buffer
	err := dropDB(context.Background(), nil, "qabz", &errOut)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "not a test run database")
	}
}

func TestDropRoleRefusesForeignName(t *testing.T) {
	for _, role := range []string{"qabz", "postgres", "qabz_test_", "qabz_test_a-b", "keep_qabz_test_ab"} {
		line, err := dropRole(context.Background(), nil, role)
		assert.Empty(t, line, role)
		if assert.Error(t, err, role) {
			assert.Contains(t, err.Error(), "not", role)
		}
	}
}

func TestUnreachableServer(t *testing.T) {
	admin := env(map[string]string{AdminDSNEnv: "postgres://u:secret@127.0.0.1:1/postgres?connect_timeout=2"})
	for _, args := range [][]string{{"create", "qabz_test_ab"}, {"drop", "qabz_test_ab"}, {"clean"}} {
		var out, errOut bytes.Buffer
		code := run(context.Background(), args, admin, &out, &errOut)
		assert.Equal(t, 1, code, "%v", args)
		assert.Empty(t, out.String(), "%v", args)
		assert.Contains(t, errOut.String(), "connect to the admin database", "%v", args)
		assert.NotContains(t, errOut.String(), "secret", "%v", args)
	}
}

func TestNewRunDBName(t *testing.T) {
	a, err := newRunDBName()
	assert.NoError(t, err)
	b, err := newRunDBName()
	assert.NoError(t, err)
	assert.Regexp(t, `^qabz_test_[0-9a-f]{16}$`, a)
	assert.NotEqual(t, a, b)
}

func TestWithDB(t *testing.T) {
	got, err := withDB("postgres://u:p%40@h:5432/postgres?sslmode=disable", "qabz_test_ab")
	assert.NoError(t, err)
	assert.Equal(t, "postgres://u:p%40@h:5432/qabz_test_ab?sslmode=disable", got)
	_, err = withDB("postgres://u:p@h:bad port/x", "qabz_test_ab")
	assert.Error(t, err)
}

// liveAdmin returns the run DSN of `make test` as the admin DSN of these tests: the run database is
// the admin connection, so the extra databases that the tests create and drop are their own
// (operator exception, task 020b) and the development database is never touched.
func liveAdmin(t *testing.T) string {
	t.Helper()
	return postgrestest.RunDSN(t)
}

// exists reports whether a database named name is on the server.
func exists(t *testing.T, ctx context.Context, admin, name string) bool {
	t.Helper()
	conn, err := connect(ctx, admin)
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	defer conn.Close(context.Background())
	var n int
	if !assert.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM pg_database WHERE datname = $1`, name).Scan(&n)) {
		t.FailNow()
	}
	return n == 1
}

// TestLiveCreateDrop creates a run database through the command, connects to it, and drops it.
func TestLiveCreateDrop(t *testing.T) {
	admin := liveAdmin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	getenv := env(map[string]string{AdminDSNEnv: admin})

	var out, errOut bytes.Buffer
	if !assert.Equal(t, 0, run(ctx, []string{"name"}, getenv, &out, &errOut), errOut.String()) {
		return
	}
	name := strings.TrimSpace(out.String())
	// Before create (the trap of make test can run then): drop does nothing.
	out.Reset()
	errOut.Reset()
	assert.Equal(t, 0, run(ctx, []string{"drop", name}, getenv, &out, &errOut), errOut.String())
	assert.Equal(t, "testdb: no database "+name+", nothing to drop\n", errOut.String())
	assert.False(t, exists(t, ctx, admin, name))

	out.Reset()
	errOut.Reset()
	if !assert.Equal(t, 0, run(ctx, []string{"create", name}, getenv, &out, &errOut), errOut.String()) {
		return
	}
	t.Cleanup(func() { // drops it if the test stops before the drop below
		var e bytes.Buffer
		_ = drop(context.Background(), admin, name, &e)
	})
	runDSN := strings.TrimSpace(out.String())
	got, err := postgrestest.RunDBName(runDSN)
	assert.NoError(t, err)
	assert.Equal(t, name, got)
	assert.Contains(t, errOut.String(), "testdb: created database "+name)
	assert.True(t, exists(t, ctx, admin, name))

	conn, err := pgx.Connect(ctx, runDSN)
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		return
	}
	var current string
	assert.NoError(t, conn.QueryRow(ctx, `SELECT current_database()`).Scan(&current))
	assert.Equal(t, name, current)
	// An open connection does not stop the drop (WITH (FORCE)).

	out.Reset()
	errOut.Reset()
	assert.Equal(t, 0, run(ctx, []string{"drop", name}, getenv, &out, &errOut), errOut.String())
	assert.Contains(t, errOut.String(), "testdb: dropped database "+name)
	assert.False(t, exists(t, ctx, admin, name))
	_ = conn.Close(context.Background()) // the server ended it

	// A second drop of the same database is not an error and does nothing.
	errOut.Reset()
	assert.Equal(t, 0, run(ctx, []string{"drop", name}, getenv, &out, &errOut), errOut.String())
	assert.Contains(t, errOut.String(), "nothing to drop")
}

// TestLiveSpecialPassword runs admin-dsn, create and drop end to end for a role whose password has
// ', ", a backtick, $, @, /, : and a space (M3) and random hex (a new password for each run, decision
// 7). The role and its database are the test's own objects (operator exception, task 020b); both are
// dropped at the end. A run killed before that leaves them; make test-db-clean drops both (D4).
func TestLiveSpecialPassword(t *testing.T) {
	admin := liveAdmin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cfg, err := pgconn.ParseConfig(admin)
	if !assert.NoError(t, err) || !assert.NotNil(t, cfg) {
		t.FailNow()
	}
	role, err := newRunDBName()
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	conn, err := connect(ctx, admin)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	// The cleanup below needs conn, so conn is closed there (a defer would close it first).
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	password := randomPassword(t)
	literal := "'" + strings.ReplaceAll(password, "'", "''") + "'"
	if _, err := conn.Exec(ctx, `CREATE ROLE `+pgx.Identifier{role}.Sanitize()+` LOGIN CREATEDB PASSWORD `+literal); !assert.NoError(t, err) {
		t.FailNow()
	}
	name := role + "_db"
	t.Cleanup(func() {
		bg := context.Background()
		_, err := conn.Exec(bg, `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`)
		assert.NoError(t, err)
		_, err = conn.Exec(bg, `DROP ROLE IF EXISTS `+pgx.Identifier{role}.Sanitize())
		assert.NoError(t, err)
	})

	path := writeEnv(t, envFile(cfg.Host, strconv.Itoa(int(cfg.Port)), role, password))
	var out, errOut bytes.Buffer
	if !assert.Equal(t, 0, run(ctx, []string{"admin-dsn", path}, env(nil), &out, &errOut), errOut.String()) {
		return
	}
	getenv := env(map[string]string{AdminDSNEnv: strings.TrimSpace(out.String())})
	out.Reset()
	errOut.Reset()
	if !assert.Equal(t, 0, run(ctx, []string{"create", name}, getenv, &out, &errOut), errOut.String()) {
		return
	}
	runConn, err := pgx.Connect(ctx, strings.TrimSpace(out.String()))
	if !assert.NoError(t, err) || !assert.NotNil(t, runConn) {
		return
	}
	var user, db string
	assert.NoError(t, runConn.QueryRow(ctx, `SELECT current_user, current_database()`).Scan(&user, &db))
	assert.Equal(t, role, user)
	assert.Equal(t, name, db)
	_ = runConn.Close(context.Background())
	errOut.Reset()
	assert.Equal(t, 0, run(ctx, []string{"drop", name}, getenv, &out, &errOut), errOut.String())
	assert.Contains(t, errOut.String(), "testdb: dropped database "+name)
	assert.False(t, exists(t, ctx, admin, name))
	assert.NotContains(t, errOut.String(), password)
}

// TestLiveClean creates two databases with a prefix of its own and drops them with clean. The prefix
// is longer than qabz_test_, so the run database and other runs are not touched.
func TestLiveClean(t *testing.T) {
	admin := liveAdmin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tag, err := newRunDBName()
	if !assert.NoError(t, err) {
		return
	}
	prefix := tag + "_"
	names := []string{prefix + "a", prefix + "b"}
	for _, name := range names {
		if !assert.NoError(t, createDB(ctx, admin, name)) {
			t.FailNow()
		}
	}
	t.Cleanup(func() {
		var e bytes.Buffer
		_, _ = clean(context.Background(), admin, prefix, &e)
	})
	runName, err := postgrestest.RunDBName(admin)
	if !assert.NoError(t, err) {
		return
	}

	var errOut bytes.Buffer
	kept, err := clean(ctx, admin, prefix, &errOut)
	assert.NoError(t, err)
	assert.Equal(t, 0, kept)
	assert.Contains(t, errOut.String(), "testdb: 2 test database(s) dropped")
	for _, name := range names {
		assert.False(t, exists(t, ctx, admin, name), name)
	}
	assert.True(t, exists(t, ctx, admin, runName), "the run database stays")

	// Nothing left: clean drops nothing.
	errOut.Reset()
	assert.Equal(t, 0, runClean(ctx, admin, prefix, &errOut), errOut.String())
	assert.Equal(t, "testdb: 0 test database(s) dropped\ntestdb: 0 test role(s) dropped\n", errOut.String())
}

// roleExists reports whether a role named name is on the server.
func roleExists(t *testing.T, ctx context.Context, admin, name string) bool {
	t.Helper()
	conn, err := connect(ctx, admin)
	if !assert.NoError(t, err) || !assert.NotNil(t, conn) {
		t.FailNow()
	}
	defer conn.Close(context.Background())
	var n int
	if !assert.NoError(t, conn.QueryRow(ctx, `SELECT count(*) FROM pg_roles WHERE rolname = $1`, name).Scan(&n)) {
		t.FailNow()
	}
	return n == 1
}

// snapshot returns the owners and the ACLs of the databases runName and xdb, of the schema and of
// every relation in it, and the pg_shdepend rows of role (read only), one line each, sorted.
func snapshot(t *testing.T, ctx context.Context, conn *pgx.Conn, runName, xdb, schema, role string) []string {
	t.Helper()
	rows, err := conn.Query(ctx, `SELECT 'database ' || datname || ' owner ' || datdba::regrole::text || ' acl ' || COALESCE(datacl::text, '-')
			FROM pg_database WHERE datname IN ($1, $4)
		UNION ALL SELECT 'schema ' || nspname || ' owner ' || nspowner::regrole::text || ' acl ' || COALESCE(nspacl::text, '-')
			FROM pg_namespace WHERE nspname = $2
		UNION ALL SELECT 'relation ' || c.relname || ' owner ' || c.relowner::regrole::text || ' acl ' || COALESCE(c.relacl::text, '-')
			FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace WHERE n.nspname = $2
		UNION ALL SELECT 'shdepend ' || s.dbid::text || ' ' || s.classid::regclass::text || ' ' || s.objid::text || ' ' || s.deptype::text
			FROM pg_shdepend s JOIN pg_roles r ON r.oid = s.refobjid AND s.refclassid = 'pg_authid'::regclass
			WHERE r.rolname = $3
		ORDER BY 1`, runName, schema, role, xdb)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	lines, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	return lines
}

// TestLiveCleanRoles runs clean on the two kinds of leftover role (task 020b decision 8). Its prefix
// is <tag>_, its own, so for this clean the run database is "not a test database": it stands in for
// the development database, and the test creates no database outside the qabz_test_ names.
//  1. <tag>_r1 owns <tag>_db (a database that clean drops), a table in it, and has CONNECT on it:
//     clean drops the database, then the role.
//  2. <tag>_r2 owns a table and has privileges (USAGE on a schema, SELECT on a table, CONNECT on the
//     database) in the run database, and CONNECT on <tag>xdb (a qabz_test_ database of the test that
//     the prefix <tag>_ of clean does not match), which clean does not drop: clean KEEPS the role,
//     names both databases, exits 1, and both are unchanged (owners, ACLs and the pg_shdepend rows of
//     the role, recorded before and after). <tag>xdb is reached only through a shared pg_shdepend row
//     of pg_database, so the kept line shows that such a row names the database it is about, not the
//     database of the connection (the run database).
//
// A role whose name does not start with the prefix of clean (<tag>xr: a LIKE pattern <tag>_% would
// match it) is never touched. Every object is named qabz_test_..., so a killed run of this test leaves
// only what make test-db-clean removes (or keeps and names).
func TestLiveCleanRoles(t *testing.T) {
	admin := liveAdmin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tag, err := newRunDBName()
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	prefix := tag + "_"
	r1, r2, keep, db, xdb, schema := prefix+"r1", prefix+"r2", tag+"xr", prefix+"db", tag+"xdb", prefix+"s"
	runName, err := postgrestest.RunDBName(admin)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	conn, err := connect(ctx, admin)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	id := func(n string) string { return pgx.Identifier{n}.Sanitize() }
	// The test's own objects, also when it stops early: only statements on the run database and on
	// the test's own qabz_test_ objects (no DROP OWNED: it would also revoke grants on shared objects).
	// The schema (CASCADE) takes the tables of r2, the database takes the table of r1.
	t.Cleanup(func() {
		bg := context.Background()
		_, _ = conn.Exec(bg, `DROP SCHEMA IF EXISTS `+id(schema)+` CASCADE`)
		_, _ = conn.Exec(bg, `DROP DATABASE IF EXISTS `+id(db)+` WITH (FORCE)`)
		_, _ = conn.Exec(bg, `DROP DATABASE IF EXISTS `+id(xdb)+` WITH (FORCE)`)
		for _, r := range []string{r1, r2, keep} {
			_, _ = conn.Exec(bg, `REVOKE ALL ON DATABASE `+id(runName)+` FROM `+id(r))
			_, _ = conn.Exec(bg, `DROP ROLE IF EXISTS `+id(r))
		}
		assert.False(t, roleExists(t, bg, admin, keep), keep)
	})
	exec := func(c *pgx.Conn, stmts ...string) {
		t.Helper()
		for _, stmt := range stmts {
			if _, err := c.Exec(ctx, stmt); !assert.NoError(t, err, stmt) {
				t.FailNow()
			}
		}
	}
	exec(conn,
		`CREATE ROLE `+id(r1)+` LOGIN`,
		`CREATE ROLE `+id(r2)+` LOGIN`,
		`CREATE ROLE `+id(keep)+` NOLOGIN`,
		// 1: every dependency of r1 is in <tag>_db.
		`CREATE DATABASE `+id(db)+` OWNER `+id(r1),
		`GRANT CONNECT ON DATABASE `+id(db)+` TO `+id(r1),
		// 2: the dependencies of r2 (and keep) are in the run database.
		`CREATE SCHEMA `+id(schema),
		`CREATE TABLE `+id(schema)+`.owned (id int)`,
		`ALTER TABLE `+id(schema)+`.owned OWNER TO `+id(r2),
		`CREATE TABLE `+id(schema)+`.granted (id int)`,
		`GRANT SELECT ON `+id(schema)+`.granted TO `+id(r2),
		`GRANT USAGE ON SCHEMA `+id(schema)+` TO `+id(r2)+`, `+id(keep),
		`GRANT CONNECT ON DATABASE `+id(runName)+` TO `+id(r2),
		`CREATE DATABASE `+id(xdb),
		`GRANT CONNECT ON DATABASE `+id(xdb)+` TO `+id(r2),
	)
	dbDSN, err := withDB(admin, db)
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	dbConn, err := pgx.Connect(ctx, dbDSN)
	if !assert.NoError(t, err) || !assert.NotNil(t, dbConn) {
		t.FailNow()
	}
	exec(dbConn, `CREATE TABLE t (id int)`, `ALTER TABLE t OWNER TO `+id(r1))
	_ = dbConn.Close(context.Background())

	places := []string{"database " + runName, "database " + xdb}
	sort.Strings(places)
	keptLine := "testdb: kept role " + r2 + ": it has objects or privileges in " + strings.Join(places, ", ") + "; remove them by hand\n"
	before := snapshot(t, ctx, conn, runName, xdb, schema, r2)
	if !assert.NotEmpty(t, before) {
		t.FailNow()
	}
	var errOut bytes.Buffer
	assert.Equal(t, 1, runClean(ctx, admin, prefix, &errOut))
	assert.Equal(t, "testdb: dropped database "+db+"\n"+
		"testdb: 1 test database(s) dropped\n"+
		"testdb: dropped role "+r1+"\n"+
		keptLine+
		"testdb: 1 test role(s) dropped\n"+
		"testdb: 1 test role(s) kept\n", errOut.String())
	assert.False(t, exists(t, ctx, admin, db), db)
	assert.False(t, roleExists(t, ctx, admin, r1), r1)
	assert.True(t, roleExists(t, ctx, admin, r2), "a role with a dependency outside the clean stays")
	assert.True(t, roleExists(t, ctx, admin, keep), "a role without the prefix stays")
	assert.True(t, exists(t, ctx, admin, runName), "the run database stays")
	assert.True(t, exists(t, ctx, admin, xdb), "a test database outside the clean stays")
	assert.Equal(t, before, snapshot(t, ctx, conn, runName, xdb, schema, r2), "the database outside the clean is unchanged")

	// Again: the role is kept again, nothing changes.
	errOut.Reset()
	assert.Equal(t, 1, runClean(ctx, admin, prefix, &errOut))
	assert.Equal(t, "testdb: 0 test database(s) dropped\n"+
		keptLine+
		"testdb: 0 test role(s) dropped\n"+
		"testdb: 1 test role(s) kept\n", errOut.String())
	assert.Equal(t, before, snapshot(t, ctx, conn, runName, xdb, schema, r2))

	// The dependencies removed by hand (here by the test, in its own objects): clean drops the role.
	exec(conn,
		`DROP SCHEMA `+id(schema)+` CASCADE`,
		`REVOKE CONNECT ON DATABASE `+id(runName)+` FROM `+id(r2),
		`DROP DATABASE `+id(xdb)+` WITH (FORCE)`,
	)
	errOut.Reset()
	assert.Equal(t, 0, runClean(ctx, admin, prefix, &errOut), errOut.String())
	assert.Equal(t, "testdb: 0 test database(s) dropped\n"+
		"testdb: dropped role "+r2+"\n"+
		"testdb: 1 test role(s) dropped\n", errOut.String())
	assert.False(t, roleExists(t, ctx, admin, r2), r2)
	assert.True(t, roleExists(t, ctx, admin, keep), "a role without the prefix stays")
}
