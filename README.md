# PEARL Mining Pool

Production-ready Stratum mining pool for PEARL cryptocurrency (Proof-of-Useful-Work blockchain).

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

### Pool Settings (`config.yaml`)

```yaml
pool:
  name: "PEARL Mining Pool"
  fee: 1.0                    # Pool fee percentage
  rewardMode: "pplns"         # pplns or prop
  pplnsWindow: 10000          # PPLNS share window
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
