.PHONY: deps run build-nvidia test gen build-llama build-llama-cublas

deps:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
	&& go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
	$(MAKE) -C llama -f Makefile deps

run:
	go run -tags="llama,nvidia" ./cmd/llm-runner

build-nvidia:
	@mkdir -p build
	go build -tags="llama,nvidia" -o build/llm-runner ./cmd/llm-runner

test:
	go test ./...

gen:
	mkdir -p ./pb
	protoc --proto_path=./ \
		--go_out=paths=source_relative:./pb \
		--go-grpc_out=paths=source_relative:./pb \
		./llmrunner.proto

build-llama:
	$(MAKE) -C llama libllama.a

build-llama-cublas:
	$(MAKE) -C llama libllama.a BUILD_TYPE=cublas
