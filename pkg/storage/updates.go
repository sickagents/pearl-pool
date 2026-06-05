package storage

// UpdateBlockStatus updates block status and confirmations
func (s *PostgresStore) UpdateBlockStatus(hash string, status string, confirmations int) error {
	query := `
		UPDATE blocks
		SET status = $1, confirmations = $2
		WHERE hash = $3
	`
	_, err := s.db.Exec(query, status, confirmations, hash)
	return err
}

// GetPendingShares returns last N shares that haven't been credited yet
func (s *PostgresStore) GetPendingShares(limit int) ([]Share, error) {
	query := `
		SELECT id, address, worker, difficulty, height, is_block, block_hash, created_at
		FROM shares
		WHERE credited = false
		ORDER BY created_at DESC
		LIMIT $1
	`
	
	rows, err := s.db.Query(query, limit)
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

// MarkSharesCredited marks shares as credited
func (s *PostgresStore) MarkSharesCredited(shareIDs []int64) error {
	if len(shareIDs) == 0 {
		return nil
	}
	
	query := `UPDATE shares SET credited = true WHERE id = ANY($1)`
	_, err := s.db.Exec(query, shareIDs)
	return err
}

// CreditBalance adds amount to miner's balance
func (s *PostgresStore) CreditBalance(address string, amount int64, blockID int64) error {
	query := `
		INSERT INTO balances (address, balance, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (address)
		DO UPDATE SET
			balance = balances.balance + $2,
			updated_at = NOW()
	`
	_, err := s.db.Exec(query, address, amount)
	return err
}

// RecordPayout records a payout transaction
func (s *PostgresStore) RecordPayout(address string, amount int64, txid string) error {
	query := `
		INSERT INTO payouts (address, amount, txid, status, created_at)
		VALUES ($1, $2, $3, 'sent', NOW())
	`
	_, err := s.db.Exec(query, address, amount, txid)
	return err
}
