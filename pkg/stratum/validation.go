package stratum

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"time"
)

// ValidateShare validates a submitted share
func (s *Server) validateShare(conn *Connection, jobID, extraNonce2Hex, nTimeHex, nonceHex string) (*ShareValidation, error) {
	job, exists := s.jobManager.GetJob(jobID)
	if !exists {
		return nil, fmt.Errorf("job not found")
	}
	
	// Parse nonce
	nonceBytes, err := hex.DecodeString(nonceHex)
	if err != nil {
		return nil, fmt.Errorf("invalid nonce")
	}
	if len(nonceBytes) != 4 {
		return nil, fmt.Errorf("nonce must be 4 bytes")
	}
	nonce := uint32(nonceBytes[0])<<24 | uint32(nonceBytes[1])<<16 | uint32(nonceBytes[2])<<8 | uint32(nonceBytes[3])
	
	// Parse nTime
	nTime, err := strconv.ParseInt(nTimeHex, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ntime")
	}
	
	// Check nTime range
	if nTime < job.CurTime-7200 || nTime > time.Now().Unix()+7200 {
		return nil, fmt.Errorf("ntime out of range")
	}
	
	// Parse extraNonce2
	extraNonce2, err := hex.DecodeString(extraNonce2Hex)
	if err != nil {
		return nil, fmt.Errorf("invalid extranonce2")
	}
	if len(extraNonce2) != conn.extraNonce2Size {
		return nil, fmt.Errorf("extranonce2 wrong size")
	}
	
	// Build block header (simplified - production needs proper coinbase construction)
	// For PEARL, we'd need to construct:
	// - Coinbase transaction with extraNonce1 + extraNonce2
	// - Merkle root from [coinbase] + job.Transactions
	// - Block header with all fields
	
	// TODO: Implement full block header construction
	blockHeader := s.buildBlockHeader(job, conn.extraNonce1, extraNonce2Hex, nTimeHex, nonceHex)
	
	// Hash the header
	hash := s.hashBlockHeader(blockHeader)
	
	// Convert hash to big.Int for comparison
	hashBig := new(big.Int).SetBytes(reverseBytes(hash))
	
	// Check against miner difficulty
	minerTarget := s.difficultyToTarget(conn.difficulty)
	meetsPoolDiff := hashBig.Cmp(minerTarget) <= 0
	
	if !meetsPoolDiff {
		return &ShareValidation{
			Valid:       false,
			IsBlock:     false,
			Difficulty:  conn.difficulty,
			BlockHash:   "",
		}, nil
	}
	
	// Check against network difficulty
	meetsNetworkDiff := hashBig.Cmp(job.Target) <= 0
	
	return &ShareValidation{
		Valid:       true,
		IsBlock:     meetsNetworkDiff,
		Difficulty:  conn.difficulty,
		BlockHash:   hex.EncodeToString(hash),
		BlockHeight: job.Height,
		BlockHex:    hex.EncodeToString(blockHeader),
	}, nil
}

type ShareValidation struct {
	Valid       bool
	IsBlock     bool
	Difficulty  float64
	BlockHash   string
	BlockHeight int64
	BlockHex    string
}

// buildBlockHeader constructs a block header (simplified)
func (s *Server) buildBlockHeader(job *Job, extraNonce1, extraNonce2, nTime, nonce string) []byte {
	// PEARL block header structure (similar to Bitcoin):
	// - version (4 bytes)
	// - prev_block_hash (32 bytes)
	// - merkle_root (32 bytes)
	// - timestamp (4 bytes)
	// - bits (4 bytes)
	// - nonce (4 bytes)
	// Total: 80 bytes
	
	// This is a placeholder - production needs proper implementation
	header := make([]byte, 80)
	
	// TODO: Implement proper header construction with:
	// - Coinbase transaction (with extraNonce1 + extraNonce2)
	// - Merkle tree calculation
	// - All header fields
	
	return header
}

// hashBlockHeader computes SHA256d hash
func (s *Server) hashBlockHeader(header []byte) []byte {
	first := sha256.Sum256(header)
	second := sha256.Sum256(first[:])
	return second[:]
}

// difficultyToTarget converts difficulty to target
func (s *Server) difficultyToTarget(difficulty float64) *big.Int {
	// diff = max_target / target
	// target = max_target / diff
	
	maxTarget := new(big.Int).SetBytes([]byte{
		0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	})
	
	diffBig := new(big.Float).SetFloat64(difficulty)
	maxTargetFloat := new(big.Float).SetInt(maxTarget)
	
	targetFloat := new(big.Float).Quo(maxTargetFloat, diffBig)
	target, _ := targetFloat.Int(nil)
	
	return target
}

// reverseBytes reverses byte slice (for endianness)
func reverseBytes(b []byte) []byte {
	reversed := make([]byte, len(b))
	for i := range b {
		reversed[i] = b[len(b)-1-i]
	}
	return reversed
}
