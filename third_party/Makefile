LLAMA_CPP_TAG ?= b7789

.PHONY: deps
deps: deps-llama.cpp

deps-llama.cpp:
	@if [ ! -d "llama.cpp/.git" ]; then \
		echo "Клонируем llama.cpp $(LLAMA_CPP_TAG) в llama.cpp..."; \
		git clone --depth 1 --branch $(LLAMA_CPP_TAG) https://github.com/ggml-org/llama.cpp.git llama.cpp; \
	else \
		echo "llama.cpp уже существует"; \
	fi

.PHONY: llama.cpp-update
llama.cpp-update:
	@if [ ! -d "llama.cpp/.git" ]; then $(MAKE) deps-llama.cpp; exit 0; fi
	cd llama.cpp \
		&& git fetch --depth 1 origin tag $(LLAMA_CPP_TAG) \
		&& git checkout $(LLAMA_CPP_TAG)
