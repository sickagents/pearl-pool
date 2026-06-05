package stratum

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
)

// ValidateShare checks if a submitted share is valid
func ValidateShare(job *Job, nonce uint32, extraNonce2 string, ntime int64, minerAddress string, difficulty float64) (*Share, error) {
	// Decode extranonce2
	extraNonce2Bytes, err := hex.DecodeString(extraNonce2)
	if err != nil {
		return nil, fmt.Errorf("invalid extranonce2: %w", err)
	}
	
	// Build block header
	// TODO: This is simplified - need proper coinbase construction
	header := buildBlockHeader(job, nonce, extraNonce2Bytes, ntime)
	
	// Compute hash
	hash := sha256.Sum256(header)
	hash = sha256.Sum256(hash[:]) // Double SHA256
	
	// Convert hash to big-endian for comparison
	hashBig := reverseBytes(hash[:])
	
	// Check against difficulty
	shareDiff := hashToDifficulty(hashBig)
	
	if shareDiff < difficulty {
		return nil, fmt.Errorf("share difficulty too low: got %.2f, need %.2f", shareDiff, difficulty)
	}
	
	// Check if meets network target
	isBlock := false
	var blockHash string
	
	if job.Target != nil {
		hashInt := bytesToBigInt(hashBig)
		if hashInt.Cmp(job.Target) <= 0 {
			isBlock = true
			blockHash = hex.EncodeToString(hashBig)
		}
	}
	
	share := &Share{
		JobID:      job.ID,
		Miner:      minerAddress,
		Nonce:      nonce,
		ExtraNonce: extraNonce2,
		Time:       ntime,
		Difficulty: shareDiff,
		Height:     job.Height,
		IsBlock:    isBlock,
		BlockHash:  blockHash,
	}
	
	return share, nil
}

func buildBlockHeader(job *Job, nonce uint32, extraNonce2 []byte, ntime int64) []byte {
	// Bitcoin block header format:
	// version (4) + prevhash (32) + merkleroot (32) + time (4) + bits (4) + nonce (4) = 80 bytes
	// PEARL adds: proof commitment + certificate
	
	header := make([]byte, 0, 80)
	
	// Version (placeholder)
	version := make([]byte, 4)
	binary.LittleEndian.PutUint32(version, 1)
	header = append(header, version...)
	
	// Previous block hash
	prevHash, _ := hex.DecodeString(job.PrevHash)
	header = append(header, reverseBytes(prevHash)...)
	
	// Merkle root (placeholder - needs proper coinbase + tx merkle tree)
	merkleRoot := make([]byte, 32)
	header = append(header, merkleRoot...)
	
	// Time
	timeBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(timeBytes, uint32(ntime))
	header = append(header, timeBytes...)
	
	// Bits
	bitsBytes, _ := hex.DecodeString(job.Bits)
	header = append(header, bitsBytes...)
	
	// Nonce
	nonceBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(nonceBytes, nonce)
	header = append(header, nonceBytes...)
	
	return header
}

func reverseBytes(b []byte) []byte {
	r := make([]byte, len(b))
	for i := 0; i < len(b); i++ {
		r[i] = b[len(b)-1-i]
	}
	return r
}

func hashToDifficulty(hash []byte) float64 {
	// Simplified difficulty calculation
	// Real implementation: diff = maxTarget / currentTarget
	if len(hash) < 32 {
		return 0
	}
	
	// Get first 8 bytes as uint64
	val := binary.BigEndian.Uint64(hash[:8])
	if val == 0 {
		return 0
	}
	
	// Difficulty is inversely proportional to hash value
	maxVal := float64(^uint64(0))
	return maxVal / float64(val)
}

func bytesToBigInt(b []byte) *big.Int {
	return new(big.Int).SetBytes(b)
}
