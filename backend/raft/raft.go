package raft

import (
	"fmt"
	"math/rand"
	"time"
)

func (svr *Server) ApplyToStateMachine(args EmptyArgument, reply *EmptyReply) error {
	fmt.Println("Applying to StateMachine")
	for svr.State.LastApplied < svr.State.CommitIndex {
		svr.State.LastApplied += 1
		entry := svr.State.Logs[svr.State.LastApplied]
		fmt.Println()
		svr.RWMutex.Lock()
		err := svr.Inventory.HandleNonRead(entry.Method, entry.Variables)
		if err == nil {
			svr.RWMutex.Unlock()
		} else {
			svr.State.LastApplied -= 1
			svr.RWMutex.Unlock()
			break
		}
	}
	return nil
}

func (svr *Server) AppendEntries(args *AppendEntriesArguments, reply *AppendEntriesReply) error {
	// Check concensus status
	if args.Term < svr.State.CurrentTerm {
		// reply false
		reply.Term = svr.State.CurrentTerm
		reply.Success = false
		return nil
	} else {
		svr.RWMutex.Lock()
		// become follower
		svr.State.VotedFor = args.LeaderId
		svr.State.IsLeader = false
		// Update term if needed
		svr.State.CurrentTerm = args.Term
		svr.RWMutex.Unlock()
		svr.HeartBeatReceived <- true
	}
	if len(svr.State.Logs) < args.PrevLogIndex+1 || svr.State.Logs[args.PrevLogIndex].Term != args.PrevLogTerm {
		// reply false
		reply.Term = svr.State.CurrentTerm
		reply.Success = false
		return nil
	}
	// Append entries (delete existing entries if needed)
	svr.RWMutex.Lock()
	svr.State.Logs = append(svr.State.Logs[:args.PrevLogIndex+1], args.Entries...)
	// Update Commit
	svr.State.CommitIndex = args.LeaderCommit
	svr.RWMutex.Unlock()
	// Apply to state machine
	if svr.State.LastApplied < svr.State.CommitIndex {
		svr.ApplyToStateMachine(FOOARGS, &FOOREPLY)
	}
	reply.Term = svr.State.CurrentTerm
	reply.Success = true
	return nil
}

func (svr *Server) Concensus(args ConcensusArguments, reply *EmptyReply) error {
	var appendEntriesReply AppendEntriesReply
	var appendEntriesArgs AppendEntriesArguments
	appendEntriesArgs = args.AppendEntriesArgs
	connected := call(svr.PeerIdAddresses[args.TargetId], "Server.AppendEntries", appendEntriesArgs, &appendEntriesReply)
	for connected && svr.State.IsLeader && !appendEntriesReply.Success && appendEntriesReply.Term <= svr.State.CurrentTerm && appendEntriesArgs.PrevLogIndex > 0 {
		fmt.Print("Resending Concensus to", args.TargetId)
		appendEntriesReply.Success = false
		appendEntriesArgs.PrevLogIndex -= 1
		appendEntriesArgs.PrevLogTerm = svr.State.Logs[appendEntriesArgs.PrevLogIndex].Term
		appendEntriesArgs.Entries = svr.State.Logs[appendEntriesArgs.PrevLogIndex+1:]
		_ = call(svr.PeerIdAddresses[args.TargetId], "Server.AppendEntries", appendEntriesArgs, &appendEntriesReply)
	}
	if appendEntriesReply.Success {
		svr.RWMutex.Lock()
		svr.State.MatchIndex[args.TargetId] = len(svr.State.Logs) - 1
		svr.State.NextIndex[args.TargetId] = len(svr.State.Logs)
		svr.RWMutex.Unlock()
	}
	args.ConcensusResult <- struct {
		int
		bool
	}{args.TargetId, appendEntriesReply.Success}
	return nil
}

