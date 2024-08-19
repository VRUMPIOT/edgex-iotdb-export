.PHONY: build test unittest lint clean prepare update docker

GO=CGO_ENABLED=0 go

MICROSERVICE=iotdb-export

.PHONY: $(MICROSERVICE)

ARCH=$(shell uname -m)

DOCKERS=docker_iotdb_export
.PHONY: $(DOCKERS)

GIT_SHA=$(shell git rev-parse HEAD)

APPVERSION=$(shell cat ./VERSION 2>/dev/null || echo 0.0.0)

SDKVERSION=$(shell cat ./go.mod | grep 'github.com/edgexfoundry/app-functions-sdk-go v' | sed 's/require//g' | awk '{print $$2}')

GOFLAGS=-ldflags "-X github.com/edgexfoundry/app-functions-sdk-go/internal.SDKVersion=$(SDKVERSION) \
	-X github.com/edgexfoundry/app-functions-sdk-go/internal.ApplicationVersion=$(APPVERSION)"

build:
	go mod tidy
	$(GO) build $(GOFLAGS) -o $(MICROSERVICE) .

tidy:
	go mod tidy

unittest:
	go test ./... -coverprofile=coverage.out

lint:
	@which golangci-lint >/dev/null || echo "WARNING: go linter not installed. To install, run make install-lint"
	@if [ "z${ARCH}" = "zx86_64" ] && which golangci-lint >/dev/null ; then golangci-lint run --config .golangci.yml ; else echo "WARNING: Linting skipped (not on x86_64 or linter not installed)"; fi

install-lint:
	sudo curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin v1.54.2

test: unittest lint
	go vet ./...
	gofmt -l $$(find . -type f -name '*.go'| grep -v "/vendor/")
	[ "`gofmt -l $$(find . -type f -name '*.go'| grep -v "/vendor/")`" = "" ]
	./bin/test-attribution-txt.sh
	
clean:
	rm -f $(MICROSERVICE)

docker: $(DOCKERS)

docker_iotdb_export:
	docker build \
		--build-arg ADD_BUILD_TAGS=$(ADD_BUILD_TAGS) \
		--label "git_sha=$(GIT_SHA)" \
		-t vrump/edgex-iotdb-export:$(GIT_SHA) \
		-t vrump/edgex-iotdb-export:$(APPVERSION) \
		.

vendor:
	$(GO) mod vendor