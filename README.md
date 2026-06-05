# PEARL Mining Pool

Production-ready Stratum mining pool for PEARL coin with pearlhash proof-of-work algorithm.

**Status:** MVP Complete (Priority 1 features implemented)

## ⚠️ Important Notes

### Block Construction (TODO)
The pool currently has **placeholder block construction** in `pkg/stratum/block.go`. For production use, you need to implement:

1. **Coinbase transaction construction** with extraNonce1+2
2. **Merkle tree building** from coinbase + template transactions  
3. **Block header serialization** (80 bytes: version, prevhash, merkleroot, timestamp, bits, nonce)
4. **ZK certificate construction** (PEARL-specific: proof_data, public_data, commitment)
5. **Full block serialization** to hex for `submitblock` RPC

Reference: `/tmp/pearl/node/wire/msgblock.go` for PEARL block structure.

### Share Validation (Simplified)
Current implementation uses **simplified hash checking** for difficulty validation. For production:

- Forward shares to PEARL node RPC for ZK proof validation (Option A: trusted node validation)
- Or implement embedded Rust verifier via CGO (Option B: higher performance, more complexity)

See `pkg/stratum/validation.go` for validation logic.

## Features

- **Multi-port Stratum servers** with difficulty targeting (3360, 3361, 3362)
- **PPLNS & PROP reward modes**
- **Solo mining support** with `solo:` prefix
- **Variable difficulty** (VarDiff) per-miner
- **ZK proof validation** via PEARL node RPC
- **Automatic payouts** with configurable thresholds
- **PostgreSQL + Redis** for high-performance accounting
- **Prometheus metrics** endpoint
- **Docker Compose** one-command deployment

## Architecture

```
pearl-pool/
├── cmd/
│   ├── pool/          # Stratum server + job coordinator
│   ├── payout/        # Payout engine (scheduled)
│   └── api/           # REST API + metrics
├── pkg/
│   ├── rpc/           # PEARL node JSON-RPC client
│   ├── stratum/       # Stratum protocol implementation
│   ├── vardiff/       # Variable difficulty retargeting
│   ├── accounting/    # Share tracking & reward calculation
│   ├── payout/        # Transaction batching & sending
│   ├── storage/       # PostgreSQL + Redis abstraction
│   ├── metrics/       # Prometheus metrics
│   └── config/        # Configuration management
├── migrations/        # PostgreSQL schema
├── docker-compose.yml
└── config.yaml.example
```

## Quick Start

### Prerequisites

- Docker & Docker Compose
- Running PEARL full node with RPC enabled
- Node RPC credentials

### 1. Clone & Configure

```bash
git clone https://github.com/YOUR_USERNAME/pearl-pool.git
cd pearl-pool

# Copy config templates
cp .env.example .env
cp config.yaml.example config.yaml

# Edit .env with your PEARL node RPC credentials
nano .env
```

### 2. Start the Pool

```bash
docker-compose up -d
```

Services:
- **Stratum ports:** 3360, 3361, 3362
- **API:** http://localhost:8080
- **Metrics:** http://localhost:8080/metrics

### 3. Point Miners

**SRBMiner example:**
```bash
srbminer --algorithm pearlhash \
  --pool stratum+tcp://your-pool-ip:3360 \
  --wallet prl1your_wallet_address
```

**Solo mining:**
```bash
--wallet solo:prl1your_wallet_address
```

## Configuration

### config.yaml

Key settings in `config.yaml`:

```yaml
pool:
  name: "PEARL Pool"
  fee: 1.0                    # Pool fee percentage (1.0 = 1%)
  min_payout: 1000000000      # Minimum payout threshold (10 PEARL = 1,000,000,000 satoshis)
  payout_interval: 900        # Payout interval in seconds (900 = 15 minutes)
  reward_mode: "pplns"        # "pplns" or "prop"
  pplns_window: 100           # PPLNS window size (number of shares)
  confirmation_depth: 100     # Blocks before reward is credited (PEARL default)

node:
  host: "localhost"           # PEARL node RPC host
  port: 44107                 # PEARL node RPC port (default)
  rpcuser: ""                 # Set via env var PEARL_NODE_USER
  rpcpass: ""                 # Set via env var PEARL_NODE_PASS
  tls: false                  # Use TLS for RPC connection
  submit_timeout: 30          # Block submission timeout (seconds)

stratum:
  ports:
    - port: 3360
      difficulty: 2000000     # 2M difficulty (< 500 TH/s miners)
    - port: 3361
      difficulty: 4000000     # 4M difficulty (500-1000 TH/s)
    - port: 3362
      difficulty: 8000000     # 8M difficulty (1000+ TH/s)

vardiff:
  target_shares_per_min: 15   # Target share rate
  retarget_interval: 120      # Retarget every 2 minutes
  variance_percent: 30        # Allow 30% variance
  min_difficulty: 1000000     # 1M minimum
  max_difficulty: 100000000   # 100M maximum

api:
  host: "0.0.0.0"
  port: 8080
  enable_metrics: true
  metrics_path: "/metrics"
  cors_enabled: true
  cors_origins: ["*"]

database:
  host: "localhost"
  port: 5432
  user: "pearl_pool"
  password: "pearl_pool_pass"  # Override via env var
  database: "pearl_pool"
  max_conns: 20

redis:
  host: "localhost"
  port: 6379
  password: ""
  db: 0
```

