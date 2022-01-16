package main

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
	"strconv"
)

// send Message struct over established connection
func sendMessage(conn net.Conn, m Message) {
	// encode message as json
	mb, _ := json.Marshal(m)

	log.Printf("sendMessage: Sending message to %s\n", conn.RemoteAddr())
	log.Printf("sendMessage: Message sent: %s\n", mb)

	// send message + "\n" as messages are separated by newline characters
	conn.Write(mb)
	conn.Write([]byte("\n"))
}

// send WelcomeMessage over connection with assigned id
// used only by bootstrap
func sendWelcomeMessage(conn net.Conn, id string) {
	log.Printf("sendWelcomeMessage: Send id %s\n", id)

	// add new Neighbor
	thisNode.neighborMap[id] = new(Neighbor)

	// create WelcomeMessage with assigned id
	wm := WelcomeMessage{id}

	// encode message as json and create Message to be sent
	wmb, _ := json.Marshal(wm)
	m := Message{WelcomeMessageType, wmb}

	// send created Message
	log.Println("sendWelcomeMessage: Calling sendMessage")
	sendMessage(conn, m)
}

func sendNewConnMessage(conn net.Conn) {
	log.Printf("sendNewConnMessage: Send id to %s\n", conn.RemoteAddr())

	// create NewConnMessage containing source's id
	ncm := NewConnMessage{thisNode.id}

	// encode to add it to Message
	ncmb, err := json.Marshal(ncm)
	if err != nil {
		log.Fatal("sendNewConnMessage:", err)
	}

	// create Message
	m := Message{NewConnMessageType, ncmb}

	// send created message over connection
	log.Println("sendNewConnMessage: Calling sendMessage")
	sendMessage(conn, m)
}

// send own information to bootstrap over established connection
func sendSelfInfoMessage(conn net.Conn) {
	log.Println("sendSelfInfoMessage: Creating message")

	// create SelfInfoMessage containing ID, PublicKey and Address
	sim := SelfInfoMessage{
		ID:        thisNode.id,
		PublicKey: thisNode.publicKey,
		Address:   thisNode.address}

	// encode message as json
	simb, _ := json.Marshal(sim)

	// create Message containing encoded message
	m := Message{
		MessageType: SelfInfoMessageType,
		Data:        simb}

	// send Message over given connection
	log.Println("sendSelfInfoMessage: Calling sendMessage")
	sendMessage(conn, m)
}

// send information about all connected nodes over established connection
func (*Node) sendNeighborMessage(conn net.Conn) {
	log.Println("sendNeighborMessage: Creating message")

	// create map to add to NeighborsMessage
	neighbors := make(map[string]Neighbor)
	for k, v := range thisNode.neighborMap {
		neighbors[k] = Neighbor{Address: v.Address, PublicKey: v.PublicKey}
	}

	// create NeighborsMessage
	nm := NeighborsMessage{neighbors}
	log.Println("sendNeighborMessage: message sent", nm)

	// encode NeighborsMessage as json
	nmb, err := json.Marshal(nm)
	if err != nil {
		log.Println("sendNeighborMessage:")
		log.Println(err)
	}

	// create Message
	m := Message{
		MessageType: NeighborsMessageType,
		Data:        nmb}

	// send Message to connection
	log.Println("sendNeighborMessage: Calling sendMessage")
	sendMessage(conn, m)
}

// listen to connection and get messages
// return Message struct
func receiveMessage(conn net.Conn) (Message, error) {
	messageBytes, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		log.Println("receiveMessage:")
		log.Println(err)
		log.Println("Closing connection")
		conn.Close()
		return Message{}, err
	}

	log.Printf("receiveMessage: Message received: %s\n", messageBytes)
	var m Message

	// decode []byte message into Message
	err = json.Unmarshal([]byte(messageBytes), &m)
	if err != nil {
		log.Println("receiveMessage:")
		log.Println(err)
	}

	// return Message m
	return m, nil
}

// update neighbors' information based on received NeighborsMessage
func receiveNeighborsMessage(nmb []byte) {

	// decode message
	var nm NeighborsMessage
	err := json.Unmarshal(nmb, &nm)
	if err != nil {
		log.Println("receiveNeighborMessage:")
		log.Println(err)
	}
	log.Printf("receiveNeighborMessage: Message received: %s\n", nmb)

	// for each node in the message update stored info
	for k, v := range nm.Neighbors {

		// if we already have info about node skip
		if _, ok := thisNode.neighborMap[k]; ok {
			continue

			// if this is a new node create Neighbor struct and store info
		} else {
			thisNode.neighborMap[k] = &Neighbor{Address: v.Address, PublicKey: v.PublicKey}
			log.Printf("receiveNeighborMessage: %s Address: %s, PublicKey: %s", k, v.Address, v.PublicKey)
		}
	}

	// call establishConnections to create connections to
	// all new nodes
	thisNode.establishConnections()
}

// extract information from SelfInfoMessage and store it
func receiveSelfInfoMessage(simb []byte) {

	// decode received message
	var sim SelfInfoMessage
	err := json.Unmarshal(simb, &sim)
	if err != nil {
		log.Println("receiveSelfInfoMessage:")
		log.Println(err)
	}

	// store info in neighborMap
	// TODO: update for read PublicKey
	thisNode.neighborMap[sim.ID].PublicKey = strconv.Itoa(sim.PublicKey)
	thisNode.neighborMap[sim.ID].Address = sim.Address

	log.Printf("receiveSelfInfoMessage: Message from node %s with publicKey %d and address %s\n",
		sim.ID, sim.PublicKey, sim.Address)
}

// receive assigned id from bootstrap node
func receiveWelcomeMessage(wmb []byte) {

	// decode message
	var wm WelcomeMessage
	err := json.Unmarshal(wmb, &wm)
	if err != nil {
		log.Println("receiveWelcomeMessage:", err)
	}

	// update own id with value received
	thisNode.id = wm.ID
	log.Printf("receiveWelcomeMessage: Assigned %s\n", thisNode.id)
}

func receiveNewConnMessage(ncmb []byte) string {
	var ncm NewConnMessage
	err := json.Unmarshal(ncmb, &ncm)
	if err != nil {
		log.Println("receiveNewConnMessage: ", err)
	}

	log.Printf("receiveNewConnMessage: received message with %s\n", ncm.ID)

	return ncm.ID
}
