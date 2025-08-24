.PHONY: help up down dev dev-down logs logs-backend logs-frontend clean rebuild

# Default target
help:
	@echo "🚀 Business Exchange Marketplace - Docker Commands"
	@echo ""
	@echo "Commands:"
	@echo "  up          - Start production stack (backend + frontend + db)"
	@echo "  down        - Stop all services"
	@echo "  dev         - Start development stack with hot reload"
	@echo "  dev-down    - Stop development stack"
	@echo "  logs        - View all service logs"
	@echo "  logs-backend- View backend logs only"
	@echo "  logs-frontend- View frontend logs only"
	@echo "  clean       - Remove all containers and volumes"
	@echo "  rebuild     - Rebuild all services"
	@echo ""

# Production stack
up:
	@echo "🚀 Starting production stack..."
	docker compose up -d

down:
	@echo "🛑 Stopping all services..."
	docker compose down

# Development stack with hot reload
dev:
	@echo "🔥 Starting development stack with hot reload..."
	docker compose -f docker-compose.dev.yml up --build -d

dev-down:
	@echo "🛑 Stopping development stack..."
	docker compose -f docker-compose.dev.yml down

# Logs
logs:
	@echo "📋 Viewing all service logs..."
	docker compose -f docker-compose.dev.yml logs -f

logs-backend:
	@echo "📋 Viewing backend logs..."
	docker compose -f docker-compose.dev.yml logs -f backend

logs-frontend:
	@echo "📋 Viewing frontend logs..."
	docker compose -f docker-compose.dev.yml logs -f frontend

# Maintenance
clean:
	@echo "🧹 Cleaning up all containers and volumes..."
	docker compose -f docker-compose.dev.yml down -v
	docker system prune -f

rebuild:
	@echo "🔨 Rebuilding all services..."
	docker compose -f docker-compose.dev.yml down
	docker compose -f docker-compose.dev.yml up --build -d

# Quick status check
status:
	@echo "📊 Service Status:"
	docker compose -f docker-compose.dev.yml ps
