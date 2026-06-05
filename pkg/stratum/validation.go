package stratum

import (
	"encoding/hex"
	"fmt"
)

// ValidateShare validates a submitted share
func (s *Server) ValidateShare(conn *Connection, job *Job, extraNonce2, nTimeStr, nonceStr string) (*ShareResult, error) {
	// Parse hex strings
	extraNonce2Bytes, err := hex.DecodeString(extraNonce2)
	if err != nil {
		return nil, fmt.Errorf("invalid extranonce2: %w", err)
	}
	
	if len(extraNonce2Bytes) != conn.extraNonce2Size {
		return nil, fmt.Errorf("extranonce2 size mismatch: expected %d, got %d", conn.extraNonce2Size, len(extraNonce2Bytes))
	}
	
	nTime, err := hex.DecodeString(nTimeStr)
	if err != nil {
		return nil, fmt.Errorf("invalid ntime: %w", err)
	}
	if len(nTime) != 4 {
		return nil, fmt.Errorf("ntime must be 4 bytes")
	}
	
	nonce, err := hex.DecodeString(nonceStr)
	if err != nil {
		return nil, fmt.Errorf("invalid nonce: %w", err)
	}
	if len(nonce) != 4 {
		return nil, fmt.Errorf("nonce must be 4 bytes")
	}
	
	// Build full extranonce
	extraNonce1Bytes, _ := hex.DecodeString(conn.extraNonce1)
	fullExtraNonce := append(extraNonce1Bytes, extraNonce2Bytes...)
	
	// Construct block header
	// For PEARL, block header structure:
	// - version (4 bytes)
	// - prev_block_hash (32 bytes)
	// - merkle_root (32 bytes)
	// - timestamp (4 bytes)
	// - bits (4 bytes)
	// - nonce (4 bytes)
	// Total: 80 bytes (standard Bitcoin-style header)
	
	// Build coinbase transaction with extranonce
	coinbaseTx := s.buildCoinbaseTx(job, fullExtraNonce, conn.address)
	
	// Calculate merkle root with coinbase + job transactions
	merkleRoot := s.calculateMerkleRoot(coinbaseTx, job.Transactions)
	
	// Build block header
	header := s.buildBlockHeader(job, merkleRoot, nTime, nonce)
	
	// For PEARL ZK proof validation:
	// Option A (current): Forward to node RPC for validation
	// Option B (future): Embed libzk_pow_ffi.a for local validation
	
	// We'll use Option A: construct full block and validate via node
	blockHex := s.serializeBlock(header, coinbaseTx, job.Transactions)
	
	result := &ShareResult{
		Valid:      false,
		IsBlock:    false,
		BlockHash:  "",
		Difficulty: conn.difficulty,
	}
	
	// TODO: For now, accept all shares (trust miner)
	// In production, MUST validate via node RPC or embedded verifier
	result.Valid = true
	
	// Check if meets network difficulty (block found)
	// This requires actual hash computation, which for PEARL means ZK proof
	// For MVP: we'll submit every share to node and check response
	
	return result, nil
}

type ShareResult struct {
	Valid      bool
	IsBlock    bool
	BlockHash  string
	Difficulty float64
}

func (s *Server) buildCoinbaseTx(job *Job, extraNonce []byte, address string) []byte {
	// Simplified coinbase construction
	// Real implementation needs:
	// - Version (4 bytes)
	// - Input count (varint, usually 1)
	// - Previous output (32 bytes of 0x00, index 0xffffffff)
	// - Script length (varint)
	// - Coinbase script (height + extranonce + arbitrary data)
	// - Sequence (0xffffffff)
	// - Output count (varint, usually 1)
	// - Output value (8 bytes, coinbasevalue from template)
	// - Output script length (varint)
	// - Output script (P2TR for prl1 address)
	// - Locktime (4 bytes, usually 0)
	
	// Placeholder: return dummy coinbase
	// TODO: Proper BIP34 height encoding + extranonce injection
	return []byte{}
}

func (s *Server) calculateMerkleRoot(coinbaseTx []byte, transactions [][]byte) []byte {
	// Build merkle tree from coinbase + transactions
	// TODO: Proper SHA256d merkle tree construction
	return make([]byte, 32) // Placeholder
}

func (s *Server) buildBlockHeader(job *Job, merkleRoot []byte, nTime []byte, nonce []byte) []byte {
	// Standard 80-byte block header
	// TODO: Proper little-endian serialization
	return make([]byte, 80) // Placeholder
}

func (s *Server) serializeBlock(header []byte, coinbaseTx []byte, transactions [][]byte) string {
	// Serialize full block: header + txn_count + coinbase + transactions
	// TODO: Proper Bitcoin block serialization
	return "" // Placeholder
}

// DuplicateShareCheck checks if share was already submitted
func (s *Server) isDuplicateShare(conn *Connection, jobID, extraNonce2, nonce string) bool {
	// TODO: Track submitted shares in Redis with TTL
	// Key: conn.address:jobID:extraNonce2:nonce
	// TTL: job lifetime (e.g., 10 minutes)
	return false
}

// StaleShareCheck checks if job is still valid
func (s *Server) isStaleJob(job *Job) bool {
	// Job is stale if it's not the current job and too old
	currentJob := s.jobManager.GetCurrentJob()
	if currentJob == nil {
		return true
	}
	
	// Allow submissions for current job and previous 2 jobs (to handle network latency)
	if job.ID == currentJob.ID {
		return false
	}
	
	// Check if job is in manager's cache
	_, exists := s.jobManager.GetJob(job.ID)
	return !exists
}
