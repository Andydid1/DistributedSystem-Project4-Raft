package main

import (
	"fmt"
	"gw1035/project4/backend/models"
	"gw1035/project4/backend/raft"
	"net"
	"net/rpc"
	"strconv"
	"strings"
	"time"
)

var (
	listeningPort int
	backendInput  string
	idInput       string
)

// Initialize Server
func InitServer(id int, peerIdAddresses map[int]string) *raft.Server {
	Server := new(raft.Server)
	Server.Id = id
	Server.Address = "localhost:" + strconv.Itoa(listeningPort)
	Server.HeartBeatReceived = make(chan bool)
	Server.Terminate = make(chan bool)
	Server.PeerIdAddresses = peerIdAddresses
	Server.Inventory = InitInventory()
	Server.State.CurrentTerm = 0
	Server.State.VotedFor = -1
	fooEntry := new(raft.Entry)
	Server.State.Logs = append(Server.State.Logs, *fooEntry)
	Server.State.IsLeader = false
	Server.State.CommitIndex = 0
	Server.State.LastApplied = 0
	Server.State.MatchIndex = make(map[int]int)
	Server.State.NextIndex = make(map[int]int)
	return Server
}

// Initialize Inventory
func InitInventory() *models.Inventory {
	var Inventory = new(models.Inventory)
	appleItem := models.Item{Id: 1, ItemName: "Apple", ItemDescription: "A fresh apple just picked from the farm", UnitPrice: 2}
	orangeItem := models.Item{Id: 2, ItemName: "Orange", ItemDescription: "A fresh orange just picked from the farm", UnitPrice: 2}
	Inventory.Items = make(map[int]models.Item)
	Inventory.Items[1] = appleItem
	Inventory.Items[2] = orangeItem
	Inventory.NewId = 3
	return Inventory
}

func ProcessFlags() (int, string, map[int]string) {
	listeningPortStr := ":" + strconv.Itoa(listeningPort)
	backendInputSlice := strings.Split(backendInput, ",")
	for i, input := range backendInputSlice {
		if strings.Index(input, ":") == 0 {
			backendInputSlice[i] = "localhost" + input
		}
	}
	idInputSlice := strings.Split(idInput, ",")
	var idInputIntSlice []int
	for _, val := range idInputSlice {
		intVal, _ := strconv.Atoi(val)
		idInputIntSlice = append(idInputIntSlice, intVal)
	}
	var ServerSocket = make(map[int]string)
	ServerSocket[idInputIntSlice[1]] = backendInputSlice[0]
	ServerSocket[idInputIntSlice[2]] = backendInputSlice[1]
	return idInputIntSlice[0], listeningPortStr, ServerSocket
}

func RegisterAndListen(Server *raft.Server, address string) net.Listener {
	rpc.Register(Server)
	rpc.HandleHTTP()
	fmt.Println("Listening :" + address)
	l, e := net.Listen("tcp", address)
	if e != nil {
		fmt.Println(e)
	}
	return l
}

func Monitor(Server *raft.Server) {
	for {
		<-time.After(1 * time.Second)
		fmt.Println("====================")
		fmt.Println("Leader?", Server.State.IsLeader)
		fmt.Println("Term?", Server.State.CurrentTerm)
		fmt.Println("Log?", Server.State.Logs)
		fmt.Println("Inventory?", Server.Inventory)
		fmt.Println("Commit", Server.State.CommitIndex)
		fmt.Println("Applied", Server.State.LastApplied)
		fmt.Println("====================")
	}
}
