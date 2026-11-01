#!/usr/bin/env bash
# ==============================================================
# VERSION: 0.2
# Tests for yml_to_env.sh. Plain bash, no test framework.
#
# Usage:
#   bash scripts/yml_to_env/yml_to_env_test.sh
# Exit code: 0 when every test passes, 1 when a test fails.
# Run it with the oldest bash you support (macOS /bin/bash is 3.2) and with a new bash.
#
# CHANGE LOG
# v0.2 — 2026-11-01 18:30 — Tests for -u / --upper-case; the default key case is now lower.
# v0.1 — 2026-11-01 17:00 — Initial version.
# ==============================================================
set -u

HERE=$(cd "$(dirname "$0")" && pwd)
TOOL="$HERE/yml_to_env.sh"
TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

passed=0
failed=0

pass() { passed=$((passed + 1)); }
bad() {
  failed=$((failed + 1))
  printf 'FAIL: %s\n' "$1" >&2
  shift
  for line in "$@"; do printf '      %s\n' "$line" >&2; done
}

# convert INPUT [args...]: runs the tool on stdin. Sets CODE, STDOUT, STDERR.
convert() {
  local input=$1
  shift
  printf '%s' "$input" | bash "$TOOL" "$@" - >"$TMP/stdout" 2>"$TMP/stderr"
  CODE=$?
  STDOUT=$(cat "$TMP/stdout"; printf x)
  STDOUT=${STDOUT%x}
  STDERR=$(cat "$TMP/stderr")
}

# t_out NAME EXPECTED_STDOUT INPUT
t_out() {
  convert "$3"
  if [[ $CODE -ne 0 ]]; then bad "$1" "exit code $CODE, stderr: $STDERR"; return; fi
  if [[ $STDOUT != "$2" ]]; then bad "$1" "want: $(printf '%q' "$2")" "got:  $(printf '%q' "$STDOUT")"; return; fi
  pass
}

# t_err NAME MESSAGE_FRAGMENT INPUT: exit code 1, nothing on stdout, fragment on stderr
t_err() {
  convert "$3"
  if [[ $CODE -ne 1 ]]; then bad "$1" "exit code $CODE, want 1"; return; fi
  if [[ -n $STDOUT ]]; then bad "$1" "stdout must be empty, got: $(printf '%q' "$STDOUT")"; return; fi
  if [[ $STDERR != *"$2"* ]]; then bad "$1" "stderr does not contain: $2" "stderr: $STDERR"; return; fi
  pass
}

# --- values -----------------------------------------------------------------
t_out "flat" $'host=localhost\nport=5432\n' $'host: localhost\nport: 5432\n'
t_out "nested map joins with underscore" $'db_host=h\ndb_port=1\n' $'db:\n  host: h\n  port: 1\n'
t_out "deep nesting" $'a_b_c=v\n' $'a:\n  b:\n    c: v\n'
t_out "4-space indent" $'a_b=v\n' $'a:\n    b: v\n'
t_out "list of scalars gets indexes" $'db_0=a\ndb_1=b\n' $'db:\n  - a\n  - b\n'
t_out "list dash at the key column" $'db_0=a\ndb_1=b\nnext=1\n' $'db:\n- a\n- b\nnext: 1\n'
t_out "list of maps" $'srv_0_host=x\nsrv_0_port=9\nsrv_1_host=y\n' $'srv:\n  - host: x\n    port: 9\n  - host: y\n'
t_out "list item with nested map" $'a_0_b_c=1\na_1_b_c=2\n' $'a:\n  - b:\n      c: 1\n  - b:\n      c: 2\n'
t_out "list under a list item key" $'a_0_ports_0=1\na_0_ports_1=2\n' $'a:\n  - ports:\n      - 1\n      - 2\n'
t_out "null forms are empty" $'a=\nb=\nc=\nd=\ne=\n' $'a: ~\nb:\nc: null\nd: Null\ne: NULL\n'
t_out "null at the end of a nested map" $'a_b=\nc=1\n' $'a:\n  b:\nc: 1\n'
t_out "null list item" $'a_0=\na_1=x\n' $'a:\n  -\n  - x\n'
t_out "number and bool text is kept" $'on=true\nver=1.10\nhex=0x1F\nzero=007\n' $'on: true\nver: 1.10\nhex: 0x1F\nzero: 007\n'
t_out "document order is kept" $'z=1\na=2\nm=3\n' $'z: 1\na: 2\nm: 3\n'
t_out "empty map and list give nothing" $'c=1\n' $'a: {}\nb: []\nc: 1\n'
t_out "empty input" '' ''
t_out "comments and blank lines only" '' $'# nothing\n\n   \n'
t_out "comments are skipped" $'a=1\nb=2\n' $'# top\na: 1 # inline\n  # indented comment\nb: 2\n'
t_out "hash inside a word is kept (and quoted)" $'a=\'x#y\'\n' $'a: x#y\n'
t_out "leading --- is accepted" $'a=1\n' $'---\na: 1\n'
t_out "no final newline" $'a=1\n' 'a: 1'
t_out "CRLF line endings" $'a=1\nb_c=2\n' $'a: 1\r\nb:\r\n  c: 2\r\n'
t_out "underscore start" $'_x=1\n' $'_x: 1\n'
t_out "numeric key after a prefix" $'a_1=x\n' $'a:\n  1: x\n'
t_out "value with colons" $'url=http://a:80/b\n' $'url: http://a:80/b\n'
t_out "key case is lowered by default" $'db_host=h\n' $'DB:\n  Host: h\n'
t_out "lower case: digits and _ stay, list index kept" $'srv_0_host_2=x\n' $'SRV:\n  - Host_2: x\n'
t_out "lower case: values are not changed" $'k=ABC\nv=\'Mixed Case\'\n' $'K: ABC\nV: Mixed Case\n'

