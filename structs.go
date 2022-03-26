package main

import (
	"crypto/rsa"
	"net"
)

// struct representing a client
type Node struct {
	id                   string
	privateKey           rsa.PrivateKey
	publicKey            rsa.PublicKey
	address              string
	neighborMap          map[string]*Neighbor
	connectionMap        map[string]net.Conn
	utxos                map[[32]byte]TXOutput
	own_utxos            utxoStack
	broadcast            chan bool
	broadcastType        MessageTypeVar
	initiatedTransaction Transaction
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

// each message has a type to differentiate treatment
type MessageTypeVar int

const (
	NullMessageType MessageTypeVar = iota
	WelcomeMessageType
	SelfInfoMessageType
	NeighborsMessageType
	NewConnMessageType
	TransactionMessageType
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

// general message type sent over established connections
// contains MessageType and a json encoded message of type MessageTypeVal
type Message struct {
	MessageType MessageTypeVar
	Data        []byte
}