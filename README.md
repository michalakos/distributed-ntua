# distributed-ntua
Simple blockchain based cryptocurrency developed for Distributed Systems course at NTUA during 2021-22 year.

Requirements:
* Go 1.17

Steps:
* Run `make clean; make all` to create the executables.
* Start the bootstrap node by running <br>`node.exe -bootstrap -local ip_address:port -file transactions_file`<br>where `ip_address` and `port` are the network address to the port on which the other nodes will connect with this one, and `transactions_file` is a file containing a new transaction in each line in format `idx y`, where *idx* is the node to which coins are sent and *y* is the number of coins sent. The -file option is not necessary as a terminal is also provided.
* Start the rest nodes until all the clients in the system are equal to the `clients` constant on the `node.go` file. To start them run<br>`./node.exe -local own_ip_address:own_port -remote bootstrap_ip_address:bootstrap_port -file transaction_file`<br>The `own_ip_address` and `own_port` options are the network address on which other nodes can contact this node and `bootstrap_ip_address` and `bootstrap_port` is the network address specified when running the bootstrap node.

Cli:
* A cli environment is provided from where transactions can be created, and information about the latest block's transactions and the remaining coins in its wallet can be accessed. Try `help` on the command line for more information.