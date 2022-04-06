package main

import (
	"fmt"
	"log"
	"strconv"
)

func (n *Node) cli(words []string) {
	if len(words) == 3 && words[0] == "t" {
		if _, ok := n.neighborMap[words[1]]; !ok {
			fmt.Println("No client with id", words[1])
			n.help()
			return
		}

		parsed, err := strconv.ParseUint(words[2], 10, 32)
		if err != nil {
			log.Println("cli: ParseUint", err)
			n.help()
			return
		}
		amount := uint(parsed)

		n.transaction(words[1], amount)

	} else if len(words) == 1 {
		if words[0] == "view" {
			n.view()
		} else if words[0] == "balance" {
			n.balance()
		} else if words[0] == "help" {
			n.help()
		} else if words[0] == "txs" {
			n.showTransactions()
		} else if words[0] == "all" {
			n.all_balances()
		} else if words[0] == "hashes" {
			n.hashes()
		} else {
			n.help()
		}
	} else {
		n.help()
	}
}

func (n *Node) transaction(id string, amount uint) {
	n.sendCoins(id, amount)
}

func (n *Node) view() {
	n.viewTransactions()
}
func (n *Node) balance() {
	fmt.Println("\nWallet balance is:", n.walletBalance(n.id))
}
func (n *Node) help() {
	fmt.Println("\nsupported commands:")
	fmt.Println("")

	// transaction help
	fmt.Println("t <recipient_id> <amount>")
	fmt.Println("\tsends <amount> coins to client with <recipient_id>")
	fmt.Println("\t\t<amount>: uint")
	fmt.Println("\t\t<recipient_id>: string (e.g. id0, id1, id2...)")
	fmt.Println("")

	// view help
	fmt.Println("view")
	fmt.Println("\tprints the details of all transactions in the last block")
	fmt.Println("")

	// balance help
	fmt.Println("balance")
	fmt.Println("\tprints the client's remaining coins")
	fmt.Println("")

	// help help
	fmt.Println("help")
	fmt.Println("\thelp about cli commands")
}

func (n *Node) all_balances() {
	fmt.Println("\nThe balances of all wallets are:")
	for id := range n.neighborMap {
		fmt.Println(id, n.walletBalance(id))
	}
}

func (n *Node) hashes() {
	fmt.Println("\nThe hashes of the blocks in the blockchain are:")
	n.blockchain_lock.Lock()
	for _, bl := range n.blockchain {
		fmt.Println("Block", bl.Index, "with hash", bl.Hash)
	}
	n.blockchain_lock.Unlock()
}
