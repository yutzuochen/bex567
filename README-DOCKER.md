# 🐳 Docker Compose Setup - Business Exchange Marketplace

This directory contains Docker Compose files to run the entire Business Exchange Marketplace stack from a single location.

## 📁 **Directory Structure**

```
~/Documents/bex567/
├── docker-compose.yml          # Production stack
├── docker-compose.dev.yml      # Development stack with hot reload
├── Makefile                    # Easy commands for managing services
├── README-DOCKER.md            # This documentation
├── business_exchange_marketplace/     # Backend (Go)
└── business_exchange_marketplace_frontend/  # Frontend (Next.js)
```

## 🚀 **Quick Start**

### **1. Start Development Stack (Recommended)**
```bash
# From ~/Documents/bex567 directory
make dev
```

This will start:
- **MySQL** database on port 3306
- **Redis** cache on port 6379
- **Backend** (Go) on port 8080 with hot reload
- **Frontend** (Next.js) on port 3000 with hot reload
- **Adminer** database admin on port 8081

### **2. Start Production Stack**
```bash
make up
```

### **3. Stop All Services**
```bash
make dev-down    # Stop development stack
make down        # Stop production stack
```

## 📱 **Service URLs**

| Service | URL | Port | Description |
|---------|-----|------|-------------|
| **Frontend** | http://localhost:3000 | 3000 | Next.js application |
| **Backend** | http://localhost:8080 | 8080 | Go API server |
| **Database** | localhost:3306 | 3306 | MySQL database |
| **Redis** | localhost:6379 | 6379 | Redis cache |
| **Adminer** | http://localhost:8081 | 8081 | Database admin tool |

## 🔧 **Available Commands**

```bash
# Help
make help              # Show all available commands

# Development
make dev               # Start development stack
make dev-down          # Stop development stack

# Production
make up                # Start production stack
make down              # Stop production stack

# Logs
make logs              # View all service logs
make logs-backend      # View backend logs only
make logs-frontend     # View frontend logs only

# Maintenance
make status            # Check service status
make clean             # Remove all containers and volumes
make rebuild           # Rebuild all services
```

## 🎯 **Next Steps**

1. **Start development**: `make dev`
2. **Open frontend**: http://localhost:3000
3. **Test API**: http://localhost:8080/healthz
4. **Access database**: http://localhost:8081

