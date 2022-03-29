package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

const capacity = 2
const clients = 4
const difficulty = 5

// TODO:
// transfer funds to each connected client
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

	if *bootstrap {

		log.Printf("Starting bootstrap node in %s\n", *localAddr)

		thisNode.id = "id0"
		go thisNode.bootstrapStart(*localAddr)

		for {
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			words := strings.Fields(text)
			thisNode.cli(words)
		}

		// time.Sleep(time.Duration(1<<63 - 1))

	} else {

		log.Printf("Starting node in %s\n", *localAddr)
		log.Printf("Connecting to bootstrap at %s\n", *remoteAddr)

		go thisNode.nodeStart(*localAddr)
		go thisNode.connectionStart("id0", *remoteAddr)

		time.Sleep(time.Second * 15)
		thisNode.sendCoins("id0", 1)
		thisNode.sendCoins("id0", 1)
		thisNode.sendCoins("id0", 1)
		thisNode.sendCoins("id0", 1)
		// time.Sleep(time.Second * 20)
		// thisNode.validateChain()
		for {
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			words := strings.Fields(text)
			thisNode.cli(words)
		}

		// time.Sleep(time.Duration(1<<63 - 1))
	}
}
