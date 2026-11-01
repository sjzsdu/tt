APP := tt
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

# The version comes from the git tag so there is a single source of truth.
# Builds from a tagged commit report the tag; local builds append the distance
# from the last tag. Release CI overrides VERSION with the pushed tag.
VERSION ?= $(or $(shell git describe --tags --always --dirty 2>/dev/null | sed 's/^v//'),dev)
GO_LDFLAGS ?= -X github.com/sjzsdu/tt/cmd.version=$(VERSION)

.PHONY: build web-install web-build web-build-force web-build-markdown web-build-formula web-build-agent web-build-slide web-build-team web-build-beads install install-force install-system clean run fmt tidy help

build: web-build skill-check
	go build -ldflags "$(GO_LDFLAGS)" -o $(APP) .

web-install:
	bash scripts/web-install-if-needed.sh

# The diagram-design skill is fetched at build time (commit + sha256 pinned)
# and embedded into the binary via go:embed. It is gitignored.
skill-fetch:
	bash scripts/fetch-diagram-design.sh

skill-check:
	@test -f internal/docsvisual/skilldata/diagram-design/SKILL.md || { echo "diagram-design skill data missing; run: make skill-fetch" >&2; exit 1; }

web-build: web-install web-build-markdown web-build-formula web-build-agent web-build-slide web-build-team web-build-beads

web-build-markdown:
	bash scripts/web-build-if-needed.sh markdown build:markdown

web-build-formula:
	bash scripts/web-build-if-needed.sh formula build:formula

web-build-agent:
	bash scripts/web-build-if-needed.sh agent build:agent

web-build-slide:
	bash scripts/web-build-if-needed.sh slide build:slide

web-build-team:
	bash scripts/web-build-if-needed.sh team build:team

web-build-beads:
	bash scripts/web-build-if-needed.sh beads build:beads

web-build-force: web-install
	cd web && npm run build:markdown
	cd web && npm run build:formula
	cd web && npm run build:agent
	cd web && npm run build:slide
	cd web && npm run build:team
	cd web && npm run build:beads
	rm -rf internal/webui/markdown/dist internal/webui/formula/dist internal/webui/agent/dist internal/webui/slide/dist internal/webui/team/dist internal/webui/beads/dist
	mkdir -p internal/webui/markdown internal/webui/formula internal/webui/agent internal/webui/slide internal/webui/team internal/webui/beads
	cp -R web/apps/markdown/dist internal/webui/markdown/dist
	cp -R web/apps/formula/dist internal/webui/formula/dist
	cp -R web/apps/agent/dist internal/webui/agent/dist
	cp -R web/apps/slide/dist internal/webui/slide/dist
	cp -R web/apps/team/dist internal/webui/team/dist
	cp -R web/apps/beads/dist internal/webui/beads/dist

install: build
	mkdir -p $(BINDIR)
	install -m 755 $(APP) $(BINDIR)/$(APP)
	@printf "Installed to $(BINDIR)/$(APP)\n"

install-force: web-build-force
	go build -ldflags "$(GO_LDFLAGS)" -o $(APP) .
	mkdir -p $(BINDIR)
	install -m 755 $(APP) $(BINDIR)/$(APP)
	@printf "Installed to $(BINDIR)/$(APP)\n"

install-system: build
	mkdir -p /usr/local/bin
	install -m 755 $(APP) /usr/local/bin/$(APP)

clean:
	rm -f $(APP)
	rm -rf web/node_modules web/apps/markdown/dist web/apps/formula/dist web/apps/agent/dist web/apps/slide/dist web/apps/team/dist internal/webui/markdown/dist internal/webui/formula/dist internal/webui/agent/dist internal/webui/slide/dist internal/webui/team/dist internal/docsvisual/skilldata

run:
	go run .

fmt:
	gofmt -w main.go cmd/*.go

tidy:
	go mod tidy

help:
	@printf "Targets:\n"
	@printf "  make build           Build web UI and ./$(APP)\n"
	@printf "  make web-build       Incrementally build React web UIs into internal/webui\n"
	@printf "  make web-build-team  Build the Team React web UI into internal/webui\n"
	@printf "  make web-build-force Force rebuild all React web UIs into internal/webui\n"
	@printf "  make install         Install to $(BINDIR)/$(APP)\n"
	@printf "  make install-force   Force rebuild + install to $(BINDIR)/$(APP)\n"
	@printf "  make install-system  Install to /usr/local/bin/$(APP)\n"
	@printf "  make clean           Remove local binary\n"
	@printf "  make skill-fetch     Fetch pinned diagram-design skill for go:embed\n"
	@printf "  make run             Run with go run\n"
	@printf "  make fmt             Format Go files\n"
	@printf "  make tidy            Tidy Go modules\n"
