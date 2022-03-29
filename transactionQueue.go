package main

import (
	"errors"
	"sync"
)

// stack of Unspent Transaction Outputs
// uses mutex to ensure no concurrent access
type transactionQueue struct {
	lock sync.Mutex
	txs  []Transaction
}

// create new stack
func NewQueue() *transactionQueue {
	return &transactionQueue{lock: sync.Mutex{}, txs: make([]Transaction, 0)}
}

// copy original stack onto this object's
func (s *transactionQueue) Copy(original *transactionQueue) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.txs = make([]Transaction, len(original.txs))
	copy(s.txs, original.txs)
}

// returns the size of the stack
func (s *transactionQueue) Size() int {
	return len(s.txs)
}

// push TXOutput to the stack
func (s *transactionQueue) Push(tx Transaction) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.txs = append(s.txs, tx)
}

// returns TXOutpus last added to the stack if not empty
// if empty error is not nil
func (s *transactionQueue) Pop() (Transaction, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	l := len(s.txs)
	if l == 0 {
		return Transaction{}, errors.New("empty_transactionStack")
	}

	res := s.txs[0]
	s.txs = s.txs[1:]
	return res, nil
}
