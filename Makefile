#!/usr/bin/make
SHELL  := /bin/bash

export PATH := $(PATH):/usr/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/bin:/sbin:/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/build/bin

BINPATH := bin
GO_DIR := src/github.com/jimmystewpot/pdns-statsd-proxy/
DOCKER_IMAGE := golang:1.19-bullseye
SYNK_IMAGE := snyk/snyk:golang
TOOL := pdns-statsd-proxy
INTERACTIVE := $(shell [ -t 0 ] && echo 1)

build-all: deps lint clean-arch test pdns-statsd-proxy

deps:
	@echo ""
	@echo "***** Installing dependencies for PowerDNS statistics proxy *****"
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.50.1
	go install github.com/roblaszczak/go-cleanarch@latest



lint:
ifdef INTERACTIVE
	@echo $$PATH
	@echo $$GOPATH
	golangci-lint run -v $(TEST_DIRS)
else
	@echo $$PATH
	@echo $$GOPATH
	golangci-lint run --out-format checkstyle -v $(TEST_DIRS) 1> reports/checkstyle-lint.xml
endif
.PHONY: lint

clean-arch: deps
	@echo ""
	@echo "***** Testing clean arch for PowerDNS statistics proxy *****"
	go-cleanarch cmd/pdns-statsd-proxy/

get-golang:
	docker pull ${DOCKER_IMAGE}

get-synk:
	docker pull ${SYNK_IMAGE}

.PHONY: clean
clean:
	@echo $(shell docker images -qa -f 'dangling=true'|egrep '[a-z0-9]+' && docker rmi $(shell docker images -qa -f 'dangling=true'))

#
# build the software
#
build: get-golang
	@docker run \
		--rm \
		-v $(CURDIR):/build/$(GO_DIR) \
		--workdir /build/$(GO_DIR) \
		-e GOPATH=/build \
		-e PATH=$(PATH) \
		-t ${DOCKER_IMAGE} \
		make build-all

pdns-statsd-proxy:
	@echo ""
	@echo "***** Building PowerDNS statistics proxy *****"
	GOOS=linux GOARCH=amd64 \
	go build -race -ldflags="-s -w" -o $(BINPATH)/$(TOOL) ./cmd/$(TOOL)
	@echo ""

test:
	@echo ""
	@echo "***** Testing PowerDNS statistics proxy *****"
	GOOS=linux GOARCH=amd64 \
	go test -a -v -race -coverprofile=coverage.txt -covermode=atomic ./cmd/$(TOOL)
	@echo ""

test-synk: get-synk
	@echo ""
	@echo "***** Testing vulnerabilities using Synk *****"
	@docker run \
		--rm \
		-v $(CURDIR):/build/$(GO_DIR) \
		--workdir /build/$(GO_DIR) \
		-e SNYK_TOKEN=${SYNK_TOKEN} \
		-e MONITOR=true \
		-t ${SYNK_IMAGE}

# install used when building locally.
install:
	install -g 0 -o 0 -m 0755 -D ./$(BINPATH)/$(TOOL) /opt/$(TOOL)/$(TOOL)
	install -g 0 -o 0 -m 0755 -D ./systemd/$(TOOL).service /usr/lib/systemd/system/$(TOOL).service
