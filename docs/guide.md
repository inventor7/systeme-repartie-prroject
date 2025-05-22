# Deployment & Configuration Guide

## 🚀 Production Deployment

### Docker Deployment

#### Super-Peer Dockerfile
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o super-peer main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/super-peer .
COPY --from=builder /app/web ./web
COPY --from=builder /app/config ./config

EXPOSE 8080
CMD ["./super-peer"]
```

#### Peer Dockerfile
```dockerfile
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o peer-server peer_server.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates tzdata
WORKDIR /root/

COPY --from=builder /app/peer-server .
COPY --from=builder /app/web ./web
COPY --from=builder /app/config ./config

EXPOSE 9001
VOLUME ["/root/shared_files"]
CMD ["./peer-server"]
```

#### Docker Compose
```yaml
version: '3.8'

services:
  super-peer:
    build:
      context: .
      dockerfile: Dockerfile.super-peer
    ports:
      - "8080:8080"
    environment:
      - LOG_LEVEL=info
      - MAX_PEERS=1000
    volumes:
      - ./logs:/root/logs
      - ./config:/root/config
    restart: unless-stopped

  peer1:
    build:
      context: .
      dockerfile: Dockerfile.peer
    ports:
      - "9001:9001"
    environment:
      - SUPER_PEER_ADDRESS=super-peer:8080
      - PEER_PORT=9001
    volumes:
      - ./shared_files_1:/root/shared_files
      - ./logs:/root/logs
    depends_on:
      - super-peer
    restart: unless-stopped

  peer2:
    build:
      context: .
      dockerfile: Dockerfile.peer
    ports:
      - "9002:9001"
    environment:
      - SUPER_PEER_ADDRESS=super-peer:8080
      - PEER_PORT=9001
    volumes:
      - ./shared_files_2:/root/shared_files
      - ./logs:/root/logs
    depends_on:
      - super-peer
    restart: unless-stopped

networks:
  default:
    name: p2p-network
```

### Kubernetes Deployment

#### Super-Peer Deployment
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: super-peer
  labels:
    app: super-peer
spec:
  replicas: 2
  selector:
    matchLabels:
      app: super-peer
  template:
    metadata:
      labels:
        app: super-peer
    spec:
      containers:
      - name: super-peer
        image: p2p-professional/super-peer:latest
        ports:
        - containerPort: 8080
        env:
        - name: LOG_LEVEL
          value: "info"
        - name: MAX_PEERS
          value: "1000"
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
        livenessProbe:
          httpGet:
            path: /api/v1/stats
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/v1/stats
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: super-peer-service
spec:
  selector:
    app: super-peer
  ports:
  - protocol: TCP
    port: 8080
    targetPort: 8080
  type: LoadBalancer
```

#### Peer Deployment
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: peer-nodes
spec:
  serviceName: peer-nodes
  replicas: 5
  selector:
    matchLabels:
      app: peer-node
  template:
    metadata:
      labels:
        app: peer-node
    spec:
      containers:
      - name: peer
        image: p2p-professional/peer:latest
        ports:
        - containerPort: 9001
        env:
        - name: SUPER_PEER_ADDRESS
          value: "super-peer-service:8080"
        volumeMounts:
        - name: shared-storage
          mountPath: /root/shared_files
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "256Mi"
            cpu: "200m"
  volumeClaimTemplates:
  - metadata:
      name: shared-storage
    spec:
      accessModes: ["ReadWriteOnce"]
      resources:
        requests:
          storage: 10Gi
```

## ⚙️ Configuration Files

### config.yaml
```yaml
# Super-Peer Configuration
super_peer:
  # Server settings
  host: "0.0.0.0"
  port: 8080
  max_peers: 1000
  
  # Timing configurations
  heartbeat_interval: 30s
  peer_timeout: 2m
  cleanup_interval: 5m
  stats_update_interval: 10s
  
  # File management
  max_file_size: 100MB
  allowed_extensions: [".txt", ".pdf", ".jpg", ".png", ".mp4", ".mp3", ".zip"]
  
  # Database (for persistent storage)
  database:
    type: "sqlite" # or "postgres", "mysql"
    connection_string: "p2p.db"
    max_connections: 25
    max_idle_connections: 10

