package main

import (
	"flag"
	"log"
	"time"
)

// TODO:
// create genesis block
// transfer funds to each connected client
func main() {
	var thisNode Node

	localAddr := flag.String("local", "localhost:50001", "local node's [IP:port]")
	remoteAddr := flag.String("remote", "localhost:50000", "bootstrap node's [IP:port]")
	bootstrap := flag.Bool("bootstrap", false, "declare node as bootstrap")
	flag.Parse()

	thisNode.Setup(*localAddr)

	if *bootstrap {

		log.Printf("Starting bootstrap node in %s\n", *localAddr)

		thisNode.id = "id0"
		thisNode.bootstrapStart(*localAddr)

		time.Sleep(time.Duration(1<<63 - 1))

	} else {

		log.Printf("Starting node in %s\n", *localAddr)
		log.Printf("Connecting to bootstrap at %s\n", *remoteAddr)

		go thisNode.nodeStart(*localAddr)
		thisNode.connectionStart("id0", *remoteAddr)

		time.Sleep(time.Duration(1<<63 - 1))
	}
}
