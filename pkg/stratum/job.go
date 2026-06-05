package stratum

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/big"
	"sync"
	"time"
)

// Job represents a mining job
type Job struct {
	ID                string
	PrevHash          string
	CoinbaseValue     int64
	Height            int64
	Bits              string
	CurTime           int64
	CleanJobs         bool
	Transactions      [][]byte
	CreatedAt         time.Time
	Target            *big.Int
	MerkleRoot        string
	CoinbaseTx        []byte
}

// JobManager manages mining jobs
type JobManager struct {
	mu           sync.RWMutex
	currentJob   *Job
	jobs         map[string]*Job // jobID -> Job
	maxJobs      int
	jobIDCounter uint64
}

// NewJobManager creates a new job manager
func NewJobManager(maxJobs int) *JobManager {
	return &JobManager{
		jobs:    make(map[string]*Job),
		maxJobs: maxJobs,
	}
}

// NewJob creates a new job from block template
func (jm *JobManager) NewJob(template map[string]interface{}) (*Job, error) {
	jm.mu.Lock()
	defer jm.mu.Unlock()
	
	jm.jobIDCounter++
	jobID := fmt.Sprintf("%08x", jm.jobIDCounter)
	
	// Parse template
	prevHash, ok := template["previousblockhash"].(string)
	if !ok {
		return nil, fmt.Errorf("missing previousblockhash")
	}
	
	coinbaseValue, ok := template["coinbasevalue"].(float64)
	if !ok {
		return nil, fmt.Errorf("missing coinbasevalue")
	}
	
	height, ok := template["height"].(float64)
	if !ok {
		return nil, fmt.Errorf("missing height")
	}
	
	bits, ok := template["bits"].(string)
	if !ok {
		return nil, fmt.Errorf("missing bits")
	}
	
	curTime, ok := template["curtime"].(float64)
	if !ok {
		return nil, fmt.Errorf("missing curtime")
	}
	
	// Parse target from bits
	target := compactToBig(bits)
	
	job := &Job{
		ID:            jobID,
		PrevHash:      prevHash,
		CoinbaseValue: int64(coinbaseValue),
		Height:        int64(height),
		Bits:          bits,
		CurTime:       int64(curTime),
		CleanJobs:     true,
		CreatedAt:     time.Now(),
		Target:        target,
	}
	
	// Parse transactions
	if txs, ok := template["transactions"].([]interface{}); ok {
		for _, tx := range txs {
			txMap, ok := tx.(map[string]interface{})
			if !ok {
				continue
			}
			data, ok := txMap["data"].(string)
			if !ok {
				continue
			}
			txBytes, err := hex.DecodeString(data)
			if err != nil {
				continue
			}
			job.Transactions = append(job.Transactions, txBytes)
		}
	}
	
	// Store job
	jm.jobs[jobID] = job
	jm.currentJob = job
	
	// Cleanup old jobs
	if len(jm.jobs) > jm.maxJobs {
		jm.cleanupOldJobs()
	}
	
	return job, nil
}

// GetJob returns a job by ID
func (jm *JobManager) GetJob(jobID string) (*Job, bool) {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	
	job, ok := jm.jobs[jobID]
	return job, ok
}

// GetCurrentJob returns the current job
func (jm *JobManager) GetCurrentJob() *Job {
	jm.mu.RLock()
	defer jm.mu.RUnlock()
	
	return jm.currentJob
}

func (jm *JobManager) cleanupOldJobs() {
	// Keep only the most recent maxJobs
	if len(jm.jobs) <= jm.maxJobs {
		return
	}
	
	// Find oldest jobs
	type jobAge struct {
		id  string
		age time.Time
	}
	
	var ages []jobAge
	for id, job := range jm.jobs {
		ages = append(ages, jobAge{id: id, age: job.CreatedAt})
	}
	
	// Sort by age (oldest first) - simple bubble sort for small N
	for i := 0; i < len(ages)-1; i++ {
		for j := i + 1; j < len(ages); j++ {
			if ages[i].age.After(ages[j].age) {
				ages[i], ages[j] = ages[j], ages[i]
			}
		}
	}
	
	// Remove oldest
	toRemove := len(jm.jobs) - jm.maxJobs
	for i := 0; i < toRemove; i++ {
		delete(jm.jobs, ages[i].id)
	}
}

