.PHONY: deps deps-llama.cpp llama.cpp-update clean

LLAMA_CPP_TAG ?= b7860

deps: deps-llama.cpp
deps-llama.cpp:
	@if [ ! -d "llama.cpp/.git" ]; then \
		echo "Клонируем llama.cpp $(LLAMA_CPP_TAG) в llama.cpp..."; \
		git clone --depth 1 --branch $(LLAMA_CPP_TAG) https://github.com/ggml-org/llama.cpp.git llama.cpp; \
	else \
		echo "llama.cpp уже существует"; \
	fi

llama.cpp-update:
	@if [ ! -d "llama.cpp/.git" ]; then $(MAKE) deps-llama.cpp; exit 0; fi
	cd llama.cpp \
		&& git fetch --depth 1 origin tag $(LLAMA_CPP_TAG) \
		&& git checkout $(LLAMA_CPP_TAG)

INCLUDE_PATH := $(abspath ./)
LIBRARY_PATH := $(abspath ./)

ifndef UNAME_S
UNAME_S := $(shell uname -s)
endif

ifndef UNAME_P
UNAME_P := $(shell uname -p)
endif

ifndef UNAME_M
UNAME_M := $(shell uname -m)
endif

CCV := $(shell $(CC) --version | head -n 1)
CXXV := $(shell $(CXX) --version | head -n 1)

