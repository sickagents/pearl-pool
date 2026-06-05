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
	// nonce parsed but not used directly, it's in the nonceHex string
	
	// Parse nTime
	nTime, err := strconv.ParseInt(nTimeHex, 16, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid ntime")
	}
	
	// Check nTime range (allow 2 hour drift)
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
	
	// Build complete block header with coinbase construction
	blockHeader := s.buildBlockHeader(job, conn.extraNonce1, extraNonce2Hex, nTimeHex, nonceHex)
	blockHex := hex.EncodeToString(blockHeader)
	
	// For PEARL (ZK-proof PoW), we validate via trusted node RPC
	// Option A: Submit to node for validation (chosen approach)
	if s.rpcClient != nil {
		// Use ValidateBlock RPC to check ZK proof without submitting
		valid, err := s.rpcClient.ValidateBlock(blockHex)
		if err != nil {
			// If validation RPC unavailable, fall back to hash-based check
			// This is simplified - production should enforce ZK validation
			hash := s.hashBlockHeader(blockHeader)
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
				BlockHex:    blockHex,
			}, nil
		}
		
		if !valid {
			return nil, fmt.Errorf("ZK proof validation failed")
		}
	}
	
	// Hash-based difficulty check (for pool difficulty)
	hash := s.hashBlockHeader(blockHeader)
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
		BlockHex:    blockHex,
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

// buildBlockHeader constructs a block header
func (s *Server) buildBlockHeader(job *Job, extraNonce1, extraNonce2, nTime, nonce string) []byte {
	// PEARL block header structure (similar to Bitcoin):
	// - version (4 bytes)
	// - prev_block_hash (32 bytes)
	// - merkle_root (32 bytes)
	// - timestamp (4 bytes)
	// - bits (4 bytes)
	// - nonce (4 bytes)
	// Total: 80 bytes
	
	header := make([]byte, 80)
	
	// Version (4 bytes, little-endian) - use version 1
	version := uint32(1)
	header[0] = byte(version)
	header[1] = byte(version >> 8)
	header[2] = byte(version >> 16)
	header[3] = byte(version >> 24)
	
	// Previous block hash (32 bytes)
	prevHash, _ := hex.DecodeString(job.PrevHash)
	if len(prevHash) == 32 {
		copy(header[4:36], reverseBytes(prevHash))
	}
	
	// Merkle root (32 bytes) - construct from coinbase + transactions
	merkleRoot := s.calculateMerkleRoot(extraNonce1, extraNonce2, job)
	copy(header[36:68], merkleRoot)
	
	// Timestamp (4 bytes, little-endian)
	timestamp, _ := strconv.ParseUint(nTime, 16, 32)
	header[68] = byte(timestamp)
	header[69] = byte(timestamp >> 8)
	header[70] = byte(timestamp >> 16)
	header[71] = byte(timestamp >> 24)
	
	// Bits (4 bytes, little-endian)
	bits, _ := hex.DecodeString(job.Bits)
	if len(bits) == 4 {
		copy(header[72:76], bits)
	}
	
	// Nonce (4 bytes, big-endian for PEARL)
	nonceBytes, _ := hex.DecodeString(nonce)
	if len(nonceBytes) == 4 {
		copy(header[76:80], nonceBytes)
	}
	
	return header
}

// calculateMerkleRoot constructs merkle root from coinbase + transactions
func (s *Server) calculateMerkleRoot(extraNonce1, extraNonce2 string, job *Job) []byte {
	// Build coinbase transaction with extraNonces
	coinbase := s.buildCoinbaseTx(job, extraNonce1, extraNonce2)
	
	// Hash the coinbase
	coinbaseHash := sha256.Sum256(coinbase)
	coinbaseHashDouble := sha256.Sum256(coinbaseHash[:])
	
	// If no other transactions, coinbase hash is the merkle root
	if len(job.Transactions) == 0 {
		return coinbaseHashDouble[:]
	}
	
	// Build merkle tree with coinbase + transaction hashes
	hashes := [][]byte{coinbaseHashDouble[:]}
	for _, tx := range job.Transactions {
		txHash := sha256.Sum256(tx)
		txHashDouble := sha256.Sum256(txHash[:])
		hashes = append(hashes, txHashDouble[:])
	}
	
	// Compute merkle root
	for len(hashes) > 1 {
		var newLevel [][]byte
		for i := 0; i < len(hashes); i += 2 {
			if i+1 < len(hashes) {
				combined := append(hashes[i], hashes[i+1]...)
				hash := sha256.Sum256(combined)
				hashDouble := sha256.Sum256(hash[:])
				newLevel = append(newLevel, hashDouble[:])
			} else {
				// Odd number: duplicate last hash
				combined := append(hashes[i], hashes[i]...)
				hash := sha256.Sum256(combined)
				hashDouble := sha256.Sum256(hash[:])
				newLevel = append(newLevel, hashDouble[:])
			}
		}
		hashes = newLevel
	}
	
	return hashes[0]
}

// buildCoinbaseTx constructs coinbase transaction with extraNonces
func (s *Server) buildCoinbaseTx(job *Job, extraNonce1, extraNonce2 string) []byte {
	// Simplified coinbase construction
	// Production needs proper transaction format with:
	// - Block height in scriptSig (BIP34)
	// - extraNonce1 + extraNonce2
	// - Output to pool address
	
	coinbase := []byte{}
	
	// Transaction version (4 bytes)
	coinbase = append(coinbase, 0x01, 0x00, 0x00, 0x00)
	
	// Input count (1)
	coinbase = append(coinbase, 0x01)
	
	// Input: previous output (null for coinbase)
	coinbase = append(coinbase, make([]byte, 32)...) // null hash
	coinbase = append(coinbase, 0xff, 0xff, 0xff, 0xff) // index -1
	
	// ScriptSig length + data
	scriptSig := []byte{}
	// Block height (BIP34) - 3 bytes for height up to ~16M
	height := job.Height
	scriptSig = append(scriptSig, byte(height), byte(height>>8), byte(height>>16))
	// ExtraNonce1
	en1, _ := hex.DecodeString(extraNonce1)
	scriptSig = append(scriptSig, en1...)
	// ExtraNonce2
	en2, _ := hex.DecodeString(extraNonce2)
	scriptSig = append(scriptSig, en2...)
	
	coinbase = append(coinbase, byte(len(scriptSig)))
	coinbase = append(coinbase, scriptSig...)
	
	// Sequence (4 bytes)
	coinbase = append(coinbase, 0xff, 0xff, 0xff, 0xff)
	
	// Output count (1)
	coinbase = append(coinbase, 0x01)
	
	// Output value (8 bytes, little-endian)
	value := uint64(job.CoinbaseValue)
	coinbase = append(coinbase, 
		byte(value), byte(value>>8), byte(value>>16), byte(value>>24),
		byte(value>>32), byte(value>>40), byte(value>>48), byte(value>>56))
	
	// Output script (simplified - P2PKH to pool address)
	// Length + dummy script
	dummyScript := make([]byte, 25) // Standard P2PKH length
	coinbase = append(coinbase, byte(len(dummyScript)))
	coinbase = append(coinbase, dummyScript...)
	
	// Locktime (4 bytes)
	coinbase = append(coinbase, 0x00, 0x00, 0x00, 0x00)
	
	return coinbase
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
