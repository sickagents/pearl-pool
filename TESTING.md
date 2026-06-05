# PEARL Mining Pool - Testing Guide

## Prerequisites

1. PEARL full node running with RPC enabled
2. PostgreSQL 16
3. Redis 7
4. Go 1.22+ (for local build) or Docker

## Setup Steps

### 1. Clone and Configure

```bash
git clone https://github.com/sickagents/pearl-pool.git
cd pearl-pool

# Copy config templates
cp config.yaml.example config.yaml
cp .env.example .env
```

### 2. Edit Configuration

**`.env` file:**
```bash
PEARL_NODE_HOST=localhost
PEARL_NODE_PORT=9332
PEARL_NODE_USER=your_rpc_user
PEARL_NODE_PASS=your_rpc_pass

POSTGRES_HOST=localhost
POSTGRES_PORT=5432
POSTGRES_USER=pearl_pool
POSTGRES_PASSWORD=pearl_pool_pass
POSTGRES_DB=pearl_pool

REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
```

**`config.yaml`** - adjust pool fee, confirmation depth, payout settings as needed

### 3. Start Infrastructure

```bash
# Start PostgreSQL + Redis
docker-compose up -d postgres redis

# Wait for PostgreSQL to be ready
sleep 5

# Run migrations
docker exec -i pearl-pool-postgres psql -U pearl_pool -d pearl_pool < migrations/001_init.sql
```

### 4. Start Pool

**Option A: Docker (recommended)**
```bash
docker-compose up -d pool payout api
```

**Option B: Local build**
```bash
go build -o bin/pool ./cmd/pool
go build -o bin/payout ./cmd/payout
go build -o bin/api ./cmd/api

# Start pool
./bin/pool &

# Start payout daemon
./bin/payout &

# Start API server
./bin/api &
```

### 5. Verify Pool is Running

```bash
# Check logs
docker-compose logs -f pool

# Test API
curl http://localhost:8080/api/pool/stats
curl http://localhost:8080/health

# Check Stratum ports
nc -zv localhost 3360
nc -zv localhost 3361
nc -zv localhost 3362
```

## Mining Test

### Test with SRBMiner

```bash
# Download SRBMiner
wget https://github.com/doktor83/SRBMiner-Multi/releases/download/2.4.8/SRBMiner-Multi-2-4-8-Linux.tar.xz
tar -xf SRBMiner-Multi-2-4-8-Linux.tar.xz
cd SRBMiner-Multi-2-4-8

# Mine to pool (replace with your PEARL address)
./SRBMiner-MULTI \
  --algorithm pearlhash \
  --pool localhost:3360 \
  --wallet prl1qxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx.worker1 \
  --password x
```

### Monitor Mining

```bash
# Watch pool logs for share submissions
docker-compose logs -f pool | grep "Share accepted"

# Check miner stats via API
curl http://localhost:8080/api/miner/prl1qxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

# Monitor blocks found
curl http://localhost:8080/api/pool/blocks
```

## Troubleshooting

### Pool won't start
```bash
# Check RPC connection to PEARL node
curl --user $PEARL_NODE_USER:$PEARL_NODE_PASS \
  --data '{"jsonrpc":"1.0","id":"test","method":"getblockcount","params":[]}' \
  http://$PEARL_NODE_HOST:$PEARL_NODE_PORT

# Check PostgreSQL
docker exec pearl-pool-postgres psql -U pearl_pool -d pearl_pool -c "SELECT 1"

# Check Redis
docker exec pearl-pool-redis redis-cli ping
```

### Shares rejected
```bash
# Enable debug logging in config.yaml
log_level: debug

# Watch detailed share validation
docker-compose logs -f pool | grep -A5 "Share"
```

### Block not confirmed
```bash
# Check block monitor is running
docker-compose logs pool | grep "Block monitor"

# Query pending blocks
docker exec pearl-pool-postgres psql -U pearl_pool -d pearl_pool \
  -c "SELECT hash, height, status, confirmations FROM blocks ORDER BY height DESC LIMIT 10"

# Check PEARL node block height
curl --user $PEARL_NODE_USER:$PEARL_NODE_PASS \
  --data '{"jsonrpc":"1.0","id":"test","method":"getblockcount","params":[]}' \
  http://$PEARL_NODE_HOST:$PEARL_NODE_PORT
```

## Known Limitations (MVP)

1. **Share validation is simplified** - `buildBlockHeader()` in `pkg/stratum/validation.go` needs full coinbase + merkle tree construction
2. **No VarDiff retargeting** - difficulty is static per-port
3. **Block reward hardcoded** - uses placeholder 100 PEARL, should query from `getblocktemplate`
4. **No TLS support** - Stratum and API are plaintext
5. **No rate limiting** - DoS protection not implemented
6. **No Prometheus metrics** - monitoring endpoints exist but not integrated

## Next Steps

1. Test with real PEARL node on testnet
2. Implement full block header construction (coinbase + merkle tree)
3. Add VarDiff retargeting logic
4. Add unit tests for accounting/rewards
5. Implement TLS for production
6. Add Prometheus metrics
7. Build frontend dashboard

## Security Notes

- **Never expose RPC credentials** - keep `.env` out of git (already in `.gitignore`)
- **Use TLS in production** - plaintext Stratum can be MitM attacked
- **Firewall API server** - only expose to trusted networks
- **Database security** - use strong passwords, restrict network access
- **Wallet security** - pool wallet should have separate hot/cold storage strategy
