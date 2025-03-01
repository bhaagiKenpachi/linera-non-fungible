package solver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
	"github.com/gorilla/websocket"
	"github.com/linera-protocol/examples/universal-solver/client/solver/keys"
	"github.com/mr-tron/base58"
)

// Add at the top with other package-level variables
var (
	// RPC endpoints
	EthereumRPC string
	SolanaRPC   string
	// NFT contract address
	NFTAddress string
	// Chain keys
	chainKeys *keys.ChainKeys
	// Marketplace ABI
	marketplaceABI abi.ABI
)

func init() {
	// Initialize the ABI
	var err error
	marketplaceABI, err = abi.JSON(strings.NewReader(marketplaceABIJson))
	if err != nil {
		panic(fmt.Sprintf("failed to parse marketplace ABI: %v", err))
	}
}

// Add a function to initialize RPC URLs
func InitConfig(ethereumURL, solanaURL, nftAddress string) {
	EthereumRPC = ethereumURL
	SolanaRPC = solanaURL
	NFTAddress = nftAddress
}

// InitKeys initializes the private keys from a seed phrase
func InitKeys(seedPhrase string) error {
	var err error
	chainKeys, err = keys.DeriveKeysFromSeedPhrase(seedPhrase)
	if err != nil {
		return fmt.Errorf("failed to derive keys: %w", err)
	}
	return nil
}

// Add these types for WebSocket messages
type WSMessage struct {
	Type  string      `json:"type"`
	Data  interface{} `json:"data"`
	Error string      `json:"error,omitempty"`
}

type Client struct {
	solverURL      string
	nonFungibleURL string
	lineraURL      string
	http           *http.Client

	// WebSocket related fields
	upgrader    websocket.Upgrader
	clients     map[*websocket.Conn]bool
	clientsLock sync.RWMutex
	broadcast   chan WSMessage
}

func NewClient(solverURL, nonFungibleURL, lineraURL string) *Client {
	client := &Client{
		solverURL:      solverURL,
		nonFungibleURL: nonFungibleURL,
		lineraURL:      lineraURL,
		http:           &http.Client{},

		// Initialize WebSocket fields
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins in development
			},
		},
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan WSMessage),
	}

	// Start broadcast handler
	go client.handleBroadcasts()

	return client
}

// GetSolanaTransaction fetches transaction details from Solana
func (c *Client) GetSolanaTransaction(_, txHash string) (interface{}, error) {
	// Prepare the JSON-RPC request
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "getTransaction",
		"params": []interface{}{
			txHash,
			map[string]interface{}{
				"encoding":                       "json",
				"maxSupportedTransactionVersion": 0,
			},
		},
	}

	// Make the request with retries
	var response interface{}
	var err error
	for i := 0; i < 10; i++ {
		response, err = c.makeRPCRequest(SolanaRPC, requestBody)
		if responseMap, ok := response.(map[string]interface{}); ok {
			if responseMap["result"] == nil {
				time.Sleep(5 * time.Second)
				continue // Retry if result is nil
			}
		}

		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get Solana transaction after 10 retries: %w", err)
	}

	return response, nil
}

// GetEthereumTransaction fetches transaction details from Ethereum
func (c *Client) GetEthereumTransaction(_, txHash string) (interface{}, error) {
	client, err := ethclient.Dial(EthereumRPC)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum node: %w", err)
	}
	defer client.Close()

	hash := common.HexToHash(txHash)
	tx, isPending, err := client.TransactionByHash(context.Background(), hash)
	if err != nil {
		return nil, fmt.Errorf("failed to get Ethereum transaction: %w", err)
	}

	// Convert transaction to map for consistent response format
	return map[string]interface{}{
		"hash":      tx.Hash().Hex(),
		"value":     tx.Value().String(),
		"gas":       tx.Gas(),
		"gasPrice":  tx.GasPrice().String(),
		"nonce":     tx.Nonce(),
		"isPending": isPending,
	}, nil
}

func (c *Client) makeRPCRequest(endpoint string, requestBody interface{}) (interface{}, error) {
	jsonBody, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result, nil
}