// compactToBig converts compact difficulty bits to target
func compactToBig(bits string) *big.Int {
	// Parse hex string to uint32
	bitsBytes, err := hex.DecodeString(bits)
	if err != nil {
		return big.NewInt(0)
	}
	
	if len(bitsBytes) != 4 {
		return big.NewInt(0)
	}
	
	compact := uint32(bitsBytes[0])<<24 | uint32(bitsBytes[1])<<16 | uint32(bitsBytes[2])<<8 | uint32(bitsBytes[3])
	
	// Extract exponent and mantissa
	exponent := compact >> 24
	mantissa := compact & 0x00ffffff
	
	// Calculate target = mantissa * 256^(exponent - 3)
	target := big.NewInt(int64(mantissa))
	if exponent > 3 {
		target.Lsh(target, uint(8*(exponent-3)))
	} else if exponent < 3 {
		target.Rsh(target, uint(8*(3-exponent)))
	}
	
	return target
}

// Share represents a submitted share
type Share struct {
	JobID      string
	Miner      string
	Worker     string
	Nonce      uint32
	ExtraNonce string
	Time       int64
	Difficulty float64
	Height     int64
	IsBlock    bool
	BlockHash  string
}

// StratumMessage represents a Stratum protocol message
type StratumMessage struct {
	ID     interface{}   `json:"id"`
	Method string        `json:"method,omitempty"`
	Params []interface{} `json:"params,omitempty"`
	Result interface{}   `json:"result,omitempty"`
	Error  interface{}   `json:"error,omitempty"`
}

// MarshalJSON implements json.Marshaler
func (m *StratumMessage) MarshalJSON() ([]byte, error) {
	if m.Method != "" {
		// Request/notification
		return json.Marshal(struct {
			ID     interface{}   `json:"id"`
			Method string        `json:"method"`
			Params []interface{} `json:"params"`
		}{
			ID:     m.ID,
			Method: m.Method,
			Params: m.Params,
		})
	}
	
	// Response
	return json.Marshal(struct {
		ID     interface{} `json:"id"`
		Result interface{} `json:"result"`
		Error  interface{} `json:"error"`
	}{
		ID:     m.ID,
		Result: m.Result,
		Error:  m.Error,
	})
}

// BuildMiningNotify creates a mining.notify message
func BuildMiningNotify(job *Job, extraNonce1 string) *StratumMessage {
	return &StratumMessage{
		ID:     nil, // Notification has null ID
		Method: "mining.notify",
		Params: []interface{}{
			job.ID,
			job.PrevHash,
			"", // coinbase1 (simplified, needs proper construction)
			"", // coinbase2
			[]string{}, // merkle branches
			fmt.Sprintf("%08x", job.CurTime),
			job.Bits,
			true, // clean_jobs
		},
	}
}

// BuildSetDifficulty creates a mining.set_difficulty message
func BuildSetDifficulty(difficulty float64) *StratumMessage {
	return &StratumMessage{
		ID:     nil,
		Method: "mining.set_difficulty",
		Params: []interface{}{difficulty},
	}
}

// ParseAddress extracts wallet address from username (supports solo: prefix)
func ParseAddress(username string) (address string, isSolo bool) {
	if len(username) > 5 && username[:5] == "solo:" {
		return username[5:], true
	}
	return username, false
}

// ValidateAddress performs basic validation on PEARL address
func ValidateAddress(address string) bool {
	// PEARL uses Bech32 with HRP "prl"
	if len(address) < 10 || address[:3] != "prl" {
		return false
	}
	// TODO: Full Bech32 validation
	return len(address) >= 42 && len(address) <= 90
}
