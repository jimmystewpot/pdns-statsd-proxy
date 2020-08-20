#!/usr/bin/make
SHELL  := /bin/bash

export PATH = /usr/bin:/usr/local/bin:/usr/local/sbin:/usr/sbin:/bin:/sbin:/go/bin:/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/build/bin

BINPATH := bin
GO_DIR := src/github.com/jimmystewpot/pdns-statsd-proxy/
DOCKER_IMAGE := golang:1.15-buster
TOOL := pdns-statsd-proxy

get-golang:
	docker pull ${DOCKER_IMAGE}

.PHONY: clean
clean:
	@echo $(shell docker images -qa -f 'dangling=true'|egrep '[a-z0-9]+' && docker rmi $(shell docker images -qa -f 'dangling=true'))

#
# dependency update. Look for updated versions and add them to vendoring. Or simply download the versions in the yaml file.
#
dep-update:
	dep ensure -update

dep-download:
	dep ensure

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

build-all: pdns-statsd-proxy

pdns-statsd-proxy:
	@echo ""
	@echo "***** Building PowerDNS statistics proxy *****"
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
	go build -ldflags="-s -w" -o $(BINPATH)/$(TOOL) ./cmd/$(TOOL)
	@echo ""

# install used when building locally.
install:
	install -g 0 -o 0 -m 0755 -D ./$(BINPATH)/$(TOOL) /opt/$(TOOL)/$(TOOL)
	install -g 0 -o 0 -m 0755 -D ./systemd/$(TOOL).service /usr/lib/systemd/system/$(TOOL).service
