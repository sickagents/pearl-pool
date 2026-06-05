package storage

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type PostgresStore struct {
	db *sql.DB
}

func NewPostgresStore(host string, port int, user, password, database string, maxConns int) (*PostgresStore, error) {
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, database)
	
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	
	db.SetMaxOpenConns(maxConns)
	db.SetMaxIdleConns(maxConns / 2)
	db.SetConnMaxLifetime(time.Hour)
	
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}
	
	return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Close() error {
	return s.db.Close()
}

func (s *PostgresStore) RecordShare(address, worker string, difficulty float64, height int64, isBlock bool, blockHash string) error {
	query := `
		INSERT INTO shares (address, worker, difficulty, height, is_block, block_hash, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := s.db.Exec(query, address, worker, difficulty, height, isBlock, blockHash, time.Now())
	return err
}

func (s *PostgresStore) GetPendingShares(limit int) ([]Share, error) {
	query := `
		SELECT id, address, worker, difficulty, height, is_block, block_hash, created_at
		FROM shares
		WHERE credited = false
		ORDER BY created_at ASC
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

func (s *PostgresStore) MarkSharesCredited(shareIDs []int64) error {
	query := `UPDATE shares SET credited = true WHERE id = ANY($1)`
	_, err := s.db.Exec(query, shareIDs)
	return err
}

func (s *PostgresStore) RecordBlock(hash string, height int64, reward int64, finder string) error {
	query := `
		INSERT INTO blocks (hash, height, reward, finder, status, created_at)
		VALUES ($1, $2, $3, $4, 'pending', $5)
	`
	_, err := s.db.Exec(query, hash, height, reward, finder, time.Now())
	return err
}

func (s *PostgresStore) UpdateBlockStatus(hash string, status string, confirmations int) error {
	query := `UPDATE blocks SET status = $1, confirmations = $2 WHERE hash = $3`
	_, err := s.db.Exec(query, status, confirmations, hash)
	return err
}

func (s *PostgresStore) GetPendingBlocks() ([]Block, error) {
	query := `
		SELECT id, hash, height, reward, finder, status, confirmations, created_at
		FROM blocks
		WHERE status IN ('pending', 'confirming')
		ORDER BY height ASC
	`
	
	rows, err := s.db.Query(query)
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

func (s *PostgresStore) CreditBalance(address string, amount int64, blockID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	query := `
		INSERT INTO balances (address, balance, created_at, updated_at)
		VALUES ($1, $2, $3, $3)
		ON CONFLICT (address) DO UPDATE SET balance = balances.balance + $2, updated_at = $3
	`
	_, err = tx.Exec(query, address, amount, time.Now())
	if err != nil {
		return err
	}
	
	creditQuery := `
		INSERT INTO balance_credits (address, amount, block_id, created_at)
		VALUES ($1, $2, $3, $4)
	`
	_, err = tx.Exec(creditQuery, address, amount, blockID, time.Now())
	if err != nil {
		return err
	}
	
	return tx.Commit()
}

func (s *PostgresStore) GetPayoutDue(minAmount int64) ([]Balance, error) {
	query := `
		SELECT address, balance
		FROM balances
		WHERE balance >= $1
		ORDER BY balance DESC
	`
	
	rows, err := s.db.Query(query, minAmount)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	var balances []Balance
	for rows.Next() {
		var b Balance
		if err := rows.Scan(&b.Address, &b.Balance); err != nil {
			return nil, err
		}
		balances = append(balances, b)
	}
	
	return balances, rows.Err()
}

func (s *PostgresStore) RecordPayout(address string, amount int64, txid string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	
	deductQuery := `UPDATE balances SET balance = balance - $1, updated_at = $2 WHERE address = $3`
	_, err = tx.Exec(deductQuery, amount, time.Now(), address)
	if err != nil {
		return err
	}
	
	payoutQuery := `
		INSERT INTO payouts (address, amount, txid, status, created_at)
		VALUES ($1, $2, $3, 'sent', $4)
	`
	_, err = tx.Exec(payoutQuery, address, amount, txid, time.Now())
	if err != nil {
		return err
	}
	
	return tx.Commit()
}

type Share struct {
	ID         int64
	Address    string
	Worker     string
	Difficulty float64
	Height     int64
	IsBlock    bool
	BlockHash  string
	CreatedAt  time.Time
}

type Block struct {
	ID            int64
	Hash          string
	Height        int64
	Reward        int64
	Finder        string
	Status        string
	Confirmations int
	CreatedAt     time.Time
}

type Balance struct {
	Address string
	Balance int64
}
