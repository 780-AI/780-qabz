PB_DIR    ?= qabz
PB_PROTO  := pb/proto
PB_AUX    := pb/aux
PB_OUT    := src/api/pb
SWAG_OUT  := src/api/swagger

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
