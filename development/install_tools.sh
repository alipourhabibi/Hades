#!/bin/bash

set -e

if ! command -v go &> /dev/null; then
    echo "Go is not installed. Please install Go before running this script."
    exit 1
fi

install_buf() {
    echo "Checking for buf CLI..."
    if ! command -v buf &> /dev/null; then
        echo "buf CLI not found. Installing..."
        go install github.com/bufbuild/buf/cmd/buf@latest
        echo "buf CLI installed successfully."
    else
        echo "buf CLI is already installed."
    fi
}

install_grpcurl() {
    echo "Checking for grpcurl..."
    if ! command -v grpcurl &> /dev/null; then
        echo "grpcurl not found. Installing..."
        go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest
        echo "grpcurl installed successfully."
    else
        echo "grpcurl is already installed."
    fi
}

echo "Starting installation of development tools..."
install_buf
install_grpcurl
echo "All tools installed successfully."
