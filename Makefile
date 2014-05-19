VERSION  := 0.0.1

TARGET   := pushgateway

OS   := $(subst Darwin,darwin,$(subst Linux,linux,$(shell uname)))
ARCH := $(subst x86_64,amd64,$(shell uname -m))

GOOS   ?= $(OS)
GOARCH ?= $(ARCH)
GOPKG  := go1.2.$(OS)-$(ARCH).tar.gz
GOROOT ?= $(CURDIR)/.deps/go
GOPATH ?= $(CURDIR)/.deps/gopath
GOCC   := $(GOROOT)/bin/go
GOLIB  := $(GOROOT)/pkg/$(GOOS)_$(GOARCH)
GO     := GOROOT=$(GOROOT) GOPATH=$(GOPATH) $(GOCC)

SUFFIX  := $(GOOS)-$(GOARCH)
BINARY  := bin/$(TARGET)
ARCHIVE := $(TARGET)-$(VERSION).$(SUFFIX).tar.gz

default: build

build: $(BINARY)

.deps/$(GOPKG):
	mkdir -p .deps
	curl -o .deps/$(GOPKG) http://go.googlecode.com/files/$(GOPKG)

$(GOCC): .deps/$(GOPKG)
	tar -C .deps -xzf .deps/$(GOPKG)
	touch $@

$(GOLIB):
	cd .deps/go/src && CGO_ENABLED=0 ./make.bash

dependencies:
	$(GO) get -d

$(BINARY): $(GOCC) $(GOLIB) dependencies
	$(GO) build -o $@

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

mrproper: clean
	rm -rf .deps
	rm -rf $(ARCHIVE)

.PHONY: test tag dependencies clean release upload
