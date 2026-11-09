// Command testdb manages the run database of `make test` (task 020b). It is for tests only: the App
// never runs it, and only the Makefile targets `test` and `test-db-clean` call it.
//
//	testdb admin-dsn <env file>  print the admin DSN (database "postgres" of the server), built from
//	                             POSTGRES_HOST, POSTGRES_PORT, POSTGRES_USER and POSTGRES_PASSWORD of
//	                             the env file (config_generated.env; parsed, never run as shell)
//	testdb name                  print a new random name qabz_test_<16 hex>
//	testdb create <name>         create that database and print its DSN (the run DSN)
//	testdb drop <name>           drop that database (DROP DATABASE ... WITH (FORCE)); no database of
//	                             that name: nothing is done
//	testdb clean                 drop every qabz_test_% database on the server, then every qabz_test_
//	                             role (leftovers of killed runs); a role that still has objects or
//	                             privileges in another database is kept (exit 1)
//
// create, drop and clean connect with the admin DSN in QABZ_TEST_ADMIN_DSN (the environment, never
// an argument: no DSN or password is ever on a command line). create and drop refuse a name that is
// not qabz_test_ + letters, digits and _ before they connect; clean drops only qabz_test_ names
// (databases and roles). No output but the stdout of admin-dsn and create has the password.
//
// Operator rule (task 020b decision 8): nothing in a database other than a qabz_test_ database is
// touched. The admin connection (database "postgres") only reads catalogs and runs server-level
// CREATE DATABASE / DROP DATABASE / DROP ROLE for qabz_test_ names; testdb never writes inside any
// database (no REASSIGN OWNED, DROP OWNED, GRANT, REVOKE, ALTER).
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"qabz/system/postgres/postgrestest"
)

// AdminDSNEnv names the variable with the admin DSN (server only). Only the Makefile sets it; the
// tests never get it.
const AdminDSNEnv = "QABZ_TEST_ADMIN_DSN"

// opTimeout bounds each database step (connect, create, drop, list).
const opTimeout = 30 * time.Second

const usage = "usage: testdb admin-dsn <env file> | name | create <name> | drop <name> | clean"

// argCounts is the number of arguments of each verb, the verb included.
var argCounts = map[string]int{"admin-dsn": 2, "name": 1, "create": 2, "drop": 2, "clean": 1}

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

