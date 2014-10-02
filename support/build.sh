#!/bin/bash -eu

echo "==> Getting dependencies..."
go get -v ./...

echo "==> Building..."
go build -v ./...
