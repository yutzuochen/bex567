#!/bin/bash

# Script to run database migrations on Cloud SQL
set -e

echo "🔧 Running database migrations on Cloud SQL..."

# Set environment variables for Cloud SQL connection
export APP_ENV=production
export DB_HOST=/cloudsql/businessexchange-468413:us-central1:trade-sql
export DB_PORT=3306
export DB_USER=app
export DB_PASSWORD=app_password
export DB_NAME=business_exchange


mysql -h /cloudsql/businessexchange-468413:us-central1:trade-sql \
      -u $DB_USER -p$DB_PASSWORD \
      -e "CREATE DATABASE IF NOT EXISTS business_exchange CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;"
      
# Run migrations
echo "📋 Running migrations..."
go run ./cmd/migrate -action=up

echo "✅ Migrations completed successfully!"

# Check database status
echo "📊 Checking migration status..."
go run ./cmd/migrate -action=status
