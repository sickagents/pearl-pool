package storage

import (
	"time"
)

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

func (s *PostgresStore) GetBalance(address string) (int64, error) {
	query := `SELECT balance FROM balances WHERE address = $1`
	
	var balance int64
	err := s.db.QueryRow(query, address).Scan(&balance)
	if err != nil {
		return 0, nil // Return 0 if not found
	}
	
	return balance, nil
}

func (s *PostgresStore) GetPaymentHistory(address string, limit int) ([]Payout, error) {
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
		if err := rows.Scan(&p.ID, &p.Address, &p.Amount, &p.Txid, &p.Status, &p.CreatedAt); err != nil {
			return nil, err
		}
		payouts = append(payouts, p)
	}
	
	return payouts, rows.Err()
}

type Payout struct {
	ID        int64
	Address   string
	Amount    int64
	Txid      string
	Status    string
	CreatedAt time.Time
}

type PoolStats struct {
	TotalShares   int64
	ActiveMiners  int
	TotalBlocks   int64
	PendingBlocks int
}

func (s *PostgresStore) GetPoolStats() (*PoolStats, error) {
	stats := &PoolStats{}
	
	// Count total shares in last 24 hours
	query := `SELECT COUNT(*) FROM shares WHERE created_at > NOW() - INTERVAL '24 hours'`
	s.db.QueryRow(query).Scan(&stats.TotalShares)
	
	// Count active miners (miners with shares in last hour)
	query = `SELECT COUNT(DISTINCT address) FROM shares WHERE created_at > NOW() - INTERVAL '1 hour'`
	s.db.QueryRow(query).Scan(&stats.ActiveMiners)
	
	// Count total blocks
	query = `SELECT COUNT(*) FROM blocks`
	s.db.QueryRow(query).Scan(&stats.TotalBlocks)
	
	// Count pending blocks
	query = `SELECT COUNT(*) FROM blocks WHERE status IN ('pending', 'confirming')`
	s.db.QueryRow(query).Scan(&stats.PendingBlocks)
	
	return stats, nil
}
