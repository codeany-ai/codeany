VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  = -s -w \
  -X github.com/codeany-ai/codeany/internal/version.Version=$(VERSION) \
  -X github.com/codeany-ai/codeany/internal/version.Commit=$(COMMIT) \
  -X github.com/codeany-ai/codeany/internal/version.Date=$(DATE)

.PHONY: build install clean dev vet

build:
	go build -ldflags "$(LDFLAGS)" -o codeany ./cmd/codeany

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/codeany

dev:
	go run ./cmd/codeany

vet:
	go vet ./...

clean:
	rm -f codeany
	rm -rf dist

dist:
	mkdir -p dist
	@for pair in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/arm64 windows/amd64; do \
		os=$${pair%/*}; arch=$${pair#*/}; \
		ext=""; [ "$$os" = "windows" ] && ext=".exe"; \
		echo "Building $$os/$$arch..."; \
		GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 \
			go build -ldflags "$(LDFLAGS)" \
			-o "dist/codeany_$${os}_$${arch}/codeany$$ext" \
			./cmd/codeany; \
	done
	@cd dist && for d in codeany_darwin_* codeany_linux_*; do \
		tar -czf "$${d}.tar.gz" -C "$$d" codeany; \
	done
	@cd dist && for d in codeany_windows_*; do \
		(cd "$$d" && zip -q "../$${d}.zip" codeany.exe); \
	done
