package main

import (
	"log"
	"net"
	"strconv"
	"time"
)

// establish new connection to node with known id and address
func (n *Node) connectionStart(id string, remoteAddr string) {
	log.Printf("connectionStart: Attempting to connect to %s\n", id)

	conn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		log.Fatal("connectionStart:", err)
	}
	log.Printf("connectionStart: Connected to %s\n", id)

	// send NewConnMessage to give id to connection receiver
	// only if target isn't bootstrap
	if id != "id0" {
		n.sendNewConnMessage(conn)
	}

	// add connection to map of established connections
	log.Printf("connectionStart: Adding connection to connectionMap\n")
	n.connectionMap[id] = net.Conn(conn)

	// monitor this connection for incoming messages
	log.Printf("connectionStart: Launching monitorConnection for connection with %s\n", id)

	n.monitorConnection(conn, id)
}

// monitor given connection for incoming messages
func (n *Node) monitorConnection(conn net.Conn, conn_id string) {
	for {
		// wait until a message is read
		m, err := receiveMessage(conn)
		log.Printf("monitorConnection: received message from node %s", conn_id)
		if err != nil {
			log.Println("monitorConnection:", err)
		}

		// each message received should be of type Message
		// decide on action based on MessageType field
		switch m.MessageType {

		// no message - sent on connection shutdown
		case NullMessageType:
			log.Println("monitorConnection: NullMessage received. Closing connection")
			delete(n.neighborMap, conn_id)
			delete(n.connectionMap, conn_id)
			conn.Close()
			return

		// received welcome from bootstrap with assigned id (regular nodes)
		case WelcomeMessageType:
			log.Println("monitorConnection: WelcomeMessage received")

			// read incoming message and updating own id
			log.Println("monitorConnection: Calling receiveWelcomeMessage")
			n.receiveWelcomeMessage(m.Data)

			// send public key and address to the bootstrap node
			log.Println("monitorConnection: Calling sendSelfInfoMessage")
			n.sendSelfInfoMessage(conn)

		// received message with node's information (bootstrap node)
		case SelfInfoMessageType:
			log.Println("monitorConnection: SelfInfoMessage received")

			// read message and store node's info
			log.Println("monitorConnection: Calling receiveSelfInfoMessage")
			id := n.receiveSelfInfoMessage(m.Data)
			log.Println("monitorConnection: SelfInfoMessage received from id", id)

			// broadcast updated neighbors' information
			// TODO: remove sleep
			// time.Sleep(2 * time.Second)
			log.Println("monitorConnection: Preparing for broadcast")
			n.broadcastType = NeighborsMessageType
			n.broadcast <- true

			// TODO: remove transactions to new nodes
			// time.Sleep(1 * time.Second)
			// log.Println("monitorConnection: Sending tokens to connected node")
			// log.Println("monitorConnection: Creating transaction")
			// utx, err := n.createTransaction(id, 1)
			// if err != nil {
			// 	log.Println("monitorConnection: Invalid transaction")
			// 	continue
			// }

			// log.Println("monitorConnection: Signing transaction")
			// tx := n.signTransaction(utx)

			// log.Println("monitorConnection: Broadcasting transaction")
			// // TODO: remove debugging remains
			// for i := range n.neighborMap {
			// 	fmt.Println(i, n.walletBalance(i))
			// }
			// n.validateTransaction(tx)
			// n.broadcastTransaction(tx)
			// for i := range n.neighborMap {
			// 	fmt.Println(i, n.walletBalance(i))
			// }

		// received message containing bootstrap's info about connected nodes
		case NeighborsMessageType:
			log.Println("monitorConnection: NeighborsMessage received")

			// read message and update own neighbors' info
			log.Println("monitorConnection: Calling receiveNeighborMessage")
			n.receiveNeighborsMessage(m.Data)

		// TODO: add valid transactions to block and check if block is completed
		case TransactionMessageType:
			log.Println("monitorConnection: TransactionMessage received")

			log.Println("monitorConnection: Calling receiveTransactionMessage")
			tx := n.receiveTransactionMessage(m.Data)

			log.Println("monitorConnection: Calling validateTransaction")
			verified := n.validateTransaction(tx)
			log.Println("monitorConnection: validation status -> ", verified)

			if verified {
				n.uncommitedTXs.Push(tx)
			}

			// TODO: remove debugging remains
			// for i := range n.neighborMap {
			// 	fmt.Println(i, n.walletBalance(i))
			// }

		case BlockMessageType:
			log.Println("monitorConnection: BlockMessage received")
			log.Println("monitorConnection: Calling receiveBlockMessage")

			bl := n.receiveBlockMessage(m.Data)

			valid := n.validateBlock(bl)

			if valid {
				// TODO: update transactions
				log.Println("monitorConnection: block validated")
			}

		// unknown message received
		default:
			log.Println("monitorConnection: Unrecognized message type")
		}
	}
}

