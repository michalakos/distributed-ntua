package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/big"
	"net"
	"time"
)

// TODO: update utxos and own_utxos on each new block
// TODO: implement blockchain as list of blocks (slice ?)
// TODO: create genesis block
// TODO: broadcast blocks
// TODO: validate blocks and add utxos (special case for genesis block)

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
	n.uncommitedTXs = *NewQueue()

	// TODO: remove utxos after testing
	// FIXME: each node thinks utxos are his own, intended use was for bootstrap alone
	// n.utxos[sha256.Sum256([]byte("hey1"))] = TXOutput{
	// 	ID:               sha256.Sum256([]byte("hey1")),
	// 	TransactionID:    sha256.Sum256([]byte("hey")),
	// 	RecipientAddress: n.publicKey,
	// 	Amount:           5,
	// }
	// n.utxos[sha256.Sum256([]byte("hey2"))] = TXOutput{
	// 	ID:               sha256.Sum256([]byte("hey2")),
	// 	TransactionID:    sha256.Sum256([]byte("hey")),
	// 	RecipientAddress: n.publicKey,
	// 	Amount:           2,
	// }
	// n.utxos[sha256.Sum256([]byte("hey3"))] = TXOutput{
	// 	ID:               sha256.Sum256([]byte("hey3")),
	// 	TransactionID:    sha256.Sum256([]byte("hey")),
	// 	RecipientAddress: n.publicKey,
	// 	Amount:           10,
	// }
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

	// n.own_utxos.Push(TXOutput{
	// 	ID:               sha256.Sum256([]byte("hey1")),
	// 	TransactionID:    sha256.Sum256([]byte("hey")),
	// 	RecipientAddress: n.publicKey,
	// 	Amount:           5,
	// })
	// n.own_utxos.Push(TXOutput{
	// 	ID:               sha256.Sum256([]byte("hey2")),
	// 	TransactionID:    sha256.Sum256([]byte("hey")),
	// 	RecipientAddress: n.publicKey,
	// 	Amount:           2,
	// })
	// n.own_utxos.Push(TXOutput{
	// 	ID:               sha256.Sum256([]byte("hey3")),
	// 	TransactionID:    sha256.Sum256([]byte("hey")),
	// 	RecipientAddress: n.publicKey,
	// 	Amount:           10,
	// })

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

		fmt.Println("creds:", total_creds, "from required:", amount)
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

	fmt.Println("sender has:", total_creds-amount, "and receiver:", amount)

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
	if !n.validateTransaction(tx) {
		log.Println("broadcastTransaction: transaction invalid")
	}

	n.initiatedTransaction = tx
	n.broadcastType = TransactionMessageType
	n.broadcast <- true
}

