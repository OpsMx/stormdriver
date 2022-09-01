#
# Copyright 2021-2022 OpsMx, Inc.
#
# Licensed under the Apache License, Version 2.0 (the "License")
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#

TARGETS=test local
PLATFORM=linux/amd64,linux/arm64
BUILDX=docker buildx build --pull --platform ${PLATFORM}
IMAGE_PREFIX=docker.flame.org/library/

#
# Build targets.  Adding to these will cause magic to occur.
#

# These are targets for "make local"
BINARIES = stormdriver

# These are the targets for Docker images, used both for the multi-arch and
# single (local) Docker builds.
# Dockerfiles should have a target that ends in -image, e.g. stormdriver-image.
IMAGE_TARGETS = stormdriver

#
# Below here lies magic...
#

all_deps := $(shell find * -name '*.go' | grep -v _test)

now := $(shell date -u +%Y%m%dT%H%M%S)

#
# Default target.
#

.PHONY: all
all: ${TARGETS}

#
# make a buildtime directory to hold the build timestamp files
#
buildtime:
	[ ! -d buildtime ] && mkdir buildtime

#
# set git info details
#
set-git-info:
	@$(eval GIT_BRANCH=$(shell git describe --tags))
	@$(eval GIT_HASH=$(shell git rev-parse ${GIT_BRANCH}))

#
# Build locally, mostly for development speed.
#

.PHONY: local
local: $(addprefix bin/,$(BINARIES))

bin/%:: set-git-info ${all_deps}
	@[ -d bin ] || mkdir bin
	go build -o $@ \
		-ldflags="-X github.com/OpsMx/go-app-base/version.buildType=dev' -X 'github.com/OpsMx/go-app-base/version.gitHash=${GIT_HASH}' -X 'github.com/OpsMx/go-app-base/version.gitBranch=${GIT_BRANCH}'" \
		app/$(@F)/*.go

#
# Multi-architecture image builds
#
.PHONY: images
images: buildtime clean-image-names set-git-info $(addsuffix .tstamp, $(addprefix buildtime/,$(IMAGE_TARGETS)))

buildtime/%.tstamp:: ${all_deps} Dockerfile
	${BUILDX} \
		--tag ${IMAGE_PREFIX}$(patsubst %.tstamp,%,$(@F)):latest \
		--tag ${IMAGE_PREFIX}$(patsubst %.tstamp,%,$(@F)):${GIT_BRANCH} \
		--target $(patsubst %.tstamp,%,$(@F))-image \
		--build-arg GIT_HASH=${GIT_HASH} \
		--build-arg GIT_BRANCH=${GIT_BRANCH} \
		--build-arg BUILD_TYPE=release \
		-f Dockerfile \
		--push .
	echo >> buildtime/image-names.txt ${IMAGE_PREFIX}$(patsubst %.tstamp,%,$(@F)):latest
	echo >> buildtime/image-names.txt ${IMAGE_PREFIX}$(patsubst %.tstamp,%,$(@F)):${GIT_BRANCH}
	@touch $@

.PHONY: image-names
image-names:
	@echo ::set-output name=imageNames::$(shell echo `cat buildtime/image-names.txt` | sed 's/\ /,\ /g')

#
# Test targets
#

.PHONY: test
test:
	go test -race ./...

#
# Clean the world.
#

.PHONY: clean
clean: clean-image-names
	rm -f buildtime/*.tstamp
	rm -f bin/*

.PHONY: really-clean
really-clean: clean

.PHONY: clean-image-names
clean-image-names:
	rm -f buildtime/image-names.txt
