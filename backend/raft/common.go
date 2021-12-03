package raft

import (
	"gw1035/project4/backend/models"
	"net/rpc"
	"sync"
)

var (
	TIMEOUTMILLIONSEC int = 500
	TIMEOUTMIN        int = 150
	TIMEOUTMAX        int = 300
	FOOREPLY          EmptyReply
	FOOARGS           EmptyArgument
)

type Server struct {
	Id                int
	Address           string
	HeartBeatReceived chan bool
	Terminate         chan bool
	PeerIdAddresses   map[int]string
	RWMutex           sync.RWMutex
	State             State
	Inventory         *models.Inventory
}

type State struct {
	CurrentTerm int
	VotedFor    int // nil if no leader is known(happens during election as non-candidate, or just started); self if candidate or leader
	Logs        []Entry
	IsLeader    bool

	CommitIndex int
	LastApplied int

	NextIndex  map[int]int
	MatchIndex map[int]int
	Connection map[int]bool
}

type Entry struct {
	Index     int
	Term      int
	Method    string
	Variables []byte
}

type AppendEntriesArguments struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []Entry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

type RequestVoteArguments struct {
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

type SendRequestVoteArguments struct {
	Address         string
	VoteResults     chan bool
	AbandonElection chan bool
}

type ConcensusArguments struct {
	TargetId        int
	ConcensusResult chan struct {
		int
		bool
	}
	AppendEntriesArgs AppendEntriesArguments
}

type ReplicateArguments struct {
	Entry            Entry
	MajorityAchieved chan bool
}

type HandleClientRequestArguments struct {
	Method    string
	Variables []byte
}

type HandleClientRequestReply struct {
	Result       bool
	ResponseData []byte
}

type IsForumReply struct {
	IsForum bool
}

type EmptyArgument struct{}

type EmptyReply struct{}

func call(address string, calling string, args interface{}, reply interface{}) bool {
	c, dialErr := rpc.DialHTTP("tcp", address)
	if dialErr != nil {
		// fmt.Println(dialErr.Error())
		return false
	}
	defer c.Close()

	callErr := c.Call(calling, args, reply)
	if callErr != nil {
		// fmt.Println(callErr.Error())
		return false
	}
	return true
}
