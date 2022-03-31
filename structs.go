package main

import (
	"crypto/rsa"
	"net"
)

// struct representing a client
type Node struct {
	// node's private information
	id         string
	privateKey rsa.PrivateKey
	publicKey  rsa.PublicKey
	address    string
	wait       bool

	// info about system
	neighborMap   map[string]*Neighbor
	connectionMap map[string]net.Conn

	// current blockchain
	blockchain []Block

	// info about state of blockchain
	utxos      map[[32]byte]TXOutput
	own_utxos  map[[32]byte]bool
	tx_queue   transactionQueue
	unused_txs map[[32]byte]bool

	// copy of blockchain state with changes not yet commited to ledger
	// used for selecting transactions to add to a block before starting mining
	utxos_uncommited     map[[32]byte]TXOutput
	own_utxos_uncommited utxoQueue
	tx_queue_uncommited  transactionQueue

	// auxiliary fields for message broadcasting
	broadcast            chan bool
	broadcastType        MessageTypeVar
	initiatedTransaction Transaction
	minedBlock           Block
	resReqM              ResolveRequestMessage
}

// struct containing connected node's PublicKey and Address
type Neighbor struct {
	PublicKey rsa.PublicKey
	Address   string
}

// struct containing all necessary transaction info without sender's signature
type UnsignedTransaction struct {
	SenderAddress      rsa.PublicKey
	ReceiverAddress    rsa.PublicKey
	Amount             uint
	TransactionID      [32]byte
	TransactionInputs  []TXInput
	TransactionOutputs [2]TXOutput
}

// struct containing all necessary transaction info
type Transaction struct {
	SenderAddress      rsa.PublicKey
	ReceiverAddress    rsa.PublicKey
	Amount             uint
	TransactionID      [32]byte
	TransactionInputs  []TXInput
	TransactionOutputs [2]TXOutput
	Signature          []byte
}

// struct representing Transaction Input
// PreviousOutputID is the ID of a TXOutput not yet spent
type TXInput struct {
	PreviousOutputID [32]byte
}

// struct representing Transaction Output
// ID is unique for each TXOutput
// TransactionID is the unique ID of the Transaction which created this TXOutput
// RecipientAddress is the PublicKey of the node to which the TXOutput is credited
// Amount is the number of coins contained in this TXOutput
type TXOutput struct {
	ID               [32]byte
	TransactionID    [32]byte
	RecipientAddress rsa.PublicKey
	Amount           uint
}

// struct which represents all of a block's info
// before it is hashed
type UnhashedBlock struct {
	Index        uint
	Timestamp    int64 // Timestamp := time.now().Unix()
	PreviousHash [32]byte
	Transactions [capacity]Transaction
	Nonce        [32]byte
}

// struct on which the blockchain is built
type Block struct {
	Index        uint
	Timestamp    int64
	PreviousHash [32]byte
	Transactions [capacity]Transaction
	Nonce        [32]byte
	Hash         [32]byte
}

// each message has a type to differentiate treatment
type MessageTypeVar int

const (
	NullMessageType MessageTypeVar = iota
	WelcomeMessageType
	SelfInfoMessageType
	NeighborsMessageType
	NewConnMessageType
	TransactionMessageType
	BlockMessageType
	ResolveRequestMessageType
	ResolveResponseMessageType
)

// sent on connection closing
type NullMessage struct{}

// sent from bootstrap to newcomer nodes
// contains newcomer's assigned id
type WelcomeMessage struct {
	ID string
}

// sent from newcomer nodes to bootstrap
// contains own id, public key and listening address
type SelfInfoMessage struct {
	ID        string
	PublicKey rsa.PublicKey
	Address   string
}

// broadcast from bootstrap to all nodes
// contains map from ids to Neighbor structs ({Address, PublicKey})
type NeighborsMessage struct {
	Neighbors map[string]Neighbor
}

// sent on non-bootstrap node
// when a new connection is established
// contains source node's id
type NewConnMessage struct {
	ID string
}

// sent to share a transaction with other nodes
type TransactionMessage struct {
	TX Transaction
}

// broadcast message containing new mined block
type BlockMessage struct {
	B Block
}

// broadcast message containing length of blockchain
// and the blocks' hashes
// if another node has longer blockchain it sends the missing/different blocks
type ResolveRequestMessage struct {
	ChainSize uint
	Hashes    [][32]byte
}

// response message to ResolveRequestMessage containing different/new blocks
type ResolveResponseMessage struct {
	Blocks []Block
}

// general message type sent over established connections
// contains MessageType and a json encoded message of type MessageTypeVal
type Message struct {
	MessageType MessageTypeVar
	Data        []byte
}
