package rpc

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a JSON-RPC client for PEARL node
type Client struct {
	url      string
	user     string
	pass     string
	client   *http.Client
	timeout  time.Duration
}

// NewClient creates a new RPC client
func NewClient(host string, port int, user, pass string, useTLS bool, timeout time.Duration) *Client {
	scheme := "http"
	if useTLS {
		scheme = "https"
	}
	
	url := fmt.Sprintf("%s://%s:%d", scheme, host, port)
	
	// Skip TLS verification for self-signed certs (production should verify)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: useTLS},
	}
	
	return &Client{
		url:     url,
		user:    user,
		pass:    pass,
		timeout: timeout,
		client: &http.Client{
			Transport: transport,
			Timeout:   timeout,
		},
	}
}

// Request represents a JSON-RPC request
type Request struct {
	Jsonrpc string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

// Response represents a JSON-RPC response
type Response struct {
	Result json.RawMessage `json:"result"`
	Error  *RPCError       `json:"error"`
	ID     int             `json:"id"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("RPC error %d: %s", e.Code, e.Message)
}

// Call makes a JSON-RPC call
func (c *Client) Call(method string, params ...interface{}) (json.RawMessage, error) {
	req := Request{
		Jsonrpc: "1.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}
	
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	
	httpReq, err := http.NewRequest("POST", c.url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	
	httpReq.SetBasicAuth(c.user, c.pass)
	httpReq.Header.Set("Content-Type", "application/json")
	
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(bodyBytes))
	}
	
	var rpcResp Response
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	
	if rpcResp.Error != nil {
		return nil, rpcResp.Error
	}
	
	return rpcResp.Result, nil
}

// BlockTemplate represents getblocktemplate response
type BlockTemplate struct {
	Version              int32                  `json:"version"`
	PreviousBlockHash    string                 `json:"previousblockhash"`
	Transactions         []Transaction          `json:"transactions"`
	CoinbaseAux          map[string]string      `json:"coinbaseaux"`
	CoinbaseValue        int64                  `json:"coinbasevalue"`
	Target               string                 `json:"target"`
	MinTime              int64                  `json:"mintime"`
	Mutable              []string               `json:"mutable"`
	NonceRange           string                 `json:"noncerange"`
	SigOpLimit           int64                  `json:"sigoplimit"`
	SizeLimit            int64                  `json:"sizelimit"`
	WeightLimit          int64                  `json:"weightlimit"`
	CurTime              int64                  `json:"curtime"`
	Bits                 string                 `json:"bits"`
	Height               int64                  `json:"height"`
	DefaultWitnessCommitment string             `json:"default_witness_commitment,omitempty"`
}

// Transaction represents a transaction in block template
type Transaction struct {
	Data    string `json:"data"`
	TxID    string `json:"txid"`
	Hash    string `json:"hash"`
	Depends []int  `json:"depends"`
	Fee     int64  `json:"fee"`
	SigOps  int64  `json:"sigops"`
	Weight  int64  `json:"weight"`
}

// GetBlockTemplate fetches a new block template
func (c *Client) GetBlockTemplate() (*BlockTemplate, error) {
	result, err := c.Call("getblocktemplate", map[string]interface{}{
		"rules": []string{"segwit"},
	})
	if err != nil {
		return nil, err
	}
	
	var template BlockTemplate
	if err := json.Unmarshal(result, &template); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block template: %w", err)
	}
	
	return &template, nil
}

// SubmitBlock submits a block to the network
func (c *Client) SubmitBlock(blockHex string) error {
	result, err := c.Call("submitblock", blockHex)
	if err != nil {
		return err
	}
	
	// submitblock returns null on success, or rejection reason as string
	var rejection *string
	if err := json.Unmarshal(result, &rejection); err != nil {
		return fmt.Errorf("failed to unmarshal submitblock response: %w", err)
	}
	
	if rejection != nil {
		return fmt.Errorf("block rejected: %s", *rejection)
	}
	
	return nil
}

// ValidateBlock validates a block without submitting (custom RPC method or testblockvalidity)
func (c *Client) ValidateBlock(blockHex string) (bool, error) {
	// Try testblockvalidity first (Bitcoin Core compatible)
	result, err := c.Call("testblockvalidity", blockHex)
	if err != nil {
		// Fallback: try submitblock with check-only flag (not standard, may not exist)
		return false, fmt.Errorf("block validation RPC not available: %w", err)
	}
	
	var valid bool
	if err := json.Unmarshal(result, &valid); err != nil {
		return false, fmt.Errorf("failed to unmarshal validation response: %w", err)
	}
	
	return valid, nil
}

// GetBlockCount returns the current block height
func (c *Client) GetBlockCount() (int64, error) {
	result, err := c.Call("getblockcount")
	if err != nil {
		return 0, err
	}
	
	var height int64
	if err := json.Unmarshal(result, &height); err != nil {
		return 0, fmt.Errorf("failed to unmarshal block count: %w", err)
	}
	
	return height, nil
}

// GetBlockHash returns the block hash at given height
func (c *Client) GetBlockHash(height int64) (string, error) {
	result, err := c.Call("getblockhash", height)
	if err != nil {
		return "", err
	}
	
	var hash string
	if err := json.Unmarshal(result, &hash); err != nil {
		return "", fmt.Errorf("failed to unmarshal block hash: %w", err)
	}
	
	return hash, nil
}

// BlockInfo represents getblock response
type BlockInfo struct {
	Hash              string   `json:"hash"`
	Confirmations     int64    `json:"confirmations"`
	Size              int64    `json:"size"`
	Height            int64    `json:"height"`
	Version           int32    `json:"version"`
	MerkleRoot        string   `json:"merkleroot"`
	Tx                []string `json:"tx"`
	Time              int64    `json:"time"`
	Nonce             uint32   `json:"nonce"`
	Bits              string   `json:"bits"`
	Difficulty        float64  `json:"difficulty"`
	PreviousBlockHash string   `json:"previousblockhash"`
	NextBlockHash     string   `json:"nextblockhash,omitempty"`
}

// GetBlock returns block info by hash
func (c *Client) GetBlock(hash string, verbosity int) (*BlockInfo, error) {
	result, err := c.Call("getblock", hash, verbosity)
	if err != nil {
		return nil, err
	}
	
	var block BlockInfo
	if err := json.Unmarshal(result, &block); err != nil {
		return nil, fmt.Errorf("failed to unmarshal block info: %w", err)
	}
	
	return &block, nil
}

// SendMany sends to multiple addresses (for payouts)
func (c *Client) SendMany(fromAccount string, amounts map[string]float64) (string, error) {
	result, err := c.Call("sendmany", fromAccount, amounts)
	if err != nil {
		return "", err
	}
	
	var txid string
	if err := json.Unmarshal(result, &txid); err != nil {
		return "", fmt.Errorf("failed to unmarshal txid: %w", err)
	}
	
	return txid, nil
}