# Peer Configuration
peer:
  # Server settings
  host: "0.0.0.0"
  port: 9001
  peer_id: "" # Auto-generated if empty
  
  # Super-peer connection
  super_peer_address: "localhost:8080"
  registration_retry_interval: 10s
  heartbeat_interval: 30s
  
  # File sharing
  shared_directory: "./shared_files"
  max_file_size: 100MB
  auto_scan_interval: 30s
  
  # Upload/Download limits
  max_concurrent_uploads: 10
  max_concurrent_downloads: 5
  bandwidth_limit: "10MB/s"
  
  # Security
  enable_file_verification: true
  quarantine_suspicious_files: true

# Security Configuration
security:
  # Encryption
  enable_tls: false
  cert_file: "server.crt"
  key_file: "server.key"
  
  # Authentication
  enable_auth: false
  jwt_secret: "your-secret-key"
  token_expiry: "24h"
  
  # Rate limiting
  enable_rate_limiting: true
  requests_per_minute: 100
  burst_size: 200
  
  # File security
  scan_files: true
  max_file_size: 100MB
  blocked_extensions: [".exe", ".bat", ".sh", ".cmd"]

# Logging Configuration
logging:
  level: "info" # debug, info, warn, error
  format: "json" # json, text
  output: "stdout" # stdout, file
  file_path: "./logs/p2p.log"
  max_size: 100MB
  max_backups: 5
  max_age: 30
  compress: true

# Monitoring Configuration
monitoring:
  enable_metrics: true
  metrics_port: 9090
  enable_pprof: false
  pprof_port: 6060
  
  # Health check endpoints
  health_check_path: "/health"
  readiness_check_path: "/ready"

# Network Configuration
network:
  # Connection settings
  read_timeout: 30s
  write_timeout: 30s
  idle_timeout: 60s
  max_header_bytes: 1MB
  
  # WebSocket settings
  ping_period: 54s
  pong_wait: 60s
  write_wait: 10s
  max_message_size: 512KB
  
  # CORS settings
  cors_enabled: true
  allowed_origins: ["*"]
  allowed_methods: ["GET", "POST", "PUT", "DELETE", "OPTIONS"]
  allowed_headers: ["*"]

# Development Configuration
development:
  debug: false
  hot_reload: false
  profiling: false
  mock_data: false
```

### Environment Variables
```bash
# Super-Peer Environment Variables
export SUPER_PEER_PORT=8080
export SUPER_PEER_HOST=0.0.0.0
export MAX_PEERS=1000
export LOG_LEVEL=info
export DATABASE_URL=sqlite://p2p.db

# Peer Environment Variables
export PEER_PORT=9001
export PEER_HOST=0.0.0.0
export SUPER_PEER_ADDRESS=localhost:8080
export SHARED_DIRECTORY=./shared_files
export MAX_FILE_SIZE=100MB

# Security Environment Variables
export ENABLE_TLS=false
export JWT_SECRET=your-secret-key
export ENABLE_AUTH=false

# Performance Environment Variables
export MAX_CONCURRENT_UPLOADS=10
export MAX_CONCURRENT_DOWNLOADS=5
export BANDWIDTH_LIMIT=10MB/s
```

## 🔧 System Requirements

### Minimum Requirements
- **CPU**: 1 core, 1 GHz
- **RAM**: 512 MB
- **Storage**: 1 GB free space
- **Network**: 1 Mbps bandwidth

### Recommended Requirements
- **CPU**: 2+ cores, 2+ GHz
- **RAM**: 2+ GB
- **Storage**: 10+ GB free space
- **Network**: 10+ Mbps bandwidth

### Production Requirements
- **CPU**: 4+ cores, 3+ GHz
- **RAM**: 8+ GB
- **Storage**: 100+ GB SSD
- **Network**: 100+ Mbps bandwidth

## 🚦 Load Balancing

### Nginx Configuration
```nginx
upstream super_peers {
    server super-peer-1:8080 weight=3;
    server super-peer-2:8080 weight=2;
    server super-peer-3:8080 weight=1;
}

