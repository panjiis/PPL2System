PROTO_ROOT_DIR = "D:\Developments\Go\SYNTRA\PPL2System staging\proto"
ANALYTICS_PROTO_FILE = $(PROTO_ROOT_DIR)/analytics/analytics_service.proto
PY_OUT_DIR = .

gen-proto-py:
	@echo "Generating Python gRPC code for Analytics..."
	python -m grpc_tools.protoc \
		-I=$(PROTO_ROOT_DIR) \
		--python_out=$(PY_OUT_DIR) \
		--grpc_python_out=$(PY_OUT_DIR) \
		$(ANALYTICS_PROTO_FILE)
	@echo "Done."

.PHONY: gen-proto-py