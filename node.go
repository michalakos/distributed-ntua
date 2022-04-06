package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"strings"
)

const capacity = 5
const clients = 4
const difficulty = 2

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Try './node.exe -h' for help")
		os.Exit(0)
	}

	var thisNode Node

	localAddr := flag.String("local", "localhost:50001", "local node's [IP:port]")
	remoteAddr := flag.String("remote", "localhost:50000", "bootstrap node's [IP:port]")
	bootstrap := flag.Bool("bootstrap", false, "declare node as bootstrap")

	flag.Parse()

	thisNode.Setup(*localAddr)
	go thisNode.collectTransactions()

	if *bootstrap {

		thisNode.id = "id0"
		go thisNode.bootstrapStart(*localAddr)

		for len(thisNode.blockchain) == 0 {
			continue
		}

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

		for {
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			words := strings.Fields(text)
			thisNode.cli(words)
		}

	} else {
		// go thisNode.collectTransactions()

		go thisNode.nodeStart(*localAddr)
		go thisNode.connectionStart("id0", *remoteAddr)

		// time.Sleep(time.Second * 30)
		// thisNode.unspentUtxos()

		for len(thisNode.blockchain) < clients+1 {
			continue
		}

		thisNode.sendCoins("id0", 1)
		// thisNode.sendCoins("id0", 2)
		// thisNode.sendCoins("id0", 3)
		// thisNode.sendCoins("id0", 4)
		// thisNode.sendCoins("id0", 5)

		for {
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			words := strings.Fields(text)
			thisNode.cli(words)
		}
	}
}
