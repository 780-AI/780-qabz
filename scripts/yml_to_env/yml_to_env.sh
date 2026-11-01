#!/usr/bin/env bash
# ==============================================================
# VERSION: 0.1
# yml_to_env — convert a YAML config file into environment variables (.env format).
#
# Usage:
#   bash scripts/yml_to_env/yml_to_env.sh config.yml          # print to stdout
#   bash scripts/yml_to_env/yml_to_env.sh -o .env config.yml  # write a file (mode 0600)
#   cat config.yml | bash scripts/yml_to_env/yml_to_env.sh -  # read stdin
#
# Rules:
#   - Nested keys join with "_":  db: {host: x}  ->  db_host=x   (key case is kept)
#   - A list item gets its index:  db: [a, b]  ->  db_0=a  db_1=b ;  - host: x  ->  srv_0_host=x
#   - A null (key: , key: ~, key: null) gives an empty value. {} and [] give nothing.
#   - Values keep their YAML text (1.10 stays 1.10). A value with a space or a special
#     character is single-quoted; a value with ' or a line break is double-quoted.
#
# Supported YAML (a strict subset; anything else stops with an error, never a wrong result):
#   block mappings and block lists, indented with spaces; plain, 'single' and "double" quoted
#   scalars (inline # comments); empty {} and []; one document (an optional leading ---).
# Not supported: anchors/aliases (&, *), tags (!), merge keys (<<), block scalars (| and >),
#   flow collections with content ([a, b], {a: 1}), quoted keys, tabs, several documents.
#
# Needs only bash 3.2 or newer (macOS default bash works). No other tool is called,
# except mktemp when -o is used.
# Exit codes: 0 ok, 1 input/conversion error (no output written), 2 usage error.
#
# CHANGE LOG
# v0.1 — 2026-11-01 17:00 — Initial version.
# ==============================================================
set -euo pipefail
export LC_ALL=C

SELF=yml_to_env
re_key='^([^:]+):([[:space:]]+(.*))?$'
re_name='^[A-Za-z_][A-Za-z0-9_]*$'
re_part='^[A-Za-z0-9_]+$'
re_plain='^[A-Za-z0-9_@%+=:,./-]*$'

OUT=""
NAMES=$'\n'
LINE_NO=0
depth=0
# The path stack: one entry per open key, list item or scalar (arrays are indexed by depth).
S_col=()   # column of the entry
S_seg=()   # name part (key, or list index)
S_kind=()  # k = key, i = list item, l = leaf (has a value)
S_pend=()  # 1 = no value yet; if nothing nested follows, the value is null
S_cnt=()   # number of list items seen under this key
S_item=()  # 1 = the entry is a list item (its kind may later change to l)
VALUE=""
VKIND=""
QUOTED=""
PATHV=""

die() { printf '%s: %s\n' "$SELF" "$*" >&2; exit 1; }
fail() { die "line $LINE_NO: $*"; }

usage() { printf 'usage: yml_to_env.sh [-o file] <file.yml | ->\n' >&2; }

push() { # col seg kind
  S_col[depth]=$1; S_seg[depth]=$2; S_kind[depth]=$3; S_pend[depth]=0; S_cnt[depth]=0; S_item[depth]=0
  if [[ $3 == i ]]; then S_item[depth]=1; fi
  depth=$((depth + 1))
}

build_path() { # sets PATHV from the whole stack
  local i
  PATHV=""
  for ((i = 0; i < depth; i++)); do
    if ((i == 0)); then PATHV=${S_seg[0]}; else PATHV="${PATHV}_${S_seg[i]}"; fi
  done
}

