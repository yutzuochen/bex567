# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a business exchange marketplace platform (類似 BizBuySell) built with a Go backend and Next.js frontend. The system allows users to list, search, and trade business opportunities.

## Architecture

**Backend** (`business_exchange_marketplace/`):
- Go 1.23 with Gin web framework
- GORM for database operations with MySQL 8
- Redis for caching and sessions
- GraphQL (gqlgen) and REST APIs
- JWT authentication
- Structured logging with Zap

**Frontend** (`business_exchange_marketplace_frontend/`):
- Next.js 14.2.5 with TypeScript
- React 18 with Tailwind CSS
- API integration with backend services

## Development Commands

### Backend Development
```bash
# Run backend locally
cd business_exchange_marketplace
make run

# Build backend
make build

# Database migrations
make migrate          # Run migrations
make migrate-down     # Rollback migrations
make migrate-status   # Check migration status

# Code generation
make gqlgen          # Generate GraphQL code
make wire            # Generate dependency injection

# Dependencies
make tidy            # Update Go dependencies
```

### Frontend Development
```bash
# Run frontend locally
cd business_exchange_marketplace_frontend
npm run dev          # Development server on :3000

# Build frontend
npm run build        # Production build
npm start            # Start production build
npm run lint         # Run ESLint
```

### Docker Development
```bash
# Root level commands (both services)
make dev             # Start development stack with hot reload
make dev-down        # Stop development stack
make up              # Start production stack
make down            # Stop all services
make logs            # View all service logs
make logs-backend    # Backend logs only
make logs-frontend   # Frontend logs only
make clean           # Remove containers and volumes
make rebuild         # Rebuild all services

# Backend specific Docker
cd business_exchange_marketplace
make docker-dev      # Start backend dev environment
make docker-up       # Start backend production
make docker-debug    # Start debug environment
```

## Key Directories

**Backend Structure:**
- `cmd/` - Application entry points (server, migrate, seed)
- `internal/` - Internal packages (handlers, models, auth, middleware)
- `graph/` - GraphQL schema and resolvers
- `migrations/` - Database migration files
- `templates/` - HTML templates
- `static/` - Static files and images

**Frontend Structure:**
- `src/app/` - Next.js app router pages
- `src/components/` - Reusable React components
- `src/lib/` - Utility functions and API client
- `src/types/` - TypeScript type definitions

## Database

The system uses MySQL 8 with the following core models:
- Users (authentication and profiles)
- Listings (business opportunities)
- Images (listing photos)
- Favorites (user bookmarks)
- Messages (communication system)
- Transactions (deal tracking)

## API Endpoints

- REST API: `/api/v1/`
- GraphQL: `/graphql` with playground at `/playground`
- Health check: `/healthz`
- Static files: `/static/`

## Environment Setup

Both services require environment configuration:
- Backend: Copy `env.example` to `.env` in backend directory
- Frontend: Configure production variables in `env.production`

## Common Development Workflow

1. Use `make dev` from root to start both services with hot reload
2. Backend runs on `:8080`, frontend on `:3000`
3. Check service status with `make status`
4. View logs with `make logs-backend` or `make logs-frontend`

## Build and Deployment

The project includes Docker configurations for local development, debugging, and production deployment to GCP Cloud Run.