# Copyright 2014 Prometheus Team
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

VERSION  := 0.0.1

TARGET   := pushgateway

OS   := $(subst Darwin,darwin,$(subst Linux,linux,$(shell uname)))
ARCH := $(subst x86_64,amd64,$(shell uname -m))

GOOS   ?= $(OS)
GOARCH ?= $(ARCH)
GOVER  ?= 1.2.2
GOPKG  := $(subst darwin-amd64,darwin-amd64-osx10.8,go$(GOVER).$(OS)-$(ARCH).tar.gz)
GOROOT ?= $(CURDIR)/.deps/go
GOPATH ?= $(CURDIR)/.deps/gopath
GOCC   := $(GOROOT)/bin/go
GOLIB  := $(GOROOT)/pkg/$(GOOS)_$(GOARCH)
GO     := GOROOT=$(GOROOT) GOPATH=$(GOPATH) $(GOCC)

SUFFIX  := $(GOOS)-$(GOARCH)
BINARY  := bin/$(TARGET)
ARCHIVE := $(TARGET)-$(VERSION).$(SUFFIX).tar.gz

REV        := $(shell git rev-parse --short HEAD)
BRANCH     := $(shell git rev-parse --abbrev-ref HEAD)
HOSTNAME   := $(shell hostname -f)
BUILD_DATE := $(shell date +%Y%m%d-%H:%M:%S)
BUILDFLAGS := -ldflags \
        "-X main.buildVersion $(VERSION)\
         -X main.buildRev $(REV)\
         -X main.buildBranch $(BRANCH)\
         -X main.buildUser $(USER)@$(HOSTNAME)\
         -X main.buildDate $(BUILD_DATE)"

default: build

build: $(BINARY)

.deps/$(GOPKG):
	mkdir -p .deps
	curl -L -o .deps/$(GOPKG) http://storage.googleapis.com/golang/$(GOPKG)

$(GOCC): .deps/$(GOPKG)
	tar -C .deps -xzf .deps/$(GOPKG)
	touch $@

$(GOLIB):
	cd .deps/go/src && CGO_ENABLED=0 ./make.bash

dependencies:
	$(GO) get -d

$(BINARY): $(GOCC) $(GOLIB) dependencies bindata.go
	$(GO) build $(BUILDFLAGS) -o $@

bindata.go: $(GOPATH)/bin/go-bindata resources/*
	$(GOPATH)/bin/go-bindata resources/

# Unconditional compile of the debug bindata.
bindata-debug: $(GOPATH)/bin/go-bindata
	$(GOPATH)/bin/go-bindata -debug resources/

# Unconditional compile of the embedded bindata.
bindata-embed: $(GOPATH)/bin/go-bindata
	$(GOPATH)/bin/go-bindata resources/

$(GOPATH)/bin/go-bindata:
	$(GO) get github.com/jteeuwen/go-bindata/...

$(ARCHIVE): $(BINARY)
	tar -czf $@ bin/

upload: REMOTE     ?= $(error "can't upload, REMOTE not set")
upload: REMOTE_DIR ?= $(error "can't upload, REMOTE_DIR not set")
upload: $(ARCHIVE)
	scp $(ARCHIVE) $(REMOTE):$(REMOTE_DIR)/$(ARCHIVE)

release: REMOTE     ?= $(error "can't release, REMOTE not set")
release: REMOTE_DIR ?= $(error "can't release, REMOTE_DIR not set")
release:
	GOOS=linux  REMOTE=$(REMOTE) REMOTE_DIR=$(REMOTE_DIR) $(MAKE) upload
	GOOS=darwin REMOTE=$(REMOTE) REMOTE_DIR=$(REMOTE_DIR) $(MAKE) upload

test:
	$(GO) test ./...

clean:
	rm -rf bin

# Mr. Proper cleans up .deps and the tar ball.
mrproper: clean
	rm -rf .deps
	rm -rf $(ARCHIVE)

.PHONY: test tag dependencies clean release upload bindata-debug bindata-embed mrproper