ifeq ($(UNAME_S),Darwin)
	ifneq ($(UNAME_P),arm)
		SYSCTL_M := $(shell sysctl -n hw.optional.arm64 2>/dev/null)
		ifeq ($(SYSCTL_M),1)
			# UNAME_P := arm
			# UNAME_M := arm64
			warn := $(warning Архитектура объявлена как x86_64, но похоже фактически это ARM64. Без исправления возможна низкая производительность. Подробнее: https://github.com/ggerganov/whisper.cpp/issues/66\#issuecomment-1282546789)
		endif
	endif
endif

BUILD_TYPE?=
CFLAGS   = -I./llama.cpp -I. -O3 -DNDEBUG -std=c11 -fPIC
CXXFLAGS = -I./llama.cpp -I. -I./llama.cpp/common -I./common -I./llama.cpp/ggml/include -I./llama.cpp/include -I./llama.cpp/vendor -O3 -DNDEBUG -std=c++17 -fPIC
LDFLAGS  =

CFLAGS   += -Wall -Wextra -Wpedantic -Wcast-qual -Wdouble-promotion -Wshadow -Wstrict-prototypes -Wpointer-arith -Wno-unused-function
CXXFLAGS += -Wall -Wextra -Wpedantic -Wcast-qual -Wno-unused-function

ifeq ($(UNAME_S),Linux)
	CFLAGS   += -pthread
	CXXFLAGS += -pthread
endif
ifeq ($(UNAME_S),Darwin)
	CFLAGS   += -pthread
	CXXFLAGS += -pthread
endif
ifeq ($(UNAME_S),FreeBSD)
	CFLAGS   += -pthread
	CXXFLAGS += -pthread
endif
ifeq ($(UNAME_S),NetBSD)
	CFLAGS   += -pthread
	CXXFLAGS += -pthread
endif
ifeq ($(UNAME_S),OpenBSD)
	CFLAGS   += -pthread
	CXXFLAGS += -pthread
endif
ifeq ($(UNAME_S),Haiku)
	CFLAGS   += -pthread
	CXXFLAGS += -pthread
endif

GGML_CUDA_OBJ_PATH=ggml/src/ggml-cuda/CMakeFiles/ggml-cuda.dir/ggml-cuda.cu.o


ifeq ($(UNAME_M),$(filter $(UNAME_M),x86_64 i686))
	CFLAGS += -march=native -mtune=native
endif
ifneq ($(filter ppc64%,$(UNAME_M)),)
	POWER9_M := $(shell grep "POWER9" /proc/cpuinfo)
	ifneq (,$(findstring POWER9,$(POWER9_M)))
		CFLAGS += -mcpu=power9
		CXXFLAGS += -mcpu=power9
	endif
	ifeq ($(UNAME_M),ppc64)
		CXXFLAGS += -std=c++23 -DGGML_BIG_ENDIAN
	endif
endif
ifndef LLAMA_NO_ACCELERATE
	ifeq ($(UNAME_S),Darwin)
		CFLAGS  += -DGGML_USE_ACCELERATE
		LDFLAGS += -framework Accelerate
	endif
endif
ifdef LLAMA_GPROF
	CFLAGS   += -pg
	CXXFLAGS += -pg
endif
ifneq ($(filter aarch64%,$(UNAME_M)),)
	CFLAGS += -mcpu=native
	CXXFLAGS += -mcpu=native
endif
ifneq ($(filter armv6%,$(UNAME_M)),)
	CFLAGS += -mfpu=neon-fp-armv8 -mfp16-format=ieee -mno-unaligned-access
endif
ifneq ($(filter armv7%,$(UNAME_M)),)
	CFLAGS += -mfpu=neon-fp-armv8 -mfp16-format=ieee -mno-unaligned-access -funsafe-math-optimizations
endif
ifneq ($(filter armv8%,$(UNAME_M)),)
	CFLAGS += -mfp16-format=ieee -mno-unaligned-access
endif

ifeq ($(BUILD_TYPE),blis)
	EXTRA_LIBS=
	CMAKE_ARGS+=-DGGML_BLAS=ON -DGGML_BLAS_VENDOR=FLAME
endif

ifeq ($(BUILD_TYPE),cublas)
	EXTRA_LIBS=
	CMAKE_ARGS+=-DGGML_CUDA=ON -DGGML_CUDA_FA_ALL_QUANTS=ON -DGGML_CUDA_GRAPHS=ON
	CXXFLAGS+=-DGGML_USE_CUDA
	ifdef CUDA_ARCHITECTURES
		CMAKE_ARGS+=-DCMAKE_CUDA_ARCHITECTURES="$(CUDA_ARCHITECTURES)"
	endif
	EXTRA_TARGETS+=llama.cpp/ggml-cuda.o
endif

ifdef CLBLAST_DIR
	CMAKE_ARGS+=-DCLBlast_dir=$(CLBLAST_DIR)
endif

ifeq ($(GPU_TESTS),true)
	CGO_LDFLAGS="-lcublas -lcudart -L/usr/local/cuda/lib64/"
	TEST_LABEL=gpu
else
	TEST_LABEL=!gpu
endif

$(info I llama.cpp build info: )
$(info I UNAME_S:  $(UNAME_S))
$(info I UNAME_P:  $(UNAME_P))
$(info I UNAME_M:  $(UNAME_M))
$(info I CFLAGS:   $(CFLAGS))
$(info I CXXFLAGS: $(CXXFLAGS))
$(info I CGO_LDFLAGS:  $(CGO_LDFLAGS))
$(info I LDFLAGS:  $(LDFLAGS))
$(info I BUILD_TYPE:  $(BUILD_TYPE))
$(info I CMAKE_ARGS:  $(CMAKE_ARGS))
$(info I EXTRA_TARGETS:  $(EXTRA_TARGETS))
$(info I CC:       $(CCV))
$(info I CXX:      $(CXXV))
$(info )

llama.cpp/ggml-alloc.o: llama.cpp/ggml.o
	cd build && cp -rf ggml/src/CMakeFiles/ggml-base.dir/ggml-alloc.c.o ../llama.cpp/ggml-alloc.o

llama.cpp/ggml.o:
	mkdir -p build
	cd build && CC="$(CC)" CXX="$(CXX)" cmake ../llama.cpp $(CMAKE_ARGS) -DLLAMA_CURL=OFF && VERBOSE=1 cmake --build . --config Release --target ggml llama mtmd && cp -rf ggml/src/CMakeFiles/ggml-base.dir/ggml.c.o ../llama.cpp/ggml.o

llama.cpp/ggml-cuda.o: llama.cpp/ggml.o
	cd build && cp -rf "$(GGML_CUDA_OBJ_PATH)" ../llama.cpp/ggml-cuda.o

llama.cpp/k_quants.o: llama.cpp/ggml.o
	cd build && cp -rf ggml/src/CMakeFiles/ggml-base.dir/ggml-quants.c.o ../llama.cpp/k_quants.o

llama.cpp/llama.o: llama.cpp/ggml.o
	cd build && cp -rf src/CMakeFiles/llama.dir/llama.cpp.o ../llama.cpp/llama.o

llama.cpp/common.o: llama.cpp/ggml.o
	$(CXX) $(CXXFLAGS) -I./llama.cpp -I./llama.cpp/common -I./llama.cpp/ggml/include -I./llama.cpp/include llama.cpp/common/common.cpp -o llama.cpp/common.o -c $(LDFLAGS)

llama.cpp/sampling.o: llama.cpp/ggml.o
	$(CXX) $(CXXFLAGS) -I./llama.cpp -I./llama.cpp/common -I./llama.cpp/ggml/include -I./llama.cpp/include llama.cpp/common/sampling.cpp -o llama.cpp/sampling.o -c $(LDFLAGS)

llama.cpp/log.o: llama.cpp/ggml.o
	$(CXX) $(CXXFLAGS) -I./llama.cpp -I./llama.cpp/common -I./llama.cpp/ggml/include -I./llama.cpp/include llama.cpp/common/log.cpp -o llama.cpp/log.o -c $(LDFLAGS)

wrapper.o:
	$(CXX) $(CXXFLAGS) -I./llama.cpp -I./llama.cpp/common -I./llama.cpp/ggml/include -I./llama.cpp/include wrapper.cpp -o wrapper.o -c $(LDFLAGS)

libbinding.a: llama.cpp/ggml.o wrapper.o $(EXTRA_TARGETS)
	cd build && cmake --build . --target common
	ar crs libbinding.a wrapper.o $(EXTRA_TARGETS)
	cp build/common/libcommon.a .
ifneq (,$(findstring -DBUILD_SHARED_LIBS=OFF,$(CMAKE_ARGS)))
	cp build/src/libllama.a .
	cp build/ggml/src/libggml.a .
	cp build/ggml/src/libggml-base.a .
	cp build/ggml/src/libggml-cpu.a .
else
	cp build/bin/libllama.so .
	cp build/bin/libggml.so .
	cp build/bin/libggml-base.so .
	cp build/bin/libggml-cpu.so .
	cp build/bin/libmtmd.so .
	ln -sf libllama.so libllama.so.0
	ln -sf libggml.so libggml.so.0
	ln -sf libggml-base.so libggml-base.so.0
	ln -sf libggml-cpu.so libggml-cpu.so.0
	ln -sf libmtmd.so libmtmd.so.0
ifeq ($(BUILD_TYPE),cublas)
	cp build/bin/libggml-cuda.so .
	ln -sf libggml-cuda.so libggml-cuda.so.0
endif
endif

clean:
	rm -rf *.o
	rm -rf *.a
	rm -rf *.so
	rm -rf llama.cpp/*.o
	rm -rf build
