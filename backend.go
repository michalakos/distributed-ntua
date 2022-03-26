package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log"
	"net"
)

// setup new node information
func (n *Node) Setup(localAddr string) {
	n.generateWallet()
	log.Println("Setup: Public key is: ", n.publicKey)
	n.address = localAddr
	n.neighborMap = make(map[string]*Neighbor)
	n.connectionMap = make(map[string]net.Conn)
	n.utxos = make(map[[32]byte]TXOutput)
	n.broadcast = make(chan bool)
	n.own_utxos = *NewStack()

	// TODO: remove utxos after testing
	n.utxos[sha256.Sum256([]byte("hey1"))] = TXOutput{
		ID:               sha256.Sum256([]byte("hey1")),
		TransactionID:    sha256.Sum256([]byte("hey")),
		RecipientAddress: n.publicKey,
		Amount:           5,
	}
	n.utxos[sha256.Sum256([]byte("hey2"))] = TXOutput{
		ID:               sha256.Sum256([]byte("hey2")),
		TransactionID:    sha256.Sum256([]byte("hey")),
		RecipientAddress: n.publicKey,
		Amount:           2,
	}
	n.utxos[sha256.Sum256([]byte("hey3"))] = TXOutput{
		ID:               sha256.Sum256([]byte("hey3")),
		TransactionID:    sha256.Sum256([]byte("hey")),
		RecipientAddress: n.publicKey,
		Amount:           19,
	}
}

// create pair of private - public key
func (n *Node) generateWallet() {
	privkey, _ := rsa.GenerateKey(rand.Reader, 2048)
	n.privateKey = *privkey
	n.publicKey = privkey.PublicKey
}

// startup routine for bootstrap node
// TODO: remove utxos - used for testing
func (n *Node) bootstrapStart(localAddr string) {
	n.neighborMap[n.id] = &Neighbor{
		PublicKey: n.publicKey,
		Address:   n.address,
	}

	n.own_utxos.Push(TXOutput{
		ID:               sha256.Sum256([]byte("hey1")),
		TransactionID:    sha256.Sum256([]byte("hey")),
		RecipientAddress: n.publicKey,
		Amount:           5,
	})
	n.own_utxos.Push(TXOutput{
		ID:               sha256.Sum256([]byte("hey2")),
		TransactionID:    sha256.Sum256([]byte("hey")),
		RecipientAddress: n.publicKey,
		Amount:           2,
	})
	n.own_utxos.Push(TXOutput{
		ID:               sha256.Sum256([]byte("hey3")),
		TransactionID:    sha256.Sum256([]byte("hey")),
		RecipientAddress: n.publicKey,
		Amount:           19,
	})

	ln, _ := net.Listen("tcp", localAddr)
	defer ln.Close()

	log.Println("bootstrapStart: Launching acceptConnectionsBootstrap")
	go n.acceptConnectionsBootstrap(ln)

	// check for messages to broadcast
	log.Println("bootstrapStart: Launching broadcastMessages")
	n.broadcastMessages()
}

// startup routine for normal nodes
func (n *Node) nodeStart(localAddr string) {
	// listen to designated port
	ln, _ := net.Listen("tcp", localAddr)
	defer ln.Close()

	// look for incoming connections from nodes with greater id
	log.Println("nodeStart: Lauching acceptConnections")
	go n.acceptConnections(ln)

	// check for messages to broadcast
	log.Printf("nodeStart: Launching broadcastMessages\n")
	n.broadcastMessages()
}