func (c *Client) GetFile(id string) (*SolverFile, error) {
	query := fmt.Sprintf(`{
		"query": "query { getFileSolverApp(id: \"%s\") { solverFileId owner name payload } }"
	}`, id)

	req, err := http.NewRequest("POST", c.solverURL, bytes.NewBuffer([]byte(query)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	var result GraphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return &result.Data.GetFileSolverApp, nil
}

func (c *Client) GetTransactionByHash(hash string) (*Transaction, error) {
	query := fmt.Sprintf(`{
		"query": "query { getTransaction(hash: \"%s\") { 
			hash
			blockHash
			blockNumber
			from
			to
			value
			gasPrice
			gas
			nonce
			input
			transactionIndex
			v
			r
			s
	 }}"
	}`, hash)

	req, err := http.NewRequest("POST", c.solverURL, bytes.NewBuffer([]byte(query)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			GetTransaction *Transaction `json:"getTransaction"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return result.Data.GetTransaction, nil
}

// CalculateSwap calculates swap details without executing the swap
func (c *Client) CalculateSwap(fromToken, toToken string, amount float64) (*SwapResult, error) {
	query := fmt.Sprintf(`{
		"query": "query { calculateSwap(fromToken:\"%s\",toToken:\"%s\",amount:%f) { fromToken toToken fromAmount toAmount exchangeRate } }"
	}`, fromToken, toToken, amount)

	req, err := http.NewRequest("POST", c.solverURL, bytes.NewBuffer([]byte(query)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error executing request: %w", err)
	}
	defer resp.Body.Close()

	// Parse response
	var result struct {
		Data struct {
			CalculateSwap struct {
				FromToken    string  `json:"fromToken"`
				ToToken      string  `json:"toToken"`
				FromAmount   float64 `json:"fromAmount"`
				ToAmount     float64 `json:"toAmount"`
				ExchangeRate float64 `json:"exchangeRate"`
			} `json:"calculateSwap"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error decoding response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	return &SwapResult{
		FromToken:    result.Data.CalculateSwap.FromToken,
		ToToken:      result.Data.CalculateSwap.ToToken,
		FromAmount:   result.Data.CalculateSwap.FromAmount,
		ToAmount:     result.Data.CalculateSwap.ToAmount,
		ExchangeRate: result.Data.CalculateSwap.ExchangeRate,
	}, nil
}

// ExecuteSwap performs the swap operation
func (c *Client) ExecuteSwap(fromToken, toToken string, amount float64, destinationAddress string) (*SwapResponse, error) {
	// First calculate the swap
	swapResult, err := c.CalculateSwap(fromToken, toToken, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate swap: %w", err)
	}

	// Execute the swap mutation
	mutation := fmt.Sprintf(`{"query":"mutation calSwap{swap(fromToken:\"%s\",toToken:\"%s\",amount:\"%v\",destinationAddress:\"%s\")}"}`, fromToken, toToken, amount, destinationAddress)

	req, err := http.NewRequest("POST", c.solverURL, bytes.NewBuffer([]byte(mutation)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	var rawResponse map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rawResponse); err != nil {
		return nil, fmt.Errorf("error parsing raw response: %w", err)
	}

	// Create properly structured result
	var result struct {
		Data   string `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	// Re-encode and decode to ensure proper type conversion
	jsonData, err := json.Marshal(rawResponse)
	if err != nil {
		return nil, fmt.Errorf("error re-encoding response: %w", err)
	}

	if err := json.Unmarshal(jsonData, &result); err != nil {
		return nil, fmt.Errorf("error parsing structured response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	swapResponse := &SwapResponse{
		TxHash:             result.Data,
		SwapResult:         *swapResult,
		Status:             "pending",
		DestinationAddress: destinationAddress,
	}

	// Prepare transaction for signing based on chain
	chain := c.determineChain(toToken)
	if err := c.PrepareTransaction(chain, swapResponse); err != nil {
		return nil, fmt.Errorf("failed to prepare transaction: %w", err)
	}

	// Sign the prepared transaction
	if err := c.SignTransaction(swapResponse); err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Submit the signed transaction
	if err := c.SubmitTransaction(swapResponse); err != nil {
		return nil, fmt.Errorf("failed to submit transaction: %w", err)
	}

	return swapResponse, nil
}

func (c *Client) determineChain(token string) string {
	switch token {
	case "ETH":
		return "ethereum"
	case "SOL":
		return "solana"
	default:
		return "unknown"
	}
}

// PrepareTransaction prepares a transaction for signing based on chain type
func (c *Client) PrepareTransaction(chain string, swap *SwapResponse) error {
	switch chain {
	case "ethereum":
		return c.prepareEthereumTransaction(swap)
	case "solana":
		return c.prepareSolanaTransaction(swap)
	default:
		return fmt.Errorf("unsupported chain: %s", chain)
	}
}

// GetAllPools fetches all pool addresses
func (c *Client) GetAllPools() ([]Pool, error) {
	query := `{"query":"query pools{getAllPools{chainName poolAddress}}"}`

	req, err := http.NewRequest("POST", c.solverURL, bytes.NewBuffer([]byte(query)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			GetAllPools []Pool `json:"getAllPools"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	// Accumulate pools from response
	var pools []Pool
	for _, pool := range result.Data.GetAllPools {
		pools = append(pools, Pool{
			ChainName:   pool.ChainName,
			PoolAddress: pool.PoolAddress,
		})
	}

	return pools, nil
}

// GetAllPoolBalances fetches all pool balances
func (c *Client) GetAllPoolBalances() ([]PoolBalance, error) {
	query := `{"query":"query balances{getAllPoolBalances{poolAddress balance}}"}`

	req, err := http.NewRequest("POST", c.solverURL, bytes.NewBuffer([]byte(query)))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			GetAllPoolBalances []PoolBalance `json:"getAllPoolBalances"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", result.Errors[0].Message)
	}

	return result.Data.GetAllPoolBalances, nil
}

// GetPool fetches pool address for a specific chain
func (c *Client) GetPool(chain string) (string, error) {
	// Reuse existing getPoolAddress method
	return c.getPoolAddress(chain)
}

// getPoolAddress gets the pool address for a given token
func (c *Client) getPoolAddress(token string) (string, error) {
	pools, err := c.GetAllPools()
	if err != nil {
		return "", fmt.Errorf("failed to get pools: %w", err)
	}

	for _, pool := range pools {
		if pool.ChainName == token {
			return pool.PoolAddress, nil
		}
	}

	return "", fmt.Errorf("pool not found for token: %s", token)
}

// Update the prepareEthereumTransaction method
func (c *Client) prepareEthereumTransaction(swap *SwapResponse) error {
	// Get pool address for the token
	fromAddress, err := c.getPoolAddress(swap.SwapResult.ToToken)
	if err != nil {
		return fmt.Errorf("failed to get source pool address: %w", err)
	}

	// Query Ethereum node for current gas price
	client, err := ethclient.Dial(EthereumRPC)
	if err != nil {
		return fmt.Errorf("failed to connect to Ethereum node: %w", err)
	}
	defer client.Close()

	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get gas price: %w", err)
	}

	// Get nonce for the from address
	nonce, err := client.PendingNonceAt(context.Background(), common.HexToAddress(fromAddress))
	if err != nil {
		return fmt.Errorf("failed to get nonce: %w", err)
	}

	// Prepare transaction parameters
	swap.TxToSign = &TransactionPrep{
		Chain: "ethereum",
		RawTx: "", // Will be filled by the signer
		ChainParams: ChainParams{
			FromAddress: fromAddress,
			ToAddress:   swap.DestinationAddress,
			Amount:      fmt.Sprintf("%f", swap.SwapResult.ToAmount),
			GasPrice:    gasPrice.String(),
			GasLimit:    21000, // Standard ETH transfer gas limit
			Nonce:       nonce,
		},
	}
	return nil
}

// Update the prepareSolanaTransaction method
func (c *Client) prepareSolanaTransaction(swap *SwapResponse) error {
	// Get pool address for the token
	fromAddress, err := c.getPoolAddress(swap.SwapResult.ToToken)
	if err != nil {
		return fmt.Errorf("failed to get source pool address: %w", err)
	}

	// Query Solana node for recent blockhash
	client := rpc.New(SolanaRPC)
	resp, err := client.GetLatestBlockhash(context.Background(), rpc.CommitmentConfirmed)
	if err != nil {
		return fmt.Errorf("failed to get recent blockhash: %w", err)
	}

	// Prepare transaction parameters
	swap.TxToSign = &TransactionPrep{
		Chain: "solana",
		RawTx: "", // Will be filled by the signer
		ChainParams: ChainParams{
			FromAddress:     fromAddress,
			ToAddress:       swap.DestinationAddress,
			Amount:          fmt.Sprintf("%f", swap.SwapResult.ToAmount),
			RecentBlockhash: resp.Value.Blockhash.String(),
			Lamports:        swap.SwapResult.ToAmount,
		},
	}
	return nil
}

// SignTransaction signs the prepared transaction based on chain type
func (c *Client) SignTransaction(swap *SwapResponse) error {
	if swap.TxToSign == nil {
		return fmt.Errorf("no transaction prepared for signing")
	}

	switch swap.TxToSign.Chain {
	case "ethereum":
		return c.signEthereumTransaction(swap)
	case "solana":
		return c.signSolanaTransaction(swap)
	default:
		return fmt.Errorf("unsupported chain for signing: %s", swap.TxToSign.Chain)
	}
}

func (c *Client) signEthereumTransaction(swap *SwapResponse) error {
	// Get derived Ethereum key instead of environment variable
	if chainKeys == nil || chainKeys.EthereumKey == nil {
		return fmt.Errorf("ethereum private key not initialized")
	}

	// Create the transaction object
	tx := types.NewTransaction(
		swap.TxToSign.ChainParams.Nonce,
		common.HexToAddress(swap.TxToSign.ChainParams.ToAddress),
		func() *big.Int {
			// Convert decimal to integer by multiplying by 10^18 (standard ETH decimals)
			amountFloat, _ := strconv.ParseFloat(swap.TxToSign.ChainParams.Amount, 64)
			amountBigFloat := new(big.Float).SetFloat64(amountFloat)
			multiplier := new(big.Float).SetFloat64(1e18)
			result := new(big.Float).Mul(amountBigFloat, multiplier)

			amountBigInt := new(big.Int)
			result.Int(amountBigInt)
			return amountBigInt
		}(),
		swap.TxToSign.ChainParams.GasLimit,
		func() *big.Int {
			gasPrice, _ := new(big.Int).SetString(swap.TxToSign.ChainParams.GasPrice, 10)
			return gasPrice
		}(),
		nil, // data
	)

	// Get the signer
	chainID := big.NewInt(1337) // mainnet, adjust as needed
	signer := types.NewEIP155Signer(chainID)

	// Sign the transaction
	signedTx, err := types.SignTx(tx, signer, chainKeys.EthereumKey)
	if err != nil {
		return fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Convert to raw bytes
	rawTxBytes, err := signedTx.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to encode signed transaction: %w", err)
	}

	// Store the raw signed transaction
	swap.TxToSign.RawTx = hexutil.Encode(rawTxBytes)
	return nil
}

func (c *Client) signSolanaTransaction(swap *SwapResponse) error {
	// Get derived Solana key instead of environment variable
	if chainKeys == nil || chainKeys.SolanaKey == nil {
		return fmt.Errorf("solana private key not initialized")
	}

	from_address, err := solana.PublicKeyFromBase58(swap.TxToSign.ChainParams.FromAddress)
	if err != nil {
		return fmt.Errorf("failed to get from address: %w", err)
	}
	to_address, err := solana.PublicKeyFromBase58(swap.TxToSign.ChainParams.ToAddress)

	if err != nil {
		return fmt.Errorf("failed to get to address: %w", err)
	}

	// Create a new transaction
	tx, err := solana.NewTransaction(
		[]solana.Instruction{
			system.NewTransferInstruction(
				uint64(swap.TxToSign.ChainParams.Lamports),
				from_address,
				to_address,
			).Build(),
		},
		solana.MustHashFromBase58(swap.TxToSign.ChainParams.RecentBlockhash),
	)

	// Sign the transaction
	_, _ = tx.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if chainKeys.SolanaKey.PublicKey().Equals(key) {
				return chainKeys.SolanaKey
			}
			return nil
		},
	)

	// Store the raw signed transaction
	rawTx, err := tx.MarshalBinary()
	if err != nil {
		return fmt.Errorf("failed to serialize signed transaction: %w", err)
	}
	swap.TxToSign.RawTx = base58.Encode(rawTx)

	return nil
}

// SubmitTransaction submits the signed transaction to the appropriate chain
func (c *Client) SubmitTransaction(swap *SwapResponse) error {
	if swap.TxToSign == nil || swap.TxToSign.RawTx == "" {
		return fmt.Errorf("no signed transaction available")
	}

	switch swap.TxToSign.Chain {
	case "ethereum":
		return c.submitEthereumTransaction(swap)
	case "solana":
		return c.submitSolanaTransaction(swap)
	default:
		return fmt.Errorf("unsupported chain for submission: %s", swap.TxToSign.Chain)
	}
}

func (c *Client) submitEthereumTransaction(swap *SwapResponse) error {
	// Connect to Ethereum node
	client, err := ethclient.Dial(EthereumRPC)
	if err != nil {
		return fmt.Errorf("failed to connect to Ethereum node: %w", err)
	}
	defer client.Close()

	// Decode raw transaction
	rawTxBytes, err := hexutil.Decode(swap.TxToSign.RawTx)
	if err != nil {
		return fmt.Errorf("failed to decode raw transaction: %w", err)
	}

	var tx types.Transaction
	if err := tx.UnmarshalBinary(rawTxBytes); err != nil {
		return fmt.Errorf("failed to unmarshal transaction: %w", err)
	}

	// Submit transaction
	if err := client.SendTransaction(context.Background(), &tx); err != nil {
		return fmt.Errorf("failed to submit transaction: %w", err)
	}

	// Update response with transaction hash
	swap.TxHash = tx.Hash().Hex()
	swap.Status = "submitted"

	return nil
}

func (c *Client) submitSolanaTransaction(swap *SwapResponse) error {
	// Create RPC request
	requestBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "sendTransaction",
		"params": []interface{}{
			swap.TxToSign.RawTx,
			map[string]interface{}{
				"encoding": "base58",
			},
		},
	}

	// Submit transaction
	response, err := c.makeRPCRequest(SolanaRPC, requestBody)
	if err != nil {
		return fmt.Errorf("failed to submit transaction: %w", err)
	}

	// Extract transaction signature
	result, ok := response.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid response format")
	}

	if errMsg, hasError := result["error"]; hasError {
		return fmt.Errorf("RPC error: %v", errMsg)
	}

	signature, ok := result["result"].(string)
	if !ok {
		return fmt.Errorf("invalid signature format in response")
	}

	// Update response with transaction signature
	swap.TxHash = signature
	swap.Status = "submitted"

	return nil
}

func (c *Client) RequestSolanaAirdrop(address string) (map[string]interface{}, error) {
	// Create RPC client
	client := rpc.New(SolanaRPC)

	// Parse address
	pubKey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return nil, fmt.Errorf("invalid Solana address: %w", err)
	}

	// Request airdrop (2 SOL)
	sig, err := client.RequestAirdrop(
		context.Background(),
		pubKey,
		2*solana.LAMPORTS_PER_SOL,
		rpc.CommitmentFinalized,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to request airdrop: %w", err)
	}

	// Wait for confirmation
	// _, err = client.GetConfirmedTransactionWithOpts(context.Background(), sig, &rpc.GetTransactionOpts{
	// 	Commitment: rpc.CommitmentConfirmed,
	// })
	// if err != nil {
	// 	return nil, fmt.Errorf("failed to confirm airdrop: %w", err)
	// }

	return map[string]interface{}{
		"signature": sig.String(),
		"amount":    "2 SOL",
		"address":   address,
	}, nil
}

func (c *Client) RequestEthereumFaucet(address string) (map[string]interface{}, error) {
	// For testnet/local network only
	if !common.IsHexAddress(address) {
		return nil, fmt.Errorf("invalid Ethereum address")
	}

	client, err := ethclient.Dial(EthereumRPC)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum node: %w", err)
	}
	defer client.Close()

	// Get the faucet's private key
	if chainKeys == nil || chainKeys.EthereumKey == nil {
		return nil, fmt.Errorf("ethereum faucet key not initialized")
	}

	// Create transaction
	nonce, err := client.PendingNonceAt(context.Background(), crypto.PubkeyToAddress(chainKeys.EthereumKey.PublicKey))
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	value := big.NewInt(1000000000000000000) // 1 ETH
	gasLimit := uint64(21000)
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price: %w", err)
	}

	tx := types.NewTransaction(
		nonce,
		common.HexToAddress(address),
		value,
		gasLimit,
		gasPrice,
		nil,
	)

	chainID, err := client.NetworkID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain id: %w", err)
	}

	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), chainKeys.EthereumKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	return map[string]interface{}{
		"txHash":  signedTx.Hash().String(),
		"amount":  "1 ETH",
		"address": address,
	}, nil
}

// GetSolanaBalance fetches SOL balance for an address
func (c *Client) GetSolanaBalance(address string) (*Balance, error) {
	// Create RPC client
	client := rpc.New(SolanaRPC)

	// Parse address
	pubKey, err := solana.PublicKeyFromBase58(address)
	if err != nil {
		return nil, fmt.Errorf("invalid Solana address: %w", err)
	}

	// Get balance
	balance, err := client.GetBalance(
		context.Background(),
		pubKey,
		rpc.CommitmentFinalized,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// Convert lamports to SOL
	solBalance := float64(balance.Value) / float64(solana.LAMPORTS_PER_SOL)

	return &Balance{
		Address: address,
		Amount:  solBalance,
		Symbol:  "SOL",
	}, nil
}

// GetEthereumBalance fetches ETH balance for an address
func (c *Client) GetEthereumBalance(address string) (*Balance, error) {
	// Validate address
	if !common.IsHexAddress(address) {
		return nil, fmt.Errorf("invalid Ethereum address")
	}

	// Connect to Ethereum node
	client, err := ethclient.Dial(EthereumRPC)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum node: %w", err)
	}
	defer client.Close()

	// Get balance
	account := common.HexToAddress(address)
	balance, err := client.BalanceAt(context.Background(), account, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get balance: %w", err)
	}

	// Convert wei to ETH
	fbalance := new(big.Float)
	fbalance.SetString(balance.String())
	ethValue := new(big.Float).Quo(fbalance, big.NewFloat(1e18))
	amount, _ := ethValue.Float64()

	return &Balance{
		Address: address,
		Amount:  amount,
		Symbol:  "ETH",
	}, nil
}

// Add these types at the top with other types
type TransferParams struct {
	SourceOwner   string `json:"sourceOwner"`
	TokenId       string `json:"tokenId"`
	TargetChainId string `json:"targetChainId"`
	TargetOwner   string `json:"targetOwner"`
	ChainOwner    string `json:"chainOwner"`
	BuyFromToken  string `json:"buyFromToken"`
	ToToken       string `json:"toToken"`
	Amount        string `json:"amount"`
	BlobHash      string `json:"blobHash"`
	NftId         string `json:"nftId"`
}

// Add this type to handle the transfer mutation response
type TransferResponse struct {
	Data   string `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// Update NFTQueryResponse type to match new structure
type NFTQueryResponse struct {
	Data struct {
		NftUsingBlobHash struct {
			Token       string `json:"token"`
			TokenId     string `json:"tokenId"`
			Price       string `json:"price"`
			ChainOwner  string `json:"chainOwner"`
			ChainMinter string `json:"chainMinter"`
			Name        string `json:"name"`
			Owner       string `json:"owner"`
			ID          int    `json:"id"`
			Minter      string `json:"minter"`
			Payload     []int  `json:"payload"`
		} `json:"nftUsingBlobHash"`
	} `json:"data"`
}

// Update GetNFTDetails function
func (c *Client) GetNFTDetails(id string) (*NFTQueryResponse, error) {
	Logger.Printf("Fetching NFT details for blobHash: %s", id)
	query := `{
		"query": "query nft{nftUsingBlobHash(id:` + id + `){token tokenId price chainOwner chainMinter name owner id minter payload}}"
	}`

	req, err := http.NewRequest("POST", c.nonFungibleURL, bytes.NewBuffer([]byte(query)))
	if err != nil {
		Logger.Printf("Error creating NFT query request: %v", err)
		return nil, fmt.Errorf("error creating NFT query request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Logger.Printf("Error executing NFT query: %v", err)
		return nil, fmt.Errorf("error executing NFT query: %w", err)
	}
	defer resp.Body.Close()

	var nftResp NFTQueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&nftResp); err != nil {
		Logger.Printf("Error parsing NFT query response: %v", err)
		return nil, fmt.Errorf("error parsing NFT query response: %w", err)
	}
	return &nftResp, nil
}

// Update ExecuteNFTContractTransaction to use the NFT ID
func (c *Client) ExecuteNFTContractTransaction(tokenId int, calSwapAmount float64, listedPrice float64) (string, error) {
	Logger.Printf("Executing NFT contract transaction for tokenId: %d", tokenId)
	// Connect to Ethereum node
	client, err := ethclient.Dial(EthereumRPC)
	if err != nil {
		Logger.Printf("Failed to connect to Ethereum node: %v", err)
		return "", fmt.Errorf("failed to connect to Ethereum node: %w", err)
	}
	defer client.Close()

	// Create contract instance
	contractAddress := common.HexToAddress(NFTAddress)
	contract := bind.NewBoundContract(contractAddress, marketplaceABI, client, client, client)
	var amount float64
	if calSwapAmount > listedPrice {
		amount = listedPrice
	}

	// Convert amount to Wei (1 ETH = 10^18 Wei)
	amountWei := new(big.Int)
	amountFloat := new(big.Float).SetFloat64(amount)
	amountFloat.Mul(amountFloat, new(big.Float).SetFloat64(1e18))
	amountFloat.Int(amountWei)

	// Create transaction
	auth, err := bind.NewKeyedTransactorWithChainID(chainKeys.EthereumKey, big.NewInt(1337))
	if err != nil {
		Logger.Printf("Failed to create auth: %v", err)
		return "", fmt.Errorf("failed to create auth: %w", err)
	}
	auth.Value = amountWei

	// Use the NFT ID from the query
	tokenIdInt, ok := new(big.Int).SetString(strconv.Itoa(tokenId), 10)
	if !ok {
		Logger.Printf("Failed to parse token ID: %d", tokenId)
		return "", fmt.Errorf("failed to parse token ID: %d", tokenId)
	}

	// Execute sale transaction
	tx, err := contract.Transact(auth, "executeSale", tokenIdInt)
	if err != nil {
		Logger.Printf("Failed to execute sale: %v", err)
		return "", fmt.Errorf("failed to execute sale: %w", err)
	}

	// Wait for transaction to be mined
	_, err = bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		Logger.Printf("Failed to wait for transaction: %v", err)
		return "", fmt.Errorf("failed to wait for transaction: %w", err)
	}

	Logger.Printf("Successfully executed NFT contract transaction: %s", tx.Hash().Hex())
	return tx.Hash().String(), nil
}

func (c *Client) ExecuteTransferMutation(params TransferParams) (*TransferResponse, string, error) {
	Logger.Printf("Executing transfer mutation with params: %+v", params)
	// First get the NFT details to get the ID
	c.broadcast <- WSMessage{
		Type: "nft_transfer_initiated",
		Data: map[string]interface{}{
			"status":  "initiated",
			"message": "fetching nft detail from nft solver",
		},
	}

	nftDetails, err := c.GetNFTDetails(params.NftId)

	if err != nil {
		Logger.Printf("Failed to get NFT details: %v", err)
		return nil, "", fmt.Errorf("failed to get NFT details: %w", err)
	}
	var hash string
	// After successful transfer mutation, if this is an ETH transfer, execute the NFT contract transaction
	if params.ToToken == "ETH" {
		hash, err = c.ExecuteNFTContractTransaction(nftDetails.Data.NftUsingBlobHash.ID, parseFloat64(params.Amount), parseFloat64(nftDetails.Data.NftUsingBlobHash.Price))
		if err != nil {
			Logger.Printf("Failed to execute NFT contract transaction after transfer: %v", err)
			return nil, "", fmt.Errorf("failed to execute NFT contract transaction after transfer: %w", err)
		}
	}

	// Preserve special chars while escaping / and +

	c.broadcast <- WSMessage{
		Type: "nft_transfer_pending",
		Data: map[string]interface{}{
			"status":  "pending",
			"message": "Dex solver and Nft solver will be coordinated for transfer",
		},
	}

	mutation := `{
    "query": "mutation transfer{transfer(sourceOwner:\"` + params.SourceOwner + `\", tokenId:\"` + params.TokenId + `\", targetAccount: { chainId:\"` + params.TargetChainId + `\", owner:\"` + params.TargetOwner + `\"}, chainOwner:\"` + params.ChainOwner + `\", buyFromToken:\"` + params.BuyFromToken + `\",toToken:\"` + params.ToToken + `\", amount:\"` + fmt.Sprintf("%v", params.Amount) + `\")}"
}`
	req, err := http.NewRequest("POST", c.nonFungibleURL, bytes.NewBuffer([]byte(mutation)))
	if err != nil {
		Logger.Printf("Error creating transfer request: %v", err)
		return nil, "", fmt.Errorf("error creating transfer request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Logger.Printf("Error executing transfer: %v", err)
		return nil, "", fmt.Errorf("error executing transfer: %w", err)
	}
	defer resp.Body.Close()

	var transferResp TransferResponse
	if err := json.NewDecoder(resp.Body).Decode(&transferResp); err != nil {
		Logger.Printf("Error parsing transfer response: %v", err)
		return nil, "", fmt.Errorf("error parsing transfer response: %w", err)
	}

	if len(transferResp.Errors) > 0 {
		Logger.Printf("Transfer error: %s", transferResp.Errors[0].Message)
		return nil, "", fmt.Errorf("transfer error: %s", transferResp.Errors[0].Message)
	}

	c.broadcast <- WSMessage{
		Type: "nft_transfer_completed",
		Data: map[string]interface{}{
			"status":  "completed",
			"message": "Successfully coordination complete between solvers and executed transfer",
		},
	}

	// Logger.Printf("Successfully executed transfer mutation: %+v", transferResp)
	return &transferResp, hash, nil
}

// Helper function to parse float64
func parseFloat64(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}

// Add the ABI JSON as a constant
const marketplaceABIJson = `[
	{
		"inputs": [
			{
				"internalType": "uint256",
				"name": "tokenId",
				"type": "uint256"
			}
		],
		"name": "executeSale",
		"outputs": [],
		"stateMutability": "payable",
		"type": "function"
	},
    {
      "inputs": [
        {
          "internalType": "uint256",
          "name": "tokenId",
          "type": "uint256"
        },
        {
          "internalType": "uint256",
          "name": "price",
          "type": "uint256"
        }
      ],
      "name": "listToken",
      "outputs": [],
      "stateMutability": "payable",
      "type": "function"
    },
    {
      "inputs": [],
      "name": "getListPrice",
      "outputs": [
        {
          "internalType": "uint256",
          "name": "",
          "type": "uint256"
        }
      ],
      "stateMutability": "view",
      "type": "function"
    },
    {
      "inputs": [],
      "name": "getCurrentToken",
      "outputs": [
        {
          "internalType": "uint256",
          "name": "",
          "type": "uint256"
        }
      ],
      "stateMutability": "view",
      "type": "function"
    }
]`

// Add function to publish data blob
func (c *Client) PublishDataBlob(chainId string, imageBytes []byte) (string, error) {
	Logger.Printf("Publishing data blob for chainId: %s", chainId)

	// Convert bytes to array of integers
	byteInts := make([]int, len(imageBytes))
	for i, b := range imageBytes {
		byteInts[i] = int(b)
	}

	mutation := fmt.Sprintf(`{
		"query": "mutation datablob{publishDataBlob(chainId:\"%s\", bytes:%v)}"
	}`, chainId, byteInts)

	req, err := http.NewRequest("POST", c.lineraURL, bytes.NewBuffer([]byte(mutation)))
	if err != nil {
		Logger.Printf("Error creating publish blob request: %v", err)
		return "", fmt.Errorf("error creating publish blob request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Logger.Printf("Error publishing blob: %v", err)
		return "", fmt.Errorf("error publishing blob: %w", err)
	}
	defer resp.Body.Close()

	var blobResp DataBlobResponse
	if err := json.NewDecoder(resp.Body).Decode(&blobResp); err != nil {
		Logger.Printf("Error parsing blob response: %v", err)
		return "", fmt.Errorf("error parsing blob response: %w", err)
	}

	if len(blobResp.Errors) > 0 {
		Logger.Printf("Blob error: %s", blobResp.Errors[0].Message)
		return "", fmt.Errorf("blob error: %s", blobResp.Errors[0].Message)
	}

	Logger.Printf("Successfully published data blob: %s", blobResp.Data.PublishDataBlob)
	return blobResp.Data.PublishDataBlob, nil
}

// Add function to mint NFT
func (c *Client) MintNFT(params ListNFTParams, blobHash string, id int, token string) error {
	Logger.Printf("Minting NFT with params: %+v, blobHash: %s", params, blobHash)

	mutation := fmt.Sprintf(`{
		"query": "mutation mint{mint(minter:\"%s\",name:\"%s\",blobHash:\"%s\",token:\"%s\",price:\"%s\",id:%d,chainMinter:\"%s\",chainOwner:\"%s\",description:\"%s\")}"
	}`, params.Minter, params.Name, blobHash, token, params.Price, id, params.ChainMinter, params.ChainOwner, params.Description)

	req, err := http.NewRequest("POST", c.nonFungibleURL, bytes.NewBuffer([]byte(mutation)))
	if err != nil {
		Logger.Printf("Error creating mint request: %v", err)
		return fmt.Errorf("error creating mint request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Logger.Printf("Error minting NFT: %v", err)
		return fmt.Errorf("error minting NFT: %w", err)
	}
	defer resp.Body.Close()

	var mintResp MintResponse
	if err := json.NewDecoder(resp.Body).Decode(&mintResp); err != nil {
		Logger.Printf("Error parsing mint response: %v", err)
		return fmt.Errorf("error parsing mint response: %w", err)
	}

	if len(mintResp.Errors) > 0 {
		Logger.Printf("Mint error: %s", mintResp.Errors[0].Message)
		return fmt.Errorf("mint error: %s", mintResp.Errors[0].Message)
	}

	Logger.Printf("Successfully minted NFT with transaction hash: %s", mintResp.Data)
	return nil
}

// Update ListNFT to return the blob hash
func (c *Client) ListNFT(params ListNFTParams) (string, error) {
	// Logger.Printf("Listing NFT with params: %+v", params)

	// First publish the image data blob
	// blobHash, err := c.PublishDataBlob(params.ChainId, params.ImageBytes)
	// if err != nil {
	// 	Logger.Printf("Failed to publish data blob: %v", err)
	// 	return "", fmt.Errorf("failed to publish data blob: %w", err)
	// }

	// Mint the NFT with the blob hash
	if err := c.MintNFT(params, params.BlobHash, params.ID, params.Token); err != nil {
		Logger.Printf("Failed to mint NFT: %v", err)
		return "", fmt.Errorf("failed to mint NFT: %w", err)
	}

	Logger.Printf("Successfully listed NFT with blob hash: %s", params.BlobHash)
	return params.BlobHash, nil
}

// Add function to get all NFTs
func (c *Client) GetAllNFTs() (map[string]NFT, error) {
	Logger.Println("Getting all NFTs")

	query := `{
		"query": "query nfts{nfts}"
	}`

	req, err := http.NewRequest("POST", c.nonFungibleURL, bytes.NewBuffer([]byte(query)))
	if err != nil {
		Logger.Printf("Error creating NFTs query request: %v", err)
		return nil, fmt.Errorf("error creating NFTs query request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		Logger.Printf("Error executing NFTs query: %v", err)
		return nil, fmt.Errorf("error executing NFTs query: %w", err)
	}
	defer resp.Body.Close()

	var nftsResp NFTsResponse
	if err := json.NewDecoder(resp.Body).Decode(&nftsResp); err != nil {
		Logger.Printf("Error parsing NFTs response: %v", err)
		return nil, fmt.Errorf("error parsing NFTs response: %w", err)
	}

	if len(nftsResp.Errors) > 0 {
		Logger.Printf("NFTs query error: %s", nftsResp.Errors[0].Message)
		return nil, fmt.Errorf("NFTs query error: %s", nftsResp.Errors[0].Message)
	}

	// Logger.Printf("Successfully retrieved NFTs: %+v", nftsResp.Data.NFTs)
	return nftsResp.Data.NFTs, nil
}

// ListNftForSale executes the listNftForSale mutation and creates an Ethereum transaction
func (c *Client) ListNftForSale(owner, chainId, tokenId, price, nftId, chainOwner string) (interface{}, error) {
	Logger.Printf("Executing ListNftForSale for owner: %s, chainId: %s, tokenId: %s, price: %s",
		owner, chainId, tokenId, price)

	// Execute Ethereum transaction to list the token
	txHash, err := c.ListToken(nftId, price)
	if err != nil {
		Logger.Printf("Error listing token on Ethereum: %v", err)
		return nil, fmt.Errorf("error listing token on Ethereum: %w", err)
	}

	ethAddress := crypto.PubkeyToAddress(chainKeys.EthereumKey.PublicKey).Hex()
	Logger.Printf("Ethereum public address: %s", ethAddress)

	// First, execute the mutation to list the NFT for sale on Linera
	mutation := fmt.Sprintf(`{
		"query": "mutation listNftForSale{listNftForSale(tokenId:\"%s\", chainOwner:\"%s\")}"
	}`, tokenId, chainOwner)

	req, err := http.NewRequest("POST", c.nonFungibleURL, bytes.NewBuffer([]byte(mutation)))
	if err != nil {
		Logger.Printf("Error creating request: %v", err)
		return nil, fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		Logger.Printf("Error executing mutation: %v", err)
		return nil, fmt.Errorf("error executing mutation: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data   interface{} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		Logger.Printf("Error parsing response: %v", err)
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	if len(result.Errors) > 0 {
		Logger.Printf("Mutation error: %s", result.Errors[0].Message)
		return nil, fmt.Errorf("mutation error: %s", result.Errors[0].Message)
	}

	// Return combined response
	response := map[string]interface{}{
		"lineraData": result.Data,
		"ethereumTx": txHash,
	}

	Logger.Printf("Successfully listed NFT for sale: %s", tokenId)

	// Broadcast the new listing
	c.broadcast <- WSMessage{
		Type: "nft_listed",
		Data: map[string]interface{}{
			"tokenId": tokenId,
			"price":   price,
			"owner":   owner,
		},
	}

	return response, nil
}

// ListToken creates an Ethereum transaction to list an NFT for sale
func (c *Client) ListToken(tokenId string, price string) (string, error) {
	Logger.Printf("Creating transaction to list NFT tokenId: %s for price: %s", tokenId, price)

	// Use the NFT ID from the query
	tokenIdInt, ok := new(big.Int).SetString(tokenId, 10)
	if !ok {
		return "", fmt.Errorf("failed to parse token ID: %s", tokenId)
	}

	// Convert amount to Wei (1 ETH = 10^18 Wei)
	amountWei := new(big.Int)
	amountFloat := new(big.Float).SetFloat64(parseFloat64(price))
	amountFloat.Mul(amountFloat, new(big.Float).SetFloat64(1e18))
	amountFloat.Int(amountWei)

	// Connect to Ethereum node
	client, err := ethclient.Dial(EthereumRPC)
	if err != nil {
		Logger.Printf("Failed to connect to Ethereum node: %v", err)
		return "", fmt.Errorf("failed to connect to Ethereum node: %w", err)
	}
	defer client.Close()

	// Create contract instance
	contractAddress := common.HexToAddress(NFTAddress)
	contract := bind.NewBoundContract(contractAddress, marketplaceABI, client, client, client)

	// Get the listing price from the contract
	var listPrice *big.Int
	var result []interface{}
	err = contract.Call(&bind.CallOpts{}, &result, "getListPrice")
	if err != nil {
		Logger.Printf("Failed to get listing price: %v", err)
		return "", fmt.Errorf("failed to get listing price: %w", err)
	}
	if len(result) > 0 {
		listPrice = result[0].(*big.Int)
	}
	// Create transaction
	auth, err := bind.NewKeyedTransactorWithChainID(chainKeys.EthereumKey, big.NewInt(1337))
	if err != nil {
		Logger.Printf("Failed to create auth: %v", err)
		return "", fmt.Errorf("failed to create auth: %w", err)
	}
	auth.Value = listPrice

	// Execute list token transaction
	tx, err := contract.Transact(auth, "listToken", tokenIdInt, amountWei)
	if err != nil {
		Logger.Printf("Failed to execute list token transaction: %v", err)
		return "", fmt.Errorf("failed to execute list token transaction: %w", err)
	}

	// Wait for transaction to be mined
	_, err = bind.WaitMined(context.Background(), client, tx)
	if err != nil {
		Logger.Printf("Failed to wait for transaction: %v", err)
		return "", fmt.Errorf("failed to wait for transaction: %w", err)
	}

	Logger.Printf("Successfully created list token transaction: %s", tx.Hash().Hex())
	return tx.Hash().Hex(), nil
}

// GetCurrentTokenID fetches the current token ID from the NFT contract
func (c *Client) GetCurrentTokenID() (uint64, error) {
	Logger.Printf("Fetching current token ID from contract")

	// Connect to Ethereum node
	client, err := ethclient.Dial(EthereumRPC)
	if err != nil {
		Logger.Printf("Failed to connect to Ethereum node: %v", err)
		return 0, fmt.Errorf("failed to connect to Ethereum node: %w", err)
	}
	defer client.Close()

	// Create contract instance
	contractAddress := common.HexToAddress(NFTAddress)
	contract := bind.NewBoundContract(contractAddress, marketplaceABI, client, client, client)

	// Call getCurrentToken function
	var result []interface{}
	err = contract.Call(&bind.CallOpts{}, &result, "getCurrentToken")
	if err != nil {
		Logger.Printf("Failed to get current token ID: %v", err)
		return 0, fmt.Errorf("failed to get current token ID: %w", err)
	}

	// Extract the token ID from the result
	if len(result) > 0 {
		tokenID := result[0].(*big.Int)
		return tokenID.Uint64(), nil
	}

	return 0, fmt.Errorf("no result returned from getCurrentToken")
}

// Add these WebSocket related methods
func (c *Client) handleBroadcasts() {
	for msg := range c.broadcast {
		c.clientsLock.RLock()
		for client := range c.clients {
			err := client.WriteJSON(msg)
			if err != nil {
				Logger.Printf("Error broadcasting to client: %v", err)
				client.Close()
				delete(c.clients, client)
			}
		}
		c.clientsLock.RUnlock()
	}
}

func (c *Client) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	// Upgrade HTTP connection to WebSocket
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		Logger.Printf("Error upgrading to WebSocket: %v", err)
		return
	}

	// Register new client
	c.clientsLock.Lock()
	c.clients[conn] = true
	c.clientsLock.Unlock()

	// Clean up on disconnect
	defer func() {
		c.clientsLock.Lock()
		delete(c.clients, conn)
		c.clientsLock.Unlock()
		conn.Close()
	}()

	// Send initial connection message
	conn.WriteJSON(WSMessage{
		Type: "connected",
		Data: "Successfully connected to WebSocket",
	})

	// Handle incoming messages
	for {
		var msg WSMessage
		err := conn.ReadJSON(&msg)
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				Logger.Printf("WebSocket error: %v", err)
			}
			break
		}

		// Handle different message types
		switch msg.Type {
		case "ping":
			conn.WriteJSON(WSMessage{
				Type: "pong",
				Data: "pong",
			})
		default:
			conn.WriteJSON(WSMessage{
				Type:  "error",
				Error: "Unknown message type",
			})
		}
	}
}
