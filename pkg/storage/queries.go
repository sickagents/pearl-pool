package storage

import "time"

// GetBlocksByStatus returns blocks filtered by status
func (s *PostgresStore) GetBlocksByStatus(status string, limit int) ([]Block, error) {
	query := `
		SELECT id, hash, height, reward, finder, status, confirmations, created_at
		FROM blocks
		WHERE status = $1
		ORDER BY height DESC
		LIMIT $2
	`
	
	rows, err := s.db.Query(query, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var blocks []Block
	for rows.Next() {
		var b Block
		if err := rows.Scan(&b.ID, &b.Hash, &b.Height, &b.Reward, &b.Finder, &b.Status, &b.Confirmations, &b.CreatedAt); err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	
	return blocks, rows.Err()
}

// GetRecentBlocks returns N most recent blocks
func (s *PostgresStore) GetRecentBlocks(limit int) ([]Block, error) {
	query := `
		SELECT id, hash, height, reward, finder, status, confirmations, created_at
		FROM blocks
		ORDER BY height DESC
		LIMIT $1
	`
	
	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var blocks []Block
	for rows.Next() {
		var b Block
		if err := rows.Scan(&b.ID, &b.Hash, &b.Height, &b.Reward, &b.Finder, &b.Status, &b.Confirmations, &b.CreatedAt); err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	
	return blocks, rows.Err()
}

// GetBlocksInTimeRange returns blocks found within time range
func (s *PostgresStore) GetBlocksInTimeRange(start, end time.Time) ([]Block, error) {
	query := `
		SELECT id, hash, height, reward, finder, status, confirmations, created_at
		FROM blocks
		WHERE created_at >= $1 AND created_at <= $2
		ORDER BY height DESC
	`
	
	rows, err := s.db.Query(query, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var blocks []Block
	for rows.Next() {
		var b Block
		if err := rows.Scan(&b.ID, &b.Hash, &b.Height, &b.Reward, &b.Finder, &b.Status, &b.Confirmations, &b.CreatedAt); err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	
	return blocks, rows.Err()
}

// GetMinerBalance returns balance for specific address
func (s *PostgresStore) GetMinerBalance(address string) (int64, error) {
	var balance int64
	query := `SELECT balance FROM balances WHERE address = $1`
	err := s.db.QueryRow(query, address).Scan(&balance)
	if err != nil {
		return 0, err
	}
	return balance, nil
}

// GetMinerPayouts returns payout history for address
func (s *PostgresStore) GetMinerPayouts(address string, limit int) ([]Payout, error) {
	query := `
		SELECT id, address, amount, txid, status, created_at
		FROM payouts
		WHERE address = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	
	rows, err := s.db.Query(query, address, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var payouts []Payout
	for rows.Next() {
		var p Payout
		if err := rows.Scan(&p.ID, &p.Address, &p.Amount, &p.TxID, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		payouts = append(payouts, p)
	}
	
	return payouts, rows.Err()
}

// GetMinerShares returns share history for address
func (s *PostgresStore) GetMinerShares(address string, limit int) ([]Share, error) {
	query := `
		SELECT id, address, worker, difficulty, height, is_block, block_hash, created_at
		FROM shares
		WHERE address = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	
	rows, err := s.db.Query(query, address, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var shares []Share
	for rows.Next() {
		var s Share
		if err := rows.Scan(&s.ID, &s.Address, &s.Worker, &s.Difficulty, &s.Height, &s.IsBlock, &s.BlockHash, &s.CreatedAt); err != nil {
			return nil, err
		}
		shares = append(shares, s)
	}
	
	return shares, rows.Err()
}

type Payout struct {
	ID        int64
	Address   string
	Amount    int64
	TxID      string
	Status    string
	CreatedAt time.Time
}

// GetPoolStats returns aggregated pool statistics
func (s *PostgresStore) GetPoolStats() (*PoolStats, error) {
	stats := &PoolStats{}
	
	// Total blocks found
	err := s.db.QueryRow(`SELECT COUNT(*) FROM blocks WHERE status = 'confirmed'`).Scan(&stats.TotalBlocks)
	if err != nil {
		return nil, err
	}
	
	// Total payouts
	err = s.db.QueryRow(`SELECT COALESCE(SUM(amount), 0) FROM payouts WHERE status = 'sent'`).Scan(&stats.TotalPaid)
	if err != nil {
		return nil, err
	}
	
	// Unique miners (last 24h)
	err = s.db.QueryRow(`SELECT COUNT(DISTINCT address) FROM shares WHERE created_at > NOW() - INTERVAL '24 hours'`).Scan(&stats.ActiveMiners)
	if err != nil {
		return nil, err
	}
	
	// Total shares (last 24h)
	err = s.db.QueryRow(`SELECT COUNT(*) FROM shares WHERE created_at > NOW() - INTERVAL '24 hours'`).Scan(&stats.Shares24h)
	if err != nil {
		return nil, err
	}
	
	return stats, nil
}

type PoolStats struct {
	TotalBlocks   int64
	TotalPaid     int64
	ActiveMiners  int
	Shares24h     int64
}