// create transaction from current node to node with known id for number of coins equal to "amount"
// returns nil error if transaction was created successfully
// if successful returns UnsignedTransaction containing all necessary information
// TODO: fix ids
func (n *Node) createTransaction(receiverID string, amount uint) (UnsignedTransaction, error) {
	// log.Println("createTransaction: utxo stack size -> ", n.own_utxos.Length())
	if amount <= 0 {
		log.Println("createTransaction: Transaction amount <= 0")
		return UnsignedTransaction{}, errors.New("InvalidTransactionAmount")
	}

	if _, ok := n.neighborMap[receiverID]; !ok {
		log.Println("createTransaction: receiverID not in Neighbors list")
		return UnsignedTransaction{}, errors.New("InvalidReceiverID")
	}

	var total_creds uint = 0
	var utxos []TXOutput

	// get utxos to fund transaction
	for total_creds < amount {
		utxo, err := n.own_utxos.Pop()

		// no more utxos - transaction fails
		// return utxos to stack
		if err != nil {
			log.Println("createTransaction: ", err)

			for _, utxo := range utxos {
				n.own_utxos.Push(utxo)
			}

			return UnsignedTransaction{}, errors.New("insufficient_funds")
		}

		total_creds += utxo.Amount
		utxos = append(utxos, utxo)
	}

	recipient := n.neighborMap[receiverID].PublicKey
	var tx_id, txout1_id, txout2_id [32]byte
	key := make([]byte, 32)

	_, err := rand.Read(key)
	if err != nil {
		log.Println("createTransaction:", err)
	}
	copy(tx_id[:], key)

	_, err = rand.Read(key)
	if err != nil {
		log.Println("createTransaction:", err)
	}
	copy(txout1_id[:], key)

	_, err = rand.Read(key)
	if err != nil {
		log.Println("createTransaction:", err)
	}
	copy(txout2_id[:], key)

	var inputs []TXInput
	for _, utxo := range utxos {
		inputs = append(inputs, TXInput{utxo.ID})
	}

	outputs := [2]TXOutput{
		{
			ID:               txout1_id,
			TransactionID:    tx_id,
			RecipientAddress: n.publicKey,
			Amount:           total_creds - amount},
		{
			ID:               txout2_id,
			TransactionID:    tx_id,
			RecipientAddress: recipient,
			Amount:           amount}}

	utx := UnsignedTransaction{
		SenderAddress:      n.publicKey,
		ReceiverAddress:    recipient,
		Amount:             amount,
		TransactionID:      tx_id,
		TransactionInputs:  inputs,
		TransactionOutputs: outputs,
	}

	return utx, nil
}

// sign UnsignedTransaction with node's private key and create new Transaction object
// which can be validated by others using this node's public key
func (n *Node) signTransaction(utx UnsignedTransaction) Transaction {
	payload, err := json.Marshal(utx)
	if err != nil {
		log.Fatal("signTransaction: Marshal->", err)
	}

	hashed_payload := sha256.Sum256(payload)

	signature, err := rsa.SignPKCS1v15(rand.Reader, &n.privateKey, crypto.SHA256, hashed_payload[:])
	if err != nil {
		log.Fatal("Setup: Sign", err)
	}

	tx := Transaction{
		SenderAddress:      utx.SenderAddress,
		ReceiverAddress:    utx.ReceiverAddress,
		Amount:             utx.Amount,
		TransactionID:      utx.TransactionID,
		TransactionInputs:  utx.TransactionInputs,
		TransactionOutputs: utx.TransactionOutputs,
		Signature:          signature,
	}

	return tx
}

// broadcast given transaction to all connected nodes
func (n *Node) broadcastTransaction(tx Transaction) {
	n.initiatedTransaction = tx
	n.broadcastType = TransactionMessageType
	n.broadcast <- true
}

// check that given transaction's signature was created by its sender
func (n *Node) verifySignature(tx Transaction) bool {
	utx := UnsignedTransaction{
		SenderAddress:      tx.SenderAddress,
		ReceiverAddress:    tx.ReceiverAddress,
		Amount:             tx.Amount,
		TransactionID:      tx.TransactionID,
		TransactionInputs:  tx.TransactionInputs,
		TransactionOutputs: tx.TransactionOutputs,
	}

	payload, err := json.Marshal(utx)
	if err != nil {
		log.Fatal("verifySignature: Marshal->", err)
	}

	hashed_payload := sha256.Sum256(payload)

	err = rsa.VerifyPKCS1v15(&utx.SenderAddress, crypto.SHA256, hashed_payload[:], tx.Signature)
	if err != nil {
		return false
	} else {
		return true
	}
}

