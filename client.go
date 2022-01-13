package main

import (
	"bufio"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
)

const maxNodes int = 5
const bitSize = 4096

type transaction struct {
	sender_address   rsa.PublicKey
	receiver_address rsa.PublicKey
	amount           uint
	transaction_id   crypto.Hash
	// transaction_inputs  transactionInput
	// transaction_outputs transactionOutput
	signature []byte
}

func main() {
	var id string
	var bootstrap bool = false
	var privateKey *rsa.PrivateKey
	var publicKey *rsa.PublicKey

	privateKey, publicKey = generateKeyPair()
	fmt.Println(privateKey, publicKey)

	// check input arguments
	arguments := os.Args[1:]
	if len(arguments) != 2 {
		log.Println("Please provide client ip:port and bootstrap ip:port")
		log.Println("If this is the bootstrap node the two addresses should be identical")
	}

	// is this the bootstrap node
	if arguments[0] == arguments[1] {
		bootstrap = true
		log.Println("This is the bootstrap node")
	}

	// open listening port
	listenAddress := arguments[0]
	listener, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Println(err)
		return
	}
	defer listener.Close()

	// print address of listening port
	localAddr := listener.Addr().String()
	log.Printf("Listening on %s\n", localAddr)

	// client node code
	if !bootstrap {
		bootstrapAddr := arguments[1]
		id = clientEntry(bootstrapAddr, localAddr)
		log.Println("This is node ", id)
	}

	// bootstrap node code
	if bootstrap {
		bootstrapWelcoming(listener)
	}

}

func bootstrapWelcoming(listener net.Listener) {
	// id counter to assign number to nodes
	currId := 0

	// welcome clients until number of nodes in system
	// equals maxNodes
	for currId < maxNodes-1 {
		currId++

		// accept connections
		conn, err := listener.Accept()
		if err != nil {
			log.Println(err)
			return
		}

		// send id to connected node
		fmt.Fprintf(conn, strconv.Itoa(currId)+"\n")
		nodeAddress, _ := bufio.NewReader(conn).ReadString('\n')

		// dubugging information
		log.Println("\nIncomming message from id", currId)
		log.Printf("Node %d address is %s", currId, nodeAddress)
	}
	log.Println("All nodes connected")
}

func clientEntry(bootstrapAddr string, localAddr string) string {
	// connect with bootstrap node
	log.Println("Connecting to bootstrap node", bootstrapAddr)
	conn, err := net.Dial("tcp", bootstrapAddr)
	if err != nil {
		log.Fatal(err.Error())
	}

	// read node's id and send address
	id, _ := bufio.NewReader(conn).ReadString('\n')
	conn.Write([]byte(localAddr + "\n"))

	conn.Close()

	return id
}

func generateKeyPair() (*rsa.PrivateKey, *rsa.PublicKey) {
	// generate private key
	privateKey, err := rsa.GenerateKey(rand.Reader, bitSize)
	if err != nil {
		log.Fatal(err.Error())
	}
	err = privateKey.Validate()
	if err != nil {
		log.Fatal(err.Error())
	}
	log.Println("Private key generated")
	privateKey.Precompute()

	publicKey := &privateKey.PublicKey

	return privateKey, publicKey
}
