#! /bin/bash

set -e

export GOPATH=$PWD/Godeps/_workspace:$GOPATH
pkgs="./pkg ./cmd"
docker_tag="quay.io/porch/k8s-haproxy"

function build::clean() {
	rm ./bin/k8s-haproxy
}

function build::tools() {
	go get github.com/tools/godep
	go get github.com/golang/lint/golint
	go get golang.org/x/tools/cmd/vet
	go get golang.org/x/tools/cmd/godoc
	go get golang.org/x/tools/cmd/goimports
}

function build::deps() {
	godep restore
}

function build::analysis() {
	go vet $1
}

function build::format() {
	goimports -d -e -l -w $1
}

function build::build() {
	go build -o ./bin/k8s-haproxy ./cmd
}

function build::docker() {
	if $(command -v docker >/dev/null 2>&1); then
		docker build -t $docker_tag .
	else
		echo "Docker is not installed, skipping docker build."
	fi
}

function build::success() {
	echo "Done (checkout ./bin/)"
	exit 0
}

build::tools
build::deps

for pkg in $pkgs; do
	build::analysis $pkg
	build::format $pkg
done

build::build
build::docker
