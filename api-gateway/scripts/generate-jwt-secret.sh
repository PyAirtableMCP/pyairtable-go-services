#!/bin/bash

# Generate a secure JWT secret for PyAirtable API Gateway
# This script generates a cryptographically secure 256-bit secret

echo "Generating secure JWT secret..."
echo

# Generate 32 random bytes and encode as base64
JWT_SECRET=$(openssl rand -base64 32)

echo "Generated JWT Secret:"
echo "====================="
echo "JWT_SECRET=$JWT_SECRET"
echo
echo "Copy this to your .env file:"
echo "JWT_SECRET=$JWT_SECRET"
echo
echo "⚠️  Security Notes:"
echo "- Keep this secret secure and never commit it to version control"
echo "- Use different secrets for different environments"
echo "- Rotate secrets regularly in production"
echo "- This secret is used to sign and verify JWT tokens"
echo

# Optionally save to .env file if it doesn't exist
if [ ! -f .env ]; then
    echo "Creating .env file with generated secret..."
    cp .env.example .env 2>/dev/null || true
    if [ -f .env ]; then
        # Replace the placeholder with the actual secret
        sed -i.bak "s/your_secure_256_bit_jwt_secret_here_replace_this_in_production/$JWT_SECRET/" .env
        rm .env.bak 2>/dev/null || true
        echo "✅ .env file created with generated JWT secret"
    else
        echo "⚠️  Could not create .env file. Please create it manually."
    fi
else
    echo "⚠️  .env file already exists. Please update JWT_SECRET manually:"
    echo "JWT_SECRET=$JWT_SECRET"
fi