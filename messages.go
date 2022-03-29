package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
)

// send Message struct over established connection
func sendMessage(conn net.Conn, m Message) {
	// encode message as json
	mb, _ := json.Marshal(m)

	// log.Printf("sendMessage: Sending message to %s\n", conn.RemoteAddr())
	// log.Printf("sendMessage: Message sent: %s\n", mb)

	// send message + "\n" as messages are separated by newline characters
	conn.Write(mb)
	conn.Write([]byte("\n"))
}

// send WelcomeMessage over connection with assigned id
// used only by bootstrap
func (n *Node) sendWelcomeMessage(conn net.Conn, id string) {
	log.Printf("sendWelcomeMessage: Send id %s\n", id)

	// add new Neighbor
	n.neighborMap[id] = new(Neighbor)

	// create WelcomeMessage with assigned id
	wm := WelcomeMessage{id}

	// encode message as json and create Message to be sent
	wmb, err := json.Marshal(wm)
	if err != nil {
		log.Fatal("sendWelcomeMessage:", err)
	}

	m := Message{WelcomeMessageType, wmb}

	// send created Message
	log.Println("sendWelcomeMessage: Calling sendMessage")
	sendMessage(conn, m)
}

// send NewConnMessage to nodes (not bootstap) with which a connection is made
func (n *Node) sendNewConnMessage(conn net.Conn) {
	log.Printf("sendNewConnMessage: Send id to %s\n", conn.RemoteAddr())

	// create NewConnMessage containing source's id
	ncm := NewConnMessage{n.id}

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
func (n *Node) sendSelfInfoMessage(conn net.Conn) {
	log.Println("sendSelfInfoMessage: Creating message")

	// create SelfInfoMessage containing ID, PublicKey and Address
	sim := SelfInfoMessage{
		ID:        n.id,
		PublicKey: n.publicKey,
		Address:   n.address}

	// encode message as json
	simb, err := json.Marshal(sim)
	if err != nil {
		log.Fatal("sendSelfInfoMessage:", err)
	}

	// create Message containing encoded message
	m := Message{
		MessageType: SelfInfoMessageType,
		Data:        simb}

	// send Message over given connection
	log.Println("sendSelfInfoMessage: Calling sendMessage")
	sendMessage(conn, m)
}

// send information about all connected nodes over established connection
func (n *Node) sendNeighborMessage(conn net.Conn) {
	log.Println("sendNeighborMessage: Creating message")

	// create map to add to NeighborsMessage
	neighbors := make(map[string]Neighbor)
	for k, v := range n.neighborMap {
		neighbors[k] = Neighbor{Address: v.Address, PublicKey: v.PublicKey}
	}

	// create NeighborsMessage
	nm := NeighborsMessage{neighbors}
	// log.Println("sendNeighborMessage: message sent", nm)

	// encode NeighborsMessage as json
	nmb, err := json.Marshal(nm)
	if err != nil {
		log.Println("sendNeighborMessage:", err)
	}

	// create Message
	m := Message{
		MessageType: NeighborsMessageType,
		Data:        nmb}

	// send Message to connection
	log.Println("sendNeighborMessage: Calling sendMessage")
	sendMessage(conn, m)
}

// send message containing a transaction
func (n *Node) sendTransactionMessage(conn net.Conn, tx Transaction) {
	// log.Println("sendTransactionMessage: Creating message")

	txm := TransactionMessage{tx}

	txmb, err := json.Marshal(txm)
	if err != nil {
		log.Println("sendTransactionMessage:", err)
	}

	m := Message{
		MessageType: TransactionMessageType,
		Data:        txmb,
	}

	// log.Println("sendTransactionMessage: Calling sendMessage")
	sendMessage(conn, m)
}

func (n *Node) sendBlockMessage(conn net.Conn, bl Block) {
	// log.Println("sendBlockMessage: Creating message")

	bm := BlockMessage{bl}

	bmb, err := json.Marshal(bm)
	if err != nil {
		log.Println("sendBlockMessage:", err)
	}

	m := Message{
		MessageType: BlockMessageType,
		Data:        bmb,
	}

	// log.Println("sendBlockMessage: Calling sendMessage")
	sendMessage(conn, m)
}

func (n *Node) sendResolveRequestMessage(conn net.Conn, rrm ResolveRequestMessage) {
	rrmb, err := json.Marshal(rrm)
	if err != nil {
		log.Println("sendResolveRequestMessage:", err)
	}

	m := Message{
		MessageType: ResolveRequestMessageType,
		Data:        rrmb,
	}

	sendMessage(conn, m)
}

func (n *Node) sendResolveResponseMessage(conn net.Conn, rrm ResolveResponseMessage) {
	rrmb, err := json.Marshal(rrm)
	if err != nil {
		log.Println("sendResolveResponseMessage:", err)
	}

	m := Message{
		MessageType: ResolveResponseMessageType,
		Data:        rrmb,
	}

	sendMessage(conn, m)
}

// listen to connection and get messages
// return Message struct
func receiveMessage(conn net.Conn) (Message, error) {
	messageBytes, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		log.Println("receiveMessage:", err)
		log.Println("Closing connection")
		conn.Close()
		return Message{}, err
	}

	var m Message
	err = json.Unmarshal([]byte(messageBytes), &m)
	if err != nil {
		log.Println("receiveMessage:", err)
	}

	return m, nil
}

