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
	// mathrand "math/rand"
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
	n.own_utxos = make(map[[32]byte]bool)
	n.tx_queue = *NewQueue()
	n.unused_txs = make(map[[32]byte]bool)

	n.utxos_uncommited = make(map[[32]byte]TXOutput)
	fmt.Println("initializing own_utxos_uncommited")
	n.own_utxos_uncommited = *Newqueue()
	n.tx_queue_uncommited = *NewQueue()

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
	for n.wait {
		continue
	}
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
		utxo, err := n.own_utxos_uncommited.Pop()
		delete(n.own_utxos, utxo.ID)

		// no more utxos - transaction fails
		// return utxos to stack
		if err != nil {
			for _, utxo := range utxos {
				n.own_utxos_uncommited.Push(utxo)
				n.own_utxos[utxo.ID] = true
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
		return UnsignedTransaction{}, err
	}
	copy(tx_id[:], key)

	_, err = rand.Read(key)
	if err != nil {
		return UnsignedTransaction{}, err
	}
	copy(txout1_id[:], key)

	_, err = rand.Read(key)
	if err != nil {
		return UnsignedTransaction{}, err
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

	n.own_utxos_uncommited.Push(outputs[0])
	n.own_utxos[outputs[0].ID] = true

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
	if !n.wait && !n.validateBlock(bl) {
		return
	}

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
		return false
	}

	// check if inputs are valid
	input_sum := uint(0)
	for _, ti := range tx.TransactionInputs {
		if txout, ok := n.utxos[ti.PreviousOutputID]; !ok {
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
		return false
	}

	if input_sum != output_sum {
		return false
	}

	delete(n.unused_txs, tx.TransactionID)

	// remove utxos from own and total utxos list
	for _, ti := range tx.TransactionInputs {
		delete(n.utxos, ti.PreviousOutputID)
	}

	txo1 := &tx.TransactionOutputs[0]
	txo2 := &tx.TransactionOutputs[1]

	fmt.Println(txo1.Amount, txo2.Amount)
	_, ok1 := n.own_utxos[txo1.ID]
	_, ok2 := n.own_utxos[txo2.ID]

	// TXOutput0 is the remaining balance - do not reenter in stack
	if compare(n.publicKey, txo1.RecipientAddress) && ok1 {
		fmt.Println(txo1.Amount, "are returned as change")
	} else if compare(n.publicKey, txo2.RecipientAddress) && !ok2 && !n.wait {
		n.own_utxos_uncommited.Push(*txo2)
		n.own_utxos[txo2.ID] = true
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
		return false
	}

	// check if inputs are valid
	input_sum := uint(0)
	for _, ti := range tx.TransactionInputs {
		if txout, ok := n.utxos_uncommited[ti.PreviousOutputID]; !ok {
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
		return false
	}
	if input_sum != output_sum {
		return false
	}

	// remove utxos used as transaction inputs
	for _, ti := range tx.TransactionInputs {
		delete(n.utxos_uncommited, ti.PreviousOutputID)
	}

	txo1 := &tx.TransactionOutputs[0]
	txo2 := &tx.TransactionOutputs[1]

	n.utxos_uncommited[txo1.ID] = *txo1
	n.utxos_uncommited[txo2.ID] = *txo2

	return true
}

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
				if compare(n.publicKey, txo1.RecipientAddress) && !n.wait {
					n.own_utxos_uncommited.Push(*txo1)
					n.own_utxos[txo1.ID] = true
				} else if compare(n.publicKey, txo2.RecipientAddress) && !n.wait {
					n.own_utxos_uncommited.Push(*txo2)
					n.own_utxos[txo2.ID] = true
				}
				n.utxos[txo1.ID] = *txo1
				n.utxos[txo2.ID] = *txo2
			}
		}
		n.blockchain = append(n.blockchain, bl)

		// update uncommited fields after block validation
		n.updateUncommited()

		fmt.Println("New wallet balances:")
		for id := range n.neighborMap {
			fmt.Println(id, n.walletBalance(id))
		}

		return true

	} else if uint(len(n.blockchain)) > bl.Index {
		return false
	} else if uint(len(n.blockchain)) < bl.Index {
		log.Println("Calling resolveConflict()")
		n.resolveConflict()
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
		return false
	}

	// check if hash is correct
	hash := sha256.Sum256(payload)
	if hash != bl.Hash {
		return false
	}

	for i, val := range hash[:difficulty/8+1] {
		if i < difficulty/8 {
			if val != 0 {
				return false
			}
		} else if i == difficulty/8 {
			if float64(val) >= math.Pow(2, float64(8-difficulty%8)) {
				return false
			}
		} else {
			return false
		}
	}

	// check if previous block matches the one last inserted in the blockchain
	if len(n.blockchain) > 0 && (bl.PreviousHash != n.blockchain[len(n.blockchain)-1].Hash) {
		return false
	}
	n.blockchain = append(n.blockchain, bl)

	for i, tx := range bl.Transactions {
		valid := n.validateTransaction(tx)
		if valid {
			log.Println("validateBlock: transaction", i, "valid")
		} else {
			log.Println("validateBlock: transaction", i, "invalid")
		}
	}

	// update uncommited fields after block validation

	fmt.Println("New wallet balances")
	for id := range n.neighborMap {
		fmt.Println(id, n.walletBalance(id))
	}
	return true
}