# --- quoting ----------------------------------------------------------------
t_out "plain safe value" $'k=a/b:c,d.e-f_g@h+i%j=k\n' $'k: a/b:c,d.e-f_g@h+i%j=k\n'
t_out "space gets single quotes" $'k=\'a b\'\n' $'k: a b\n'
t_out "double-quoted value" $'k=\'a b\'\n' $'k: "a b"\n'
t_out "hash in quotes" $'k=\'p#ss\'\n' $'k: "p#ss"\n'
t_out "dollar stays literal" $'k=\'p$ss\'\n' $'k: \'p$ss\'\n'
t_out "double quote inside" $'k=\'say "hi"\'\n' $'k: \'say "hi"\'\n'
t_out "backslash inside single quotes" $'k=\'back\\slash\'\n' $'k: \'back\\slash\'\n'
t_out "apostrophe gets double quotes" $'k="it\'s"\n' $'k: "it\'s"\n'
t_out "doubled apostrophe in YAML" $'k="it\'s"\n' $'k: \'it\'\'s\'\n'
t_out "escaped line break" $'k="a\\nb"\n' $'k: "a\\nb"\n'
t_out "CR is escaped, tab stays literal" $'k="a\\rb\tc"\n' 'k: "a\rb\tc"'$'\n'
t_out "mixed specials" $'k="it\'s \\$x \\"y\\" \\\\z"\n' $'k: "it\'s $x \\"y\\" \\\\z"\n'
t_out "empty quoted value" $'k=\n' $'k: \'\'\n'
t_out "quoted value then comment" $'k=\'a b\'\n' $'k: "a b" # note\n'
t_out "value starting with a dash" $'k=-x\n' $'k: -x\n'

# --- errors -----------------------------------------------------------------
t_err "top level list" "top level must be a mapping" $'- a\n- b\n'
t_err "top level scalar" "expected 'key: value'" $'just text\n'
t_err "two documents" "more than one" $'a: 1\n---\nb: 2\n'
t_err "document end marker" "not supported" $'a: 1\n...\n'
t_err "tab indentation" "tabs" $'a:\n\tb: 1\n'
t_err "key with dash" "not valid" $'my-key: 1\n'
t_err "key with dot" "not valid" $'db:\n  a.b: 1\n'
t_err "key with space" "not valid" $'my key: 1\n'
t_err "quoted key" "not valid" $'"a": 1\n'
t_err "merge key" "not valid" $'base:\n  x: 1\nother:\n  <<: *b\n'
t_err "key starts with digit" "not a valid variable name" $'1a: x\n'
t_err "same name from two keys" "same variable name" $'db_host: a\ndb:\n  host: b\n'
t_err "same name from a list" "same variable name" $'a_0: x\na:\n  - y\n'
t_err "duplicate key" "same variable name" $'a: 1\na: 2\n'
t_err "anchor" "unsupported yaml" $'a: &x 1\n'
t_err "alias" "unsupported yaml" $'a: *x\n'
t_err "tag" "unsupported yaml" $'a: !!str 1\n'
t_err "literal block scalar" "unsupported yaml" $'a: |\n  text\n'
t_err "folded block scalar" "unsupported yaml" $'a: >\n  text\n'
t_err "flow list with content" "flow collections" $'a: [1, 2]\n'
t_err "flow map with content" "flow collections" $'a: {b: 1}\n'
t_err "unterminated double quote" "not closed" $'a: "abc\n'
t_err "unterminated single quote" "not closed" $'a: \'abc\n'
t_err "text after a quoted value" "unexpected text" $'a: "abc" def\n'
t_err "unknown escape" "unsupported escape" $'a: "\\q"\n'
t_err "indent under a value" "unexpected indentation" $'a: 1\n  b: 2\n'
t_err "nested list" "nested lists" $'a:\n  - - x\n'
t_err "list item under a value" "unexpected list item" $'a: 1\n- x\n'
t_err "key without colon" "expected 'key: value'" $'a:b\n'
t_err "error names the line" "line 3" $'a: 1\nb: 2\nc d: 3\n'

