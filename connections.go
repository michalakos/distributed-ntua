package main

import (
	"log"
	"net"
	"strconv"
	"time"
)

// establish new connection to node with known id and address
func (n *Node) connectionStart(id string, remoteAddr string) {

	conn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		log.Fatal("connectionStart:", err)
	}

	// send NewConnMessage to give id to connection receiver
	// only if target isn't bootstrap
	if id != "id0" {
		n.sendNewConnMessage(conn)
	}

	// add connection to map of established connections
	n.connectionMap[id] = net.Conn(conn)

	// monitor this connection for incoming messages
	n.monitorConnection(conn, id)
}

// monitor given connection for incoming messages
func (n *Node) monitorConnection(conn net.Conn, conn_id string) {
	for {
		// wait until a message is read
		m, err := receiveMessage(conn)
		if err != nil {
			continue
		}

		// each message received should be of type Message
		// decide on action based on MessageType field
		switch m.MessageType {

		// no message - sent on connection shutdown
		case NullMessageType:
			continue

		// received welcome from bootstrap with assigned id (regular nodes)
		case WelcomeMessageType:
			// read incoming message and updating own id
			n.receiveWelcomeMessage(m.Data)

			// send public key and address to the bootstrap node
			n.sendSelfInfoMessage(conn)

		// received message with node's information (bootstrap node)
		case SelfInfoMessageType:
			// read message and store node's info
			_ = n.receiveSelfInfoMessage(m.Data)

			// broadcast updated neighbors' information
			n.broadcastType = NeighborsMessageType

			n.broadcast_lock.Lock()
			n.broadcast <- true

		// received message containing bootstrap's info about connected nodes
		case NeighborsMessageType:
			// read message and update own neighbors' info
			n.receiveNeighborsMessage(m.Data)

		case TransactionMessageType:
			n.getTransactionMessage(m.Data)

		case BlockMessageType:
			n.getBlockMessage(m.Data)

		case ResolveRequestMessageType:
			n.getResolveRequestMessage(m.Data, conn)

		case ResolveResponseMessageType:
			n.receiveResolveResponseMessage(m.Data)

		// unknown message received
		default:
			continue
		}
	}
}

func (n *Node) getResolveRequestMessage(data []byte, conn net.Conn) {
	resm := n.receiveResolveRequestMessage(data)
	n.sendResolveResponseMessage(conn, resm)
}

func (n *Node) getTransactionMessage(data []byte) {
	tx := n.receiveTransactionMessage(data)

	n.tx_queue.Push(tx)
}

func (n *Node) getBlockMessage(data []byte) {
	bl := n.receiveBlockMessage(data)
	// fmt.Println("Received block with index", bl.Index)

	n.blockchain_lock.Lock()

	if n.validateBlock(bl) {
		log.Println("Block with index", bl.Index, "validated")
	}

	n.blockchain_lock.Unlock()
}

// broadcast messages if node's broadcast channel receives "true" value
func (n *Node) broadcastMessages() {
	for {
		// wait until channel receives "true" value
		flag := <-n.broadcast

		if flag {

			// send message to each connected node
			for _, c := range n.connectionMap {

				// decide what to broadcast based on broadcastType
				switch n.broadcastType {

				// broadcast neighbors' information
				case NeighborsMessageType:
					n.sendNeighborMessage(c)

				// broadcast transaction initiated from this node
				case TransactionMessageType:
					n.sendTransactionMessage(c, n.initiatedTransaction)

				// broadcast mined block to peers
				case BlockMessageType:
					n.sendBlockMessage(c, n.minedBlock)

				case ResolveRequestMessageType:
					n.resolving_conflict = true
					n.sendResolveRequestMessage(c, n.resReqM)
					time.Sleep(time.Millisecond * 200)

				default:
					log.Fatal("broadcastMessages: Trying to broadcast unknown message type")
				}
			}

			n.resolving_conflict = false
			n.broadcast_lock.Unlock()
		}
	}
}

// bootstrap node accepts incoming connections, sends welcome messages
// and assigns unique ids
func (n *Node) acceptConnectionsBootstrap(ln net.Listener) {
	currentID := 1

	for {
		// accept incoming connections
		conn, err := ln.Accept()
		if err != nil {
			continue
		}

		// store connection's information
		n.connectionMap["id"+strconv.Itoa(currentID)] = conn

		// listen to connection for incoming messages
		go n.monitorConnection(conn, "id"+strconv.Itoa(currentID))

		// send welcome message with assigned id to node
		n.sendWelcomeMessage(conn, "id"+strconv.Itoa(currentID))

		if currentID == clients-1 {
			time.Sleep(time.Millisecond * 100)

			genesis := n.createGenesisBlock()
			n.broadcastBlock(genesis)

			// after all clients are connected send 100 coins to each one of them
			// send in a way that creates one block for each one of the clients
			for id := range n.neighborMap {
				if id == n.id {
					continue
				}
				time.Sleep(time.Second * 2)
				for i := 0; i < capacity; i++ {
					if !n.sendCoins(id, 100/capacity) {
						log.Fatal("Couldn't create first transaction to node", id)
					}
				}
			}
		}

		currentID++
	}
}

// accept incoming connections and storing information
func (n *Node) acceptConnections(ln net.Listener) {
	for count := 0; count < clients-2; count++ {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		m, err := receiveMessage(conn)
		if err != nil {
			continue
		}

		id := receiveNewConnMessage(m.Data)
		n.connectionMap[id] = conn

		go n.monitorConnection(conn, id)
	}
}

// create connections to all known nodes with whom
// a connection doesn't exist and whose id is less than own id
func (n *Node) establishConnections() {
	// convert own id to integer for comparing
	myID, err := strconv.Atoi(n.id[2:])
	if err != nil {
		log.Fatal("establishConnections:", err)
	}

	// check connections for each known node
	for k, v := range n.neighborMap {
		// conver id to integer for comparing
		targetID, err := strconv.Atoi(k[2:])
		if err != nil {
			log.Fatal("establishConnections:", err)
		}

		// if connection already exists don't do anything
		if _, ok := n.connectionMap[k]; ok {
			continue

			// establish new connection only if connection doesn't exist and target id is
			// less than own id
			// this is done in order to establish only one connection
			// between each pair of nodes
		} else if myID > targetID {
			go n.connectionStart(k, v.Address)
		}
	}
}
