#!/usr/bin/make
SHELL  := /bin/bash

export PATH = /usr/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/bin:/sbin:/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/build/bin

BINPATH := bin
GO_DIR := src/github.com/jimmystewpot/pdns-statsd-proxy/
DOCKER_IMAGE := golang:1.16-stretch
SYNK_IMAGE := snyk/snyk:golang
TOOL := pdns-statsd-proxy

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

build-all: test pdns-statsd-proxy

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
