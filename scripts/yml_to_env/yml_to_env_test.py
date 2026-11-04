#!/usr/bin/env python3
# ==============================================================
# VERSION: 0.1
# Tests for yml_to_env.py (unittest; needs PyYAML).
#
# Usage:
#   .venv/bin/python -m unittest discover -s scripts/yml_to_env -p '*_test.py' -v
#
# CHANGE LOG
# v0.1 — 2026-11-02 — Initial version (task 005).
# ==============================================================
import contextlib
import io
import os
import stat
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import yml_to_env as y  # noqa: E402


def parse(text):
    """The body of an env file as a dict (the header and blank lines are skipped)."""
    out = {}
    for line in text.splitlines():
        if line and not line.startswith("#"):
            k, _, v = line.partition("=")
            out[k] = v
    return out


class Base(unittest.TestCase):
    def setUp(self):
        self._tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self._tmp.cleanup)
        self.tmp = os.path.realpath(self._tmp.name)

    def write(self, name, text):
        p = os.path.join(self.tmp, name)
        with open(p, "w", encoding="utf-8") as f:
            f.write(text)
        return p

    def read(self, path):
        with open(path, encoding="utf-8") as f:
            return f.read()

    def run_main(self, *argv):
        err = io.StringIO()
        with contextlib.redirect_stderr(err), contextlib.redirect_stdout(io.StringIO()):
            code = y.main(list(argv))
        return code, err.getvalue()


class MappingTest(Base):
    def test_compose_names(self):
        cfg = self.write("c.yml", """
database:
  postgres: {user: u, password: pw, db: d, port: 5433}
  redis: {port: 6380}
""")
        out = self.write("o.env", "")
        y.generate(cfg, out)
        env = parse(self.read(out))
        self.assertEqual(env, {"POSTGRES_USER": "u", "POSTGRES_PASSWORD": "pw", "POSTGRES_DB": "d",
                               "POSTGRES_PORT": "5433", "REDIS_PORT": "6380"})

    def test_flat_single_nested_and_lists(self):
        doc = {"one": 1, "a": {"b": {"c": {"d": "deep"}}}, "tags": ["x", "y"],
               "srv": [{"host": "h", "ports": [1, 2]}], "emptyl": [], "emptym": {}}
        self.assertEqual(parse(y.to_env(doc)), {
            "ONE": "1", "A_B_C_D": "deep", "TAGS_0": "x", "TAGS_1": "y",
            "SRV_0_HOST": "h", "SRV_0_PORTS_0": "1", "SRV_0_PORTS_1": "2"})

    def test_value_types(self):
        cfg = self.write("t.yml", """
s: text
i: 7
neg: -3
f: 1.10
b1: true
b2: false
n:
dur: 30s
d: 2026-11-02
""")
        env = parse(y.to_env(y.load_yaml(cfg)))
        self.assertEqual(env, {"S": "text", "I": "7", "NEG": "-3", "F": "1.10", "B1": "true",
                               "B2": "false", "N": "", "DUR": "30s", "D": "2026-11-02"})

    def test_missing_key_gives_no_line(self):
        env = parse(y.to_env({"database": {"postgres": {"user": "u"}}}))
        self.assertEqual(env, {"POSTGRES_USER": "u"})
        self.assertNotIn("POSTGRES_PASSWORD", env)

    def test_lower_case_and_strip_off(self):
        doc = {"database": {"postgres": {"host": "h"}}}
        self.assertEqual(parse(y.to_env(doc, lower=True)), {"postgres_host": "h"})
        self.assertEqual(parse(y.to_env(doc, strip="")), {"DATABASE_POSTGRES_HOST": "h"})

    def test_strip_keeps_a_lone_top_key(self):
        self.assertEqual(parse(y.to_env({"database": "x"})), {"DATABASE": "x"})

    def test_errors(self):
        for doc in ({"a": {"b": 1}, "A": {"B": 2}}, {"a-b": 1}, {"1a": 1}, {"a b": 1}, [1, 2], None, "x"):
            with self.assertRaises(y.ConvertError, msg=repr(doc)):
                y.to_env(doc)


class QuotingTest(Base):
    def test_quoting(self):
        for value, want in [
            ("plain", "plain"), ("a/b:c-d.e", "a/b:c-d.e"), ("", ""),
            ("two words", "'two words'"), ("a#b", "'a#b'"), ("$HOME", "'$HOME'"),
            ("it's", '"it\'s"'), ("a\nb", '"a\\nb"'), ('q"uote and \'s', '"q\\"uote and \'s"'),
            ("$x's", '"\\$x\'s"'), ("back\\slash's", '"back\\\\slash\'s"'),
        ]:
            self.assertEqual(y.quote(value), want, value)


