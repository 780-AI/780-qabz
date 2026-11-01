<!-- VERSION: 0.3 -->
<!-- Last Updated: 2026-11-01 -->
<!--
CHANGE LOG
v0.3 — 2026-11-01 — Section 1: `encoding/json` (v1) is forbidden everywhere; the `code-reviewer` rejects it.
v0.2 — 2026-11-01 — Section 2: Go modules (`go mod tidy && go mod vendor`; `vendor/` git-ignored).
v0.1 — 2026-11-01 — Created. Definitions section, not numbered (hot path, memory pool, `Poolable`). Section 1: JSON unmarshalling policy (moved out of Implementation_Plan.md Step 5).
-->

# PROJECT_SPECIFIC_CONVENTIONS.md — Qabz conventions

This file is owned by the project. It is not part of the Standard DocSet suite, so a suite sync does not change it.
It is binding for all code in this repository (linked from `CLAUDE.md` § 1). A new convention gets its own numbered section here. The "Definitions" section is not numbered.

---

## Definitions

Terms that more than one rule uses are defined here. A rule that uses a term links to the "Definitions" section and does not define the term again.

**Hot path.** A hot path is code that runs for each request, message or processing cycle. The CPU runs these regions all the time. Examples: handlers, middleware, queue consumers, cron loops, and parsing of provider responses.

**Not a hot path.** Code that runs one time or rarely is not hot. Examples: reading the config, opening connections and other mapping at bootstrap.

**Doubt.** If it is not clear whether code is hot, the author states the reason in the change and the reviewer decides (Rule 10).

**Memory pool.** A memory pool is a pool of objects, built with `sync.Pool`, that keeps a preallocated list of objects of one specific kind.

An object type can be used in a memory pool only if it meets these conditions:
- It implements the `Poolable` interface (below).
- It has a `Reset()` function. `Reset()` sets every field of the object (the struct) to the zero value of the field's type.
- It has a unit test for `Reset()`. This is mandatory, with no exception for any type used in a memory pool. The test sets every field to a non-zero value, calls `Reset()`, and checks that the object equals its zero value. The test proves that every field is in `Reset()`.
- `Reset()` does not use the `reflect` package. `reflect` is a performance killer. This is the reason the unit test is needed: nothing else guarantees that a new field is added to `Reset()`.

**Poolable.** `Poolable` is an interface that has one function only: `Reset()`.

```go
type Poolable interface {
	Reset()
}
```

Every type used in a memory pool implements `Poolable`. The pool code accepts and returns `Poolable` values, so a type without `Reset()` does not compile in a pool. The package that holds the interface is not decided yet.

---

## 1. JSON unmarshalling

This rule covers unmarshalling only. It does not cover marshalling. It uses the term "hot path" from the "Definitions" section.

**In a hot path:** unmarshal with `github.com/valyala/fastjson`.
- Take the parser from a `fastjson.ParserPool` and return it after use.
- Do not keep a parsed value after the parser goes back to the pool.

**Forbidden.** The standard `encoding/json` (v1) is illegal everywhere in the repository, in a hot path and outside it, in code and in tests. The `code-reviewer` agent finds every import of it and rejects the change.

**Outside a hot path:** unmarshal with the standard `encoding/json/v2`. This covers work that runs one time, for example reading the config at bootstrap.

**Review.** Each change that reads JSON states whether the read is on a hot path. The reviewer checks that statement (Rule 10).

**Check on 2026-11-01:** `encoding/json/v2` compiles with Go 1.27.1 and needs no `GOEXPERIMENT` flag. The module Go version must support `encoding/json/v2`.

**Open point:** the rule has no automatic check yet (for example a lint rule that bans `encoding/json` in hot-path packages). To be decided when the first Go code lands.

---

## 2. Go modules

**Always run `go mod tidy && go mod vendor`.** Never run `go mod tidy` alone. Run both commands together, in this order, after every change of the module requirements or of the imports.

**`vendor/` is git-ignored.** The `vendor/` directory is never committed. `.gitignore` lists `vendor/`. `go.mod` and `go.sum` are committed.

**Fresh clone.** Because `vendor/` is not in git, a fresh clone has no `vendor/`. Run `go mod tidy && go mod vendor` before the first build. The generated code in `api/pb/` is git-ignored too, so run `make proto` first. Otherwise `go mod tidy` removes the requirements that the generated code needs.

**Builds.** A Go build uses `vendor/` when it exists. CI and Docker builds run `go mod tidy && go mod vendor` before they build.