// run executes one verb and returns the exit code: 0 done, 1 failed, 2 bad usage.
func run(ctx context.Context, args []string, getenv func(string) string, stdout, stderr io.Writer) int {
	if len(args) == 0 || argCounts[args[0]] != len(args) {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	verb := args[0]
	switch verb {
	case "admin-dsn":
		dsn, err := adminDSNFromFile(args[1])
		if err != nil {
			fmt.Fprintf(stderr, "testdb: admin-dsn failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, dsn)
		return 0
	case "name":
		name, err := newRunDBName()
		if err != nil {
			fmt.Fprintf(stderr, "testdb: name failed: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, name)
		return 0
	}
	if verb != "clean" {
		if err := checkRunDBName(args[1]); err != nil {
			fmt.Fprintf(stderr, "testdb: %s failed: %v\n", verb, err)
			return 1
		}
	}

	admin := getenv(AdminDSNEnv)
	if admin == "" {
		fmt.Fprintf(stderr, "testdb: %s failed: %s is not set\n", verb, AdminDSNEnv)
		return 1
	}
	ctx, cancel := context.WithTimeout(ctx, opTimeout)
	defer cancel()
	var err error
	switch verb {
	case "create":
		var runDSN string
		runDSN, err = create(ctx, admin, args[1], stderr)
		if err == nil {
			fmt.Fprintln(stdout, runDSN)
		}
	case "drop":
		err = drop(ctx, admin, args[1], stderr)
	case "clean":
		return runClean(ctx, admin, postgrestest.RunDBPrefix, stderr)
	}
	if err != nil {
		fmt.Fprintf(stderr, "testdb: %s failed: %v\n", verb, err)
		return 1
	}
	return 0
}

// plainName matches a name that an error may repeat (letters, digits and _ only).
var plainName = regexp.MustCompile(`^[A-Za-z0-9_]*$`)

// checkRunDBName returns an error when name does not start with qabz_test_, has nothing after it,
// or has a character that is not a letter, a digit or _. The error repeats the name only when it has
// letters, digits and _ only, so an argument that is a DSN by mistake never reaches the output with
// its password.
func checkRunDBName(name string) error {
	if !plainName.MatchString(name) {
		return errors.New("the argument is not a database name (give the name only, never a DSN)")
	}
	if strings.HasPrefix(name, postgrestest.RunDBPrefix) && len(name) > len(postgrestest.RunDBPrefix) {
		return nil
	}
	return fmt.Errorf("database %q is not a test run database (name must start with %s)", name, postgrestest.RunDBPrefix)
}

// adminDSNFromFile reads the env file at path (parseEnvFile) and builds the admin DSN from it.
func adminDSNFromFile(path string) (string, error) {
	vals, err := parseEnvFile(path)
	if err != nil {
		return "", err
	}
	return adminDSN(func(k string) string { return vals[k] })
}

// parseEnvFile reads the KEY=VALUE lines of a file that scripts/yml_to_env writes. A value is plain
// (taken as it is), single-quoted (taken as it is between the quotes) or double-quoted (the escapes
// \\, \", \n, \r and \$ are decoded). Empty lines and # comments are skipped. Nothing is run or
// expanded. The errors name the file, the line and the key, never a value (a value can be a password).
func parseEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		var pe *os.PathError
		if errors.As(err, &pe) && pe != nil {
			err = pe.Err
		}
		return nil, fmt.Errorf("read the env file %s: %v (run make env)", path, err)
	}
	vals := map[string]string{}
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, raw, ok := strings.Cut(line, "=")
		if !ok || key == "" || strings.TrimSpace(key) != key {
			return nil, fmt.Errorf("%s line %d: not a KEY=VALUE line", path, i+1)
		}
		v, err := envValue(raw)
		if err != nil {
			return nil, fmt.Errorf("%s line %d (%s): %w", path, i+1, key, err)
		}
		vals[key] = v
	}
	return vals, nil
}

// envValue decodes one value of the env file (see parseEnvFile).
func envValue(raw string) (string, error) {
	switch {
	case strings.HasPrefix(raw, "'"):
		if len(raw) < 2 || !strings.HasSuffix(raw, "'") || strings.Contains(raw[1:len(raw)-1], "'") {
			return "", errors.New("bad single-quoted value")
		}
		return raw[1 : len(raw)-1], nil
	case strings.HasPrefix(raw, `"`):
		if len(raw) < 2 || !strings.HasSuffix(raw, `"`) {
			return "", errors.New("bad double-quoted value")
		}
		body := raw[1 : len(raw)-1]
		var b strings.Builder
		for i := 0; i < len(body); i++ {
			c := body[i]
			if c == '"' {
				return "", errors.New("bad double-quoted value")
			}
			if c != '\\' {
				b.WriteByte(c)
				continue
			}
			i++
			if i == len(body) {
				return "", errors.New("bad double-quoted value")
			}
			switch body[i] {
			case '\\', '"', '$':
				b.WriteByte(body[i])
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			default:
				return "", errors.New("bad escape in a double-quoted value")
			}
		}
		return b.String(), nil
	}
	return raw, nil
}

// adminDSN builds the admin DSN from the POSTGRES_* values of config_generated.env. It names the
// maintenance database "postgres", so the admin connection never works in the development database.
func adminDSN(getenv func(string) string) (string, error) {
	host, port, user := getenv("POSTGRES_HOST"), getenv("POSTGRES_PORT"), getenv("POSTGRES_USER")
	var missing []string
	for _, v := range [][2]string{{"POSTGRES_HOST", host}, {"POSTGRES_PORT", port}, {"POSTGRES_USER", user}} {
		if v[1] == "" {
			missing = append(missing, v[0])
		}
	}
	if len(missing) > 0 {
		return "", fmt.Errorf("%s not set (run make env)", strings.Join(missing, ", "))
	}
	u := url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(user, getenv("POSTGRES_PASSWORD")),
		Host:   net.JoinHostPort(host, port),
		Path:   "/postgres",
	}
	return u.String(), nil
}

// connect opens one admin connection.
func connect(ctx context.Context, admin string) (*pgx.Conn, error) {
	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		return nil, fmt.Errorf("connect to the admin database: %w", err)
	}
	if conn == nil {
		return nil, errors.New("connect to the admin database: no connection")
	}
	return conn, nil
}

