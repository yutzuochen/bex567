-- Initialize multiple databases for the business exchange marketplace
-- This script runs when MySQL container starts for the first time

-- Create the auction service database
CREATE DATABASE IF NOT EXISTS auction_service;

-- Grant permissions to the app user for both databases
GRANT ALL PRIVILEGES ON business_exchange.* TO 'app'@'%';
GRANT ALL PRIVILEGES ON auction_service.* TO 'app'@'%';

-- Flush privileges to ensure the changes take effect
FLUSH PRIVILEGES;