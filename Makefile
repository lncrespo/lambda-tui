NAME        := lambdatui
GO_FLAGS    ?=
CGO_ENABLED ?=0
BUILD_DIR   ?= build

run: build
	./build/lambdatui

build:
	@CGO_ENABLED=${CGO_ENABLED} go build ${GO_FLAGS} -o ${BUILD_DIR}/${NAME}
.PHONY: build

clean:
	rm -r ./${BUILD_DIR}

fix:
	gofmt -e -s -w .
