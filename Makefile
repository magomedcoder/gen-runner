.PHONY: install
install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
	&& go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

.PHONY: gen
gen:
	@for proto in ./api/proto/*.proto; do \
		name=$$(basename $$proto .proto); \
		protoc --proto_path=./api/proto \
			--go_out=paths=source_relative:./api/pb \
			--go-grpc_out=paths=source_relative:./api/pb \
			$$proto; \
	done

	protoc --proto_path=./api/proto \
		--dart_out=grpc:./client-app/lib/generated/grpc_pb \
		./api/proto/*.proto

	protoc --proto_path=./llm-runner \
		--go_opt=Mllmrunner.proto=github.com/magomedcoder/gen/api/pb/llmrunner \
		--go-grpc_opt=Mllmrunner.proto=github.com/magomedcoder/gen/api/pb/llmrunner \
		--go_out=module=github.com/magomedcoder/gen:. \
		--go-grpc_out=module=github.com/magomedcoder/gen:. \
		./llm-runner/llmrunner.proto

.PHONY: run
run:
	go run ./cmd/gen
