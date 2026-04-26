APP := tt
PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build install install-system clean run fmt tidy help

build:
	go build -o $(APP) .

install:
	mkdir -p $(BINDIR)
	go build -o $(BINDIR)/$(APP) .
	@printf "Installed to $(BINDIR)/$(APP)\n"

install-system:
	mkdir -p /usr/local/bin
	go build -o /usr/local/bin/$(APP) .

clean:
	rm -f $(APP)

run:
	go run .

fmt:
	gofmt -w main.go cmd/*.go

tidy:
	go mod tidy

help:
	@printf "Targets:\n"
	@printf "  make build           Build ./$(APP)\n"
	@printf "  make install         Install to $(BINDIR)/$(APP)\n"
	@printf "  make install-system  Install to /usr/local/bin/$(APP)\n"
	@printf "  make clean           Remove local binary\n"
	@printf "  make run             Run with go run\n"
	@printf "  make fmt             Format Go files\n"
	@printf "  make tidy            Tidy go modules\n"
