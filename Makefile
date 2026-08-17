DATE=$(shell date -u +%Y-%m-%d)
VERSION=$(shell cat VERSION | sed 's/-dev//g')

#########################################
# Targets                                 #
#########################################

.PHONY: format
format:
	@./hack/format.sh ./cmd ./pkg

.PHONY: test
test:
	@./hack/test.sh ./pkg/...

.PHONY: check
check:
	@./hack/test.sh ./pkg/...
	@./hack/check.sh ./cmd/... ./pkg/...

.PHONY: build
build: build-kswitch

.PHONY: build-kswitch
build-kswitch:
	@env GOOS=linux GOARCH=amd64 go build -ldflags "-w -X github.com/MichaelSp/kswitch/cmd/kswitch.version=${VERSION} -X github.com/MichaelSp/kswitch/cmd/kswitch.buildDate=${DATE}" -o hack/switch/kubectl-switch_linux_amd64 .
	@env GOOS=linux GOARCH=arm64 go build -ldflags "-w -X github.com/MichaelSp/kswitch/cmd/kswitch.version=${VERSION} -X github.com/MichaelSp/kswitch/cmd/kswitch.buildDate=${DATE}" -o hack/switch/kubectl-switch_linux_arm64 .
	@env GOOS=darwin GOARCH=amd64 go build -ldflags "-w -X github.com/MichaelSp/kswitch/cmd/kswitch.version=${VERSION} -X github.com/MichaelSp/kswitch/cmd/kswitch.buildDate=${DATE}" -o hack/switch/kubectl-switch_darwin_amd64 .
	@env GOOS=darwin GOARCH=arm64 go build -ldflags "-w -X github.com/MichaelSp/kswitch/cmd/kswitch.version=${VERSION} -X github.com/MichaelSp/kswitch/cmd/kswitch.buildDate=${DATE}" -o hack/switch/kubectl-switch_darwin_arm64 .
	@env GOOS=windows GOARCH=amd64 go build -ldflags "-w -X github.com/MichaelSp/kswitch/cmd/kswitch.version=${VERSION} -X github.com/MichaelSp/kswitch/cmd/kswitch.buildDate=${DATE}" -o 'hack/switch/kubectl-switch_windows_amd64.exe' .

.PHONY: all
all: format check build
