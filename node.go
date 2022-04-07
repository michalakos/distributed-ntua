package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

const capacity = 5   // number of transactions in a block
const clients = 5    // number of clients in system
const difficulty = 4 // number of nibbles (half bytes) to be zero on the start of a block's hash

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Try './node.exe -h' for help")
		os.Exit(0)
	}

	var thisNode Node

	localAddr := flag.String("local", "localhost:50001", "local node's [IP:port]")
	remoteAddr := flag.String("remote", "localhost:50000", "bootstrap node's [IP:port]")
	inputFile := flag.String("file", "none", "file containing node's transactions")
	bootstrap := flag.Bool("bootstrap", false, "declare node as bootstrap")

	flag.Parse()

	thisNode.Setup(*localAddr)

	go thisNode.collectTransactions()

	if *bootstrap {
		thisNode.id = "id0"
		go thisNode.bootstrapStart(*localAddr)
	} else {
		go thisNode.nodeStart(*localAddr)
		go thisNode.connectionStart("id0", *remoteAddr)
	}

	// wait until all clients have 100 coins
	for len(thisNode.blockchain) < clients {
		continue
	}

	// read transactions from given file
	if *inputFile != "none" {

		f, err := os.Open(*inputFile)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)

		// scan every line and create a transaction
		for scanner.Scan() {
			text := scanner.Text()
			words := strings.Fields(text)

			if len(words) < 2 {
				break
			}

			// does id match a neighbor?
			if _, ok := thisNode.neighborMap[words[0]]; !ok {
				fmt.Println("No client with id", words[0])
				break
			}

			// check if second argument is a number
			parsed, err := strconv.ParseUint(words[1], 10, 32)
			if err != nil {
				log.Println("cli: ParseUint", err)
				break
			}
			amount := uint(parsed)

			thisNode.transaction(words[0], amount)
		}

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}
	}

	for {
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		words := strings.Fields(text)
		thisNode.cli(words)
	}
}

// test for resolveConflict()

// for {
// 	thisNode.blockchain_lock.Lock()
// 	chain_len := len(thisNode.blockchain)
// 	thisNode.blockchain_lock.Unlock()

// 	if chain_len == 9 {
// 		thisNode.blockchain_lock.Lock()

// 		receivedBlock := thisNode.blockchain[6]

// 		copied_blockchain := make([]Block, 0)
// 		for i := 0; i < 3; i++ {
// 			copied_blockchain = append(copied_blockchain, thisNode.blockchain[i])
// 		}
// 		thisNode.blockchain = make([]Block, 3)
// 		copy(thisNode.blockchain, copied_blockchain)

// 		thisNode.validateBlock(receivedBlock)

// 		thisNode.blockchain_lock.Unlock()

// 		break
// 	}
// }