class OutputTest(Base):
    def setUp(self):
        super().setUp()
        self.cfg = self.write("c.yml", "database:\n  postgres:\n    password: pw\n")

    def test_header(self):
        out = os.path.join(self.tmp, "o.env")
        y.generate(self.cfg, out)
        text = self.read(out)
        first = text.splitlines()[:3]
        self.assertTrue(first[0].startswith("# AUTO-GENERATED from the YAML"))
        self.assertIn("DO NOT EDIT", first[1])
        self.assertIn("mainly used by docker-compose", first[2])
        self.assertIn("POSTGRES_PASSWORD=pw", text)

    def test_default_output_name(self):
        old = os.getcwd()
        os.chdir(self.tmp)
        self.addCleanup(os.chdir, old)
        self.assertEqual(y.generate(self.cfg), "config_generated.env")
        self.assertTrue(os.path.isfile(os.path.join(self.tmp, "config_generated.env")))

    def test_custom_file_and_directory(self):
        custom = os.path.join(self.tmp, "custom_name.env")
        self.assertEqual(y.generate(self.cfg, custom), custom)
        self.assertTrue(os.path.isfile(custom))
        sub = os.path.join(self.tmp, "sub")
        os.mkdir(sub)
        self.assertEqual(y.generate(self.cfg, sub), os.path.join(sub, "config_generated.env"))
        self.assertTrue(os.path.isfile(os.path.join(sub, "config_generated.env")))

    def test_mode_0600(self):
        out = os.path.join(self.tmp, "o.env")
        y.generate(self.cfg, out)
        self.assertEqual(stat.S_IMODE(os.stat(out).st_mode), 0o600)

    def test_rewrite_replaces_file(self):
        out = os.path.join(self.tmp, "o.env")
        y.generate(self.cfg, out)
        y.generate(self.cfg, out)
        self.assertEqual(self.read(out).count("POSTGRES_PASSWORD"), 1)
        self.assertEqual(os.listdir(self.tmp).count(".yml_to_env."), 0)
        self.assertEqual(sorted(os.listdir(self.tmp)), ["c.yml", "o.env"])

    def test_missing_folder(self):
        with self.assertRaises(y.ConvertError):
            y.generate(self.cfg, os.path.join(self.tmp, "nope", "o.env"))


class FailureTest(Base):
    def test_failures_never_write_or_touch_the_target(self):
        out = self.write("o.env", "KEEP=1\n")
        for name, text in [("invalid", "a: [\n"), ("empty", ""), ("list", "- a\n- b\n"),
                           ("dup", "a: {b: 1}\nA: {B: 2}\n"), ("badkey", "a-b: 1\n")]:
            cfg = self.write(name + ".yml", text)
            code, err = self.run_main("-o", out, cfg)
            self.assertEqual(code, 1, name)
            self.assertIn("yml_to_env:", err, name)
            self.assertEqual(self.read(out), "KEEP=1\n", name)
        code, err = self.run_main("-o", out, os.path.join(self.tmp, "missing.yml"))
        self.assertEqual(code, 1)
        self.assertIn("cannot read", err)
        self.assertEqual(self.read(out), "KEEP=1\n")

    def test_no_secret_in_errors(self):
        cfg = self.write("c.yml", "database: {postgres: {password: s3cret}}\nbad-key: 1\n")
        code, err = self.run_main("-o", os.path.join(self.tmp, "o.env"), cfg)
        self.assertEqual(code, 1)
        self.assertNotIn("s3cret", err)

    def test_usage_errors(self):
        self.assertEqual(self.run_main()[0], 2)
        self.assertEqual(self.run_main("--nope", "x.yml")[0], 2)


class MultiLevelVerificationTest(Base):
    """Task 005 Verification: a custom multi-level YAML is converted in a tmp folder."""

    def test_custom_multi_level_yaml(self):
        cfg = self.write("custom.yml", """
service_name: demo
database:
  postgres: {host: db.local, port: 6000, user: app, password: "s3 cr'et", db: demo}
  redis: {host: cache.local, port: 7000}
  migration: {on_start: false, timeout: 90s}
logs: {level: debug, otlp_endpoint: "jaeger:4317"}
metrics:
  prometheus: {addr: ":9191"}
cluster:
  nodes:
    - {name: n1, labels: [a, b]}
    - {name: n2, labels: []}
""")
        target = os.path.join(self.tmp, "out")
        os.mkdir(target)
        code, err = self.run_main("--output", target, cfg)
        self.assertEqual((code, err), (0, ""))
        path = os.path.join(target, "config_generated.env")
        self.assertTrue(os.path.isfile(path))
        env = parse(self.read(path))
        self.assertEqual(env, {
            "SERVICE_NAME": "demo",
            "POSTGRES_HOST": "db.local", "POSTGRES_PORT": "6000", "POSTGRES_USER": "app",
            "POSTGRES_PASSWORD": '"s3 cr\'et"', "POSTGRES_DB": "demo",
            "REDIS_HOST": "cache.local", "REDIS_PORT": "7000",
            "MIGRATION_ON_START": "false", "MIGRATION_TIMEOUT": "90s",
            "LOGS_LEVEL": "debug", "LOGS_OTLP_ENDPOINT": "jaeger:4317",
            "METRICS_PROMETHEUS_ADDR": ":9191",
            "CLUSTER_NODES_0_NAME": "n1", "CLUSTER_NODES_0_LABELS_0": "a", "CLUSTER_NODES_0_LABELS_1": "b",
            "CLUSTER_NODES_1_NAME": "n2",
        })


if __name__ == "__main__":
    unittest.main()
