package main

import (
	"bufio"
	"flag"
	"fmt"
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
		go thisNode.collectTransactions()

		thisNode.id = "id0"
		go thisNode.bootstrapStart(*localAddr)

		for len(thisNode.blockchain) == 0 {
			continue
		}

		for {
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			words := strings.Fields(text)
			thisNode.cli(words)
		}

	} else {
		go thisNode.collectTransactions()

		go thisNode.nodeStart(*localAddr)
		go thisNode.connectionStart("id0", *remoteAddr)

		for len(thisNode.blockchain) == 0 {
			continue
		}
		time.Sleep(time.Second * 20 * clients)

		for {
			reader := bufio.NewReader(os.Stdin)
			text, _ := reader.ReadString('\n')
			words := strings.Fields(text)
			fmt.Println(words)
			thisNode.cli(words)
		}
	}
}
