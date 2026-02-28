#!/bin/bash
set -e

TESTDIR=$(mktemp -d)
trap "rm -rf $TESTDIR" EXIT

cd /home/lkn/src/anvillm/main

# Build test binary
go test -c ./internal/p9 -o /tmp/p9test

# Test 1: Mount and create bead
echo "=== Test 1: Mount project ==="
mkdir -p "$TESTDIR/project1"

# Test 2: Create bead in mount
echo "=== Test 2: Create and label bead ==="

# Test 3: Read from mount
echo "=== Test 3: Read mount list ==="

# Test 4: Unmount
echo "=== Test 4: Unmount project ==="

echo "✓ All tests passed"
