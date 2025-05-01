# syntax=docker/dockerfile:1
# Copyright (C) 2023 Kevin Z <zyxkad@gmail.com>
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License as published by
# the Free Software Foundation, either version 3 of the License, or
# (at your option) any later version.
#
# This program is distributed in the hope that it will be useful,
# but WITHOUT ANY WARRANTY; without even the implied warranty of
# MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
# GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program.  If not, see <https://www.gnu.org/licenses/>.

ARG GO_VERSION=1.23
ARG REPO=github.com/LiterMC/go-openbmclapi
ARG NPM_DIR=dashboard

FROM node:21 AS WEB_BUILD

ARG NPM_DIR

WORKDIR /web/
COPY ["${NPM_DIR}/package.json", "${NPM_DIR}/package-lock.json", "/web/"]
RUN --mount=type=cache,target=/root/.npm/_cacache \
 npm ci --progress=false || { cat /root/.npm/_logs/*; exit 1; }
COPY ["${NPM_DIR}", "/web/"]
RUN npm run build || { cat /root/.npm/_logs/*; exit 1; }

FROM golang:${GO_VERSION}-alpine AS BUILD

ARG TAG
ARG REPO
ARG NPM_DIR

WORKDIR "/go/src/${REPO}/"

ENV CGO_ENABLED=0
COPY ./go.mod ./go.sum "/go/src/${REPO}/"
RUN go mod download
COPY . "/go/src/${REPO}"
COPY --from=WEB_BUILD "/web/dist" "/go/src/${REPO}/${NPM_DIR}/dist"

ENV ldflags="-X 'github.com/LiterMC/go-openbmclapi/internal/build.BuildVersion=$TAG'"

RUN --mount=type=cache,target=/root/.cache/go-build \
 go build -v -o "/go/bin/go-openbmclapi" -ldflags="$ldflags" "."

FROM alpine:latest

WORKDIR /opt/openbmclapi
COPY ./config.yaml /opt/openbmclapi/config.yaml

COPY --from=BUILD "/go/bin/go-openbmclapi" "/go-openbmclapi"

ENTRYPOINT ["/go-openbmclapi"]