// broadcast messages if node's broadcast channel receives "true" value
func (n *Node) broadcastMessages() {
	for {
		// wait until channel receives "true" value
		flag := <-n.broadcast
		log.Println("broadcastMessages: broadcast channel received value")

		if flag {
			// TODO: add lock for sending messages?
			log.Println("broadcastMessages: Broadcasting message")

			// send message to each connected node
			for id, c := range n.connectionMap {
				log.Printf("broadcastMessages: Broadcasting to node %s", id)

				// decide what to broadcast based on broadcastType
				switch n.broadcastType {

				// messages not designed to be broadcast
				case NullMessageType, WelcomeMessageType, SelfInfoMessageType:
					log.Fatal("broadcastMessages: Trying to broadcast illegal message type")

				// broadcast neighbors' information
				case NeighborsMessageType:
					log.Println("broadcastMessages: message type -> NeighborMessage")
					n.sendNeighborMessage(c)

				// broadcast transaction initiated from this node
				case TransactionMessageType:
					log.Println("broadcastMessages: message type -> TransactionMessage")
					n.sendTransactionMessage(c, n.initiatedTransaction)

				// broadcast mined block to peers
				case BlockMessageType:
					log.Println("broadcastMessages: message type -> BlockMessage")
					n.sendBlockMessage(c, n.minedBlock)

				default:
					log.Fatal("broadcastMessages: Trying to broadcast unknown message type")
				}
			}
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
			log.Println("acceptConnectionsBootstrap:", err)
		}

		log.Println("acceptConnectionsBootstrap: Received connection")

		// store connection's information
		log.Println("acceptConnectionsBootstrap: Adding connection to map and launching monitorConnection")
		n.connectionMap["id"+strconv.Itoa(currentID)] = conn

		// listen to connection for incoming messages
		go n.monitorConnection(conn, "id"+strconv.Itoa(currentID))

		// send welcome message with assigned id to node
		log.Printf("acceptConnectionsBootstrap: Sending WelcomeMessage to %d", currentID)
		n.sendWelcomeMessage(conn, "id"+strconv.Itoa(currentID))

		if currentID == clients {
			time.Sleep(time.Second * 2)
			genesis := n.createGenesisBlock()
			n.broadcastBlock(genesis)

			for id := range n.neighborMap {
				if id == n.id {
					continue
				}

				utx, err := n.createTransaction(id, 100)
				if err != nil {
					log.Println("acceptConnectionsBootstrap: error creating first transaction to", id)
					continue
				}
				tx := n.signTransaction(utx)
				n.broadcastTransaction(tx)
			}
		}

		currentID++
	}
}

// accept incoming connections and storing information
func (n *Node) acceptConnections(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		log.Printf("acceptConnections: Received connection from %s\n", conn.RemoteAddr())
		if err != nil {
			log.Println("acceptConnections:", err)
		}

		m, err := receiveMessage(conn)
		if err != nil {
			log.Println("acceptConnections: ", err)
		}

		log.Println("acceptConnections: Adding connection to map")
		id := receiveNewConnMessage(m.Data)
		n.connectionMap[id] = conn
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
			log.Printf("establishConnections: Connection to %s already exists\n", k)
			continue

			// establish new connection only if connection doesn't exist and target id is
			// less than own id
			// this is done in order to establish only one connection
			// between each pair of nodes
		} else if myID > targetID {
			log.Printf("establishConnections: Launching connectionStart to %s\n", k)
			go n.connectionStart(k, v.Address)
		}
	}
}
