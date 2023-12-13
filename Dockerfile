# syntax=docker/dockerfile:1

ARG GO_VERSION=1.21
ARG REPO=github.com/LiterMC/go-openbmclapi

FROM golang:${GO_VERSION}-alpine AS BUILD

ARG REPO
ARG TAG

COPY ./go.mod ./go.sum "/go/src/${REPO}/"
COPY ./src "/go/src/${REPO}/src"

RUN --mount=type=cache,target=/root/.cache/go-build cd "/go/src/${REPO}" && \
 CGO_ENABLED=0 go build -v -o "/go/bin/application" -ldflags="-X 'main.BuildVersion=${TAG}'" "./src"

FROM alpine:latest

WORKDIR /web/work
COPY ./config.json /web/work/config.json

COPY --from=BUILD "/go/bin/application" "/web/application"

CMD ["/web/application"]
