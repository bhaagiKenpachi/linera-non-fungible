# NFT Marketplace API

## Running the Client

The client requires a seed phrase to derive chain-specific keys for transaction signing. You can run the client with:

```bash
go run main.go -seed-phrase "your twelve word seed phrase here"
```

Additional optional flags:
- `-solver-url`: Universal Solver service URL (default: http://localhost:8080/)
- `-non-fungible-url`: Non-Fungible service URL (default: http://localhost:8081/)
- `-linera-url`: Linera service URL (default: http://localhost:8080/)
- `-solana-url`: Solana RPC endpoint (default: http://localhost:8899)
- `-ethereum-url`: Ethereum RPC endpoint (default: http://localhost:8545)

Example:
```bash
go run main.go \
  -seed-phrase "your twelve word seed phrase here" \
  -solver-url "http://custom-solver:8080" \
  -non-fungible-url "http://custom-non-fungible:8081" \
  -linera-url "http://custom-linera:8080" \
  -solana-url "http://custom-solana:8899" \
  -ethereum-url "http://custom-ethereum:8545"
```

**Important**: Keep your seed phrase secure and never share it. The seed phrase is used to derive private keys for both Ethereum and Solana chains.

## Configuration

The client can be configured using command line flags or environment variables:

| Flag | Environment Variable | Default | Description |
|------|---------------------|---------|-------------|
| `--solver-url` | `SOLVER_URL` | `http://localhost:8080/` | Universal Solver service URL |
| `--non-fungible-url` | `NON_FUNGIBLE_URL` | `http://localhost:8081/` | Non-Fungible service URL |
| `--linera-url` | `LINERA_URL` | `http://localhost:8080/` | Linera service URL |
| `--solana-url` | `SOLANA_RPC` | `http://localhost:8899` | Solana RPC endpoint |
| `--ethereum-url` | `ETHEREUM_RPC` | `http://localhost:8545` | Ethereum RPC endpoint |
| `--nft-address` | `NFT_ADDRESS` | `` | NFT contract address |
| `--seed-phrase` | - | - | Seed phrase for deriving chain keys (required) |

Example:
```bash
./client \
  --solver-url="http://localhost:8080/" \
  --non-fungible-url="http://localhost:8081/" \
  --linera-url="http://localhost:8080/" \
  --solana-url="http://localhost:8899" \
  --ethereum-url="http://localhost:8545" \
  --nft-address="0x1234..." \
  --seed-phrase="your seed phrase here"
```

Or using environment variables:
```bash
export SOLVER_URL="http://localhost:8080/"
export NON_FUNGIBLE_URL="http://localhost:8081/"
export LINERA_URL="http://localhost:8080/"
export SOLANA_RPC="http://localhost:8899"
export ETHEREUM_RPC="http://localhost:8545"
export NFT_ADDRESS="0x1234..."
./client --seed-phrase="your seed phrase here"
```

# NFT Marketplace API

## Endpoints

### POST /list_nft_for_sale

This endpoint allows you to list an NFT for sale on the marketplace.

#### Request Body

The request body should be a JSON object with the following structure:

```json
{
  "owner": "User:ee4d2113a5100d58758e02b7928eb71896d232cd9b6bc56a7d42a51b70a9c872",
  "chainId": "e476187f6ddfeb9d588c7b45d3df334d5501d6499b3f9ad5595cae86cce16a65",
  "tokenId": "7R8BTQ2xOO2/dkxQdAOPEdVEnuesUyekaj1aoF/Y88c",
  "price": "1000000000000000000"
}
```

- `owner`: The owner of the NFT in the format `User:<user_id>`.
- `chainId`: The ID of the blockchain where the NFT is located.
- `tokenId`: The ID of the NFT you want to list for sale.
- `price`: The price at which the NFT is to be listed, specified in wei (1 ETH = 10^18 wei).

#### Response

On success, the response will be a JSON object with the following structure:

```json
{
  "status": "success",
  "data": {
    "lineraData": {
      // Response data from the Linera mutation
    },
    "ethereumTx": "0x1234567890abcdef..."
  }
}
```

In case of an error, the response will contain an error message:

```json
{
  "status": "error",
  "message": "Error message here"
}
```

### Example Request

Here’s an example of how to call the `/list_nft_for_sale` endpoint using `curl`:

```bash
curl -X POST http://localhost:3000/list_nft_for_sale \
-H "Content-Type: application/json" \
-d '{
  "owner": "User:ee4d2113a5100d58758e02b7928eb71896d232cd9b6bc56a7d42a51b70a9c872",
  "chainId": "e476187f6ddfeb9d588c7b45d3df334d5501d6499b3f9ad5595cae86cce16a65",
  "tokenId": "7R8BTQ2xOO2/dkxQdAOPEdVEnuesUyekaj1aoF/Y88c",
  "price": "1000000000000000000"
}'
```

### Notes

- Ensure that the `owner`, `chainId`, `tokenId`, and `price` values are correctly set according to your NFT's details.
- The server must be running and accessible at the specified URL.
- The `price` should be specified in wei (1 ETH = 10^18 wei).

### Other Endpoints

#### POST /faucet
Request tokens from the faucet for testing purposes.

Parameters:
- `chain`: Chain to request tokens from (`solana` or `ethereum`)
- `address`: Recipient address
- `amount` (optional): Amount of tokens to request (e.g., "1.5" for 1.5 SOL/ETH)

Example:
```bash
# Request specific amount of Solana tokens
curl -X POST "http://localhost:3001/faucet?chain=solana&address=YOUR_SOLANA_ADDRESS&amount=1.5"

# Request specific amount of Ethereum tokens
curl -X POST "http://localhost:3001/faucet?chain=ethereum&address=YOUR_ETH_ADDRESS&amount=2.0"

# Request default amount
curl -X POST "http://localhost:3001/faucet?chain=solana&address=YOUR_SOLANA_ADDRESS"
```

Response:
```json
{
    "status": "success",
    "chain": "solana",
    "data": {
        "signature": "5UYoBkwP4UUxLm6LuYUZfsi2PJww2GXwVNhXBKCRLGUqQYN7MBHXBtxEgzqxH2Nf7FnQYYP2GNP3sABr82dhUv1D",
        "amount": "1.5 SOL",
        "address": "YOUR_SOLANA_ADDRESS"
    }
}
```

### GET /get_pool_address
Get the pool address for a specific chain.

Parameters:
- `chain`: Chain name (e.g., `ethereum` or `solana`)

Example:
```bash
curl "http://localhost:3000/get_pool_address?chain=ethereum"
```

Response:
```json
{
    "status": "success",
    "chain": "ethereum",
    "data": {
        "address": "0x1234567890abcdef1234567890abcdef12345678"
    }
}
```

### GET /fetch_balance
Get the balance for an address on a specific chain.

Parameters:
- `chain`: Chain name (`solana` or `ethereum`)
- `address`: Address to check balance for

Example:
```bash
# Get Solana balance
curl "http://localhost:3000/fetch_balance?chain=solana&address=YOUR_SOLANA_ADDRESS"

# Get Ethereum balance
curl "http://localhost:3000/fetch_balance?chain=ethereum&address=YOUR_ETH_ADDRESS"
```

Response:
```json
{
    "status": "success",
    "chain": "ethereum",
    "data": {
        "address": "0x742d35Cc6634C0532925a3b844Bc454e4438f44e",
        "amount": 1.5,
        "symbol": "ETH"
    }
}
```

### GET /quote_swap
Get a quote for swapping tokens between chains.

Parameters:
- `fromChain`: Source chain (e.g., `ethereum` or `solana`)
- `toChain`: Destination chain
- `fromAmount`: Amount to swap in source chain's native units

Example:
```bash
curl "http://localhost:3000/quote_swap?fromChain=ethereum&toChain=solana&fromAmount=1.5"
```

Response:
```json
{
    "status": "success",
    "data": {
        "fromChain": "ethereum",
        "toChain": "solana",
        "fromAmount": 1.5,
        "toAmount": 210.75,
        "exchangeRate": 140.5
    }
}
```

### POST /post_tx_hash

Processes a transaction hash and optionally executes a cross-chain NFT transfer.

#### Query Parameters

| Parameter | Required | Description |
|-----------|----------|-------------|
| `txHash` | Yes | The transaction hash to process |
| `chain` | Yes | The source chain (`solana` or `ethereum`) |
| `toToken` | No | The destination token (for transfers) |
| `destinationAddress` | No | The destination address (for transfers) |
| `sourceOwner` | No | The source owner of the NFT |
| `tokenId` | No | The NFT token ID |
| `targetChainId` | No | The target chain ID |
| `targetOwner` | No | The target owner address |

#### Example Request

```bash
curl -X POST 'http://localhost:3000/post_tx_hash?txHash=0x123...&chain=ethereum&toToken=SOL&destinationAddress=0xabc...&sourceOwner=User:123...&tokenId=xyz...&targetChainId=456...&targetOwner=User:789...' 
```

### POST /list_nft
Lists a new NFT by publishing the image data and minting the NFT.

**Request Body:**
```json
{
    "name": "nft1",
    "description": "Test NFT",
    "price": "0.005",
    "imageBytes": [1, 2, 3, 4],
    "chainId": "e476187f6ddfeb9d588c7b45d3df334d5501d6499b3f9ad5595cae86cce16a65",
    "minter": "User:d6d46f50bb21f6885bdae9884c004f77f49a9f9038bc9c12b258ed2abc6b0381",
    "chainMinter": "0x8AE98D5e0A732Ead1a5d38f5766CBE84382cD01D",
    "chainOwner": "0x0577e1E35C4f30cA8379269B7Fd85cBCE7F084f4",
    "id": 6,
    "token": "ETH"
}
```

**Response:**
```json
{
    "status": "success",
    "message": "NFT listed successfully",
    "blobHash": "8To2zwly72nvTMFagZ/rBcumhpx/hzfYHJmMZNVBAR4"
}
```

**Example Usage:**
```bash
curl -X POST http://localhost:3000/list_nft \
  -H "Content-Type: application/json" \
  -d '{
    "name": "nft1",
    "description": "Test NFT",
    "price": "0.005",
    "imageBytes": [1, 2, 3, 4],
    "chainId": "e476187f6ddfeb9d588c7b45d3df334d5501d6499b3f9ad5595cae86cce16a65",
    "minter": "User:d6d46f50bb21f6885bdae9884c004f77f49a9f9038bc9c12b258ed2abc6b0381",
    "chainMinter": "0x8AE98D5e0A732Ead1a5d38f5766CBE84382cD01D",
    "chainOwner": "0x0577e1E35C4f30cA8379269B7Fd85cBCE7F084f4",
    "id": 6,
    "token": "ETH"
}'
```

**Process:**
1. Publishes image data as a blob to Linera
2. Uses the blob hash to mint a new NFT with the specified parameters
3. Returns success once the NFT is minted

**Error Responses:**
- `400 Bad Request`: Invalid request body or parameters
- `500 Internal Server Error`: Error publishing data blob or minting NFT

### GET /nfts
Get all NFTs in the system.

**Response:**
```json
{
    "status": "success",
    "data": {
        "8To2zwly72nvTMFagZ/rBcumhpx/hzfYHJmMZNVBAR4": {
            "tokenId": "8To2zwly72nvTMFagZ/rBcumhpx/hzfYHJmMZNVBAR4",
            "owner": "User:d6d46f50bb21f6885bdae9884c004f77f49a9f9038bc9c12b258ed2abc6b0381",
            "name": "nft1",
            "minter": "User:d6d46f50bb21f6885bdae9884c004f77f49a9f9038bc9c12b258ed2abc6b0381",
            "payload": [1, 2, 3, 4],
            "token": "ETH",
            "price": "0.005",
            "id": 8,
            "chainMinter": "0x0577e1E35C4f30cA8379269B7Fd85cBCE7F084f4",
            "chainOwner": "0x8AE98D5e0A732Ead1a5d38f5766CBE84382cD01D"
        }
        // ... more NFTs
    }
}
```

**Example Usage:**
```bash
curl http://localhost:3000/nfts
```

**Error Responses:**
- `405 Method Not Allowed`: If not using GET method
- `500 Internal Server Error`: Error fetching NFTs 


# NFT Marketplace API

## Endpoints

### GET /next_nft_id

This endpoint returns the next available NFT ID that can be used when minting a new NFT.

#### Request

No parameters are required.

#### Response

On success, the response will be a JSON object with the following structure:

```json
{
  "status": "success",
  "data": {
    "currentId": 5,
    "nextId": 6
  }
}
```

- `currentId`: The current token ID in the contract.
- `nextId`: The next available token ID that can be used for minting.

In case of an error, the response will contain an error message:

```json
{
  "status": "error",
  "message": "Error message here"
}
```

### Example Request

Here’s an example of how to call the `/next_nft_id` endpoint using `curl`:

```bash
curl -X GET http://localhost:3000/next_nft_id
```

### Notes

- The endpoint returns both the current ID and the next available ID for convenience.
- The server must be running and accessible at the specified URL.
- The endpoint requires a connection to the Ethereum network to read the contract state.



### WebSocket /ws

This endpoint provides real-time updates about NFT listings and transactions.

#### Connection

Connect to the WebSocket endpoint:

```javascript
const ws = new WebSocket('ws://localhost:3000/ws');

ws.onopen = () => {
    console.log('Connected to WebSocket');
};

ws.onmessage = (event) => {
    const message = JSON.parse(event.data);
    console.log('Received:', message);
};

ws.onerror = (error) => {
    console.error('WebSocket error:', error);
};

ws.onclose = () => {
    console.log('Disconnected from WebSocket');
};
```

#### Message Format

Messages sent through the WebSocket connection follow this format:

```json
{
    "type": "message_type",
    "data": {
        // Message specific data
    },
    "error": "Optional error message"
}
```

#### Message Types

1. **Connected**
```json
{
    "type": "connected",
    "data": "Successfully connected to WebSocket"
}
```

2. **NFT Listed**
```json
{
    "type": "nft_listed",
    "data": {
        "tokenId": "123",
        "price": "1000000000000000000",
        "owner": "0x..."
    }
}
```

3. **Error**
```json
{
    "type": "error",
    "error": "Error message here"
}
```

#### Example Usage

```javascript
// Connect to WebSocket
const ws = new WebSocket('ws://localhost:3000/ws');

// Listen for NFT listings
ws.onmessage = (event) => {
    const message = JSON.parse(event.data);
    
    if (message.type === 'nft_listed') {
        console.log('New NFT listed:', message.data);
    }
};

// Send ping message
ws.send(JSON.stringify({
    type: 'ping'
}));
```

### Notes

- The WebSocket connection will automatically receive updates when new NFTs are listed
- The server must be running and accessible at the specified URL
- WebSocket connections are persistent until either the client or server closes them