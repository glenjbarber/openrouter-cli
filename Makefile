# Makefile for openrouter-cli.
#
# The tree follows what a FreeBSD port expects, so this file serves two
# audiences. A port maintainer reading it finds the metadata variables a port
# Makefile is written around. A developer running it directly finds the usual
# build, test, and lint targets.
#
# The file is written in the syntax common to BSD make and GNU make, because
# the port host uses BSD make while a developer may reach for GNU make. No
# GNU-only construct is used, since a Makefile that BSD make cannot parse is
# useless to the port.

PORTNAME=	openrouter-cli
PORTVERSION=	0.0.0-dev
PORTREV=	0
CATEGORIES=	devel
MASTER_SITES=	# to be filled in at release

MAINTAINER=	gjb@FreeBSD.org
COMMENT=	Terminal client for the OpenRouter.AI API
WWW=		https://github.com/glenjbarber/openrouter-cli

LICENSE=	BSD3CLAUSE
LICENSE_FILE=	LICENSE
LICENSE_COMMIT=	9b2c308

USES=		go
USE_GITHUB=	yes
GH_ACCOUNT=	glenjbarber
GH_PROJECT=	openrouter-cli

# The Go directive in go.mod is the source of the toolchain version, so it is
# not duplicated here. A second copy would drift.
GO_MODULE=	github.com/glenjbarber/openrouter-cli

# A development build is not stamped with a release version, so the linker
# value falls back to the port version.
GO_LDFLAGS=	-X main.version=${PORTVERSION}

# The build does not stamp VCS information into the binary.
#
# A tree that has no .git beside it refuses to build at all, reporting only
# "error obtaining VCS status" and naming nothing in the source. That is the
# distfile a port is built from, an exported archive, and a copy made by hand,
# so the flag is what keeps those building rather than only a checkout.
#
# What is given up is the revision recorded in the binary, which the port
# Makefile above already states in LICENSE_COMMIT and PORTVERSION, so a reader
# of the build has the revision without it being carried by the executable.
GO_BUILD_FLAGS=	-buildvcs=false

# ---------------------------------------------------------------- developer

PREFIX?=	/usr/local
BINDIR?=	${PREFIX}/bin
GO?=		go

# Output of a plain build. It is kept out of the source tree root so that the
# binary does not sit beside the directories a port expects.
BUILD_DIR?=	build
BIN=		${BUILD_DIR}/${PORTNAME}

GO_FILES=	cmd/openrouter-cli/main.go
SRC?=cmd internal

.PHONY: all build install clean test check lint fmt vet tidy crossbuild help

all: build

## build: compile the client into ${BUILD_DIR}.
build: ${BIN}

# BSD make has no $(shell ...) function, so the source list cannot be expanded
# into a dependency here. The binary is therefore rebuilt whenever the target
# is requested, which costs a second and avoids a stale build.
${BIN}: ${GO_FILES}
	@mkdir -p ${BUILD_DIR}
	${GO} build ${GO_BUILD_FLAGS} -ldflags "${GO_LDFLAGS}" -o ${BIN} ./cmd/openrouter-cli

## install: copy the client into ${BINDIR}.
install: build
	@mkdir -p ${BINDIR}
	install -m 0755 ${BIN} ${BINDIR}/${PORTNAME}

## test: run the test suite.
test:
	${GO} test ./...

## check: run the test suite under the race detector.
check:
	${GO} test -race ./...

## lint: run static analysis and the vet pass.
lint:
	${GO} vet ./...
	@test -z "$$(gofmt -l ${SRC})" || { echo "gofmt reports unformatted files:"; gofmt -l ${SRC}; exit 1; }

## fmt: rewrite the source in the canonical format.
fmt:
	gofmt -w ${SRC}

## tidy: reconcile go.mod and go.sum with the imports.
tidy:
	${GO} mod tidy

## crossbuild: compile for every supported target.
#
# The terminal layer names ioctl requests that differ between the BSD family and
# System V, so a change there breaks a platform that is not the one being
# developed on. This target is what notices, since nothing else would.
# The vet pass is included rather than the build alone, because a build does not
# typecheck test files. A test file naming an ioctl that one platform does not
# carry compiled everywhere and failed only where the constant was missing.
crossbuild:
	@for os in freebsd linux darwin netbsd openbsd; do \
		printf '%-10s ' $$os; \
		GOOS=$$os GOARCH=amd64 $(GO) build ${GO_BUILD_FLAGS} ./... || exit 1; \
		GOOS=$$os GOARCH=amd64 $(GO) vet ./... || exit 1; \
		echo ok; \
	done

## clean: remove build output.
clean:
	rm -rf ${BUILD_DIR}

## help: list the targets.
help:
	@grep -E '^## ' Makefile | sed 's/^## //'