package main

import (
	"errors"
	"sync"
)

// stack of Unspent Transaction Outputs
// uses mutex to ensure no concurrent access
type utxoStack struct {
	lock  sync.Mutex
	utxos []TXOutput
}

// create new stack
func NewStack() *utxoStack {
	return &utxoStack{lock: sync.Mutex{}, utxos: make([]TXOutput, 0)}
}

func (s *utxoStack) Copy(original *utxoStack) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.utxos = make([]TXOutput, len(original.utxos))
	copy(s.utxos, original.utxos)
}

// returns the size of the stack
func (s *utxoStack) Size() int {
	return len(s.utxos)
}

// push TXOutput to the stack
func (s *utxoStack) Push(utxo TXOutput) {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.utxos = append(s.utxos, utxo)
}

// returns TXOutpus last added to the stack if not empty
// if empty error is not nil
func (s *utxoStack) Pop() (TXOutput, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	l := len(s.utxos)
	if l == 0 {
		return TXOutput{}, errors.New("empty_utxoStack")
	}

	res := s.utxos[l-1]
	s.utxos = s.utxos[:l-1]
	return res, nil
}
