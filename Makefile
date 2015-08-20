# Copyright 2014 The Prometheus Authors
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

VERSION  := 0.2.0
TARGET   := pushgateway

REV        := $(shell git rev-parse --short HEAD 2> /dev/null  || echo 'unknown')
BRANCH     := $(shell git rev-parse --abbrev-ref HEAD 2> /dev/null  || echo 'unknown')
HOSTNAME   := $(shell hostname -f)
BUILD_DATE := $(shell date +%Y%m%d-%H:%M:%S)
GOFLAGS := -ldflags \
        "-X main.buildVersion $(VERSION)\
         -X main.buildRev $(REV)\
         -X main.buildBranch $(BRANCH)\
         -X main.buildUser $(USER)@$(HOSTNAME)\
         -X main.buildDate $(BUILD_DATE)"

include Makefile.COMMON

$(BINARY): bindata.go

bindata.go: $(GOPATH)/bin/go-bindata $(shell find resources -type f)
	$(GOPATH)/bin/go-bindata -prefix=resources resources/...

# Target to unconditionally compile the debug bindata.
.PHONY: bindata-debug
bindata-debug: $(GOPATH)/bin/go-bindata
	$(GOPATH)/bin/go-bindata -debug -prefix=resources resources/...

# Target to unconditionally compile the embedded bindata.
.PHONY: bindata-embed
bindata-embed: $(GOPATH)/bin/go-bindata
	$(GOPATH)/bin/go-bindata -prefix=resources resources/...

$(GOPATH)/bin/go-bindata:
	$(GO) get github.com/jteeuwen/go-bindata/...
