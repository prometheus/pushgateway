# Copyright 2016 The Prometheus Authors
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

include Makefile.common

STATICCHECK_IGNORE = \
  github.com/prometheus/pushgateway/handler/delete.go:SA1019 \
  github.com/prometheus/pushgateway/handler/push.go:SA1019 \
  github.com/prometheus/pushgateway/main.go:SA1019

DOCKER_IMAGE_NAME ?= pushgateway

ifdef DEBUG
	bindata_flags = -debug
endif

assets:
	@echo ">> writing assets"
	@$(GO) get -u github.com/jteeuwen/go-bindata/...
	@go-bindata $(bindata_flags) -prefix=resources resources/...

style:
	@echo ">> checking code style"
	! $(GOFMT) -d $$(find . -path ./vendor -prune -o -path ./bindata.go -prune -o -name '*.go' -print) | grep '^'
