-- Shares table
CREATE TABLE IF NOT EXISTS shares (
    id BIGSERIAL PRIMARY KEY,
    address VARCHAR(90) NOT NULL,
    worker VARCHAR(255) NOT NULL,
    difficulty DOUBLE PRECISION NOT NULL,
    height BIGINT NOT NULL,
    is_block BOOLEAN NOT NULL DEFAULT FALSE,
    block_hash VARCHAR(64),
    credited BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_shares_address ON shares(address);
CREATE INDEX idx_shares_height ON shares(height);
CREATE INDEX idx_shares_credited ON shares(credited);
CREATE INDEX idx_shares_created_at ON shares(created_at);

-- Blocks table
CREATE TABLE IF NOT EXISTS blocks (
    id BIGSERIAL PRIMARY KEY,
    hash VARCHAR(64) NOT NULL UNIQUE,
    height BIGINT NOT NULL,
    reward BIGINT NOT NULL,
    finder VARCHAR(90) NOT NULL,
    status VARCHAR(20) NOT NULL, -- pending, confirming, confirmed, orphaned
    confirmations INT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMP
);

CREATE INDEX idx_blocks_status ON blocks(status);
CREATE INDEX idx_blocks_height ON blocks(height);

-- Balances table
CREATE TABLE IF NOT EXISTS balances (
    address VARCHAR(90) PRIMARY KEY,
    balance BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_balances_balance ON balances(balance);

-- Balance credits table (audit trail)
CREATE TABLE IF NOT EXISTS balance_credits (
    id BIGSERIAL PRIMARY KEY,
    address VARCHAR(90) NOT NULL,
    amount BIGINT NOT NULL,
    block_id BIGINT REFERENCES blocks(id),
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_balance_credits_address ON balance_credits(address);
CREATE INDEX idx_balance_credits_block_id ON balance_credits(block_id);

-- Payouts table
CREATE TABLE IF NOT EXISTS payouts (
    id BIGSERIAL PRIMARY KEY,
    address VARCHAR(90) NOT NULL,
    amount BIGINT NOT NULL,
    txid VARCHAR(64) NOT NULL,
    status VARCHAR(20) NOT NULL, -- sent, confirmed
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    confirmed_at TIMESTAMP
);

CREATE INDEX idx_payouts_address ON payouts(address);
CREATE INDEX idx_payouts_txid ON payouts(txid);
CREATE INDEX idx_payouts_status ON payouts(status);

-- Pool stats table
CREATE TABLE IF NOT EXISTS pool_stats (
    id SERIAL PRIMARY KEY,
    timestamp TIMESTAMP NOT NULL DEFAULT NOW(),
    hashrate DOUBLE PRECISION NOT NULL,
    miners INT NOT NULL,
    workers INT NOT NULL,
    blocks_found INT NOT NULL
);

CREATE INDEX idx_pool_stats_timestamp ON pool_stats(timestamp);
