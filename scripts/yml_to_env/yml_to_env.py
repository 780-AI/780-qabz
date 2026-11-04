#!/usr/bin/env python3
# ==============================================================
# VERSION: 0.1
# yml_to_env — generate a .env file (for docker-compose) from the YAML config.
#
# Usage:
#   python scripts/yml_to_env/yml_to_env.py config.yml
#   python scripts/yml_to_env/yml_to_env.py --output /tmp/app.env config.yml
#   python scripts/yml_to_env/yml_to_env.py --output /tmp config.yml     # /tmp/config_generated.env
#
# Options:
#   -o, --output PATH   output file, or a directory that gets config_generated.env.
#                       Default: config_generated.env in the current directory.
#   --strip PREFIX      first path part that is not part of the name (default: database, so
#                       database.postgres.password is POSTGRES_PASSWORD). "" turns it off.
#   --lower-case        lower case names (default: upper case).
#
# Rules:
#   - A nested key joins with "_":  logs: {level: x}  ->  LOGS_LEVEL=x
#   - A list item gets its index:   hosts: [a, b]  ->  HOSTS_0=a  HOSTS_1=b
#     (a list of maps:  srv: [{host: x}]  ->  SRV_0_HOST=x). An empty list or map gives no line.
#   - bool -> true / false; null -> empty value; int, float and string keep their YAML text (1.10 stays 1.10);
#     a date or time is written in ISO 8601.
#   - A value with a space or a special character is single-quoted; a value with ' or a line break
#     is double-quoted (the way docker compose reads a .env file).
#   - Two keys with the same name, or a key that is not [A-Za-z0-9_], stop the tool with an error.
#   - The file is written whole or not at all: any error leaves the target as it was (mode 0600).
#
# Needs Python 3.9+ and PyYAML (scripts/yml_to_env/requirements.txt; `make venv` installs it).
# Exit codes: 0 ok, 1 input/conversion error (no output written), 2 usage error.
#
# CHANGE LOG
# v0.1 — 2026-11-02 — Python rewrite of yml_to_env.sh (task 005); --output takes a file or a directory.
# ==============================================================
import argparse
import datetime
import os
import re
import sys
import tempfile
from typing import Any, List, Optional, Tuple

DEFAULT_OUTPUT = "config_generated.env"
DEFAULT_STRIP = "database"
HEADER = (
    "# AUTO-GENERATED from the YAML config file {source} by scripts/yml_to_env/yml_to_env.py.\n"
    "# DO NOT EDIT: change the YAML and generate this file again (make compose).\n"
    "# This file is mainly used by docker-compose.\n"
)

_PART = re.compile(r"^[A-Za-z0-9_]+$")
_NAME = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*$")
_PLAIN = re.compile(r"^[A-Za-z0-9_@%+=:,./-]*$")


class ConvertError(Exception):
    """An input or conversion error: the tool exits with 1 and writes nothing."""


def scalar_text(value: Any) -> str:
    if value is None:
        return ""
    if isinstance(value, bool):
        return "true" if value else "false"
    if isinstance(value, (datetime.datetime, datetime.date, datetime.time)):
        return value.isoformat()
    if isinstance(value, bytes):
        raise ConvertError("binary values are not supported")
    return str(value)


def flatten(node: Any, path: List[str]) -> List[Tuple[List[str], str]]:
    """Return (path parts, text) for every scalar leaf, in document order."""
    if isinstance(node, dict):
        out: List[Tuple[List[str], str]] = []
        for key, value in node.items():
            part = scalar_text(key)
            if not _PART.match(part):
                raise ConvertError("key '%s' is not valid (use letters, digits and _ only)" % part)
            out.extend(flatten(value, path + [part]))
        return out
    if isinstance(node, (list, tuple)):
        out = []
        for i, value in enumerate(node):
            out.extend(flatten(value, path + [str(i)]))
        return out
    if not path:
        raise ConvertError("the document must be a mapping")
    return [(path, scalar_text(node))]


def quote(value: str) -> str:
    if _PLAIN.match(value):
        return value
    if "'" in value or "\n" in value or "\r" in value:
        v = (value.replace("\\", "\\\\").replace('"', '\\"').replace("\n", "\\n")
             .replace("\r", "\\r").replace("$", "\\$"))
        return '"%s"' % v
    return "'%s'" % value


