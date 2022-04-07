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
	mathrand "math/rand"
	"net"
	"sync"
	"time"
	// mathrand "math/rand"
)

// setup new node information
func (n *Node) Setup(localAddr string) {

	n.generateWallet()
	n.address = localAddr

	n.neighborMap = make(map[string]*Neighbor)
	n.connectionMap = make(map[string]net.Conn)

	n.blockchain = make([]Block, 0)
	n.blockchain_lock = sync.Mutex{}
	n.mine_lock = sync.Mutex{}

	n.resolving_conflict = false

	n.utxos_val = make(map[[32]byte]TXOutput)
	n.utxos_soft_val = make(map[[32]byte]TXOutput)
	n.utxos_commited = make(map[[32]byte]TXOutput)

	n.self_utxos = *Newqueue()

	n.tx_queue = *NewQueue()
	n.used_txs = make(map[[32]byte]bool)

	n.broadcast_lock = sync.Mutex{}
	n.broadcast = make(chan bool)
}

// create pair of private - public key
func (n *Node) generateWallet() {
	privkey, _ := rsa.GenerateKey(rand.Reader, 2048)
	n.privateKey = *privkey
	n.publicKey = privkey.PublicKey
}

// startup routine for bootstrap node
func (n *Node) bootstrapStart(localAddr string) {
	n.neighborMap[n.id] = &Neighbor{
		PublicKey: n.publicKey,
		Address:   n.address,
	}
	ln, _ := net.Listen("tcp", localAddr)
	defer ln.Close()
	go n.acceptConnectionsBootstrap(ln)
	// check for messages to broadcast
	n.broadcastMessages()
}

// startup routine for normal nodes
func (n *Node) nodeStart(localAddr string) {
	// listen to designated port
	ln, _ := net.Listen("tcp", localAddr)
	defer ln.Close()
	// look for incoming connections from nodes with greater id
	go n.acceptConnections(ln)
	// check for messages to broadcast
	n.broadcastMessages()
}

// create transaction from current node to node with known id for number of coins equal to "amount"
// returns nil error if transaction was created successfully
// if successful returns UnsignedTransaction containing all necessary information
func (n *Node) createTransaction(receiverID string, amount uint) (UnsignedTransaction, error) {
	// check inputs
	if amount <= 0 {
		return UnsignedTransaction{}, errors.New("InvalidTransactionAmount")
	}
	if _, ok := n.neighborMap[receiverID]; !ok {
		return UnsignedTransaction{}, errors.New("InvalidReceiverID")
	}

	var total_creds uint = 0
	var utxos []TXOutput

	// get utxos to fund transaction
	for total_creds < amount {
		utxo, err := n.self_utxos.Pop()

		// no more utxos - transaction fails
		// return utxos to stack
		if err != nil {
			log.Println("createTransaction: not enough coins to send", amount, "to", receiverID)
			for _, utxo := range utxos {
				n.self_utxos.Push(utxo)
			}
			return UnsignedTransaction{}, errors.New("insufficient_funds")
		}

		total_creds += utxo.Amount
		utxos = append(utxos, utxo)
	}

	recipient := n.neighborMap[receiverID].PublicKey

	var tx_id, txout1_id, txout2_id [32]byte
	key := make([]byte, 32)

	_, _ = rand.Read(key)
	copy(tx_id[:], key)
	_, _ = rand.Read(key)
	copy(txout1_id[:], key)
	_, _ = rand.Read(key)
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

	if !n.resolving_conflict {
		n.self_utxos.Push(outputs[0])
	}

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
		log.Println("signTransaction: Marshal->", err)
		return Transaction{}
	}

	hashed_payload := sha256.Sum256(payload)

	signature, _ := rsa.SignPKCS1v15(rand.Reader, &n.privateKey, crypto.SHA256, hashed_payload[:])
	if err != nil {
		log.Println("Setup: Sign", err)
		return Transaction{}
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
	n.broadcast_lock.Lock()

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
	if !n.validateBlock(bl) {
		log.Println("Error: Block not valid")
		return
	}

	n.broadcast_lock.Lock()

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
		return false
	}

	// check is transaction sender and recipient are neighbors (node has info on them)
	found_sender, found_recipient := false, false

	for _, neighbor := range n.neighborMap {
		if compare(neighbor.PublicKey, tx.SenderAddress) {
			found_sender = true
		} else if compare(neighbor.PublicKey, tx.ReceiverAddress) {
			found_recipient = true
		}
	}
	// public keys not responding to any known neighbor
	if !found_sender || !found_recipient {
		return false
	}

	// check if inputs are valid and find input sum
	input_sum := uint(0)
	for _, ti := range tx.TransactionInputs {
		if txout, ok := n.utxos_commited[ti.PreviousOutputID]; !ok {
			return false
		} else {
			input_sum += txout.Amount
		}
	}
	// get output sum
	output_sum := tx.TransactionOutputs[0].Amount + tx.TransactionOutputs[1].Amount
	// check if input sum equals to output sum
	if input_sum != output_sum {
		return false
	}

	// check utxo recipients are transaction's sender and recipient
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
		return false
	}

	for _, ti := range tx.TransactionInputs {
		delete(n.utxos_commited, ti.PreviousOutputID)
	}

	txo1 := &tx.TransactionOutputs[0]
	txo2 := &tx.TransactionOutputs[1]

	// TXOutput0 is the remaining balance - do not reenter in stack
	// first transaction output goes to sender and second to recipient of transaction
	if compare(n.publicKey, txo2.RecipientAddress) && !n.resolving_conflict {
		n.self_utxos.Push(*txo2)
	}

	n.utxos_commited[txo1.ID] = *txo1
	n.utxos_commited[txo2.ID] = *txo2

	return true
}

