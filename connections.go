package main

import (
	"fmt"
	"log"
	"net"
	"strconv"
	"time"
)

// setup new node information
func (*Node) Setup(localAddr string) {
	// TODO: create key pair
	thisNode.privateKey = 0
	thisNode.publicKey = 0
	thisNode.address = localAddr
	thisNode.neighborMap = make(map[string]*Neighbor)
	thisNode.connectionMap = make(map[string]net.Conn)
	thisNode.broadcast = make(chan bool)
}

// startup routine for bootstrap node
func (*Node) bootstrapStart(localAddr string) {
	// bootstrap node listens to its port for connections
	// and sends WelcomeMessage messages assigning unique ids
	ln, _ := net.Listen("tcp", localAddr)
	defer ln.Close()

	log.Println("bootstrapStart: Launching acceptConnectionsBootstrap")
	go acceptConnectionsBootstrap(ln)

	// check for messages to broadcast
	log.Println("bootstrapStart: Launching broadcastMessages")
	broadcastMessages()
}

// startup routine for normal nodes
func (*Node) nodeStart(localAddr string) {
	// listen to designated port
	ln, _ := net.Listen("tcp", localAddr)
	defer ln.Close()

	// look for incoming connections from nodes with greater id
	log.Println("nodeStart: Lauching acceptConnections")
	go acceptConnections(ln)

	// check for messages to broadcast
	log.Printf("nodeStart: Launching broadcastMessages\n")
	broadcastMessages()
}

// establish new connection to node with known id and address
func (*Node) connectionStart(id string, remoteAddr string) {
	log.Printf("connectionStart: Attempting to connect to %s\n", id)

	conn, err := net.Dial("tcp", remoteAddr)
	if err != nil {
		log.Fatal("connectionStart:", err)
	}
	log.Printf("connectionStart: Connected to %s\n", id)

	// send NewConnMessage to give id to connection receiver
	// only if target isn't bootstrap
	if id != "id0" {
		sendNewConnMessage(conn)
	}

	// add connection to map of established connections
	log.Printf("connectionStart: Adding connection to connectionMap\n")
	thisNode.connectionMap[id] = net.Conn(conn)

	// monitor this connection for incoming messages
	log.Printf("connectionStart: Launching monitorConnection for connection with %s\n", id)

	for k, v := range thisNode.connectionMap {
		fmt.Println(k, v.RemoteAddr())
	}
	monitorConnection(conn, id)
}

// monitor given connection for incoming messages
func monitorConnection(conn net.Conn, conn_id string) {
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
			delete(thisNode.neighborMap, conn_id)
			delete(thisNode.connectionMap, conn_id)
			conn.Close()
			return

		// received welcome from bootstrap with assigned id (regular nodes)
		case WelcomeMessageType:
			log.Println("monitorConnection: WelcomeMessage received")

			// read incoming message and updating own id
			log.Println("monitorConnection: Calling receiveWelcomeMessage")
			receiveWelcomeMessage(m.Data)

			// send public key and address to the bootstrap node
			log.Println("monitorConnection: Calling sendSelfInfoMessage")
			sendSelfInfoMessage(conn)

		// received message with node's information (bootstrap node)
		case SelfInfoMessageType:
			log.Println("monitorConnection: SelfInfoMessage received")

			// read message and store node's info
			log.Println("monitorConnection: Calling receiveSelfInfoMessage")
			receiveSelfInfoMessage(m.Data)

			// broadcast updated neighbors' information
			// TODO: remove sleep
			time.Sleep(3 * time.Second)
			log.Println("monitorConnection: Preparing for broadcast")
			thisNode.broadcastType = NeighborsMessageType
			thisNode.broadcast <- true

		// received message containing bootstrap's info about connected nodes
		case NeighborsMessageType:
			log.Println("monitorConnection: NeighborsMessage received")

			// read message and update own neighbors' info
			log.Println("monitorConnection: Calling receiveNeighborMessage")
			receiveNeighborsMessage(m.Data)

		// unknown message received
		default:
			log.Println("monitorConnection: Unrecognized message type")
		}
	}
}

// broadcast messages if node's broadcast channel receives "true" value
func broadcastMessages() {
	for {
		// wait until channel receives "true" value
		flag := <-thisNode.broadcast
		log.Println("broadcastMessages: thisNode.broadcast channel received value")

		if flag {
			// TODO: add lock for sending messages?
			log.Println("broadcastMessages: Broadcasting message")

			// send message to each connected node
			for id, c := range thisNode.connectionMap {
				log.Printf("broadcastMessages: Broadcasting to node %s", id)

				// decide what to broadcast based on broadcastType
				switch thisNode.broadcastType {

				// messages not designed to be broadcast
				case NullMessageType, WelcomeMessageType, SelfInfoMessageType:
					log.Fatal("broadcastMessages: Trying to broadcast illegal message type")

				// broadcast neighbors' information
				case NeighborsMessageType:
					log.Println("broadcastMessages: message type -> NeighborMessage")
					thisNode.sendNeighborMessage(c)

				default:
					log.Fatal("broadcastMessages: Trying to broadcast unknown message type")
				}
			}
		}
	}
}

// bootstrap node accepts incoming connections, sends welcome messages
// and assigns unique ids
func acceptConnectionsBootstrap(ln net.Listener) {
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
		thisNode.connectionMap["id"+strconv.Itoa(currentID)] = conn

		// listen to connection for incoming messages
		go monitorConnection(conn, "id"+strconv.Itoa(currentID))

		// send welcome message with assigned id to node
		log.Printf("acceptConnectionsBootstrap: Sending WelcomeMessage to %d", currentID)
		sendWelcomeMessage(conn, "id"+strconv.Itoa(currentID))

		// increment id for use with next connection
		currentID++
	}
}

// accept incoming connections and storing information
func acceptConnections(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		log.Printf("acceptConnections: Received connection from %s\n", conn.RemoteAddr())
		if err != nil {
			log.Println("acceptConnections:")
			log.Println(err)
		}

		m, err := receiveMessage(conn)
		if err != nil {
			log.Println("acceptConnections: ", err)
		}

		log.Println("acceptConnections: Adding connection to map")
		id := receiveNewConnMessage(m.Data)
		thisNode.connectionMap[id] = conn
	}
}

// create connections to all known nodes with whom
// a connection doesn't exist and whose id is less than own id
func (*Node) establishConnections() {
	// convert own id to integer for comparing
	myID, err := strconv.Atoi(thisNode.id[2:])
	if err != nil {
		log.Fatal("establishConnections:", err)
	}

	// check connections for each known node
	for k, v := range thisNode.neighborMap {
		// conver id to integer for comparing
		targetID, err := strconv.Atoi(k[2:])
		if err != nil {
			log.Fatal("establishConnections:", err)
		}

		// if connection already exists don't do anything
		if _, ok := thisNode.connectionMap[k]; ok {
			log.Printf("establishConnections: Connection to %s already exists\n", k)
			continue

			// establish new connection only if connection doesn't exist and target id is
			// less than own id
			// this is done in order to establish only one connection
			// between each pair of nodes
		} else if myID > targetID {
			log.Printf("establishConnections: Launching connectionStart to %s\n", k)
			go thisNode.connectionStart(k, v.Address)
		}
	}
}
