# Go 1.27 toolchain: the one on PATH (/usr/local/go), else the user install in $HOME/sdk.
GO      ?= $(shell command -v go 2>/dev/null || echo $(HOME)/sdk/go1.27.4/bin/go)
BIN     := bin/ytgrab
# x86-64-v3: AVX2/BMI2/FMA. Every CPU since ~2015 (Haswell/Zen). Use v1 for old machines.
GOAMD64 ?= v3

# -trimpath: reproducible, no local paths. -s -w: strip symbols/DWARF (smaller binary).
# -pgo=auto: uses cmd/ytgrab/default.pgo when present (profile-guided optimization).
BUILD_FLAGS := -trimpath -buildvcs=false -pgo=auto -ldflags="-s -w"

.PHONY: build test race bench vet pgo clean

build:
	CGO_ENABLED=0 GOAMD64=$(GOAMD64) $(GO) build $(BUILD_FLAGS) -o $(BIN) ./cmd/ytgrab

test: vet
	$(GO) test ./...

race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

bench:
	$(GO) test -run=^$$ -bench=. -benchmem ./internal/...

# Profile a real batch and feed it back to the compiler:
#   make pgo URLS=urls.txt && make build
pgo: build
	./$(BIN) -cpuprofile cmd/ytgrab/default.pgo $(URLS)

clean:
	rm -rf bin