func (n *Node) validateChain() bool {
	for n.wait {
		continue
	}
	n.wait = true
	time.Sleep(time.Millisecond * 100)

	blockchain_copy := make([]Block, len(n.blockchain))
	copy(blockchain_copy, n.blockchain)

	n.blockchain = make([]Block, 0)
	n.utxos = make(map[[32]byte]TXOutput)
	n.utxos_uncommited = make(map[[32]byte]TXOutput)

	valid := true
	for _, block := range blockchain_copy {
		valid = n.validateBlock(block)
		if !valid {
			break
		}
	}
	time.Sleep(time.Millisecond * 100)
	n.wait = false
	return valid
}

func (n *Node) walletBalance(id string) uint {
	neighbor, ok := n.neighborMap[id]
	if !ok {
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
	for len(n.blockchain) == 0 {
		continue
	}

	for {
		collected_transactions := []Transaction{}
		chain_length := uint(len(n.blockchain))

		for len(collected_transactions) < capacity {
			for n.wait {
				continue
			}
			tx, err := n.tx_queue_uncommited.Pop()
			if err != nil {
				continue
			}
			if n.softValidateTransaction(tx) {
				collected_transactions = append(collected_transactions, tx)
			}
		}

		bl, err := n.mineBlock(collected_transactions, chain_length)
		if err != nil {
			continue
		} else {
			n.broadcastBlock(bl)
		}

		// update uncommited transaction structs to restart operation
		time.Sleep(time.Millisecond * 100)
		n.updateUncommited()
	}
}

func (n *Node) mineBlock(txs []Transaction, chain_len uint) (Block, error) {
	if chain_len == 0 {
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

	for {
		if n.wait {
			return Block{}, errors.New("updating chain while mining")
		}

		// create random nonce
		var nonce [32]byte
		key := make([]byte, 32)
		_, err := rand.Read(key)
		if err != nil {
			continue
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
				mined = false
				break
			}
		}

		// new block added while mining
		if uint(len(n.blockchain)) > chain_len {
			return Block{}, errors.New("mineBlock: mining stopped after new block was added")

			// successfully mined new block
		} else if mined {
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
	// remove used transactions from transaction queue
	temp_txs := *NewQueue()
	for tx, err := n.tx_queue.Pop(); err == nil; tx, err = n.tx_queue.Pop() {
		_, ok := n.unused_txs[tx.TransactionID]
		if ok {
			temp_txs.Push(tx)
		}
	}
	n.tx_queue.Copy(&temp_txs)

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
	fmt.Println("Sending", coins, "to", id)

	for n.wait {
		continue
	}

	time.Sleep(time.Millisecond * clients * difficulty / capacity * 60)

	utx, err := n.createTransaction(id, coins)
	if err != nil {
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
	time.Sleep(time.Millisecond * 100)
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
