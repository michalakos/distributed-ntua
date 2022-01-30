package main

import "crypto/rsa"

// compare rsa.PublicKey type variables
// return true if same
func compare(pk1, pk2 rsa.PublicKey) bool {
	if pk1.N.Cmp(pk2.N) == 0 && pk1.E == pk2.E {
		return true
	} else {
		return false
	}
}