// check if a transaction is valid without commiting to it
//
// used for selecting which transactions from the node's queue will
// be added to a block before mining
func (n *Node) softValidateTransaction(tx Transaction) bool {
	// verify senders public key matches signature
	if !n.verifySignature(tx) {
		return false
	}

	// find node id corresponding to sender's and receiver's public keys
	found_sender := false
	found_receiver := false
	for _, neighbor := range n.neighborMap {
		if compare(neighbor.PublicKey, tx.SenderAddress) {
			found_sender = true
		} else if compare(neighbor.PublicKey, tx.ReceiverAddress) {
			found_receiver = true
		}
	}
	// public key not responding to any known neighbor
	if !found_sender || !found_receiver {
		return false
	}

	// check if inputs are valid and get total input sum
	input_sum := uint(0)
	for _, ti := range tx.TransactionInputs {
		if txout, ok := n.utxos_soft_val[ti.PreviousOutputID]; !ok {
			return false
		} else {
			input_sum += txout.Amount
		}
	}

	// get output sum and make sure it is equal to input sum
	output_sum := tx.TransactionOutputs[0].Amount + tx.TransactionOutputs[1].Amount
	if input_sum != output_sum {
		return false
	}

	// ensure UTXOs from output are attributed to sender and recipient of transaction
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
		return false
	}

	// all checks are successful - update UTXOs

	// remove utxos used as transaction inputs
	for _, ti := range tx.TransactionInputs {
		delete(n.utxos_soft_val, ti.PreviousOutputID)
	}

	txo1 := &tx.TransactionOutputs[0]
	txo2 := &tx.TransactionOutputs[1]

	n.utxos_soft_val[txo1.ID] = *txo1
	n.utxos_soft_val[txo2.ID] = *txo2

	return true
}

// check if current hash is correct and starts with a predetermined number of zeros
// and if the block comes after the latest block in the blockchain
func (n *Node) validateBlock(bl Block) bool {

	chain_len := uint(len(n.blockchain))

	// genesis block is just accepted as true
	if chain_len == 0 && bl.Index == 0 {
		// accept UTXOs from genesis block (only for bootstrap)
		for _, tx := range bl.Transactions {

			if tx.Amount > 0 {
				txo1 := &tx.TransactionOutputs[0]
				txo2 := &tx.TransactionOutputs[1]

				// only store in own unspent tokens if bootstrap
				if n.broadcastType != ResolveRequestMessageType {
					if compare(n.publicKey, txo1.RecipientAddress) && !n.resolving_conflict {
						n.self_utxos.Push(*txo1)
					} else if compare(n.publicKey, txo2.RecipientAddress) && !n.resolving_conflict {
						n.self_utxos.Push(*txo2)
					}
				}
				n.utxos_commited[txo1.ID] = *txo1
				n.utxos_commited[txo2.ID] = *txo2
			}
		}
		n.blockchain = append(n.blockchain, bl)

		return true

	} else if chain_len > bl.Index {
		log.Println("Older block received")
		return false
	} else if chain_len < bl.Index {
		log.Println("Possible gap in chain - calling resolveConflict()")
		n.resolveConflict()
		return false
	}

	// block with correct index is received - check if it is valid
	ubl := UnhashedBlock{
		Index:        bl.Index,
		Timestamp:    bl.Timestamp,
		PreviousHash: bl.PreviousHash,
		Transactions: bl.Transactions,
		Nonce:        bl.Nonce,
	}

	payload, _ := json.Marshal(ubl)

	// check if hash is correct
	hash := sha256.Sum256(payload)
	if hash != bl.Hash {
		return false
	}

	// check if block hash abides by difficulty rule
	for _, val := range hash[:difficulty/2] {
		if val != 0 {
			return false
		}
	}
	if difficulty%2 == 1 && hash[difficulty/2] > 15 {
		return false
	}

	// check if previous block matches the one last inserted in the blockchain
	if bl.PreviousHash != n.blockchain[bl.Index-1].Hash {
		log.Println("Previous hash not matching the one in the blockchain - calling resolveConflict")
		n.resolveConflict()
		return false
	}

	// if this point is reached then the block is accepted and added to the blockchain

	n.blockchain = append(n.blockchain, bl)
	for i, tx := range bl.Transactions {
		valid := n.validateTransaction(tx)
		if !valid {
			log.Println("validateBlock: transaction", i, "invalid")
		}
	}

	return true
}

