#!/bin/bash

# Fix Fiber v3 beta dependencies to use stable v2

echo "🔧 Fixing Fiber v3 beta dependencies to stable v2..."

# Find all go.mod files
GO_MOD_FILES=$(find . -name "go.mod" -type f)

for mod_file in $GO_MOD_FILES; do
    echo "Updating $mod_file..."
    
    # Replace Fiber v3 beta with v2
    sed -i.bak 's|github.com/gofiber/fiber/v3 v3.0.0-beta.5|github.com/gofiber/fiber/v2 v2.52.5|g' "$mod_file"
    
    # Clean up backup
    rm -f "${mod_file}.bak"
    
    # Get the directory of the go.mod file
    mod_dir=$(dirname "$mod_file")
    
    echo "Running go mod tidy in $mod_dir..."
    (cd "$mod_dir" && go mod tidy)
done

echo "✅ Fiber dependencies updated to v2"

# Also update import statements in Go files
echo "🔧 Updating import statements..."

# Find all Go files that import Fiber v3
GO_FILES=$(find . -name "*.go" -type f -exec grep -l "github.com/gofiber/fiber/v3" {} \;)

for go_file in $GO_FILES; do
    echo "Updating imports in $go_file..."
    sed -i.bak 's|github.com/gofiber/fiber/v3|github.com/gofiber/fiber/v2|g' "$go_file"
    rm -f "${go_file}.bak"
done

echo "✅ Import statements updated"

# Update Go version if needed (Go 1.24 doesn't exist)
for mod_file in $GO_MOD_FILES; do
    echo "Fixing Go version in $mod_file..."
    sed -i.bak 's|go 1.24.0|go 1.21|g' "$mod_file"
    sed -i.bak '/^toolchain/d' "$mod_file"
    rm -f "${mod_file}.bak"
done

echo "✅ Go versions fixed"
echo "🎉 All Fiber dependencies have been updated to v2!"