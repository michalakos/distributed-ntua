package main

import "net"

type Node struct {
	id            string
	privateKey    int
	publicKey     int
	address       string
	neighborMap   map[string]*Neighbor
	connectionMap map[string]net.Conn
	broadcast     chan bool
	broadcastType MessageTypeVar
}

type Neighbor struct {
	PublicKey string
	Address   string
}

// each message has a type to differentiate treatment
type MessageTypeVar int

const (
	NullMessageType MessageTypeVar = iota
	WelcomeMessageType
	SelfInfoMessageType
	NeighborsMessageType
	NewConnMessageType
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
	PublicKey int
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

// general message type sent over established connections
// contains MessageType and a json encoded message of type MessageTypeVal
type Message struct {
	MessageType MessageTypeVar
	Data        []byte
}
