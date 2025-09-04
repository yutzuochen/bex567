# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a business exchange marketplace platform (類似 BizBuySell) with a comprehensive auction system. The platform consists of three main services:

1. **Main Marketplace** (`business_exchange_marketplace/`) - Core business listing platform
2. **Auction Service** (`business_exchange_marketplace_auction/`) - Sealed-bid auction system with real-time WebSocket features
3. **Frontend** (`business_exchange_marketplace_frontend/`) - Next.js web interface

## Architecture Overview

### Main Backend (`business_exchange_marketplace/`)
- **Stack**: Go 1.23.0, Gin, GORM, MySQL 8, Redis, GraphQL (gqlgen)
- **Features**: User auth, business listings, messaging, transactions, favorites, audit logging
- **Authentication**: JWT tokens with Redis session management
- **APIs**: REST (`/api/v1/`) + GraphQL (`/graphql`)
- **Database**: `business_exchange` (18+ migrations with users, listings, images, favorites, messages, transactions, leads, password resets, audit logs, English auction support)
- **Commands**: server, migrate, seed

### Auction Service (`business_exchange_marketplace_auction/`)
- **Stack**: Go 1.23.0, Gin, GORM, MySQL 8, Redis, WebSocket (gorilla/websocket 1.5.3)
- **Auction Types**: Sealed-bid (盲標) and English auction (英式) with soft-close anti-sniping mechanism
- **Real-time**: WebSocket connections with Hub pattern, degradation control, heartbeat system
- **Features**: Price range validation, blacklist management, anonymized bidders (Bidder #N), audit logging, notification system
- **APIs**: REST (`/api/v1/`) + WebSocket (`/ws/`)
- **Database**: `business_exchange` (shared with main platform - auction tables integrated into main database)
- **Commands**: server, migrate, finalize-job
- **Background Jobs**: Auction finalization job for automated closing and auto-activation of scheduled auctions

### Frontend (`business_exchange_marketplace_frontend/`)
- **Stack**: Next.js 14.2.5, TypeScript, React 18, Tailwind CSS
- **Architecture**: App Router, centralized API client, protected routes, standalone output for Docker
- **Authentication**: HttpOnly cookie-based auth with member/guest views, protected routes using async checks
- **Integration**: Connects to both backend services via REST APIs and WebSocket for real-time auction updates
- **Member Features**: Dynamic member welcome section at `/market`, dashboard access, user profile display
- **Optimization**: Hot reload support, polling-based file watching, optimized for Cloud Run deployment

## Development Commands

### Root Level (All Services)
```bash
# Start full development stack with hot reload
make dev              # All services + databases + adminer (includes MySQL DB init script)
make dev-down         # Stop development stack
make up               # Production stack
make down             # Stop all services

# Monitoring
make logs             # All service logs
make logs-backend     # Backend logs only  
make logs-auction     # Auction service logs only
make logs-frontend    # Frontend logs only
make status           # Check service status
make clean            # Remove containers and volumes
make rebuild          # Rebuild all services
```

**Services & Ports:**
- MySQL: `:3306` (with auto-created `business_exchange` database - shared by both services)
- Redis: `:6379` (separate DB numbers: backend=0, auction=1)
- Backend: `:8080`
- Auction: `:8081` 
- Frontend: `:3000`
- Adminer: `:8082` (database administration)

### Main Backend (`business_exchange_marketplace/`)
```bash
cd business_exchange_marketplace

# Local development
make run              # Start server (:8080)
make build            # Build server binary
make clean            # Clean build artifacts
make tidy             # Update Go module dependencies

# Database operations
make migrate          # Run migrations (uses cmd/migrate)
make migrate-down     # Rollback migrations
make migrate-status   # Check migration status

# Code generation
make gqlgen           # Generate GraphQL code
make wire             # Generate dependency injection

# Docker operations
make docker-dev       # Start backend dev environment
make docker-up        # Start backend production
make docker-debug     # Start debug environment
make docker-dev-down  # Stop dev environment
make docker-debug-down # Stop debug environment
```

### Auction Service (`business_exchange_marketplace_auction/`)
```bash
cd business_exchange_marketplace_auction

# Local development
make run              # Start auction server (:8081)
make build            # Build all binaries (server, migrate, finalize-job)
make clean            # Clean build artifacts
make tidy             # Update Go module dependencies

# Database operations  
make migrate          # Run auction migrations (uses cmd/migrate)
make migrate-down     # Rollback migrations
make migrate-status   # Check migration status

# Background jobs
make finalize-job     # Run auction finalization job (closes expired auctions + auto-activates draft auctions)

# Testing
make test             # Run tests
make test-coverage    # Run tests with coverage

# Docker operations
make docker-up        # Start auction service
make docker-down      # Stop auction service
```

### Frontend (`business_exchange_marketplace_frontend/`)
```bash
cd business_exchange_marketplace_frontend

# Development
npm run dev           # Development server (:3000)
npm run build         # Production build
npm start            # Start production build
npm run lint         # Run ESLint (validates TypeScript and React code)

# Dependencies
npm install           # Install dependencies
```

## Key Architecture Patterns

### Multi-Service Communication
- **Service Isolation**: Each service has its own database and migrations
- **Shared Authentication**: JWT tokens validated across services
- **API Gateway Pattern**: Frontend communicates with multiple backends
- **Event-Driven**: Auction events trigger notifications and state changes

### Auction System Design
- **Dual Auction Types**: 
  - **Sealed-Bid Model**: Bidders cannot see others' bids until auction ends (1-61 day duration)
  - **English Auction Model**: Real-time visible bidding with transparent competition
- **Soft-Close Mechanism**: Auto-extends auction by 1 minute if bid placed in final 3 minutes (anti-sniping)
- **Price Range System**: Sellers set min/max price bounds, bidders must bid within range
- **Reserve Price Support**: English auctions support reserve prices that must be met
- **Anonymized Display**: Bidders shown as "Bidder #N" with consistent aliases per auction
- **Top 7 Results**: Only top 7 bidders + seller see final rankings after auction ends
- **WebSocket Architecture**: Real-time bidding updates with hub-based connection management
- **Reconnection Handling**: Event replay system for missed messages during disconnections
- **Degradation System**: 5-level system (0-4) with adaptive rate limiting and priority queuing
- **Blacklist Management**: Admin-controlled user exclusion from auction participation
- **Notification System**: Automated email notifications for auction events and results

### Authentication Flow
- **JWT Strategy**: HttpOnly cookies for security, tokens issued by main backend and validated across services
- **Cookie Implementation**: `SameSite=Lax` for cross-origin localhost development (frontend :3000 → backend :8080)
- **Session Management**: Redis-backed sessions with automatic token refresh
- **Protected Routes**: Frontend route guards using async authentication checks (`isAuthenticatedAsync()`)
- **Cross-Service Auth**: Shared JWT secret for service-to-service validation
- **Frontend Integration**: API client uses `credentials: 'include'` for automatic cookie handling
- **Token Validation**: `/api/v1/auth/me` endpoint for checking authentication status

### Database Strategy  
- **Consolidated Database**: Both services use the `business_exchange` database
- **Migration Management**: All migrations managed through main backend (`business_exchange_marketplace/migrations/`)
- **Auction Integration**: Auction tables integrated via migrations 000016+ (auctions, bids, blacklist, aliases, notifications, English auction support in 000018+)
- **Audit Logging**: Comprehensive tracking of user actions and system events
- **Event Sourcing**: Auction events stored for replay and analytics

### WebSocket Implementation
- **Hub Pattern**: Connection management by auction rooms with connection pooling
- **Message Types**: hello, state, bid_accepted, extended, closed, resume_ok, error
- **Rate Limiting**: Per-user throttling with degradation-aware limits (60-1 messages/min based on load)
- **Heartbeat System**: 54s ping interval, 60s timeout for connection health
- **Degradation Control**: 5-level system (0-4) with adaptive rate limiting based on system load
- **Connection Throttling**: Minimum message intervals from 100ms to 30s based on degradation level
- **Multi-Instance**: Redis pub/sub for distributed WebSocket handling across instances
- **Priority Queuing**: Separate high-priority queue for critical messages (extended, closed, error)
- **Cleanup**: Automatic cleanup of inactive rate limiters after 5 minutes

## Environment Setup

### Required Environment Files
- `business_exchange_marketplace/.env` (copy from `env.example`)
- `business_exchange_marketplace_auction/.env` (copy from `env.example`)  
- `business_exchange_marketplace_frontend/env.production`

### Database Setup
Both services require MySQL 8.0. The Docker setup automatically creates the shared database:
```bash
# Start databases for both services (auto-creates business_exchange DB - shared by both services)
make dev
```

Database initialization handled by `/scripts/init-databases.sql` which:
- Creates `business_exchange` database (shared by both main platform and auction system)
- Creates `auction_service` database (for potential separate auction service deployment)
- Grants permissions to `app` user for both databases

### Migration Management
- **Critical**: All migrations are managed through the main backend (`business_exchange_marketplace/migrations/`)
- Current migration count: 000001-000018+ 
- **Migration Version Sync**: If database shows version mismatch, use the force command with current version:
  ```bash
  cd business_exchange_marketplace
  go run ./cmd/migrate -action=force -version=18
  ```

### Service Dependencies
- **Redis**: Required for sessions, caching, and WebSocket pub/sub (separate DB numbers per service)
- **MySQL**: Shared `business_exchange` database for both main platform and auction service
- **JWT Secret**: **CRITICAL** - Must be identical across services for cross-service auth validation (check `JWT_SECRET` in both .env files)
- **Node.js**: Frontend requires Node.js with hot-reload optimizations (CHOKIDAR_USEPOLLING)

## Common Development Workflows

### Full Stack Development
1. Start all services: `make dev`
2. Main platform: http://localhost:8080
3. Auction service: http://localhost:8081  
4. Frontend: http://localhost:3000
5. Monitor: `make logs` or `make status`

### Testing Auction Features
1. Create auction via REST API: `POST /api/v1/auctions` (seller authentication required)
2. Activate auction: `POST /api/v1/auctions/:id:activate` (seller only)
3. Connect WebSocket: `ws://localhost:8081/ws/auctions/:id` (authenticated users)
4. Place bids: `POST /api/v1/auctions/:id/bids` (buyers only, rate-limited)
5. Monitor real-time updates via WebSocket (bid_accepted, extended, closed events)
6. Check results: `GET /api/v1/auctions/:id/results` (top 7 bidders shown)
7. Run finalization: `make finalize-job` (closes expired auctions and auto-activates scheduled ones)

### Database Operations
- **All Migrations**: Centralized in `business_exchange_marketplace/migrations/` (000001-000018+)
  - Core platform tables: users, listings, images, favorites, messages, transactions
  - Extended: user sessions, leads, password resets, audit logs
  - Auction tables: migrations 000016+ (auctions, bids, blacklist, aliases, notifications)
  - English auction support: migration 000018+ (reserve prices, current_price tracking, visible bidding)
- **Migration Commands**: Use main backend Makefile (`make migrate`, `make migrate-status` in `business_exchange_marketplace/`)
- **Seed Data**: Available via `cmd/seed/main.go` in main backend
- **Auction Service**: No separate migrations - uses shared database

## Build and Deployment

### Docker Strategy
- **Multi-stage builds**: Separate development and production images with Go builder pattern
- **Service orchestration**: Docker Compose for local development (docker-compose.dev.yml, docker-compose.yml)
- **Hot reload**: Development containers with volume mounts and Air for Go services
- **Cloud deployment**: Google Cloud Run with standalone Next.js output
- **Environment configs**: Multiple Dockerfile variants (dev, production, debug) per service

### Frontend Build Optimization
- **Standalone Output**: Next.js configured for container deployment
- **SWC Minification**: Fast compilation and optimization
- **Volume Caching**: Persistent node_modules and .next/cache volumes
- **Production Optimizations**: Console removal, compression, output file tracing

### Production Considerations
- **Auto-scaling**: Google Cloud Run handles traffic spikes with container-optimized deployments
- **Database**: Cloud SQL MySQL with connection pooling and health checks
- **Caching**: Redis for sessions, auction state, and WebSocket pub/sub coordination
- **Monitoring**: Structured logging with Zap, health checks (`/healthz`, `/health`)
- **WebSocket scaling**: Redis pub/sub enables multi-instance WebSocket deployment with degradation control

### Cloud Deployment Scripts
- **Frontend Deployment**: `./deploy-frontend.sh` (automated Google Cloud Run deployment)
- **Auction Service Deployment**: `./deploy.sh` or `./quick-deploy.sh` (with secrets management)
- **Secrets Management**: `./setup-secrets.sh` (automated Cloud Secret Manager setup)
- **Project ID**: `businessexchange-468413` (Google Cloud project)

## API Reference

### Authentication
Authentication uses HttpOnly cookies automatically sent with requests. For manual testing:
```bash
# Login and save cookie
curl -c cookies.txt -X POST http://localhost:8080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"user@example.com","password":"password"}'

# Use saved cookie for authenticated requests  
curl -b cookies.txt http://localhost:8080/api/v1/auth/me
```

### Main Backend APIs (`business_exchange_marketplace/`)
- **REST**: `/api/v1/` - User management, listings, messages, transactions, favorites
- **GraphQL**: `/graphql` - Flexible queries for frontend data fetching
- **Health**: `/healthz` - Simple health check

### Auction Service APIs (`business_exchange_marketplace_auction/`)

#### Auction Management
- `POST /api/v1/auctions` - Create auction (seller authentication required)
- `POST /api/v1/auctions/:id:activate` - Activate auction (seller only)
- `POST /api/v1/auctions/:id:cancel` - Cancel auction (seller/admin only)
- `GET /api/v1/auctions` - List all auctions (public)
- `GET /api/v1/auctions/:id` - Get auction details (public)

#### Bidding
- `POST /api/v1/auctions/:id/bids` - Submit bid (buyers only, rate limited to 5s intervals)
- `GET /api/v1/auctions/:id/my-bids` - View personal bid history (authenticated user)
- `GET /api/v1/auctions/:id/results` - Auction results (top 7 bidders + seller after close)
- `GET /api/v1/auctions/:id/stats/histogram` - Bid distribution statistics

#### Admin Functions
- `GET /api/v1/admin/blacklist` - List blacklisted users (admin only)
- `POST /api/v1/admin/blacklist` - Add user to blacklist (admin only)
- `DELETE /api/v1/admin/blacklist/:user_id` - Remove from blacklist (admin only)

#### WebSocket Real-time
- `WS /ws/auctions/:auction_id` - Join auction room for real-time updates (requires Authorization header)
- `WS /ws/test/:auction_id?token=<JWT_TOKEN>` - Test endpoint with JWT token in query parameter (for debugging)
- `GET /ws/stats` - WebSocket connection statistics and health

#### Health & Monitoring
- `GET /healthz` - Simple health check
- `GET /health` - Detailed system health status

### WebSocket Message Types
- `hello` - Welcome message with connection confirmation
- `state` - Auction state changes (activated, cancelled)
- `bid_accepted` - Real-time bid confirmation and broadcast
- `extended` - Soft-close extension notification (1 minute added)
- `closed` - Auction ended notification
- `resume_ok` - Reconnection acknowledgment with missed events
- `error` - Error messages and validation failures
- `price_changed` - English auction price updates (visible bidding)
- `reserve_met` - Reserve price reached notification
- `outbid` - Notification when user is outbid (English auctions)

## Testing and Quality Assurance

### Running Tests
- **Auction Service**: `make test` or `make test-coverage` (in auction directory)
- **Frontend**: `npm run lint` (ESLint validation)
- **Backend**: No specific test commands defined (manual testing via API endpoints)

### Code Generation and Dependencies
- **Backend GraphQL**: `make gqlgen` - Regenerate GraphQL resolvers and schema
- **Backend DI**: `make wire` - Generate dependency injection code  
- **Dependencies**: `make tidy` in each Go service to update modules

### Database Management
- **Migrations**: Service-specific (`make migrate`, `make migrate-down`, `make migrate-status`)
- **Seed Data**: Available in main backend via `cmd/seed/main.go`
- **Admin Interface**: Adminer available at http://localhost:8082 during development

## Troubleshooting

### Common Issues
- **Database Connection**: Ensure services start after databases are healthy (healthchecks in docker-compose)
- **Cross-Service Auth**: JWT secrets MUST be identical between main backend and auction service (common cause of WebSocket connection failures)
- **WebSocket Issues**: Check Redis connectivity for pub/sub coordination and verify JWT token validation
- **Token Signature Invalid**: Verify `JWT_SECRET` matches exactly in both service .env files
- **Frontend Hot Reload**: Uses `CHOKIDAR_USEPOLLING=true` for file watching in containers

### Authentication Issues
- **401 Unauthorized on /auth/me**: Usually expired JWT token (1-hour expiration) - login again for fresh token
- **Cookies Not Set**: Ensure CORS middleware allows credentials (`Access-Control-Allow-Credentials: true`)
- **Cross-Origin Cookie Problems**: Backend uses `SameSite=Lax` for localhost development
- **Frontend API Calls**: Must use `credentials: 'include'` in fetch requests for cookies to work
- **Protected Routes**: Use `apiClient.isAuthenticatedAsync()` instead of localStorage checks
- **Member Page Access**: After login, users should see member welcome section at `/market`
- **Cookie Expiration**: Browser automatically handles expired cookies, backend returns 401 for invalid tokens

### Migration Issues
- **Migration Version Mismatch**: If `make migrate` fails with "no migration found for version X", the database schema_migrations table version doesn't match available migration files
  - Check current version: `make migrate-status` 
  - Reset to correct version: `go run ./cmd/migrate -action=force -version=17`
  - **Dirty State**: If migration status shows "Dirty: true", use force command to clean the state

### Service Dependencies
- **Startup Order**: MySQL → Redis → Backend Services → Frontend
- **Port Conflicts**: Ensure ports 3000, 3306, 6379, 8080, 8081, 8082 are available
- **Environment Files**: Copy from respective `env.example` files in each service

## Key Development Tasks

### Adding Sample Images to Market Page
If the market page shows "無圖片" (no image) placeholders:
```bash
# Check if images exist in database
docker exec bex567-mysql-1 mysql -u app -papp_password business_exchange -e "SELECT COUNT(*) FROM images;"

# If count is 0, add sample images for existing listings
docker exec bex567-mysql-1 mysql -u app -papp_password business_exchange -e "INSERT INTO images (listing_id, filename, url, alt_text, \`order\`, is_primary, created_at, updated_at) SELECT id, CONCAT(LOWER(REPLACE(title, ' ', '_')), '.jpg'), CONCAT('http://localhost:8080/static/images/listings/', CASE WHEN id % 27 = 1 THEN 'happy_coffee.jpg' WHEN id % 27 = 2 THEN 'pet_grooming.jpg' ELSE 'bakery.jpg' END), title, 0, 1, NOW(), NOW() FROM listings WHERE id NOT IN (SELECT DISTINCT listing_id FROM images);"
```

### Database Seeding Issues
If seed data fails due to foreign key constraints:
- The auction system creates foreign key dependencies that prevent normal seeding
- Use manual SQL inserts for images as shown above
- Or temporarily disable foreign key checks during seeding

# Important Instructions
Do what has been asked; nothing more, nothing less.
NEVER create files unless they're absolutely necessary for achieving your goal.
ALWAYS prefer editing an existing file to creating a new one.
NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested by the User.