def to_env(doc: Any, strip: str = DEFAULT_STRIP, lower: bool = False) -> str:
    """Convert a loaded YAML document into the lines of a .env file."""
    if not isinstance(doc, dict):
        raise ConvertError("the document must be a mapping")
    seen = {}
    lines = []
    for path, text in flatten(doc, []):
        if strip and len(path) > 1 and path[0] == strip:
            path = path[1:]
        name = "_".join(path)
        name = name.lower() if lower else name.upper()
        if not _NAME.match(name):
            raise ConvertError("'%s' is not a valid variable name (do not start with a digit)" % name)
        if name in seen:
            raise ConvertError("two keys give the same variable name '%s'" % name)
        seen[name] = True
        lines.append("%s=%s\n" % (name, quote(text)))
    return "".join(lines)


def load_yaml(path: str) -> Any:
    try:
        import yaml
    except ImportError:
        raise ConvertError("PyYAML is not installed: run 'make venv' (or pip install -r "
                           "scripts/yml_to_env/requirements.txt)")
    class Loader(yaml.SafeLoader):
        """SafeLoader that keeps a float as its text (1.10 stays 1.10, not 1.1)."""

    Loader.yaml_implicit_resolvers = {
        ch: [(tag, rx) for tag, rx in rs if tag != "tag:yaml.org,2002:float"]
        for ch, rs in yaml.SafeLoader.yaml_implicit_resolvers.items()
    }
    try:
        with open(path, "r", encoding="utf-8") as f:
            return yaml.load(f, Loader=Loader)
    except OSError as e:
        raise ConvertError("cannot read '%s': %s" % (path, e.strerror or e))
    except (yaml.YAMLError, UnicodeDecodeError) as e:
        raise ConvertError("invalid yaml in '%s': %s" % (path, e))


def resolve_output(output: Optional[str]) -> str:
    if not output:
        return DEFAULT_OUTPUT
    if os.path.isdir(output) or output.endswith(os.sep):
        return os.path.join(output, DEFAULT_OUTPUT)
    return output


def write_file(path: str, content: str) -> None:
    """Write the whole file or nothing: a temp file in the same folder, then a rename."""
    folder = os.path.dirname(os.path.abspath(path))
    if not os.path.isdir(folder):
        raise ConvertError("cannot write '%s': the folder does not exist" % path)
    if os.path.isdir(path):
        raise ConvertError("cannot write '%s': it is a folder" % path)
    fd, tmp = tempfile.mkstemp(prefix=".yml_to_env.", dir=folder)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            f.write(content)
        os.chmod(tmp, 0o600)
        os.replace(tmp, path)
    except OSError as e:
        try:
            os.unlink(tmp)
        except OSError:
            pass
        raise ConvertError("cannot write '%s': %s" % (path, e.strerror or e))


def generate(config: str, output: Optional[str] = None, strip: str = DEFAULT_STRIP,
             lower: bool = False) -> str:
    """Generate the env file from the YAML file config. Returns the path written."""
    body = to_env(load_yaml(config), strip=strip, lower=lower)
    target = resolve_output(output)
    write_file(target, HEADER.format(source=config) + body)
    return target


def main(argv: Optional[List[str]] = None) -> int:
    p = argparse.ArgumentParser(prog="yml_to_env", description="Generate a .env file from a YAML config.")
    p.add_argument("config", help="path of the YAML config file")
    p.add_argument("-o", "--output", default=None,
                   help="output file, or a directory (default: ./%s)" % DEFAULT_OUTPUT)
    p.add_argument("--strip", default=DEFAULT_STRIP,
                   help="first path part left out of the names (default: %s; empty: none)" % DEFAULT_STRIP)
    p.add_argument("--lower-case", action="store_true", help="lower case names (default: upper case)")
    try:
        args = p.parse_args(argv)
    except SystemExit as e:  # argparse exits 2 on a usage error and 0 on --help
        return int(e.code or 0)
    try:
        generate(args.config, args.output, args.strip, args.lower_case)
    except ConvertError as e:
        print("yml_to_env: %s" % e, file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