func (svr *Server) Replicate(args ReplicateArguments, reply *EmptyReply) error {
	// Add entry to own log
	svr.RWMutex.Lock()
	svr.State.Logs = append(svr.State.Logs, args.Entry)
	svr.RWMutex.Unlock()

	// Distribute entry to followers
	var completedFollowers = make(map[int]bool)
	var ConsensusResult = make(chan struct {
		int
		bool
	})
	var appendEntriesArgs AppendEntriesArguments
	appendEntriesArgs.Term = svr.State.CurrentTerm
	appendEntriesArgs.LeaderId = svr.Id
	appendEntriesArgs.PrevLogIndex = len(svr.State.Logs) - 2
	appendEntriesArgs.PrevLogTerm = svr.State.Logs[len(svr.State.Logs)-2].Term
	appendEntriesArgs.Entries = append(appendEntriesArgs.Entries, args.Entry)
	appendEntriesArgs.LeaderCommit = svr.State.CommitIndex
	for followerId, _ := range svr.PeerIdAddresses {
		completedFollowers[followerId] = false
		var concensusArgs ConcensusArguments
		concensusArgs.TargetId = followerId
		concensusArgs.ConcensusResult = ConsensusResult
		concensusArgs.AppendEntriesArgs = appendEntriesArgs
		go svr.Concensus(concensusArgs, &FOOREPLY)
	}

	// Wait for responses, send result if majority of all servers replicate the logs
	resultReceived := 0
	successNum := 0
	resultSent := false
	for resultReceived < len(svr.PeerIdAddresses) {
		result := <-ConsensusResult
		fmt.Println("Result received from", result.int, result.bool)
		completedFollowers[result.int] = true
		resultReceived += 1
		if result.bool {
			successNum += 1
		}
		if svr.State.IsLeader && successNum == int(len(svr.PeerIdAddresses)/2) {
			if !resultSent {
				args.MajorityAchieved <- true
				resultSent = true
			}
		} else if !svr.State.IsLeader {
			if !resultSent {
				args.MajorityAchieved <- false
				resultSent = true
			}
		}
	}
	fmt.Println("Replication Complete")
	return nil
}

func (svr *Server) HandleClientRequest(args HandleClientRequestArguments, reply *HandleClientRequestReply) error {
	fmt.Println(svr.Address, "Handling Request")
	// If no leader is known, wait for a bit and redirect to self
	if svr.State.VotedFor == -1 {
		time.Sleep(50 * time.Millisecond)
		_ = call(svr.Address, "Server.HandleClientRequest", args, reply)
		return nil
	}
	// If current server is not leader, direct the request to leader
	if svr.State.VotedFor != svr.Id {
		_ = call(svr.PeerIdAddresses[svr.State.VotedFor], "Server.HandleClientRequest", args, reply)
		return nil
	}
	// If a candidate is not yet a leader, wait for abit and redirect to self
	if svr.State.VotedFor == svr.Id && !svr.State.IsLeader {
		time.Sleep(50 * time.Millisecond)
		_ = call(svr.Address, "Server.HandleClientRequest", args, reply)
		return nil
	}

	if args.Method == "inventory" {
		// If it's a read request
		reply.Result = true
		reply.ResponseData = svr.Inventory.HandleInventory()
		return nil
	} else {
		// If it's not a read request
		// Replicate the log entry to self and all followers
		var majorityAchieved = make(chan bool)
		var entry Entry
		entry.Index = len(svr.State.Logs)
		entry.Term = svr.State.CurrentTerm
		entry.Method = args.Method
		entry.Variables = args.Variables
		var replicateArgs ReplicateArguments
		replicateArgs.MajorityAchieved = majorityAchieved
		replicateArgs.Entry = entry
		go svr.Replicate(replicateArgs, &FOOREPLY)
		// Wait for response
		achieved := <-majorityAchieved
		// Commit and reply
		if achieved {
			// Commit
			svr.RWMutex.Lock()
			svr.State.CommitIndex = len(svr.State.Logs) - 1
			svr.RWMutex.Unlock()
			// Apply to state machine
			svr.ApplyToStateMachine(FOOARGS, &FOOREPLY)
			// Reply Success
			reply.Result = true
		} else {
			// Reply Error
			reply.Result = false
		}
		return nil
	}
}

func (svr *Server) RequestVote(args *RequestVoteArguments, reply *RequestVoteReply) error {
	if args.Term < svr.State.CurrentTerm {
		// fmt.Println("Condition 1")
		reply.Term = svr.State.CurrentTerm
		reply.VoteGranted = false
		return nil
	}
	if svr.State.CurrentTerm < args.Term {
		// fmt.Println("Condition 2")
		svr.RWMutex.Lock()
		svr.State.CurrentTerm = args.Term
		svr.State.VotedFor = -1
		svr.State.IsLeader = false
		svr.RWMutex.Unlock()
	}
	if (svr.State.VotedFor == -1 || svr.State.VotedFor == args.CandidateId) && (args.LastLogTerm >= svr.State.Logs[len(svr.State.Logs)-1].Term && args.LastLogIndex >= len(svr.State.Logs)-1) { //candidate up to date:
		// fmt.Println("Condition 3")
		svr.RWMutex.Lock()
		svr.State.VotedFor = args.CandidateId
		svr.RWMutex.Unlock()
		reply.Term = svr.State.CurrentTerm
		reply.VoteGranted = true
	} else {
		// fmt.Println("Condition 4")
		reply.Term = svr.State.CurrentTerm
		reply.VoteGranted = false
	}
	return nil
}

