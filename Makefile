.PHONY: install gen gen-proto run

install:
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest \
	&& go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

gen-proto:
	@for proto in ./api/proto/*.proto; do \
		name=$$(basename $$proto .proto); \
		mkdir -p ./api/pb/$${name}pb; \
		protoc --proto_path=./api/proto \
			--go_out=paths=source_relative:./api/pb/$${name}pb \
			--go-grpc_out=paths=source_relative:./api/pb/$${name}pb \
			$$proto; \
	done

	mkdir -p ./api/pb/llmrunnerpb
	protoc --proto_path=../gen-runner \
		--go_opt=Mllmrunner.proto=github.com/magomedcoder/gen/api/pb/llmrunnerpb \
		--go-grpc_opt=Mllmrunner.proto=github.com/magomedcoder/gen/api/pb/llmrunnerpb \
		--go_out=module=github.com/magomedcoder/gen:. \
		--go-grpc_out=module=github.com/magomedcoder/gen:. \
		../gen-runner/llmrunner.proto

	mkdir -p ./lib/generated/grpc_pb
	protoc --proto_path=../gen/api/proto \
		--dart_out=grpc:./lib/generated/grpc_pb \
		../gen/api/proto/*.proto

run:
	go run ./cmd/gen
