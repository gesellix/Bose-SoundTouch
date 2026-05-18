#!/bin/bash
set -e

# Directory where libraries are stored
LIB_DIR="pkg/service/soundtouchweb/static/lib"
mkdir -p "$LIB_DIR"

echo "Updating static frontend dependencies from node_modules..."

# Ensure dependencies are installed
if [ ! -d "node_modules" ] || [ ! -d "node_modules/preact" ] || [ ! -d "node_modules/htm" ]; then
    if [ "$CI" = "true" ]; then
        echo "node_modules not found or incomplete. Running npm ci in CI environment..."
        npm ci
    else
        echo "node_modules not found or incomplete. Running npm install..."
        npm install
    fi
fi

# Copy files from node_modules
echo "Copying Preact..."
cp node_modules/preact/dist/preact.module.js "$LIB_DIR/preact.module.js"

echo "Copying Preact Hooks..."
cp node_modules/preact/hooks/dist/hooks.module.js "$LIB_DIR/preact-hooks.module.js"

echo "Copying HTM..."
cp node_modules/htm/dist/htm.module.js "$LIB_DIR/htm.module.js"

echo "All dependencies updated successfully from node_modules."
