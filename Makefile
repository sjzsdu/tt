APP := tt
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build web-install web-build install install-system clean run fmt tidy help

build: web-build
	go build -o $(APP) .

web-install:
	cd web && npm install

web-build:
	cd web && npm install && npm run build:markdown
	cd web && npm run build:formula
	rm -rf internal/webui/markdown/dist
	rm -rf internal/webui/formula/dist
	mkdir -p internal/webui/markdown
	mkdir -p internal/webui/formula
	cp -R web/apps/markdown/dist internal/webui/markdown/dist
	cp -R web/apps/formula/dist internal/webui/formula/dist

install: web-build
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/$(APP) .
	@printf "Installed to $(BINDIR)/$(APP)\n"

install-system: web-build
	mkdir -p /usr/local/bin
	go build -o /usr/local/bin/$(APP) .

clean:
	rm -f $(APP)
	rm -rf web/node_modules web/apps/markdown/dist web/apps/formula/dist internal/webui/markdown/dist internal/webui/formula/dist

run:
	go run .

fmt:
	gofmt -w main.go cmd/*.go

tidy:
	go mod tidy

help:
	@printf "Targets:\n"
	@printf "  make build           Build web UI and ./$(APP)\n"
	@printf "  make web-build       Build React web UI into internal/webui\n"
	@printf "  make install         Install to $(BINDIR)/$(APP)\n"
	@printf "  make install-system  Install to /usr/local/bin/$(APP)\n"
	@printf "  make clean           Remove local binary\n"
	@printf "  make run             Run with go run\n"
	@printf "  make fmt             Format Go files\n"
	@printf "  make tidy            Tidy go modules\n"