# quote_value VALUE -> sets QUOTED. Same rules as Docker Compose reads in a .env file.
quote_value() {
  local v=$1 bs='\' dq='"'
  if [[ $v =~ $re_plain ]]; then
    QUOTED=$v
    return 0
  fi
  case $v in
    *\'* | *$'\n'* | *$'\r'*)
      v=${v//"$bs"/"$bs$bs"}
      v=${v//"$dq"/"$bs$dq"}
      v=${v//$'\n'/"${bs}n"}
      v=${v//$'\r'/"${bs}r"}
      v=${v//'$'/"$bs\$"}
      QUOTED="\"$v\""
      ;;
    *) QUOTED="'$v'" ;;
  esac
}

emit() { # name value
  local name=$1
  [[ $name =~ $re_name ]] || fail "'$name' is not a valid variable name (use letters, digits and _; do not start with a digit)"
  case $NAMES in
    *$'\n'"$name"$'\n'*) fail "two keys give the same variable name '$name'" ;;
  esac
  NAMES+="$name"$'\n'
  quote_value "$2"
  OUT+="$name=$QUOTED"$'\n'
}

emit_current() { # value; the name is the whole stack
  build_path
  emit "$PATHV" "$1"
}

pop_one() {
  local top=$((depth - 1))
  if [[ ${S_pend[top]} == 1 ]]; then emit_current ""; fi
  depth=$top
}

# parse_value RAW -> sets VKIND (empty | null | flowempty | scalar) and VALUE.
parse_value() {
  local raw=$1 s i ch next out="" rest closed=0
  VALUE=""
  VKIND=scalar
  raw=${raw%"${raw##*[![:space:]]}"}
  if [[ -z $raw || $raw == '#'* ]]; then VKIND=empty; return 0; fi
  case ${raw:0:1} in
    '"')
      s=${raw:1}
      for ((i = 0; i < ${#s}; i++)); do
        ch=${s:i:1}
        if [[ $ch == '\' ]]; then
          i=$((i + 1))
          next=${s:i:1}
          case $next in
            n) out+=$'\n' ;;
            t) out+=$'\t' ;;
            r) out+=$'\r' ;;
            '"') out+='"' ;;
            '\') out+='\' ;;
            /) out+='/' ;;
            *) fail "unsupported escape in double-quoted value: \\$next" ;;
          esac
        elif [[ $ch == '"' ]]; then
          closed=1
          rest=${s:i+1}
          break
        else
          out+=$ch
        fi
      done
      ((closed == 1)) || fail "double-quoted value is not closed"
      ;;
    "'")
      s=${raw:1}
      for ((i = 0; i < ${#s}; i++)); do
        ch=${s:i:1}
        if [[ $ch == "'" ]]; then
          if [[ ${s:i+1:1} == "'" ]]; then
            out+="'"
            i=$((i + 1))
          else
            closed=1
            rest=${s:i+1}
            break
          fi
        else
          out+=$ch
        fi
      done
      ((closed == 1)) || fail "single-quoted value is not closed"
      ;;
    '&' | '*' | '!' | '|' | '>' | '@' | '`' | '%')
      fail "unsupported yaml (anchors, aliases, tags, block scalars): $raw"
      ;;
    '[' | '{')
      rest=${raw%% \#*}
      rest=${rest%"${rest##*[![:space:]]}"}
      if [[ $rest == '[]' || $rest == '{}' ]]; then VKIND=flowempty; return 0; fi
      fail "flow collections with content are not supported: $raw"
      ;;
    *)
      raw=${raw%% \#*}
      raw=${raw%"${raw##*[![:space:]]}"}
      case $raw in
        '~' | null | Null | NULL) VKIND=null; return 0 ;;
      esac
      VALUE=$raw
      return 0
      ;;
  esac
  rest=${rest#"${rest%%[![:space:]]*}"}
  if [[ -n $rest && $rest != '#'* ]]; then fail "unexpected text after the quoted value: $rest"; fi
  VALUE=$out
}

set_value() { # applies VKIND/VALUE to the entry on top of the stack
  local top=$((depth - 1))
  case $VKIND in
    empty) S_pend[top]=1 ;;
    flowempty) S_kind[top]=l ;;
    null) emit_current ""; S_kind[top]=l ;;
    scalar) emit_current "$VALUE"; S_kind[top]=l ;;
  esac
}

