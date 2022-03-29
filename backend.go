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
	"math"
	"math/big"
	"net"
	"time"
)

// setup new node information
func (n *Node) Setup(localAddr string) {
	n.generateWallet()
	n.address = localAddr
	n.wait = false

	n.neighborMap = make(map[string]*Neighbor)
	n.connectionMap = make(map[string]net.Conn)

	n.blockchain = make([]Block, 0)

	n.utxos = make(map[[32]byte]TXOutput)
	n.own_utxos = make(map[[32]byte]TXOutput)
	n.tx_queue = *NewQueue()
	n.unused_txs = make(map[[32]byte]bool)

	n.utxos_uncommited = make(map[[32]byte]TXOutput)
	n.own_utxos_uncommited = *NewStack()
	n.tx_queue_uncommited = *NewQueue()

	n.broadcast = make(chan bool)

	go n.collectTransactions()
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
		utxo, err := n.own_utxos_uncommited.Pop()

		// no more utxos - transaction fails
		// return utxos to stack
		if err != nil {
			log.Println("createTransaction: ", err)

			for _, utxo := range utxos {
				n.own_utxos_uncommited.Push(utxo)
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

	n.own_utxos_uncommited.Push(outputs[0])

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

// broadcast a block
func (n *Node) broadcastBlock(bl Block) {
	if len(n.blockchain) > int(bl.Index) {
		log.Println("broadcastBlock: Aborting broadcast - new block added to chain")
	}

	// validate block before broadcasting
	log.Println("broadcastBlock: validating block")
	if !n.validateBlock(bl) {
		log.Println("broadcastBlock: block invalid")
		return
	}

	log.Println("broadcastBlock: broadcasting block with index", bl.Index)
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
		return false
	}

	found = false
	// find node id corresponding to receiver's public key
	for _, neighbor := range n.neighborMap {
		if compare(neighbor.PublicKey, tx.ReceiverAddress) {
			found = true
			break
		}
	}
	// public key not responding to any known neighbor
	if !found {
		log.Println("validateTransaction: failed to match ReceiverAddress to any known node")
		return false
	}

	// check if inputs are valid
	input_sum := uint(0)
	for _, ti := range tx.TransactionInputs {
		if txout, ok := n.utxos[ti.PreviousOutputID]; !ok {
			fmt.Println("unmatched utxo:", ti.PreviousOutputID)
			log.Println("validateTransaction: failed to match Transaction Inputs to UTXOs")
			return false
		} else {
			input_sum += txout.Amount
		}
	}

	outputs_ok := false
	output_sum := tx.TransactionOutputs[0].Amount + tx.TransactionOutputs[1].Amount
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
		log.Println("validateTransaction: input credits", input_sum, "not equal to output credits", output_sum)
		return false
	}

	log.Println("validateTransaction: Transaction valid")
	delete(n.unused_txs, tx.TransactionID)

	// remove utxos from own and total utxos list
	for _, ti := range tx.TransactionInputs {
		delete(n.own_utxos, ti.PreviousOutputID)
		delete(n.utxos, ti.PreviousOutputID)
	}

	txo1 := &tx.TransactionOutputs[0]
	txo2 := &tx.TransactionOutputs[1]

	if compare(n.publicKey, txo1.RecipientAddress) {
		n.own_utxos[txo1.ID] = *txo1
	} else if compare(n.publicKey, txo2.RecipientAddress) {
		n.own_utxos[txo2.ID] = *txo2
	}
	n.utxos[txo1.ID] = *txo1
	n.utxos[txo2.ID] = *txo2

	return true
}

// check if a transaction is valid without commiting to it
//
// used for selecting which transactions from the node's queue will
// be added to a block before mining
func (n *Node) softValidateTransaction(tx Transaction) bool {
	// verify public key matches signature
	if !n.verifySignature(tx) {
		log.Println("softValidateTransaction: failed to verify signature")
		return false
	}

	found := false
	// find node id corresponding to sender's public key
	for _, neighbor := range n.neighborMap {
		if compare(neighbor.PublicKey, tx.SenderAddress) {
			found = true
			break
		}
	}
	// public key not responding to any known neighbor
	if !found {
		log.Println("softValidateTransaction: failed to match SenderAddress to any known node")
		return false
	}

	found = false
	// find node id corresponding to receiver's public key
	for _, neighbor := range n.neighborMap {
		if compare(neighbor.PublicKey, tx.ReceiverAddress) {
			found = true
			break
		}
	}
	// public key not responding to any known neighbor
	if !found {
		log.Println("softValidateTransaction: failed to match ReceiverAddress to any known node")
		return false
	}

	// check if inputs are valid
	input_sum := uint(0)
	for _, ti := range tx.TransactionInputs {
		if txout, ok := n.utxos_uncommited[ti.PreviousOutputID]; !ok {
			log.Println("softValidateTransaction: failed to match Transaction Inputs to UTXOs")
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
		log.Println("softValidateTransaction: TXOutput receivers must be Transaction's Sender and Receiver")
		return false
	}

	if input_sum != output_sum {
		log.Println("softValidateTransaction: input credits not equal to output credits", input_sum, output_sum)
		return false
	}

	// remove utxos used as transaction inputs
	for _, ti := range tx.TransactionInputs {
		delete(n.utxos_uncommited, ti.PreviousOutputID)
	}

	txo1 := &tx.TransactionOutputs[0]
	txo2 := &tx.TransactionOutputs[1]

	if compare(n.publicKey, txo1.RecipientAddress) {
		n.own_utxos_uncommited.Push(*txo1)
	} else if compare(n.publicKey, txo2.RecipientAddress) {
		n.own_utxos_uncommited.Push(*txo2)
	}

	n.utxos_uncommited[txo1.ID] = *txo1
	n.utxos_uncommited[txo2.ID] = *txo2

	log.Println("softValidateTransaction: Transaction valid")
	return true
}

// check if current hash is correct and starts with a predetermined number of zeros
// and if the block comes after the latest block in the blockchain
func (n *Node) validateBlock(bl Block) bool {
	log.Println("validateBlock: new block with index", bl.Index)

	// genesis block not checked
	if len(n.blockchain) == 0 && bl.Index == 0 {
		// accept UTXOs from genesis block (only for bootstrap)
		for _, tx := range bl.Transactions {
			if tx.Amount > 0 {
				txo1 := &tx.TransactionOutputs[0]
				txo2 := &tx.TransactionOutputs[1]

				// only store in own unspent tokens if bootstrap
				// FIXME: in case we keep commited and uncommited own_utxos remove uncommited from here
				if compare(n.publicKey, txo1.RecipientAddress) {
					n.own_utxos[txo1.ID] = *txo1
					n.own_utxos_uncommited.Push(*txo1)
				} else if compare(n.publicKey, txo2.RecipientAddress) {
					n.own_utxos[txo2.ID] = *txo2
					n.own_utxos_uncommited.Push(*txo2)
				}

				n.utxos[txo1.ID] = *txo1
				n.utxos[txo2.ID] = *txo2
			}
		}

		n.blockchain = append(n.blockchain, bl)
		fmt.Println("validateBlock: new blockchain size", len(n.blockchain))

		// update uncommited fields after block validation
		log.Println("validateBlock: updating uncommited values")
		n.updateUncommited()

		fmt.Println("new wallet balances")
		for id := range n.neighborMap {
			fmt.Println(id, n.walletBalance(id))
		}

		return true

	} else if uint(len(n.blockchain)) > bl.Index {
		// TODO: possibly call resolveConflict()
		log.Println("validateBlock: waiting for block with Index", len(n.blockchain),
			"but received block with Index", bl.Index)
		return false
	} else if uint(len(n.blockchain)) < bl.Index {
		log.Println("validateBlock: missed blocks - calling resolveConflict")
		n.resolveConflict()
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
		log.Println("validateBlock: Marshal->", err)
		return false
	}

	// check if hash is correct
	hash := sha256.Sum256(payload)
	if hash != bl.Hash {
		log.Println("validateBlock: current_hash not matching hash of block")
		return false
	}

	for i, val := range hash[:difficulty/8+1] {
		if i < difficulty/8 {
			if val != 0 {
				log.Println("validateBlock: hash not matching difficulty")
				return false
			}
		} else if i == difficulty/8 {
			if float64(val) >= math.Pow(2, float64(8-difficulty%8)) {
				log.Println("validateBlock: hash not matching difficulty")
				return false
			}
		} else {
			log.Println("validateBlock: sanity check")
			return false
		}
	}

	// check if previous block matches the one last inserted in the blockchain
	// TODO: possibly call resolveConflict
	if len(n.blockchain) > 0 && bl.PreviousHash != n.blockchain[len(n.blockchain)-1].Hash {
		log.Println("validateBlock: previous_hash not matching latest block's hash")
		return false
	}

	log.Println("validateBlock: block accepted - now validating included transactions")
	n.blockchain = append(n.blockchain, bl)
	fmt.Println("validateBlock: new blockchain size", len(n.blockchain))

	// TODO: pre-commit and commit in case of false transactions
	// TODO: if transactions are invalid decline block and roll back changes
	// how? update -> softvalidate -> if all ok -> validate
	// shouldn't happen because of consensus - only needed for malicious actors
	for i, tx := range bl.Transactions {
		valid := n.validateTransaction(tx)
		if valid {
			log.Println("validateBlock: transaction", i, "valid")
		} else {
			log.Println("validateBlock: transaction", i, "invalid")
		}
	}

	// update uncommited fields after block validation
	// log.Println("validateBlock: updating uncommited values")
	// n.updateUncommited()

	fmt.Println("new wallet balances")
	for id := range n.neighborMap {
		fmt.Println(id, n.walletBalance(id))
	}
	return true
}

func (n *Node) validateChain() bool {
	fmt.Println("validateChain: blockchain length", len(n.blockchain))
	blockchain_copy := make([]Block, len(n.blockchain))
	copy(blockchain_copy, n.blockchain)

	n.blockchain = make([]Block, 0)
	n.utxos = make(map[[32]byte]TXOutput)
	n.own_utxos = make(map[[32]byte]TXOutput)
	n.utxos_uncommited = make(map[[32]byte]TXOutput)
	n.own_utxos_uncommited = *NewStack()

	valid := true
	for _, block := range blockchain_copy {
		fmt.Println("validatechain: calling validateblock for", block.Index)
		valid = n.validateBlock(block)
		if !valid {
			break
		}
	}
	return valid
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
// (100*clients + 100) coins
func (n *Node) createGenesisBlock() Block {
	var tx_id, txout1_id, txout2_id [32]byte

	key := make([]byte, 32)

	_, err := rand.Read(key)
	if err != nil {
		log.Println("createGenesisBlock:", err)
	}
	copy(tx_id[:], key)

	_, err = rand.Read(key)
	if err != nil {
		log.Println("createGenesisBlock:", err)
	}
	copy(txout1_id[:], key)

	_, err = rand.Read(key)
	if err != nil {
		log.Println("createGenesisBlock:", err)
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

// keep accepting transactions to commit
func (n *Node) collectTransactions() {
	log.Println("collectTransactions: Starting...")
	log.Println("collectTransactions: waiting for genesis block before mining")
	for len(n.blockchain) == 0 {
		continue
	}

	for {
		log.Println("collectTransactions: collecting transactions for new block")
		// fmt.Println("transactions", n.tx_queue.Size(), "uncommited", n.tx_queue_uncommited.Size())
		collected_transactions := []Transaction{}
		chain_length := uint(len(n.blockchain))

		for len(collected_transactions) < capacity {
			tx, err := n.tx_queue_uncommited.Pop()
			if err != nil {
				continue
			}

			if n.softValidateTransaction(tx) {
				collected_transactions = append(collected_transactions, tx)
			}
		}

		// for n.wait {
		// 	continue
		// }
		bl, err := n.mineBlock(collected_transactions, chain_length)
		if err != nil {
			log.Println("collectTransactions:->", err)
		} else {
			log.Println("collectTransactions: block mined successfully")
			log.Println("collectTransactions: broadcasting new block")
			n.broadcastBlock(bl)
		}

		// update uncommited transaction structs to restart operation
		log.Println("collectTransactions: updating uncommited transaction structs")
		time.Sleep(time.Millisecond * 100)
		// FIXME: update after new block fully added
		n.updateUncommited()
	}
}

func (n *Node) mineBlock(txs []Transaction, chain_len uint) (Block, error) {
	if chain_len == 0 {
		log.Println("mineBlock: waiting for genesis block")
		for len(n.blockchain) == 0 {
			continue
		}
		return Block{}, errors.New("mineBlock: genesis block isn't mined")
	}

	var transactions [capacity]Transaction
	copy(transactions[:], txs)

	uhb := UnhashedBlock{
		Index:        chain_len,
		Timestamp:    time.Now().Unix(),
		PreviousHash: n.blockchain[chain_len-1].Hash,
		Transactions: transactions,
	}

	// TODO: remove debugging i
	i := 0
	for {
		// debugging
		i++
		if i%100 == 0 {
			fmt.Println("iterations:", i)
		}

		// create random nonce
		var nonce [32]byte
		key := make([]byte, 32)
		_, err := rand.Read(key)
		if err != nil {
			log.Println("mineBlock: nonce generation", err)
		}
		copy(nonce[:], key)
		uhb.Nonce = nonce

		// hash block
		payload, err := json.Marshal(uhb)
		if err != nil {
			log.Fatal("mineBlock: Marshal->", err)
		}
		hash := sha256.Sum256(payload)

		// check if hash starts with required number of zeros
		mined := true
		for i, val := range hash[:difficulty/8+1] {
			if i < difficulty/8 {
				if val != 0 {
					mined = false
					break
				}
			} else if i == difficulty/8 {
				if float64(val) >= math.Pow(2, float64(8-difficulty%8)) {
					mined = false
					break
				}
			} else {
				log.Println("mineBlock: sanity check")
				mined = false
				break
			}
		}

		// new block added while mining
		if uint(len(n.blockchain)) > chain_len {
			return Block{}, errors.New("mineBlock: mining stopped after new block was added")

			// successfully mined new block
		} else if mined {
			log.Println("mineBlock: block successfully mined")
			block := Block{
				Index:        uhb.Index,
				Timestamp:    uhb.Timestamp,
				PreviousHash: uhb.PreviousHash,
				Transactions: uhb.Transactions,
				Nonce:        uhb.Nonce,
				Hash:         hash,
			}
			return block, nil

			// mining not successful - keep trying
		} else {
			continue
		}
	}
}

// update temp values after new block is inserted to blockchain
func (n *Node) updateUncommited() {
	log.Println("updateUncommited: updating...")
	// remove used transactions from transaction queue
	temp_txs := *NewQueue()
	for tx, err := n.tx_queue.Pop(); err == nil; tx, err = n.tx_queue.Pop() {
		_, ok := n.unused_txs[tx.TransactionID]
		if ok {
			temp_txs.Push(tx)
		}
	}
	n.tx_queue.Copy(&temp_txs)

	// remove used utxos from own_utxos stack
	temp_own := *NewStack()
	for _, tx := range n.own_utxos {
		temp_own.Push(tx)
	}
	// FIXME: i think own_utxos don't need commited and uncommited -
	// we have trust that sometime later the transaction will be accepted

	// n.own_utxos_uncommited.Copy(&temp_own)

	// update uncommited transactions with commited transaction values
	for i := range n.utxos_uncommited {
		delete(n.utxos_uncommited, i)
	}
	for i, val := range n.utxos {
		n.utxos_uncommited[i] = val
	}

	n.tx_queue_uncommited.Copy(&n.tx_queue)
}

func (n *Node) sendCoins(id string, coins uint) bool {
	log.Println("sendCoins: sending", coins, "to", id)
	time.Sleep(time.Millisecond * 10)

	utx, err := n.createTransaction(id, coins)
	if err != nil {
		log.Println("sendCoins: error creating transaction to", id, "with", coins, "coins")
		return false
	}

	tx := n.signTransaction(utx)
	n.broadcastTransaction(tx)

	n.tx_queue.Push(tx)
	n.tx_queue_uncommited.Push(tx)
	n.unused_txs[tx.TransactionID] = true

	return true
}

func (n *Node) resolveConflict() {
	n.wait = true
	hashes := make([][32]byte, 0)
	for _, block := range n.blockchain {
		hashes = append(hashes, block.Hash)
	}

	length := uint(len(n.blockchain))

	n.resReqM = ResolveRequestMessage{
		ChainSize: length,
		Hashes:    hashes,
	}

	n.broadcastType = ResolveRequestMessageType
	n.broadcast <- true
}

func (n *Node) viewTransactions() {
	bl := n.blockchain[len(n.blockchain)-1]
	fmt.Println("")

	for i, tx := range bl.Transactions {
		fmt.Println("Transaction", i)
		fmt.Println("Amount:", tx.Amount)

		var sender, receiver string
		for id, nghb := range n.neighborMap {
			if compare(nghb.PublicKey, tx.ReceiverAddress) {
				receiver = id
			} else if compare(nghb.PublicKey, tx.SenderAddress) {
				sender = id
			}
		}

		fmt.Println("From:", sender)
		fmt.Println("To:", receiver)
		fmt.Println("")
	}
}