// make all necessary checks to given transaction
// returns true if transaction is consistent with node's view of current state of system
// returns false if
// 		-> transaction was forged by a node other than the sender,
// 		-> TransactionInputs are already spent, nonexistent or not equal to TransactionOutputs,
// 		-> Sender/Receiver unknown to node,
// 		-> TransactionOutputs credited to node other than Sender/Receiver
func (n *Node) validateTransaction(tx Transaction) bool {
	// verify public key matches signature
	if !n.verifySignature(tx) {
		log.Println("validateTransaction: failed to verify signature")
		return false
	}

	found := false
	var sender_id, receiver_id string
	// find node id corresponding to sender's public key
	for id, neighbor := range n.neighborMap {
		if compare(neighbor.PublicKey, tx.SenderAddress) {
			found = true
			sender_id = id
			break
		}
	}
	// public key not responding to any known neighbor
	if !found {
		log.Println("validateTransaction: failed to match SenderAddress to any known node")
		log.Println("unmatched SenderAddress -> ", tx.SenderAddress)
		return false
	}

	found = false
	// find node id corresponding to receiver's public key
	for id, neighbor := range n.neighborMap {
		if compare(neighbor.PublicKey, tx.ReceiverAddress) {
			found = true
			receiver_id = id
			break
		}
	}
	// public key not responding to any known neighbor
	if !found {
		log.Println("validateTransaction: failed to match ReceiverAddress to any known node")
		log.Println("unmatched ReceiverAddress -> ", tx.ReceiverAddress)
		return false
	}

	// check if inputs are valid
	input_sum := uint(0)
	for _, ti := range tx.TransactionInputs {
		if txout, ok := n.utxos[ti.PreviousOutputID]; !ok {
			log.Println("validateTransaction: failed to match Transaction Inputs to UTXOs")
			return false
		} else {
			input_sum += txout.Amount
		}
	}

	output_sum := tx.TransactionOutputs[0].Amount + tx.TransactionOutputs[1].Amount
	outputs_ok := false
	if compare(tx.TransactionOutputs[0].RecipientAddress, tx.ReceiverAddress) {
		if compare(tx.TransactionOutputs[1].RecipientAddress, tx.SenderAddress) {
			outputs_ok = true
		}
	} else if compare(tx.TransactionOutputs[0].RecipientAddress, tx.SenderAddress) {
		if compare(tx.TransactionOutputs[1].RecipientAddress, tx.ReceiverAddress) {
			outputs_ok = true
		}
	}
	if !outputs_ok {
		log.Println("validateTransaction: TXOutput receivers must be Transaction's Sender and Receiver")
		return false
	}

	if input_sum != output_sum {
		log.Println("validateTransaction: input credits not equal to output credits")
		return false
	}

	// remove utxos used as transaction inputs
	for _, ti := range tx.TransactionInputs {
		delete(n.utxos, ti.PreviousOutputID)
	}

	var recTX, sendTX *TXOutput

	for id, nghb := range n.neighborMap {
		if compare(nghb.PublicKey, tx.TransactionOutputs[0].RecipientAddress) {
			if receiver_id == id {
				recTX = &tx.TransactionOutputs[0]
				sendTX = &tx.TransactionOutputs[1]
			} else {
				sendTX = &tx.TransactionOutputs[0]
				recTX = &tx.TransactionOutputs[1]
			}
		}
	}

	// add transaction outputs as utxos
	if receiver_id == n.id {
		n.own_utxos.Push(*recTX)
		n.utxos[recTX.ID] = *recTX
		n.utxos[sendTX.ID] = *sendTX
	} else if sender_id == n.id {
		n.own_utxos.Push(*sendTX)
		n.utxos[recTX.ID] = *recTX
		n.utxos[sendTX.ID] = *sendTX
	} else {
		n.utxos[recTX.ID] = *recTX
		n.utxos[sendTX.ID] = *sendTX
	}
	log.Println("validateTransaction: Transaction valid")
	return true
}
