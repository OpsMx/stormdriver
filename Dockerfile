#
# Copyright 2021 OpsMx, Inc.
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

#
# Install the latest versions of our mods.  This is done as a separate step
# so it will pull from an image cache if possible, unless there are changes.
#
FROM golang:1.17.6-alpine3.15 AS buildmod
ENV CGO_ENABLED=0
RUN mkdir /build
WORKDIR /build
COPY go.mod .
COPY go.sum .
RUN go mod download

#
# Compile the code.
#
FROM buildmod AS build-binaries
COPY . .
RUN mkdir /out
RUN go build -ldflags="-s -w" -o /out/stormdriver app/stormdriver/*.go

#
# Establish a base OS image used by all the applications.
#
FROM alpine:3.15 AS base-image
RUN apk update && apk upgrade && apk add ca-certificates curl jq && rm -rf /var/cache/apk/*
RUN update-ca-certificates
RUN mkdir /local /local/ca-certificates && rm -rf /usr/local/share/ca-certificates && ln -s  /local/ca-certificates /usr/local/share/ca-certificates
COPY docker/run.sh /app/run.sh
ENTRYPOINT ["/bin/sh", "/app/run.sh"]

#
# For a base image without an OS, this can be used:
#
#FROM scratch AS base-image
#COPY --from=alpine:3.14 /etc/ssl/cert.pem /etc/ssl/cert.pem

#
# Build the stormdriver image.  This should be a --target on docker build.
#
FROM base-image AS stormdriver-image
WORKDIR /app
COPY --from=build-binaries /out/stormdriver /app
EXPOSE 9000 9102
CMD ["/app/stormdriver"]
