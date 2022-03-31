package main

import (
	"errors"
	"sync"
)

// stack of Unspent Transaction Outputs
// uses mutex to ensure no concurrent access
type utxoQueue struct {
	lock  sync.Mutex
	utxos []TXOutput
}

// create new stack
func Newqueue() *utxoQueue {
	return &utxoQueue{lock: sync.Mutex{}, utxos: make([]TXOutput, 0)}
}

func (s *utxoQueue) Copy(original *utxoQueue) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.utxos = make([]TXOutput, len(original.utxos))
	copy(s.utxos, original.utxos)
}

// returns the size of the stack
func (s *utxoQueue) Size() int {
	return len(s.utxos)
}

// push TXOutput to the stack
func (s *utxoQueue) Push(utxo TXOutput) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.utxos = append(s.utxos, utxo)
}

// returns TXOutpus last added to the stack if not empty
// if empty error is not nil
func (s *utxoQueue) Pop() (TXOutput, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	l := len(s.utxos)
	if l == 0 {
		return TXOutput{}, errors.New("empty_utxoStack")
	}

	res := s.utxos[0]
	s.utxos = s.utxos[1:]
	return res, nil
}