key_line() { # col content
  local col=$1 content=$2 key rawvalue t
  [[ $content =~ $re_key ]] || fail "expected 'key: value' but found: $content"
  key=${BASH_REMATCH[1]}
  rawvalue=${BASH_REMATCH[3]:-}
  key=${key%"${key##*[![:space:]]}"}
  [[ $key =~ $re_part ]] || fail "key '$key' is not valid (use letters, digits and _ only; quoted keys and merge keys are not supported)"
  while ((depth > 0)) && ((S_col[depth - 1] >= col)); do pop_one; done
  if ((depth > 0)); then
    t=$((depth - 1))
    if [[ ${S_kind[t]} == l ]]; then fail "unexpected indentation under a value"; fi
    S_pend[t]=0
  fi
  push "$col" "$key" k
  parse_value "$rawvalue"
  set_value
}

item_line() { # indent content
  local indent=$1 content=$2 rest lead col t idx
  while ((depth > 0)) && { ((S_col[depth - 1] > indent)) || { ((S_col[depth - 1] == indent)) && ((S_item[depth - 1] == 1)); }; }; do
    pop_one
  done
  ((depth > 0)) || fail "the top level must be a mapping"
  t=$((depth - 1))
  [[ ${S_kind[t]} == k ]] || fail "unexpected list item"
  S_pend[t]=0
  idx=${S_cnt[t]}
  S_cnt[t]=$((idx + 1))
  push "$indent" "$idx" i
  rest=${content#-}
  lead=${rest%%[![:space:]]*}
  rest=${rest#"$lead"}
  col=$((indent + 1 + ${#lead}))
  if [[ -z $rest || $rest == '#'* ]]; then S_pend[depth - 1]=1; return 0; fi
  if [[ $rest == '-' || $rest == '- '* ]]; then fail "nested lists are not supported"; fi
  if [[ $rest =~ $re_key ]]; then key_line "$col" "$rest"; return 0; fi
  parse_value "$rest"
  set_value
}

process() {
  local line leading content indent started=0 content_seen=0
  while IFS= read -r line || [[ -n $line ]]; do
    LINE_NO=$((LINE_NO + 1))
    line=${line%$'\r'}
    if [[ $line =~ ^[[:space:]]*(#.*)?$ ]]; then continue; fi
    if [[ $line == '---' ]]; then
      if ((started == 1 || content_seen == 1)); then fail "more than one yaml document"; fi
      started=1
      continue
    fi
    if [[ $line == '...' ]]; then fail "yaml document end markers are not supported"; fi
    leading=${line%%[![:space:]]*}
    if [[ $leading == *$'\t'* ]]; then fail "tabs are not allowed in indentation"; fi
    indent=${#leading}
    content=${line#"$leading"}
    content_seen=1
    if [[ $content == '-' || $content == '- '* ]]; then
      item_line "$indent" "$content"
    else
      key_line "$indent" "$content"
    fi
  done
  while ((depth > 0)); do pop_one; done
}

main() {
  local opt outfile="" tmp
  OPTIND=1
  while getopts ":o:" opt; do
    case $opt in
      o) outfile=$OPTARG ;;
      *) usage; exit 2 ;;
    esac
  done
  shift $((OPTIND - 1))
  if (($# != 1)); then usage; exit 2; fi

  if [[ $1 == - ]]; then
    process
  else
    [[ -f $1 && -r $1 ]] || die "cannot read '$1'"
    process <"$1"
  fi

  if [[ -z $outfile ]]; then
    printf '%s' "$OUT"
    return 0
  fi
  tmp=$(mktemp "$outfile.XXXXXX") || die "cannot write '$outfile'"
  if ! printf '%s' "$OUT" >"$tmp" || ! mv -f "$tmp" "$outfile"; then
    rm -f "$tmp"
    die "cannot write '$outfile'"
  fi
}

main "$@"