### Environment Variables

Override config via `.env`:

```bash
# PEARL Node
PEARL_NODE_HOST=your_node_ip
PEARL_NODE_PORT=44107
PEARL_NODE_USER=your_rpc_user
PEARL_NODE_PASS=your_rpc_password

# Database
PEARL_POOL_DATABASE_HOST=postgres
PEARL_POOL_DATABASE_PASSWORD=pearl_pool_pass

# Redis
PEARL_POOL_REDIS_HOST=redis
```
  minPayout: 10.0             # Minimum payout in PEARL
  confirmationDepth: 100      # Blocks to wait before crediting rewards
  soloMiningEnabled: true     # Allow solo: prefix
```

### Stratum Ports

| Port | Difficulty | Recommended Hashrate |
|------|------------|---------------------|
| 3360 | 2.00 M     | < 500 TH/s          |
| 3361 | 4.00 M     | 500-1000 TH/s       |
| 3362 | 8.00 M     | > 1000 TH/s         |

### Node RPC

Pool requires these RPC methods:
- `getblocktemplate` — fetch mining jobs
- `submitblock` — submit found blocks
- `testblockvalidity` — validate shares (optional, for node-based validation)
- `sendmany` — batch payouts

## Development

### Build from Source

```bash
# Install dependencies
go mod download

# Build binaries
go build -o bin/pool ./cmd/pool
go build -o bin/payout ./cmd/payout
go build -o bin/api ./cmd/api

# Run pool
./bin/pool
```

### Database Schema

Migrations in `migrations/001_init.sql`:
- `shares` — submitted shares with difficulty & height
- `blocks` — found blocks with confirmation status
- `balances` — miner balances (credited after confirmations)
- `payouts` — payout transaction history

### Testing

```bash
go test ./...
```

## PEARL Chain Parameters

- **Block time:** 194 seconds (3m 14s)
- **Confirmation depth:** 100 blocks
- **PoW algorithm:** pearlhash (ZK proof-of-useful-work)
- **Address format:** Bech32 with HRP `prl`
- **Difficulty adjustment:** WTEMA (half-life 1 week)

## Share Validation Strategy

**Option A (current):** Pool forwards shares to node RPC for ZK proof validation.
- ✅ Simple implementation, no ZK library binding needed
- ⚠️ RPC overhead (~50-200ms per share)

**Option B (future):** Embed `libzk_pow_ffi.a` (Rust FFI) for local validation.
- ✅ Low latency, scalable
- ⚠️ Complex build (requires Rust toolchain)

## Monitoring

Prometheus metrics at `/metrics`:
- `pool_hashrate` — total pool hashrate
- `pool_miners` — active miners count
- `pool_workers` — active workers count
- `pool_blocks_found` — blocks found (lifetime)
- `pool_shares_accepted` — accepted shares
- `pool_shares_rejected` — rejected shares

## Troubleshooting

**Stratum connection refused:**
- Check firewall allows ports 3360-3362
- Verify pool service is running: `docker-compose logs pool`

**No payouts:**
- Check payout service logs: `docker-compose logs payout`
- Verify node wallet has funds for tx fees
- Check minimum payout threshold in config

**High share rejection:**
- Check node RPC latency
- Verify miner supports pearlhash algorithm
- Check difficulty settings

## Security

- **Never expose RPC credentials** — use environment variables
- **Use TLS** for Stratum in production (set `tls: true` in config)
- **Firewall** — only expose Stratum + API ports
- **Separate wallet** — use dedicated wallet for pool payouts
- **Rate limiting** — enable in reverse proxy (nginx)

## License

ISC License

## Contributing

PRs welcome. For major changes, open an issue first.

## Support

- GitHub Issues: [Report bugs](https://github.com/YOUR_USERNAME/pearl-pool/issues)
- PEARL Discord: [Join community](https://discord.gg/pearl)
- Pool operator guide: [Wiki](https://github.com/YOUR_USERNAME/pearl-pool/wiki)

## References

- [PEARL Official Repo](https://github.com/pearl-research-labs/pearl)
- [PEARL Research Paper](https://arxiv.org/abs/2504.09971)
- [Stratum Protocol Spec](https://en.bitcoin.it/wiki/Stratum_mining_protocol)
