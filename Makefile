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

.PHONY: venv env compose test-scripts

## venv: create the Python venv with the pinned PyYAML (needed by the env generator).
venv: $(VENV)/.installed
$(VENV)/.installed: $(GEN_DIR)/requirements.txt
	python3 -m venv $(VENV)
	$(VENV)/bin/pip install --quiet -r $(GEN_DIR)/requirements.txt
	touch $@

## env: generate $(ENV_OUT) from $(CONFIG). Never edit the generated file by hand.
env: venv
	$(VENV_PY) $(GEN_DIR)/yml_to_env.py --output $(ENV_OUT) $(CONFIG)

## compose: (1) generate $(ENV_OUT) from $(CONFIG), (2) docker compose up. Step 2 runs only if step 1 passes.
compose: env
	docker compose --env-file $(ENV_OUT) up

## test-scripts: run the tests of the env generator.
test-scripts: venv
	$(VENV_PY) -m unittest discover -s $(GEN_DIR) -p '*_test.py'

.PHONY: proto
## proto: regenerate Go, gRPC, gateway and swagger code from pb/proto/$(PB_DIR). Run after every fresh pull.
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
