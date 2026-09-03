.PHONY: build test coverage clean cross install tidy changelog release release-patch release-minor release-major

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	docker compose run --rm dev go build -ldflags="$(LDFLAGS)" -o bin/local-mind .

test:
	docker compose run --rm dev go test ./... -v

coverage:
	docker compose run --rm dev go test -coverprofile=coverage.out ./...
	docker compose run --rm dev go tool cover -func=coverage.out

tidy:
	docker compose run --rm dev go mod tidy

clean:
	rm -rf bin/

UNAME_S := $(shell uname -s)
ifeq ($(findstring MINGW,$(UNAME_S)),MINGW)
  GOOS ?= windows
else ifeq ($(findstring MSYS,$(UNAME_S)),MSYS)
  GOOS ?= windows
else ifeq ($(findstring Darwin,$(UNAME_S)),Darwin)
  GOOS ?= darwin
else
  GOOS ?= linux
endif
GOARCH ?= $(if $(filter arm64 aarch64,$(shell uname -m)),arm64,amd64)
EXT := $(if $(filter windows,$(GOOS)),.exe,)
BINARY := bin/local-mind$(EXT)

ifeq ($(GOOS),windows)
  INSTALL_DIR ?= $(LOCALAPPDATA)/Programs/local-mind
else
  INSTALL_DIR ?= $(HOME)/.local/bin
endif

install:
	docker compose run --rm dev sh -c "CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -ldflags='$(LDFLAGS)' -o $(BINARY) ."
	@mkdir -p "$(INSTALL_DIR)"
	cp $(BINARY) "$(INSTALL_DIR)/local-mind$(EXT)"
	@echo "installed local-mind $(VERSION) ($(GOOS)/$(GOARCH)) to $(INSTALL_DIR)/local-mind$(EXT)"

cross:
	docker compose run --rm dev sh -c "\
		CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build -ldflags='$(LDFLAGS)' -o bin/local-mind-linux-amd64 . && \
		CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build -ldflags='$(LDFLAGS)' -o bin/local-mind-linux-arm64 . && \
		CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build -ldflags='$(LDFLAGS)' -o bin/local-mind-darwin-amd64 . && \
		CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build -ldflags='$(LDFLAGS)' -o bin/local-mind-darwin-arm64 . && \
		CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags='$(LDFLAGS)' -o bin/local-mind-windows-amd64.exe ."

# --- Changelog / release (requires git-cliff) ---
.PHONY: _require-git-cliff
_require-git-cliff:
	@command -v git-cliff >/dev/null 2>&1 || { echo "git-cliff is required. See https://git-cliff.org/docs/installation"; exit 1; }

changelog: _require-git-cliff
	git-cliff --output CHANGELOG.md
	@echo "updated CHANGELOG.md"

CURRENT_TAG := $(shell git describe --tags --abbrev=0 2>/dev/null || echo v0.0.0)
MAJOR := $(shell echo $(CURRENT_TAG) | sed 's/^v//' | cut -d. -f1)
MINOR := $(shell echo $(CURRENT_TAG) | sed 's/^v//' | cut -d. -f2)
PATCH := $(shell echo $(CURRENT_TAG) | sed 's/^v//' | cut -d. -f3)

release:
	@BUMP=patch; \
	if git log $$(git describe --tags --abbrev=0)..HEAD --format="%s" | grep -qE '^feat(\(.*\))?!:'; then BUMP=major; \
	elif git log $$(git describe --tags --abbrev=0)..HEAD --format="%B" | grep -q 'BREAKING CHANGE'; then BUMP=major; \
	elif git log $$(git describe --tags --abbrev=0)..HEAD --format="%s" | grep -qE '^feat'; then BUMP=minor; fi; \
	echo "detected: $$BUMP"; \
	$(MAKE) release-$$BUMP

release-patch: _require-git-cliff
	@NEXT=v$(MAJOR).$(MINOR).$(shell echo $$(($(PATCH)+1))); \
	echo "$(CURRENT_TAG) -> $$NEXT"; \
	git-cliff --tag $$NEXT --output CHANGELOG.md && git add CHANGELOG.md && \
	git commit -m "chore: update changelog for $$NEXT" && git tag $$NEXT && \
	{ git push origin HEAD $$NEXT && echo "released $$NEXT"; } || { git tag -d $$NEXT; git reset --soft HEAD~1; echo "push failed — rolled back"; exit 1; }

release-minor: _require-git-cliff
	@NEXT=v$(MAJOR).$(shell echo $$(($(MINOR)+1))).0; \
	echo "$(CURRENT_TAG) -> $$NEXT"; \
	git-cliff --tag $$NEXT --output CHANGELOG.md && git add CHANGELOG.md && \
	git commit -m "chore: update changelog for $$NEXT" && git tag $$NEXT && \
	{ git push origin HEAD $$NEXT && echo "released $$NEXT"; } || { git tag -d $$NEXT; git reset --soft HEAD~1; echo "push failed — rolled back"; exit 1; }

release-major: _require-git-cliff
	@NEXT=v$(shell echo $$(($(MAJOR)+1))).0.0; \
	echo "$(CURRENT_TAG) -> $$NEXT"; \
	git-cliff --tag $$NEXT --output CHANGELOG.md && git add CHANGELOG.md && \
	git commit -m "chore: update changelog for $$NEXT" && git tag $$NEXT && \
	{ git push origin HEAD $$NEXT && echo "released $$NEXT"; } || { git tag -d $$NEXT; git reset --soft HEAD~1; echo "push failed — rolled back"; exit 1; }