# --- letter case flag -------------------------------------------------------
# t_flag NAME EXPECTED_STDOUT INPUT FLAGS...
t_flag() {
  local name=$1 want=$2 input=$3
  shift 3
  convert "$input" "$@"
  if [[ $CODE -ne 0 ]]; then bad "$name" "exit code $CODE, stderr: $STDERR"; return; fi
  if [[ $STDOUT != "$want" ]]; then bad "$name" "want: $(printf '%q' "$want")" "got:  $(printf '%q' "$STDOUT")"; return; fi
  pass
}
UP_IN=$'db:\n  Host: h\n  ports:\n    - 1\nSrv_Name: Mixed Case\n'
UP_OUT=$'DB_HOST=h\nDB_PORTS_0=1\nSRV_NAME=\'Mixed Case\'\n'
LOW_OUT=$'db_host=h\ndb_ports_0=1\nsrv_name=\'Mixed Case\'\n'
t_flag "-u gives upper case names, values kept" "$UP_OUT" "$UP_IN" -u
t_flag "--upper-case gives upper case names" "$UP_OUT" "$UP_IN" --upper-case
t_flag "no flag gives lower case names" "$LOW_OUT" "$UP_IN"
t_flag "-u after a name that is already upper case" $'A_B=1\n' $'A:\n  B: 1\n' -u
t_flag "-u keeps digits and underscore" $'_X1_Y=1\n' $'_x1:\n  y: 1\n' -u
t_flag "-u and --upper-case together" $'A=1\n' $'a: 1\n' -u --upper-case
t_err_flag() { # NAME MESSAGE_FRAGMENT INPUT FLAGS...
  local name=$1 frag=$2 input=$3
  shift 3
  convert "$input" "$@"
  if [[ $CODE -ne 1 || -n $STDOUT || $STDERR != *"$frag"* ]]; then bad "$name" "exit $CODE, stdout: $STDOUT, stderr: $STDERR"; return; fi
  pass
}
t_err_flag "case change makes two names equal (lower)" "same variable name 'db'" $'Db: 1\ndb: 2\n'
t_err_flag "case change makes two names equal (upper)" "same variable name 'DB'" $'Db: 1\ndb: 2\n' -u
t_err_flag "names that differ only by case, nested" "same variable name 'a_b'" $'A_b: 1\na:\n  B: 2\n'

# --- command line -----------------------------------------------------------
CFG="$TMP/config.yml"
printf 'db:\n  host: h\n' >"$CFG"

out=$(bash "$TOOL" "$CFG" 2>&1); code=$?
if [[ $code -eq 0 && $out == 'db_host=h' ]]; then pass; else bad "cli: file to stdout" "exit $code, output: $out"; fi

target="$TMP/out.env"
out=$(bash "$TOOL" -o "$target" "$CFG" 2>&1); code=$?
if [[ $code -eq 0 && -z $out && $(cat "$target") == 'db_host=h' ]]; then pass; else bad "cli: -o writes a file" "exit $code, stdout: $out"; fi
mode=$(stat -c %a "$target" 2>/dev/null || stat -f %Lp "$target" 2>/dev/null)
if [[ $mode == 600 ]]; then pass; else bad "cli: -o file mode is 0600" "mode: $mode"; fi
if ls "$TMP"/out.env.* >/dev/null 2>&1; then bad "cli: no temp file is left behind"; else pass; fi

