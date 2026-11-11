PB_DIR    ?= qabz
PB_PROTO  := pb/proto
PB_AUX    := pb/aux
PB_OUT    := src/api/pb
SWAG_OUT  := src/api/swagger

# The YAML config (git-ignored; see task 004), the generated env file and the Python venv.
CONFIG    ?= config.yml
ENV_OUT   ?= config_generated.env
VENV      := .venv
VENV_PY   := $(VENV)/bin/python
GEN_DIR   := scripts/yml_to_env

# `make compose <action>`: the docker compose actions it accepts. ARGS passes extra flags,
# for example: make compose up ARGS="-d --wait"; make compose logs ARGS=-f; make compose down ARGS=-v
COMPOSE_ACTIONS := up down ps logs
COMPOSE_ACTION  := $(word 2,$(MAKECMDGOALS))
ARGS            ?=

# `make test`: the Go tests run in ONE throw-away database qabz_test_<random hex> (task 020b).
# GO_TEST_ARGS passes the flags of `go test`, for example GO_TEST_ARGS="-race -count=1 -v -run Live".
# GO_TEST_PKGS is ./... (the whole run); a QA scenario can name packages, for example GO_TEST_PKGS=./app/.
# TESTDB runs the test database tool (tests only). It parses $(ENV_OUT) itself (never sourced as shell)
# and gets the admin DSN in QABZ_TEST_ADMIN_DSN: no DSN or password is ever on a command line.
TESTDB       := go run ./system/postgres/postgrestest/testdb
TESTDB_ENV   := $(abspath $(ENV_OUT))
GO_TEST_ARGS ?= -race -count=1 -cover
GO_TEST_PKGS ?= ./...

.PHONY: help venv env compose test-scripts test test-db-clean $(COMPOSE_ACTIONS)

## help: list the make targets with their descriptions (read from the "## target:" comments in this file).
help:
	@echo 'usage: make <target> [VAR=value ...]   (variables: CONFIG, ENV_OUT, ARGS, PB_DIR, GO_TEST_ARGS, GO_TEST_PKGS)'
	@awk -v config='$(CONFIG)' -v env_out='$(ENV_OUT)' -v pb_dir='$(PB_DIR)' ' \
		/^## [A-Za-z0-9_-]+:/ { sub(/^## /, ""); t = substr($$0, 1, index($$0, ":") - 1); \
			d = substr($$0, index($$0, ":") + 2); last = t; printf "  %-14s %s\n", t, sub_vars(d); next } \
		/^## / && last != "" { sub(/^## /, ""); printf "  %-14s %s\n", "", sub_vars($$0); next } \
		{ last = "" } \
		function sub_vars(x) { gsub(/\$$\(CONFIG\)/, config, x); gsub(/\$$\(ENV_OUT\)/, env_out, x); \
			gsub(/\$$\(PB_DIR\)/, pb_dir, x); return x }' $(firstword $(MAKEFILE_LIST))

## venv: create the Python venv with the pinned PyYAML (needed by the env generator).
venv: $(VENV)/.installed
$(VENV)/.installed: $(GEN_DIR)/requirements.txt
	python3 -m venv $(VENV)
	$(VENV)/bin/pip install --quiet -r $(GEN_DIR)/requirements.txt
	touch $@

## env: generate $(ENV_OUT) from $(CONFIG). Never edit the generated file by hand.
env: venv
	$(VENV_PY) $(GEN_DIR)/yml_to_env.py --output $(ENV_OUT) $(CONFIG)

## compose: make compose up|down|ps|logs. (1) generate $(ENV_OUT) from $(CONFIG), (2) docker compose <action> $(ARGS).
## Step 2 runs only if step 1 passes. A plain `make compose` (no action) prints the usage and fails.
compose:
	@test -n "$(filter $(COMPOSE_ACTION),$(COMPOSE_ACTIONS))" -a $(words $(MAKECMDGOALS)) -eq 2 || \
		{ echo "usage: make compose up|down|ps|logs [ARGS=\"...\"]"; exit 2; }
	@$(MAKE) --no-print-directory env
	docker compose --env-file $(ENV_OUT) $(COMPOSE_ACTION) $(ARGS)

# The action words are goals too; they do nothing by themselves (`make compose` runs them).
# Alone (for example `make up`) they print the usage and fail.
$(COMPOSE_ACTIONS):
	@test "$(firstword $(MAKECMDGOALS))" = compose || \
		{ echo "usage: make compose $@ [ARGS=\"...\"]"; exit 2; }

