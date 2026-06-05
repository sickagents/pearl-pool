package rpc

import "time"

// GetBlockHash returns block hash at given height
func (c *Client) GetBlockHash(height int64) (string, error) {
	start := time.Now()
	defer func() {
		// Metrics will be added when pkg/metrics is imported
	}()
	
	var result string
	err := c.call("getblockhash", []interface{}{height}, &result)
	return result, err
}

// GetBlock returns block details
func (c *Client) GetBlock(hash string) (*BlockInfo, error) {
	start := time.Now()
	defer func() {
		// Metrics will be added when pkg/metrics is imported
	}()
	
	var result BlockInfo
	err := c.call("getblock", []interface{}{hash, true}, &result)
	return &result, err
}

type BlockInfo struct {
	Hash          string   `json:"hash"`
	Confirmations int      `json:"confirmations"`
	Height        int64    `json:"height"`
	Version       int      `json:"version"`
	VersionHex    string   `json:"versionHex"`
	MerkleRoot    string   `json:"merkleroot"`
	Time          int64    `json:"time"`
	Nonce         uint32   `json:"nonce"`
	Bits          string   `json:"bits"`
	Difficulty    float64  `json:"difficulty"`
	PreviousHash  string   `json:"previousblockhash"`
	NextHash      string   `json:"nextblockhash,omitempty"`
	Tx            []string `json:"tx"`
}

// TestBlockValidity tests if a block is valid without submitting
func (c *Client) TestBlockValidity(blockHex string) (bool, error) {
	start := time.Now()
	defer func() {
		// Metrics will be added when pkg/metrics is imported
	}()
	
	var result bool
	err := c.call("testblockvalidity", []interface{}{blockHex}, &result)
	return result, err
}