func (svr *Server) Election(args EmptyArgument, reply *EmptyReply) error {
	//Update current term
	svr.RWMutex.Lock()
	svr.State.CurrentTerm += 1
	svr.State.VotedFor = svr.Id
	svr.RWMutex.Unlock()

	VoteResults := make(chan bool)
	AbandonElection := make(chan bool)
	for _, address := range svr.PeerIdAddresses {
		var arguments SendRequestVoteArguments
		arguments.Address = address
		arguments.VoteResults = VoteResults
		arguments.AbandonElection = AbandonElection
		go svr.SendRequestVote(arguments, &FOOREPLY)
	}
	successNum := 0
	electionResultGenerated := false
	for i := 0; i < len(svr.PeerIdAddresses); i++ {
		select {
		case success := <-VoteResults:
			if success {
				successNum += 1
			}
			if !electionResultGenerated && svr.State.VotedFor == svr.Id && successNum == int(len(svr.PeerIdAddresses)/2) {
				fmt.Println("A leader with term", svr.State.CurrentTerm)
				svr.RWMutex.Lock()
				svr.State.IsLeader = true
				var newNextIndex = make(map[int]int)
				var newMatchIndex = make(map[int]int)
				svr.State.NextIndex = newNextIndex
				svr.State.MatchIndex = newMatchIndex
				svr.RWMutex.Unlock()
				go svr.HeartBeat(FOOARGS, &FOOREPLY)
				electionResultGenerated = true
			}
			break
		case <-AbandonElection:
			svr.State.VotedFor = -1
			electionResultGenerated = true
			break
		}
	}
	fmt.Println("Election End with success", successNum)
	return nil
}

func (svr *Server) SendRequestVote(args SendRequestVoteArguments, reply *EmptyReply) error {
	var requestVoteReply RequestVoteReply
	var requestVoteArguments RequestVoteArguments
	requestVoteArguments.Term = svr.State.CurrentTerm
	requestVoteArguments.CandidateId = svr.Id
	requestVoteArguments.LastLogIndex = len(svr.State.Logs) - 1
	requestVoteArguments.LastLogTerm = svr.State.Logs[len(svr.State.Logs)-1].Term
	connected := call(args.Address, "Server.RequestVote", requestVoteArguments, &requestVoteReply)

	if connected && requestVoteReply.Term > svr.State.CurrentTerm {
		fmt.Println("Abandon Election")
		args.AbandonElection <- true
	} else {
		fmt.Println("VoteGranted!", requestVoteReply.VoteGranted)
		args.VoteResults <- requestVoteReply.VoteGranted
	}
	return nil
}

func (svr *Server) HeartBeat(EmptyArgument, *EmptyReply) error {
	var ConsensusResult = make(chan struct {
		int
		bool
	})
	var appendEntriesArgs AppendEntriesArguments
	appendEntriesArgs.Term = svr.State.CurrentTerm
	appendEntriesArgs.LeaderId = svr.Id
	appendEntriesArgs.PrevLogIndex = len(svr.State.Logs) - 1
	appendEntriesArgs.PrevLogTerm = svr.State.Logs[len(svr.State.Logs)-1].Term
	appendEntriesArgs.LeaderCommit = svr.State.CommitIndex
	for followerId, _ := range svr.PeerIdAddresses {
		var concensusArgs ConcensusArguments
		concensusArgs.TargetId = followerId
		concensusArgs.ConcensusResult = ConsensusResult
		concensusArgs.AppendEntriesArgs = appendEntriesArgs
		go svr.Concensus(concensusArgs, &FOOREPLY)
	}
	for _, _ = range svr.PeerIdAddresses {
		<-ConsensusResult
	}
	return nil
}

func (svr *Server) Run(EmptyArgument, *EmptyReply) error {
	for {
		if svr.State.IsLeader {
			<-time.After(50 * time.Millisecond)
			go svr.HeartBeat(FOOARGS, &FOOREPLY)
		} else {
			rand.Seed(time.Now().UnixNano())
			randomNum := int64(rand.Intn(TIMEOUTMAX-TIMEOUTMIN+1) + TIMEOUTMIN)
			select {
			case <-svr.HeartBeatReceived:
				break
			case <-time.After(time.Duration(randomNum) * time.Millisecond):
				go svr.Election(FOOARGS, &FOOREPLY)
				break
			}
		}
	}
}

func (svr *Server) TerminateServer(EmptyArgument, *EmptyReply) error {
	svr.Terminate <- true
	return nil
}