## test-scripts: run the tests of the env generator.
test-scripts: venv
	$(VENV_PY) -m unittest discover -s $(GEN_DIR) -p '*_test.py'

## test: run the Go tests (go test $(GO_TEST_ARGS) $(GO_TEST_PKGS)) in ONE new database qabz_test_<random hex>
## on the Postgres server of $(ENV_OUT) (make compose up; the role needs CREATEDB). The tests get its DSN
## in QABZ_TEST_RUN_DSN; the database is dropped at the end, also when a test fails or on Ctrl-C.
test: env
	@cd src && admin=$$($(TESTDB) admin-dsn "$(TESTDB_ENV)") && name=$$($(TESTDB) name) || exit 1; \
	trap 'QABZ_TEST_ADMIN_DSN="$$admin" $(TESTDB) drop "$$name"' EXIT; \
	trap 'exit 130' INT; trap 'exit 143' TERM; \
	run=$$(QABZ_TEST_ADMIN_DSN="$$admin" $(TESTDB) create "$$name") || exit 1; \
	QABZ_TEST_RUN_DSN="$$run" go test $(GO_TEST_ARGS) $(GO_TEST_PKGS)

## test-db-clean: drop every qabz_test_% database, then every qabz_test_ role, on the Postgres server of
## $(ENV_OUT): the leftovers of a killed `make test`. Nothing else removes them. A role with objects or
## privileges outside the test databases is kept and named, and the target then exits 1. It also drops the
## database of a `make test` that is still running: run it only when no `make test` is running.
test-db-clean: env
	@cd src && admin=$$($(TESTDB) admin-dsn "$(TESTDB_ENV)") && \
	QABZ_TEST_ADMIN_DSN="$$admin" $(TESTDB) clean

.PHONY: proto
## proto: regenerate Go, gRPC, gateway and swagger code from pb/proto/$(PB_DIR), the Qabzino backend client (QabzinoProxy) from pb/proto/qabzino and the irancell service client from pb/proto/irancell. Run after every fresh pull.
proto:
	@test -d $(PB_PROTO)/$(PB_DIR) || { echo "missing $(PB_PROTO)/$(PB_DIR): run 'git submodule update --init'"; exit 1; }
	@for tool in protoc protoc-gen-go protoc-gen-go-grpc protoc-gen-grpc-gateway protoc-gen-openapiv2; do \
		command -v $$tool >/dev/null || { echo "missing tool: $$tool"; exit 1; }; \
	done
	rm -rf $(PB_OUT)/$(PB_DIR) $(SWAG_OUT)/$(PB_DIR)
	mkdir -p $(PB_OUT) $(SWAG_OUT)
	protoc -I $(PB_PROTO) -I $(PB_AUX) \
		--go_out $(PB_OUT) \
		--go_opt paths=source_relative \
		--go-grpc_out $(PB_OUT) \
		--go-grpc_opt paths=source_relative \
		--grpc-gateway_out $(PB_OUT) \
		--grpc-gateway_opt logtostderr=true \
		--grpc-gateway_opt paths=source_relative \
		--openapiv2_out=logtostderr=true,repeated_path_param_separator=ssv:./$(SWAG_OUT) \
		--openapiv2_opt use_go_templates=true \
		$(PB_PROTO)/$(PB_DIR)/*.proto
	@# The Qabzino backend client (task 021): only the internal inquiry service, read-only from pb/ (conventions § 6).
	rm -rf $(PB_OUT)/qabzino
	protoc -I $(PB_PROTO) -I $(PB_AUX) \
		--go_out $(PB_OUT) \
		--go_opt paths=source_relative \
		--go-grpc_out $(PB_OUT) \
		--go-grpc_opt paths=source_relative \
		$(PB_PROTO)/qabzino/internalService.proto
	@# The irancell service client (task 035): only its internal service, read-only from pb/ (conventions § 6).
	@# The proto has the Go package of another repo, so the M options give it the package qabz/api/pb/irancell.
	rm -rf $(PB_OUT)/irancell
	protoc -I $(PB_PROTO) -I $(PB_AUX) \
		--go_out $(PB_OUT) \
		--go_opt paths=source_relative \
		--go_opt 'Mirancell/internalService.proto=qabz/api/pb/irancell;irancell' \
		--go-grpc_out $(PB_OUT) \
		--go-grpc_opt paths=source_relative \
		--go-grpc_opt 'Mirancell/internalService.proto=qabz/api/pb/irancell;irancell' \
		$(PB_PROTO)/irancell/internalService.proto
	@# Task 034: Qabz has no client of the tavanir microservice any more; remove the code of an older run.
	rm -rf $(PB_OUT)/tavanir