// newRunDBName returns qabz_test_ and 16 random hex digits.
func newRunDBName() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("random name: %w", err)
	}
	return postgrestest.RunDBPrefix + hex.EncodeToString(b), nil
}

// withDB returns dsn with the database name replaced (user, host and parameters stay).
func withDB(dsn, name string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", errors.New("the admin DSN is not a URL")
	}
	u.Path = "/" + name
	u.RawPath = ""
	return u.String(), nil
}

// create makes the run database name (checked by run) on the server of admin and returns its DSN.
func create(ctx context.Context, admin, name string, stderr io.Writer) (string, error) {
	runDSN, err := withDB(admin, name)
	if err != nil {
		return "", err
	}
	if err := createDB(ctx, admin, name); err != nil {
		return "", err
	}
	fmt.Fprintf(stderr, "testdb: created database %s\n", name)
	return runDSN, nil
}

// createDB runs CREATE DATABASE name on the server of admin.
func createDB(ctx context.Context, admin, name string) error {
	conn, err := connect(ctx, admin)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	if _, err := conn.Exec(ctx, `CREATE DATABASE `+pgx.Identifier{name}.Sanitize()); err != nil {
		return fmt.Errorf("create database %s: %w", name, err)
	}
	return nil
}

// drop drops the database name on the server of admin. It refuses a name that does not start with
// qabz_test_ before it connects. No database of that name (the run stopped before create): nothing
// is done, and drop says so.
func drop(ctx context.Context, admin, name string, stderr io.Writer) error {
	if err := checkRunDBName(name); err != nil {
		return err
	}
	conn, err := connect(ctx, admin)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())
	var n int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM pg_database WHERE datname = $1`, name).Scan(&n); err != nil {
		return fmt.Errorf("look up database %s: %w", name, err)
	}
	if n == 0 {
		fmt.Fprintf(stderr, "testdb: no database %s, nothing to drop\n", name)
		return nil
	}
	return dropDB(ctx, conn, name, stderr)
}

// dropDB drops one database that starts with qabz_test_; other connections to it are ended (FORCE).
func dropDB(ctx context.Context, conn *pgx.Conn, name string, stderr io.Writer) error {
	if !strings.HasPrefix(name, postgrestest.RunDBPrefix) {
		return fmt.Errorf("database %q is not a test run database", name)
	}
	if _, err := conn.Exec(ctx, `DROP DATABASE IF EXISTS `+pgx.Identifier{name}.Sanitize()+` WITH (FORCE)`); err != nil {
		return fmt.Errorf("drop database %s: %w", name, err)
	}
	fmt.Fprintf(stderr, "testdb: dropped database %s\n", name)
	return nil
}

// clean drops every database whose name starts with prefix, then every role whose name starts with
// prefix (cleanRoles), and returns the number of roles it kept. prefix must start with qabz_test_ (the
// command passes exactly qabz_test_; a test passes a longer prefix of its own). The databases go
// first: a role that owns a database cannot be dropped before it. clean runs only server-level
// statements (DROP DATABASE, DROP ROLE) and reads catalogs: it never changes anything inside a
// database (operator rule, task 020b decision 8).
func clean(ctx context.Context, admin, prefix string, stderr io.Writer) (int, error) {
	if !strings.HasPrefix(prefix, postgrestest.RunDBPrefix) {
		return 0, fmt.Errorf("prefix %q does not start with %s", prefix, postgrestest.RunDBPrefix)
	}
	conn, err := connect(ctx, admin)
	if err != nil {
		return 0, err
	}
	defer conn.Close(context.Background())
	rows, err := conn.Query(ctx, `SELECT datname FROM pg_database WHERE starts_with(datname, $1) ORDER BY datname`, prefix)
	if err != nil {
		return 0, fmt.Errorf("list databases: %w", err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, fmt.Errorf("list databases: %w", err)
	}
	for _, name := range names {
		if err := dropDB(ctx, conn, name, stderr); err != nil {
			return 0, err
		}
	}
	fmt.Fprintf(stderr, "testdb: %d test database(s) dropped\n", len(names))
	return cleanRoles(ctx, conn, prefix, stderr)
}

// runClean runs clean and returns the exit code of the verb: 0 done, 1 failed or at least one role
// kept (then the last line is "testdb: <n> test role(s) kept", so make test-db-clean fails visibly).
func runClean(ctx context.Context, admin, prefix string, stderr io.Writer) int {
	kept, err := clean(ctx, admin, prefix, stderr)
	if err != nil {
		fmt.Fprintf(stderr, "testdb: clean failed: %v\n", err)
		return 1
	}
	if kept > 0 {
		fmt.Fprintf(stderr, "testdb: %d test role(s) kept\n", kept)
		return 1
	}
	return 0
}

// cleanRoles drops every role whose name starts with prefix and is qabz_test_ + letters, digits and
// _ only (checkRunDBName), except the role of the admin connection, and returns the number of roles
// it kept. A role is never touched without that prefix. When DROP ROLE fails because the role still
// owns objects or has privileges somewhere (2BP01: after the test databases are dropped, that is a
// database that is not a test database), the role is KEPT and nothing is changed: cleanRoles only
// reads pg_shdepend to name those databases (roleVerdict) and prints one line for the role. It never
// runs REASSIGN OWNED or DROP OWNED.
func cleanRoles(ctx context.Context, conn *pgx.Conn, prefix string, stderr io.Writer) (int, error) {
	rows, err := conn.Query(ctx, `SELECT rolname FROM pg_roles
		WHERE starts_with(rolname, $1) AND rolname <> current_user AND rolname <> session_user
		ORDER BY rolname`, prefix)
	if err != nil {
		return 0, fmt.Errorf("list roles: %w", err)
	}
	roles, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return 0, fmt.Errorf("list roles: %w", err)
	}
	dropped, kept := 0, 0
	for _, role := range roles {
		if checkRunDBName(role) != nil {
			continue // not qabz_test_ + letters, digits and _: never touched
		}
		keptLine, err := dropRole(ctx, conn, role)
		if err != nil {
			return kept, err
		}
		if keptLine != "" {
			fmt.Fprintln(stderr, keptLine)
			kept++
			continue
		}
		fmt.Fprintf(stderr, "testdb: dropped role %s\n", role)
		dropped++
	}
	fmt.Fprintf(stderr, "testdb: %d test role(s) dropped\n", dropped)
	return kept, nil
}

// dropRole runs DROP ROLE for one qabz_test_ role (see cleanRoles). It returns "" when the role was
// dropped, and the kept line of the role (roleOutcome) when DROP ROLE failed with 2BP01: the role is
// then kept and nothing else is run (the pg_shdepend rows are only read).
func dropRole(ctx context.Context, conn *pgx.Conn, role string) (string, error) {
	if err := checkRunDBName(role); err != nil {
		return "", err
	}
	_, dropErr := conn.Exec(ctx, `DROP ROLE IF EXISTS `+pgx.Identifier{role}.Sanitize())
	var deps []shdep
	if hasDependents(dropErr) {
		var err error
		if deps, err = roleDependencies(ctx, conn, role); err != nil {
			return "", err
		}
	}
	return roleOutcome(role, dropErr, deps)
}

// hasDependents reports whether err is the SQLSTATE 2BP01 of DROP ROLE (the role still owns objects or
// has privileges somewhere).
func hasDependents(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr != nil && pgErr.Code == "2BP01"
}

// roleOutcome is the decision of cleanRoles for one role over the result of DROP ROLE and the
// pg_shdepend rows read after a 2BP01:
//   - DROP ROLE done: "" (dropped);
//   - DROP ROLE failed, not 2BP01: an error;
//   - 2BP01 and rows: the kept line that names the places of the rows (roleVerdict);
//   - 2BP01 and no rows (the dependencies went away after DROP ROLE failed): a kept line that says so;
//     the role is kept this time and the next clean drops it.
func roleOutcome(role string, dropErr error, deps []shdep) (string, error) {
	if dropErr == nil {
		return "", nil
	}
	if !hasDependents(dropErr) {
		return "", fmt.Errorf("drop role %s: %w", role, dropErr)
	}
	if drop, places := roleVerdict(deps); !drop {
		return "testdb: kept role " + role + ": it has objects or privileges in " + strings.Join(places, ", ") +
			"; remove them by hand", nil
	}
	return "testdb: kept role " + role + ": DROP ROLE reports dependencies but none were found; run make test-db-clean again", nil
}

// shdep is one pg_shdepend row of a role (roleDependencies).
type shdep struct {
	DB      string // the database of the dependent object; "" for a shared object (dbid 0)
	Class   string // the catalog of the dependent object (classid), for example pg_class or pg_database
	ObjName string // for a shared object of pg_database: the name of that database
}

// roleVerdict is the decision of cleanRoles over the pg_shdepend rows of a role: no rows → drop; any
// row → keep, with the sorted list of the places that hold the rows, each labeled for what it is
// (dbPlace). A row in a database is that database; a privilege on (or the ownership of) a database is
// that database (classid pg_database, the name of the object, never the database of the connection);
// another shared object is named by its catalog.
func roleVerdict(deps []shdep) (drop bool, places []string) {
	if len(deps) == 0 {
		return true, nil
	}
	seen := map[string]bool{}
	for _, d := range deps {
		var place string
		switch {
		case d.DB != "":
			place = dbPlace(d.DB)
		case d.Class == "pg_database" && d.ObjName != "":
			place = dbPlace(d.ObjName)
		case d.Class == "pg_database":
			place = "an unknown database"
		default:
			place = "a shared object (" + d.Class + ")"
		}
		if !seen[place] {
			seen[place] = true
			places = append(places, place)
		}
	}
	sort.Strings(places)
	return false, places
}

// dbPlace labels a database of a kept line: "database <name>", and " (not a test database)" when the
// name does not start with qabz_test_.
func dbPlace(name string) string {
	if strings.HasPrefix(name, postgrestest.RunDBPrefix) {
		return "database " + name
	}
	return "database " + name + " (not a test database)"
}

// roleDependencies READS the pg_shdepend rows of role (no write). A shared-object row (dbid 0) of
// pg_database resolves to the name of that database.
func roleDependencies(ctx context.Context, conn *pgx.Conn, role string) ([]shdep, error) {
	rows, err := conn.Query(ctx, `SELECT COALESCE(d.datname, ''), s.classid::regclass::text, COALESCE(od.datname, '')
		FROM pg_shdepend s
		JOIN pg_roles r ON r.oid = s.refobjid AND s.refclassid = 'pg_authid'::regclass
		LEFT JOIN pg_database d ON s.dbid <> 0 AND d.oid = s.dbid
		LEFT JOIN pg_database od ON s.dbid = 0 AND s.classid = 'pg_database'::regclass AND od.oid = s.objid
		WHERE r.rolname = $1 AND s.deptype <> 'p'`, role)
	if err != nil {
		return nil, fmt.Errorf("list the dependencies of role %s: %w", role, err)
	}
	deps, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (shdep, error) {
		var d shdep
		err := row.Scan(&d.DB, &d.Class, &d.ObjName)
		return d, err
	})
	if err != nil {
		return nil, fmt.Errorf("list the dependencies of role %s: %w", role, err)
	}
	return deps, nil
}