func (n *Node) validateChain() bool {

	blockchain_copy := make([]Block, len(n.blockchain))
	copy(blockchain_copy, n.blockchain)
	n.blockchain = make([]Block, 0)

	n.utxos_soft_val = make(map[[32]byte]TXOutput)
	n.utxos_soft_val = make(map[[32]byte]TXOutput)
	n.utxos_commited = make(map[[32]byte]TXOutput)
	n.used_txs = make(map[[32]byte]bool)

	valid := true
	for _, bl := range blockchain_copy {
		valid = n.validateBlock(bl)
		if !valid {
			break
		}
	}

	if valid {
		log.Println("validateChain: blockchain is valid")
	} else {
		log.Println("validateChain: blockchain is not valid")
	}

	return valid
}

func (n *Node) walletBalance(id string) uint {
	neighbor, ok := n.neighborMap[id]
	if !ok {
		return 0
	}

	address := neighbor.PublicKey

	var amount uint = 0
	for _, utxo := range n.utxos_commited {
		if compare(address, utxo.RecipientAddress) {
			amount += utxo.Amount
		}
	}
	return amount
}

// called only from bootstrap node
// generates genesis block containing transaction giving bootstrap
// 100*clients coins
func (n *Node) createGenesisBlock() Block {
	var tx_id, txout1_id, txout2_id [32]byte

	key := make([]byte, 32)

	_, err := rand.Read(key)
	if err != nil {
		return Block{}
	}
	copy(tx_id[:], key)

	_, err = rand.Read(key)
	if err != nil {
		return Block{}
	}
	copy(txout1_id[:], key)

	_, err = rand.Read(key)
	if err != nil {
		return Block{}
	}
	copy(txout2_id[:], key)

	txout1 := TXOutput{
		ID:               txout1_id,
		TransactionID:    tx_id,
		RecipientAddress: n.publicKey,
		Amount:           100 * clients,
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
	for len(n.blockchain) == 0 {
		continue
	}
	n.updateUncommited()

	// after each block is added to the blockchain collect the
	// necessary number of valid transactions to add to the next block
	//
	// stop condition -> new block added
	for {
		chain_length := uint(len(n.blockchain))
		collected_transactions := []Transaction{}

		all_txs := *NewQueue()
		all_txs.Copy(&n.tx_queue)

		// log.Println("Collecting transactions for block", chain_length)

		// collect transactions not already in the blockchain and not already selected for the new block
		// repeat until block is full
		skip := false
		for len(collected_transactions) < capacity {

			if n.broadcastType == ResolveRequestMessageType {
				time.Sleep(time.Second * 4)
				skip = true
				break
			}

			// get a transaction from queue of all transactions
			tx, err := all_txs.Pop()

			if err != nil {
				all_txs.Copy(&n.tx_queue)
				continue
			}

			// if this transaction is not already used
			already_used, exists := n.used_txs[tx.TransactionID]
			if !exists || !already_used {
				n.blockchain_lock.Lock()
				tx_ok := n.softValidateTransaction(tx)
				n.blockchain_lock.Unlock()
				if !tx_ok {
					continue
				}

				n.used_txs[tx.TransactionID] = true
				collected_transactions = append(collected_transactions, tx)
			}
		}

		// at this point we have collected the transactions which go to the next block -
		// start mining until either this block or a block received from another client is valid

		if skip {
			skip = false
		} else {
			n.mine_lock.Lock()
			block := n.mineBlock(collected_transactions, chain_length)
			n.mine_lock.Unlock()

			n.blockchain_lock.Lock()
			if chain_length == uint(len(n.blockchain)) {
				n.broadcastBlock(block)
			}
			n.blockchain_lock.Unlock()
		}

		// n.blockchain_lock.Unlock()

		n.updateUncommited()
	}
}

func (n *Node) mineBlock(txs []Transaction, chain_len uint) Block {
	fmt.Println("\nMining block", chain_len)
	start_time := time.Now().Unix()

	var transactions [capacity]Transaction
	copy(transactions[:], txs)

	// create block template on which we will try different nonce values
	ublock := UnhashedBlock{
		Index:        chain_len,
		Timestamp:    time.Now().Unix(),
		PreviousHash: n.blockchain[chain_len-1].Hash,
		Transactions: transactions,
	}

	var nonce [32]byte
	key := make([]byte, 32)

	i := 0
	for {
		i++
		if i%20000 == 0 {
			log.Println("Iterations:", i)

			// check if new block was added while mining
			// not in every iteration because checking is expensive
			n.blockchain_lock.Lock()
			if chain_len < uint(len(n.blockchain)) {
				n.blockchain_lock.Unlock()
				return Block{}
			}
			n.blockchain_lock.Unlock()
		}

		// generate random nonce value
		_, err := rand.Read(key)
		if err != nil {
			log.Println("Error on rand.Read")
			continue
		}
		copy(nonce[:], key)
		ublock.Nonce = nonce

		// hash block
		payload, err := json.Marshal(ublock)
		if err != nil {
			log.Println("Error in json.Marshal")
			continue
		}
		hash := sha256.Sum256(payload)

		// check if hash is acceptable based on difficulty
		mined := true
		for _, bt := range hash[:difficulty/2] {
			if bt != 0 {
				mined = false
				break
			}
		}
		if difficulty%2 == 1 && hash[difficulty/2] > 15 {
			mined = false
		}

		if mined {
			stop_time := time.Now().Unix()
			block_time := stop_time - start_time
			log.Println("Block mined")

			fmt.Println("\n>>Block time:", block_time)
			fmt.Println("")

			return Block{
				Index:        ublock.Index,
				Timestamp:    ublock.Timestamp,
				PreviousHash: ublock.PreviousHash,
				Transactions: ublock.Transactions,
				Nonce:        ublock.Nonce,
				Hash:         hash,
			}
		}
	}
}

// update temp values after new block is inserted to blockchain
func (n *Node) updateUncommited() {

	n.utxos_soft_val = make(map[[32]byte]TXOutput)
	for tid, utxos := range n.utxos_commited {
		n.utxos_soft_val[tid] = utxos
	}

	n.used_txs = make(map[[32]byte]bool)

	n.blockchain_lock.Lock()
	for _, bl := range n.blockchain {
		for _, tx := range bl.Transactions {
			n.used_txs[tx.TransactionID] = true
		}
	}
	n.blockchain_lock.Unlock()
}

func (n *Node) sendCoins(id string, coins uint) bool {
	time.Sleep(time.Millisecond*100 + time.Millisecond*time.Duration(mathrand.Intn(clients))*200)

	n.mine_lock.Lock()

	utx, err := n.createTransaction(id, coins)
	if err != nil {
		n.mine_lock.Unlock()
		return false
	}

	tx := n.signTransaction(utx)

	n.broadcastTransaction(tx)

	n.tx_queue.Push(tx)

	n.mine_lock.Unlock()

	return true
}

func (n *Node) showTransactions() {
	temp_txs := NewQueue()
	temp_txs.Copy(&n.tx_queue)
	tx, err := temp_txs.Pop()
	for err == nil {
		fmt.Println("tx from", n.getId(tx.SenderAddress), "to", n.getId(tx.ReceiverAddress), "for", tx.Amount)
		tx, err = temp_txs.Pop()
	}
}

func (n *Node) resolveConflict() {
	hashes := make([][32]byte, 0)
	for _, block := range n.blockchain {
		hashes = append(hashes, block.Hash)
	}

	length := uint(len(n.blockchain))

	n.broadcast_lock.Lock()
	n.resReqM = ResolveRequestMessage{
		ChainSize: length,
		Hashes:    hashes,
	}
	n.broadcastType = ResolveRequestMessageType
	n.broadcast <- true
	time.Sleep(time.Second)
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

func (n *Node) getId(pk rsa.PublicKey) string {
	for id, ngh := range n.neighborMap {
		if compare(ngh.PublicKey, pk) {
			return id
		}
	}
	return ""
}
