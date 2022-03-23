EXECUTABLE ?= bin/prometheus-conviva-exporter
GOLINT ?= $(shell $(GO) env GOPATH)/bin/golint
GO := CGO_ENABLED=0 go
DATE := $(shell date -u '+%FT%T%z')

LDFLAGS += -X "main.BuildDate=$(DATE)"
LDFLAGS += -X "main.Version=`git rev-parse --short HEAD`"
LDFLAGS += -extldflags '-static'

PACKAGES = $(shell go list ./...)

.PHONY: all
all: build

.PHONY: clean
clean:
	$(GO) clean -i ./...
	rm -rf bin/

.PHONY: fmt
fmt:
	$(GO) fmt $(PACKAGES)

.PHONY: vet
vet:
	$(GO) vet $(PACKAGES)

.PHONY: lint
lint:
	@which $(GOLINT) > /dev/null; if [ $$? -ne 0 ]; then \
		$(GO) get -u golang.org/x/lint/golint; \
	fi
	for PKG in $(PACKAGES); do $(GOLINT) -set_exit_status $$PKG || exit 1; done;

.PHONY: test
test:
	@for PKG in $(PACKAGES); do $(GO) test -cover $$PKG || exit 1; done;

.PHONY: build
build:
	$(GO) build -o $(EXECUTABLE) -v -ldflags '-w $(LDFLAGS)'

.PHONY: install
install:
	$(GO) install -v -ldflags '-w $(LDFLAGS)'

docker:
	docker build -t conviva-prometheus-exporter .
