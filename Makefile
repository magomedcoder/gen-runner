.PHONY: install gen gen-proto build-libs-cpu build-libs-gpu build-cpu build-gpu build run-cpu run-gpu run test-llama-cpu test-llama-gpu test clean

install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
	&& go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

LLAMA_DIR := llm-runner/llama
RUN_ENV := LD_LIBRARY_PATH="$(PWD)/$(LLAMA_DIR):$(LD_LIBRARY_PATH)" LIBRARY_PATH="$(PWD)/$(LLAMA_DIR):$(LIBRARY_PATH)"
deps-llm-runner:
	$(MAKE) -C $(LLAMA_DIR) -f Makefile deps

gen-proto:
	@for proto in ./api/proto/app/*.proto; do \
		name=$$(basename $$proto .proto); \
		mkdir -p ./api/pb/app/$${name}pb; \
		protoc --proto_path=./api/proto/app \
			--go_out=paths=source_relative:./api/pb/app/$${name}pb \
			--go-grpc_out=paths=source_relative:./api/pb/app/$${name}pb \
			$$proto; \
	done

	mkdir -p ./api/pb/llm-runner/llmrunnerpb
	protoc --proto_path=./api/proto/llm-runner \
		--go_out=paths=source_relative:./api/pb/llm-runner/llmrunnerpb \
		--go-grpc_out=paths=source_relative:./api/pb/llm-runner/llmrunnerpb \
		./api/proto/llm-runner/llmrunner.proto

	mkdir -p ./client-app/lib/generated/grpc_pb
	protoc --proto_path=./api/proto/app \
		--dart_out=grpc:./client-app/lib/generated/grpc_pb \
		./api/proto/app/*.proto

build:
	mkdir -p ./build
	go build -o ./build/gen-server ./cmd/gen

build-libs-cpu:
	$(MAKE) -C $(LLAMA_DIR) libbinding.a
	ln -sf libllama.so $(LLAMA_DIR)/libllama.so.0
	ln -sf libggml.so $(LLAMA_DIR)/libggml.so.0
	ln -sf libggml-base.so $(LLAMA_DIR)/libggml-base.so.0
	ln -sf libggml-cpu.so $(LLAMA_DIR)/libggml-cpu.so.0

build-libs-gpu:
	$(MAKE) -C $(LLAMA_DIR) libbinding.a BUILD_TYPE=cublas
	ln -sf libllama.so $(LLAMA_DIR)/libllama.so.0
	ln -sf libggml.so $(LLAMA_DIR)/libggml.so.0
	ln -sf libggml-base.so $(LLAMA_DIR)/libggml-base.so.0
	ln -sf libggml-cpu.so $(LLAMA_DIR)/libggml-cpu.so.0
	ln -sf libggml-cuda.so $(LLAMA_DIR)/libggml-cuda.so.0

build-cpu: build-libs-cpu
	@mkdir -p build
	go build -tags="llama" -o build/gen-llm-runner ./cmd/gen-llm-runner

build-gpu: build-libs-gpu
	@mkdir -p build
	go build -tags="llama,nvidia" -o build/gen-llm-runner ./cmd/gen-llm-runner

run-cpu: build-libs-cpu
	$(RUN_ENV) go run -tags="llama" ./cmd/gen-llm-runner serve

run-gpu: build-libs-gpu
	$(RUN_ENV) go run -tags="llama,nvidia" ./cmd/gen-llm-runner serve

run:
	go run ./cmd/gen

clean:
	rm -rf build

test:
	go test ./... -race -count=1

test-llama-cpu: build-libs-cpu
	$(RUN_ENV) go test -tags="llama" ./provider ./service

test-llama-gpu:
	@if command -v nvidia-smi >/dev/null 2>&1 && $(MAKE) build-libs-gpu >/dev/null 2>&1; then \
		$(RUN_ENV) go test -tags="llama,nvidia" ./provider ./service; \
	else \
		echo "Пропуск test-llama-gpu: библиотеки GPU/CUDA недоступны в этой среде"; \
	fi
