package main

import (
	"bufio"
	"encoding/json"
	"log"
	"net"
)

// send Message struct over established connection
func sendMessage(conn net.Conn, m Message) {
	// encode message as json
	mb, _ := json.Marshal(m)

	// send message + "\n" as messages are separated by newline characters
	conn.Write(mb)
	conn.Write([]byte("\n"))
}

// send WelcomeMessage over connection with assigned id
// used only by bootstrap
func (n *Node) sendWelcomeMessage(conn net.Conn, id string) {

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
	sendMessage(conn, m)
}

// send NewConnMessage to nodes (not bootstap) with which a connection is made
func (n *Node) sendNewConnMessage(conn net.Conn) {

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
	sendMessage(conn, m)
}

// send own information to bootstrap over established connection
func (n *Node) sendSelfInfoMessage(conn net.Conn) {

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
	sendMessage(conn, m)
}

// send information about all connected nodes over established connection
func (n *Node) sendNeighborMessage(conn net.Conn) {

	// create map to add to NeighborsMessage
	neighbors := make(map[string]Neighbor)
	for k, v := range n.neighborMap {
		neighbors[k] = Neighbor{Address: v.Address, PublicKey: v.PublicKey}
	}

	// create NeighborsMessage
	nm := NeighborsMessage{neighbors}

	// encode NeighborsMessage as json
	nmb, err := json.Marshal(nm)
	if err != nil {
		return
	}

	// create Message
	m := Message{
		MessageType: NeighborsMessageType,
		Data:        nmb}

	// send Message to connection
	sendMessage(conn, m)
}

// send message containing a transaction
func (n *Node) sendTransactionMessage(conn net.Conn, tx Transaction) {

	txm := TransactionMessage{tx}

	txmb, err := json.Marshal(txm)
	if err != nil {
		return
	}

	m := Message{
		MessageType: TransactionMessageType,
		Data:        txmb,
	}

	sendMessage(conn, m)
}

func (n *Node) sendBlockMessage(conn net.Conn, bl Block) {

	bm := BlockMessage{bl}

	bmb, err := json.Marshal(bm)
	if err != nil {
		return
	}

	m := Message{
		MessageType: BlockMessageType,
		Data:        bmb,
	}

	sendMessage(conn, m)
}

func (n *Node) sendResolveRequestMessage(conn net.Conn, rrm ResolveRequestMessage) {
	rrmb, err := json.Marshal(rrm)
	if err != nil {
		return
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
		return
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
		conn.Close()
		return Message{}, err
	}

	var m Message
	err = json.Unmarshal([]byte(messageBytes), &m)
	if err != nil {
		return Message{}, err
	}

	return m, nil
}

// update neighbors' information based on received NeighborsMessage
func (n *Node) receiveNeighborsMessage(nmb []byte) {

	// decode message
	var nm NeighborsMessage
	err := json.Unmarshal(nmb, &nm)
	if err != nil {
		return
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
		return ""
	}

	// store info in neighborMap
	n.neighborMap[sim.ID].PublicKey = sim.PublicKey
	n.neighborMap[sim.ID].Address = sim.Address

	return sim.ID
}

// receive assigned id from bootstrap node
func (n *Node) receiveWelcomeMessage(wmb []byte) {

	// decode message
	var wm WelcomeMessage
	err := json.Unmarshal(wmb, &wm)
	if err != nil {
		return
	}

	// update own id with value received
	n.id = wm.ID

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
		return Transaction{}
	}

	return txm.TX
}

func (n *Node) receiveBlockMessage(bmb []byte) Block {
	for n.wait {
		continue
	}
	var bm BlockMessage
	err := json.Unmarshal(bmb, &bm)
	if err != nil {
		return Block{}
	}

	return bm.B
}

func (n *Node) receiveResolveRequestMessage(rrmb []byte) ResolveResponseMessage {
	var rrm ResolveRequestMessage
	err := json.Unmarshal(rrmb, &rrm)
	if err != nil {
		return ResolveResponseMessage{}
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
		return
	}

	if len(rrm.Blocks) == 0 {
		return
	}

	first_index := rrm.Blocks[0].Index
	last_index := rrm.Blocks[len(rrm.Blocks)-1].Index

	if last_index > uint(len(n.blockchain)) {
		temp := make([]Block, first_index)
		copy(temp[0:first_index], n.blockchain)

		temp = append(temp, rrm.Blocks...)

		n.blockchain = make([]Block, last_index+1)
		copy(n.blockchain, temp)
	}

	n.validateChain()
}

// received on new connections with nodes that aren't the bootstrap
// returns the id of the node to which the connection is made
func receiveNewConnMessage(ncmb []byte) string {
	var ncm NewConnMessage
	err := json.Unmarshal(ncmb, &ncm)
	if err != nil {
		return ""
	}

	return ncm.ID
}