printf 'old=1\n' >"$TMP/keep.env"
printf 'my-key: 1\n' >"$TMP/bad.yml"
out=$(bash "$TOOL" -o "$TMP/keep.env" "$TMP/bad.yml" 2>&1); code=$?
if [[ $code -eq 1 && $(cat "$TMP/keep.env") == 'old=1' && $out == *"not valid"* ]]; then pass; else bad "cli: bad input leaves the output file untouched" "exit $code, output: $out"; fi
out=$(bash "$TOOL" -o "$TMP/never.env" "$TMP/bad.yml" 2>&1); code=$?
if [[ $code -eq 1 && ! -e "$TMP/never.env" ]]; then pass; else bad "cli: bad input creates no output file" "exit $code"; fi

out=$(bash "$TOOL" "$TMP/none.yml" 2>&1); code=$?
if [[ $code -eq 1 && $out == *"cannot read"* ]]; then pass; else bad "cli: missing input file" "exit $code, output: $out"; fi
out=$(bash "$TOOL" "$TMP" 2>&1); code=$?
if [[ $code -eq 1 && $out == *"cannot read"* ]]; then pass; else bad "cli: a directory as input" "exit $code, output: $out"; fi
out=$(bash "$TOOL" -o "$TMP/no/dir/x.env" "$CFG" 2>&1); code=$?
if [[ $code -eq 1 && $out == *"cannot write"* ]]; then pass; else bad "cli: unwritable output path" "exit $code, output: $out"; fi
out=$(bash "$TOOL" 2>&1); code=$?
if [[ $code -eq 2 && $out == *"usage"* ]]; then pass; else bad "cli: no argument" "exit $code, output: $out"; fi
out=$(bash "$TOOL" -z "$CFG" 2>&1); code=$?
if [[ $code -eq 2 && $out == *"usage"* ]]; then pass; else bad "cli: unknown flag" "exit $code, output: $out"; fi
out=$(bash "$TOOL" "$CFG" "$CFG" 2>&1); code=$?
if [[ $code -eq 2 ]]; then pass; else bad "cli: two input files" "exit $code"; fi
out=$(bash "$TOOL" -o 2>&1); code=$?
if [[ $code -eq 2 ]]; then pass; else bad "cli: -o without a value" "exit $code"; fi

out=$(bash "$TOOL" -u "$CFG" 2>&1); code=$?
if [[ $code -eq 0 && $out == 'DB_HOST=h' ]]; then pass; else bad "cli: -u with a file" "exit $code, output: $out"; fi
out=$(bash "$TOOL" --upper-case -o "$TMP/up.env" "$CFG" 2>&1); code=$?
if [[ $code -eq 0 && -z $out && $(cat "$TMP/up.env") == 'DB_HOST=h' ]]; then pass; else bad "cli: --upper-case with -o" "exit $code, output: $out"; fi
out=$(bash "$TOOL" -uo "$TMP/joined.env" "$CFG" 2>&1); code=$?
if [[ $code -eq 0 && -z $out && $(cat "$TMP/joined.env") == 'DB_HOST=h' ]]; then pass; else bad "cli: -uo joined flags" "exit $code, output: $out"; fi
out=$(bash "$TOOL" -o "$TMP/lo.env" -u "$CFG" 2>&1); code=$?
if [[ $code -eq 0 && $(cat "$TMP/lo.env") == 'DB_HOST=h' ]]; then pass; else bad "cli: -o before -u" "exit $code, output: $out"; fi
out=$(bash "$TOOL" --lower-case "$CFG" 2>&1); code=$?
if [[ $code -eq 2 && $out == *"usage"* ]]; then pass; else bad "cli: unknown long option" "exit $code, output: $out"; fi
out=$(bash "$TOOL" --upper-case=1 "$CFG" 2>&1); code=$?
if [[ $code -eq 2 && $out == *"usage"* ]]; then pass; else bad "cli: --upper-case with a value" "exit $code, output: $out"; fi
out=$(bash "$TOOL" -u 2>&1); code=$?
if [[ $code -eq 2 && $out == *"usage"* ]]; then pass; else bad "cli: -u without an input file" "exit $code, output: $out"; fi

printf '\n%d passed, %d failed (bash %s)\n' "$passed" "$failed" "$BASH_VERSION"
if [[ $failed -ne 0 ]]; then exit 1; fi
