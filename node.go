package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

const clients = 5    // number of clients in system
const difficulty = 4 // number of nibbles (half bytes) to be zero on the start of a block's hash
const capacity = 5   // number of transactions in a block

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

	for {
		thisNode.blockchain_lock.Lock()
		if len(thisNode.blockchain) > 0 {
			thisNode.blockchain_lock.Unlock()
			break
		}
		thisNode.blockchain_lock.Unlock()
	}

	start_time := time.Now().Unix()

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

		end_time := time.Now().Unix()
		block_time := float32(end_time-start_time) / float32(len(thisNode.blockchain))
		fmt.Println("\n\n~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
		fmt.Println("Blocks in chain:", len(thisNode.blockchain))
		fmt.Println("in", end_time-start_time, "seconds")
		fmt.Println("Mean block time:", block_time, "seconds")
		fmt.Println("Throughput:", float32(capacity)/block_time)
		fmt.Println("~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~")
		fmt.Println("")
		fmt.Println("")
	}

	for {
		reader := bufio.NewReader(os.Stdin)
		text, _ := reader.ReadString('\n')
		words := strings.Fields(text)
		thisNode.cli(words)
	}
}
