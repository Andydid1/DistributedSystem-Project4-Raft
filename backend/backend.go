package main

import (
	"flag"
	"gw1035/project4/backend/raft"
	"net/http"
)

func init() {
	// Retrieve the listen input
	flag.IntVar(&listeningPort, "listen", 8090, "listen")
	flag.StringVar(&backendInput, "backend", "localhost:8091,localhost:8092", "backend")
	flag.StringVar(&idInput, "id", "1,2,3", "id")
	flag.Parse()
}

func main() {
	thisId, thisAddress, peerIdAddresses := ProcessFlags()
	Server := InitServer(thisId, peerIdAddresses)
	Listener := RegisterAndListen(Server, thisAddress)
	// Serve the listener in new goroutine
	go http.Serve(Listener, nil)
	// Run the Server in new goroutine
	go Server.Run(raft.FOOARGS, &raft.FOOREPLY)
	// Run monitor in new goroutine, comment it out if not needed
	go Monitor(Server)
	// Wait until Server Termination is called
	<-Server.Terminate
}