server {
    listen 80;
    server_name p2p.example.com;
    
    location / {
        proxy_pass http://super_peers;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
    
    location /ws {
        proxy_pass http://super_peers;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
    }
}
```

### HAProxy Configuration
```
global
    daemon
    maxconn 4096

defaults
    mode http
    timeout connect 5000ms
    timeout client 50000ms
    timeout server 50000ms

frontend p2p_frontend
    bind *:80
    default_backend super_peers

backend super_peers
    balance roundrobin
    server super-peer-1 192.168.1.10:8080 check
    server super-peer-2 192.168.1.11:8080 check
    server super-peer-3 192.168.1.12:8080 check
```

## 📊 Monitoring & Observability

### Prometheus Configuration
```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'super-peer'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 5s
    metrics_path: /metrics

  - job_name: 'peers'
    static_configs:
      - targets: ['localhost:9001', 'localhost:9002', 'localhost:9003']
    scrape_interval: 10s
```

### Grafana Dashboard
```json
{
  "dashboard": {
    "title": "P2P Network Dashboard",
    "panels": [
      {
        "title": "Active Peers",
        "type": "stat",
        "targets": [
          {
            "expr": "p2p_active_peers_total"
          }
        ]
      },
      {
        "title": "File Transfers",
        "type": "graph",
        "targets": [
          {
            "expr": "rate(p2p_file_transfers_total[5m])"
          }
        ]
      },
      {
        "title": "Network Health",
        "type": "gauge",
        "targets": [
          {
            "expr": "p2p_network_health_percentage"
          }
        ]
      }
    ]
  }
}
```

## 🔐 Security Hardening

### SSL/TLS Configuration
```bash
# Generate self-signed certificate for development
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt -days 365 -nodes

# For production, use Let's Encrypt
certbot certonly --standalone -d p2p.example.com
```

### Firewall Rules
```bash
# Allow only necessary ports
sudo ufw allow 8080/tcp  # Super-peer
sudo ufw allow 9001/tcp  # Peer
sudo ufw deny 22/tcp     # Disable SSH if not needed
sudo ufw enable
```

### Security Headers
```nginx
server {
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "no-referrer-when-downgrade" always;
    add_header Content-Security-Policy "default-src 'self'" always;
}
```

## 🧪 Testing in Production

### Health Checks
```bash
# Super-peer health check
curl -f http://localhost:8080/api/v1/stats || exit 1

# Peer health check
curl -f http://localhost:9001/api/v1/info || exit 1
```

### Load Testing
```bash
# Install wrk
sudo apt-get install wrk

# Test super-peer
wrk -t12 -c400 -d30s http://localhost:8080/api/v1/stats

# Test file download
wrk -t8 -c200 -d30s http://localhost:9001/api/v1/files
```

### Backup & Recovery
```bash
# Backup configuration
tar -czf p2p-backup-$(date +%Y%m%d).tar.gz config/ shared_files/ logs/

# Automated backup script
#!/bin/bash
BACKUP_DIR="/backups/p2p"
DATE=$(date +%Y%m%d_%H%M%S)
mkdir -p $BACKUP_DIR
tar -czf $BACKUP_DIR/p2p-backup-$DATE.tar.gz config/ shared_files/ logs/
find $BACKUP_DIR -name "*.tar.gz" -mtime +7 -delete
```

This comprehensive deployment guide provides everything needed to run the P2P system in production with proper monitoring, security, and scalability configurations.