// update neighbors' information based on received NeighborsMessage
func (n *Node) receiveNeighborsMessage(nmb []byte) {

	// decode message
	var nm NeighborsMessage
	err := json.Unmarshal(nmb, &nm)
	if err != nil {
		log.Println("receiveNeighborMessage:", err)
	}

	// for each node in the message update stored info
	for k, v := range nm.Neighbors {

		// if we already have info about node skip
		if _, ok := n.neighborMap[k]; ok {
			continue

			// if this is a new node create Neighbor struct and store info
		} else {
			n.neighborMap[k] = &Neighbor{Address: v.Address, PublicKey: v.PublicKey}
		}
	}

	// call establishConnections to create connections to all new nodes
	n.establishConnections()
}

// extract information from SelfInfoMessage and store it
func (n *Node) receiveSelfInfoMessage(simb []byte) string {

	// decode received message
	var sim SelfInfoMessage
	err := json.Unmarshal(simb, &sim)
	if err != nil {
		log.Println("receiveSelfInfoMessage:", err)
	}

	// store info in neighborMap
	n.neighborMap[sim.ID].PublicKey = sim.PublicKey
	n.neighborMap[sim.ID].Address = sim.Address

	log.Printf("receiveSelfInfoMessage: Message from node %s\n", sim.ID)
	return sim.ID
}

// receive assigned id from bootstrap node
func (n *Node) receiveWelcomeMessage(wmb []byte) {

	// decode message
	var wm WelcomeMessage
	err := json.Unmarshal(wmb, &wm)
	if err != nil {
		log.Println("receiveWelcomeMessage:", err)
	}

	// update own id with value received
	n.id = wm.ID
	log.Printf("receiveWelcomeMessage: Assigned %s\n", n.id)
	n.neighborMap[n.id] = &Neighbor{
		PublicKey: n.publicKey,
		Address:   n.address,
	}
}

// decode TransactionMessage and return a Transaction struct
func (n *Node) receiveTransactionMessage(txmb []byte) Transaction {
	var txm TransactionMessage
	err := json.Unmarshal(txmb, &txm)
	if err != nil {
		log.Println("receiveTransactionMessage: ", err)
	}

	fmt.Println("receiveTransactionMessage:", txm.TX.Amount)
	return txm.TX
}

func (n *Node) receiveBlockMessage(bmb []byte) Block {
	var bm BlockMessage
	err := json.Unmarshal(bmb, &bm)
	if err != nil {
		log.Println("receiveBlockMessage:", err)
	}

	fmt.Println("received block index:", bm.B.Index)
	return bm.B
}

func (n *Node) receiveResolveRequestMessage(rrmb []byte) ResolveResponseMessage {
	var rrm ResolveRequestMessage
	err := json.Unmarshal(rrmb, &rrm)
	if err != nil {
		log.Println("receiveResolveRequestMessage:", err)
	}

	if rrm.ChainSize < uint(len(n.blockchain)) {
		blocks := make([]Block, 0)

		var i int
		for i = range rrm.Hashes {
			if rrm.Hashes[i] != n.blockchain[i].Hash {
				break
			}
		}
		for ; i < len(n.blockchain); i++ {
			blocks = append(blocks, n.blockchain[i])
		}

		resM := ResolveResponseMessage{
			Blocks: blocks,
		}

		return resM
	}

	return ResolveResponseMessage{}
}

func (n *Node) receiveResolveResponseMessage(rrmb []byte) {
	var rrm ResolveResponseMessage
	err := json.Unmarshal(rrmb, &rrm)
	if err != nil {
		log.Println("receiveResolveResponseMessage:", err)
	}

	first_index := rrm.Blocks[0].Index
	last_index := rrm.Blocks[len(rrm.Blocks)-1].Index

	if last_index > uint(len(n.blockchain)) {
		temp := make([]Block, first_index)
		copy(temp[0:first_index], n.blockchain)

		// copy(n.blockchain[0:first_index], n.blockchain[0:first_index])
		fmt.Println("receive.. should be ", first_index, "is", len(temp))

		temp = append(temp, rrm.Blocks...)
		fmt.Println("receive.. should be", last_index+1, "is", len(temp))

		n.blockchain = make([]Block, last_index+1)
		copy(n.blockchain, temp)
	}
	fmt.Println("receive.. chain len", len(n.blockchain))
	for _, bl := range n.blockchain {
		fmt.Println("receiveresolve...: index", bl.Index)
	}

	n.validateChain()
}

// received on new connections with nodes that aren't the bootstrap
// returns the id of the node to which the connection is made
func receiveNewConnMessage(ncmb []byte) string {
	var ncm NewConnMessage
	err := json.Unmarshal(ncmb, &ncm)
	if err != nil {
		log.Println("receiveNewConnMessage: ", err)
	}

	log.Printf("receiveNewConnMessage: received message from %s\n", ncm.ID)
	return ncm.ID
}
