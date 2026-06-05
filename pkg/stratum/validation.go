package stratum

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// ValidateShare validates a submitted share
func ValidateShare(job *Job, nonce uint32, extraNonce2 string, nTime string) (*ShareValidation, error) {
	// Parse extra nonce 2
	extraNonce2Bytes, err := hex.DecodeString(extraNonce2)
	if err != nil {
		return nil, fmt.Errorf("invalid extranonce2: %w", err)
	}
	
	// Parse nTime
	nTimeBytes, err := hex.DecodeString(nTime)
	if err != nil {
		return nil, fmt.Errorf("invalid ntime: %w", err)
	}
	
	if len(nTimeBytes) != 4 {
		return nil, fmt.Errorf("ntime must be 4 bytes")
	}
	
	nTimeInt := binary.LittleEndian.Uint32(nTimeBytes)
	
	// Validate nTime is not too far in the future or past
	if int64(nTimeInt) > job.CurTime+7200 {
		return nil, fmt.Errorf("ntime too far in future")
	}
	
	if int64(nTimeInt) < job.CurTime-7200 {
		return nil, fmt.Errorf("ntime too far in past")
	}
	
	// Compute block hash (simplified - real implementation needs full block construction)
	// For PEARL, this would construct the block header with ZK certificate
	// and compute the hash for difficulty check
	
	// NOTE: This is a placeholder. Real implementation requires:
	// 1. Construct coinbase transaction with extraNonce1 + extraNonce2
	// 2. Build merkle tree with coinbase + template transactions
	// 3. Construct block header with merkle root
	// 4. Compute block hash
	// 5. Check hash meets difficulty target
	
	blockHash := computeBlockHash(job, nonce, extraNonce2Bytes, nTimeInt)
	
	// Check if meets difficulty
	meetsDifficulty := checkDifficulty(blockHash, job.Target)
	
	return &ShareValidation{
		Valid:           true,
		BlockHash:       hex.EncodeToString(blockHash),
		MeetsDifficulty: meetsDifficulty,
		Nonce:           nonce,
		ExtraNonce2:     extraNonce2,
		NTime:           nTimeInt,
	}, nil
}

// ShareValidation represents validation result
type ShareValidation struct {
	Valid           bool
	BlockHash       string
	MeetsDifficulty bool
	Nonce           uint32
	ExtraNonce2     string
	NTime           uint32
	ErrorReason     string
}

// computeBlockHash computes block hash (placeholder)
func computeBlockHash(job *Job, nonce uint32, extraNonce2 []byte, nTime uint32) []byte {
	// PLACEHOLDER: Real implementation needs full block header construction
	// This is simplified for MVP - actual hash comes from PEARL node validation
	
	h := sha256.New()
	h.Write([]byte(job.PrevHash))
	h.Write([]byte(job.MerkleRoot))
	
	nonceBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(nonceBytes, nonce)
	h.Write(nonceBytes)
	
	h.Write(extraNonce2)
	
	timeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(timeBytes, nTime)
	h.Write(timeBytes)
	
	return h.Sum(nil)
}

// checkDifficulty checks if hash meets target difficulty
func checkDifficulty(hash []byte, target *big.Int) bool {
	// Convert hash to big.Int (little-endian)
	hashInt := new(big.Int).SetBytes(reverseBytes(hash))
	
	// Hash must be less than target
	return hashInt.Cmp(target) < 0
}

// reverseBytes reverses byte slice
func reverseBytes(b []byte) []byte {
	reversed := make([]byte, len(b))
	for i := range b {
		reversed[i] = b[len(b)-1-i]
	}
	return reversed
}

// DuplicateShareChecker tracks recent shares to prevent duplicates
type DuplicateShareChecker struct {
	recent map[string]bool
	mu     sync.RWMutex
}

// NewDuplicateShareChecker creates a new checker
func NewDuplicateShareChecker() *DuplicateShareChecker {
	return &DuplicateShareChecker{
		recent: make(map[string]bool),
	}
}

// Check checks if share is duplicate
func (d *DuplicateShareChecker) Check(jobID string, nonce uint32, extraNonce2 string) bool {
	key := fmt.Sprintf("%s-%d-%s", jobID, nonce, extraNonce2)
	
	d.mu.RLock()
	exists := d.recent[key]
	d.mu.RUnlock()
	
	if exists {
		return true // duplicate
	}
	
	d.mu.Lock()
	d.recent[key] = true
	
	// Cleanup old entries (simple approach - keep last 10000)
	if len(d.recent) > 10000 {
		// Remove oldest half
		count := 0
		for k := range d.recent {
			delete(d.recent, k)
			count++
			if count > 5000 {
				break
			}
		}
	}
	d.mu.Unlock()
	
	return false // not duplicate
}