// broadcast a block
func (n *Node) broadcastBlock(bl Block) {
	// validate block before broadcasting
	if !n.validateBlock(bl) {
		log.Println("broadcastBlock: block invalid")
		return
	}

	log.Println("broadcastBlock: broadcasting block with id", bl.Index)
	n.minedBlock = bl
	n.broadcastType = BlockMessageType
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
	// var receiver_id string
	// find node id corresponding to sender's public key
	for _, neighbor := range n.neighborMap {
		if compare(neighbor.PublicKey, tx.SenderAddress) {
			found = true
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
	for _, neighbor := range n.neighborMap {
		if compare(neighbor.PublicKey, tx.ReceiverAddress) {
			found = true
			// receiver_id = id
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
	fmt.Println(tx.TransactionOutputs[0].Amount, tx.TransactionOutputs[1].Amount)
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
		log.Println("validateTransaction: input credits not equal to output credits", input_sum, output_sum)
		return false
	}

	// remove utxos used as transaction inputs
	for _, ti := range tx.TransactionInputs {
		fmt.Println("removing utxo with amount", n.utxos[ti.PreviousOutputID].Amount)
		delete(n.utxos, ti.PreviousOutputID)
	}

	// FIXME: don't think the distinction is necessary
	// var recTX, sendTX *TXOutput

	// // find which transaction output is
	// for id, nghb := range n.neighborMap {
	// 	// transaction outputs are 2 - one for the node sending coins and one
	// 	// for the node accepting them
	// 	// the order they are in is unknown so check if any node matches the 1st
	// 	// transaction output's recipient address and if so we know which
	// 	// transaction output is the one with the sender's change and which with the
	// 	// receiver's new coins
	// 	if compare(nghb.PublicKey, tx.TransactionOutputs[0].RecipientAddress) {
	// 		if receiver_id == id {
	// 			recTX = &tx.TransactionOutputs[0]
	// 			sendTX = &tx.TransactionOutputs[1]
	// 		} else {
	// 			sendTX = &tx.TransactionOutputs[0]
	// 			recTX = &tx.TransactionOutputs[1]
	// 		}
	// 	}
	// }

	// // add transaction outputs as utxos
	// if receiver_id == n.id {
	// 	n.own_utxos.Push(*recTX)
	// }
	// n.utxos[recTX.ID] = *recTX
	// n.utxos[sendTX.ID] = *sendTX

	// log.Println("validateTransaction: Transaction valid")

	txo1 := &tx.TransactionOutputs[0]
	txo2 := &tx.TransactionOutputs[1]

	if compare(n.publicKey, txo1.RecipientAddress) {
		n.own_utxos.Push(*txo1)
	} else if compare(n.publicKey, txo2.RecipientAddress) {
		n.own_utxos.Push(*txo2)
	}
	n.utxos[txo1.ID] = *txo1
	n.utxos[txo2.ID] = *txo2

	log.Println("validateTransaction: Transaction valid")

	fmt.Println("printing new wallet balances")
	for i := range n.neighborMap {
		fmt.Println(i, n.walletBalance(i))
	}

	return true
}

// TODO: validate transactions?
// check if current hash is correct and starts with a predetermined number of zeros
// and if the block comes after the latest block in the blockchain
func (n *Node) validateBlock(bl Block) bool {
	// genesis block not checked
	if len(n.blockchain) == 0 && bl.Index == 0 {
		// accept UTXOs from genesis block (only for bootstrap)
		for _, tx := range bl.Transactions {
			if tx.Amount > 0 {
				txo1 := &tx.TransactionOutputs[0]
				txo2 := &tx.TransactionOutputs[1]

				// only store in own unspent tokens if bootstrap
				if compare(n.publicKey, txo1.RecipientAddress) {
					n.own_utxos.Push(*txo1)
				} else if compare(n.publicKey, txo2.RecipientAddress) {
					n.own_utxos.Push(*txo2)
				}

				n.utxos[txo1.ID] = *txo1
				n.utxos[txo2.ID] = *txo2
			}
		}

		n.blockchain = append(n.blockchain, bl)
		fmt.Println("validateBlock: new blockchain size", len(n.blockchain))

		return true
	} else if uint(len(n.blockchain)) != bl.Index {
		// TODO: possibly call resolveConflict()
		log.Println("validateBlock: waiting for block", len(n.blockchain), "received block", bl.Index)
		return false
	}

	ubl := UnhashedBlock{
		Index:        bl.Index,
		Timestamp:    bl.Timestamp,
		PreviousHash: bl.PreviousHash,
		Transactions: bl.Transactions,
		Nonce:        bl.Nonce,
	}

	payload, err := json.Marshal(ubl)
	if err != nil {
		log.Fatal("validateBlock: Marshal->", err)
	}

	hash := sha256.Sum256(payload)

	if hash != bl.Hash {
		log.Println("validateBlock: current_hash not matching hash of block")
		return false
	}

	for _, val := range bl.Hash[:difficulty] {
		if val != 0 {
			log.Println("validateBlock: current_hash not abiding by difficulty constraint")
			return false
		}
	}

	if bl.PreviousHash != n.blockchain[len(n.blockchain)-1].Hash {
		log.Println("validateBlock: previous_hash not matching latest block's hash")
		return false
	}

	fmt.Println("printing new wallet balances")
	for i := range n.neighborMap {
		fmt.Println(i, n.walletBalance(i))
	}

	n.blockchain = append(n.blockchain, bl)
	fmt.Println("validateBlock: new blockchain size", len(n.blockchain))

	return true
}

func (n *Node) walletBalance(id string) uint {
	neighbor, ok := n.neighborMap[id]
	if !ok {
		log.Println("walletBalance: no record for client with id", id)
		return 0
	}

	address := neighbor.PublicKey

	var amount uint = 0
	for _, utxo := range n.utxos {
		if compare(address, utxo.RecipientAddress) {
			amount += utxo.Amount
		}
	}
	return amount
}

// called only from bootstrap node
// generates genesis block containing transaction giving bootstrap
// 100 * capacity coins
func (n *Node) createGenesisBlock() Block {
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

	txout1 := TXOutput{
		ID:               txout1_id,
		TransactionID:    tx_id,
		RecipientAddress: n.publicKey,
		Amount:           100*clients + 100,
	}
	txout2 := TXOutput{
		ID:               txout2_id,
		TransactionID:    tx_id,
		RecipientAddress: rsa.PublicKey{N: big.NewInt(0), E: 1},
		Amount:           0,
	}

	transaction := Transaction{
		SenderAddress:      rsa.PublicKey{N: big.NewInt(0), E: 1},
		ReceiverAddress:    n.publicKey,
		Amount:             100 * clients,
		TransactionID:      tx_id,
		TransactionInputs:  []TXInput{},
		TransactionOutputs: [2]TXOutput{txout1, txout2},
		Signature:          []byte{0},
	}

	uhBlock := UnhashedBlock{
		Index:     0,
		Timestamp: time.Now().Unix(),
		// previous hash in genesis block is 1
		PreviousHash: [...]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1},
		Transactions: [capacity]Transaction{transaction},
		// nonce in genesis block is 0
		Nonce: [...]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	}

	payload, err := json.Marshal(uhBlock)
	if err != nil {
		log.Fatal("createGenesisBlock: Marshal->", err)
	}

	hash := sha256.Sum256(payload)

	block := Block{
		Index:        uhBlock.Index,
		Timestamp:    uhBlock.Timestamp,
		PreviousHash: uhBlock.PreviousHash,
		Transactions: uhBlock.Transactions,
		Nonce:        uhBlock.Nonce,
		Hash:         hash,
	}

	return block
}
