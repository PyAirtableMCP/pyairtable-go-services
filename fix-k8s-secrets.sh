#!/bin/bash

# Script to remove hardcoded credentials from Kubernetes ConfigMap files
# and replace them with security-conscious placeholders

# Find all configmap.yaml files
CONFIGMAP_FILES=$(find . -name "configmap.yaml" -path "*/k8s/*")

echo "🔒 Fixing hardcoded credentials in Kubernetes ConfigMap files..."

for file in $CONFIGMAP_FILES; do
    echo "Processing: $file"
    
    # Create backup
    cp "$file" "$file.backup"
    
    # Replace hardcoded base64 credentials with placeholders
    sed -i.tmp \
        -e 's/DB_USER: cG9zdGdyZXM=/DB_USER: "PLACEHOLDER_DB_USER_BASE64"/g' \
        -e 's/DB_PASSWORD: cG9zdGdyZXM=/DB_PASSWORD: "PLACEHOLDER_DB_PASSWORD_BASE64"/g' \
        -e 's/JWT_SECRET: eW91ci1zZWNyZXQta2V5/JWT_SECRET: "PLACEHOLDER_JWT_SECRET_BASE64"/g' \
        -e 's/REDIS_PASSWORD: ""/REDIS_PASSWORD: "PLACEHOLDER_REDIS_PASSWORD_BASE64"/g' \
        -e '/# Base64 encoded values - replace with actual encoded secrets/c\
  # SECURITY WARNING: These values must be replaced with actual secrets in production\
  # Use GitHub secrets or external secret management system\
  # These are placeholders and MUST be overridden' \
        "$file"
    
    # Remove temporary file
    rm -f "$file.tmp"
    
    echo "✅ Fixed: $file"
done

echo "🎯 All Kubernetes ConfigMap files have been secured!"
echo "⚠️  Remember to update your deployment scripts to inject actual secrets!"