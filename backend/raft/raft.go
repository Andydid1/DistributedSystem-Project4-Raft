package raft

import (
	"fmt"
	"math/rand"
	"time"
)

// Return true if the number of alive followers is able to form a forum
func (svr *Server) IsForum(args EmptyArgument, reply *IsForumReply) error {
	totalFollower := len(svr.State.Connection)
	connectedCount := 0
	for _, connection := range svr.State.Connection {
		if connection {
			connectedCount += 1
		}
	}
	reply.IsForum = connectedCount >= int(totalFollower/2)
	return nil
}

// Method used to apply all commited-but-not-applied entries to state machine
func (svr *Server) ApplyToStateMachine(args EmptyArgument, reply *EmptyReply) error {
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

// Method for endpoints to handle AppendEntries requests
// AppendEntriesReply.Success is the result of AppendEntries, AppendEntriesRely.Term is the term of this endpoint
func (svr *Server) AppendEntries(args *AppendEntriesArguments, reply *AppendEntriesReply) error {
	// Reply false if term < currentTerm
	if args.Term < svr.State.CurrentTerm {
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
		// Refresh timeout
		svr.HeartBeatReceived <- true
	}
	// Reply false if log doesn’t contain an entry at prevLogIndex whose term matches prevLogTerm
	if len(svr.State.Logs) < args.PrevLogIndex+1 || svr.State.Logs[args.PrevLogIndex].Term != args.PrevLogTerm {
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

// Method for a leader to ensure all logs of a targeted follower is identical to the leader
// If logs are identical in the end, the follower id and true is sent to ConcensusArguments.ConcensusResult
func (svr *Server) Concensus(args ConcensusArguments, reply *EmptyReply) error {
	// Send the first request
	var appendEntriesReply AppendEntriesReply
	var appendEntriesArgs AppendEntriesArguments
	appendEntriesArgs = args.AppendEntriesArgs
	connected := call(svr.PeerIdAddresses[args.TargetId], "Server.AppendEntries", appendEntriesArgs, &appendEntriesReply)
	// If the connection is successful and the AppliedEntry failed because of log inconsistency, decrement and retry
	for connected && svr.State.IsLeader && !appendEntriesReply.Success && appendEntriesReply.Term <= svr.State.CurrentTerm && appendEntriesArgs.PrevLogIndex > 0 {
		fmt.Print("Resending Concensus to", args.TargetId)
		appendEntriesReply.Success = false
		appendEntriesArgs.PrevLogIndex -= 1
		appendEntriesArgs.PrevLogTerm = svr.State.Logs[appendEntriesArgs.PrevLogIndex].Term
		appendEntriesArgs.Entries = svr.State.Logs[appendEntriesArgs.PrevLogIndex+1:]
		_ = call(svr.PeerIdAddresses[args.TargetId], "Server.AppendEntries", appendEntriesArgs, &appendEntriesReply)
	}
	// Update connection status
	svr.RWMutex.Lock()
	svr.State.Connection[args.TargetId] = connected
	svr.RWMutex.Unlock()
	// If the final result is successful, update MatchIndex and NextIndex
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

// The method used to replicate the entry from client request to all followers
// The boolean indicating if majority of endpoints successfully replicated is sent to ReplicateArguments.MajorityAchieved
func (svr *Server) Replicate(args ReplicateArguments, reply *EmptyReply) error {
	// Add entry to own log
	svr.RWMutex.Lock()
	svr.State.Logs = append(svr.State.Logs, args.Entry)
	svr.RWMutex.Unlock()

	// Distribute entry to followers and keep followers concensus with self through Concensus
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
		resultReceived += 1
		if result.bool {
			successNum += 1
		}
		if svr.State.IsLeader && successNum == int(len(svr.PeerIdAddresses)/2) {
			// If majority is achieved
			if !resultSent {
				args.MajorityAchieved <- true
				resultSent = true
			}
		} else if !svr.State.IsLeader {
			// If self is no longer a leader (interupted by RequestVote or AppendEntries with higher term)
			if !resultSent {
				args.MajorityAchieved <- false
				resultSent = true
			}
		}
	}
	// If all results are received and majority is not achived, respond the result
	if !resultSent {
		args.MajorityAchieved <- false
	}
	return nil
}

// Method to handle request from frontend
// Reply.Result is true if the request is successful
func (svr *Server) HandleClientRequest(args HandleClientRequestArguments, reply *HandleClientRequestReply) error {
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
	fmt.Println(svr.Address, "Handling Request")

	if args.Method == "inventory" {
		// If it's a read request
		reply.Result = true
		reply.ResponseData = svr.Inventory.HandleInventory()
		return nil
	} else {
		// If it's not a read request
		// Reply failure if forum is not formed
		var isForumReply IsForumReply
		_ = svr.IsForum(FOOARGS, &isForumReply)
		if !isForumReply.IsForum {
			reply.Result = false
			return nil
		}

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
		// Commit and apply to state machine if replication is completed by majority of followers
		if achieved {
			// Commit
			svr.RWMutex.Lock()
			svr.State.CommitIndex = len(svr.State.Logs) - 1
			svr.RWMutex.Unlock()
			// Apply the entry to state machine
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

// Method for an endpoint to respond to RequestVote requests
func (svr *Server) RequestVote(args *RequestVoteArguments, reply *RequestVoteReply) error {
	// Reply false if term < currentTerm
	if args.Term < svr.State.CurrentTerm {
		reply.Term = svr.State.CurrentTerm
		reply.VoteGranted = false
		return nil
	}
	// If the candidate has higher term, stop being a candidate or a leader
	// Become a follower with no known leader, update the term to the newest
	if svr.State.CurrentTerm < args.Term {
		svr.RWMutex.Lock()
		svr.State.CurrentTerm = args.Term
		svr.State.VotedFor = -1
		svr.State.IsLeader = false
		svr.RWMutex.Unlock()
	}
	// If votedFor is null or candidateId, and candidate’s log is at least as up-to-date as receiver’s log, grant vote
	if (svr.State.VotedFor == -1 || svr.State.VotedFor == args.CandidateId) && (args.LastLogTerm >= svr.State.Logs[len(svr.State.Logs)-1].Term && args.LastLogIndex >= len(svr.State.Logs)-1) {
		svr.RWMutex.Lock()
		svr.State.VotedFor = args.CandidateId
		svr.RWMutex.Unlock()
		reply.Term = svr.State.CurrentTerm
		reply.VoteGranted = true
	} else {
		reply.Term = svr.State.CurrentTerm
		reply.VoteGranted = false
	}
	return nil
}

// Method for a follower to start an election and complete an election
func (svr *Server) Election(args EmptyArgument, reply *EmptyReply) error {
	//Update current term and vote for self
	svr.RWMutex.Lock()
	svr.State.CurrentTerm += 1
	svr.State.VotedFor = svr.Id
	svr.RWMutex.Unlock()

	// Set up SendRequestVote arguments and send to all endpoints
	VoteResults := make(chan bool)
	AbandonElection := make(chan bool)
	for _, address := range svr.PeerIdAddresses {
		var arguments SendRequestVoteArguments
		arguments.Address = address
		arguments.VoteResults = VoteResults
		arguments.AbandonElection = AbandonElection
		go svr.SendRequestVote(arguments, &FOOREPLY)
	}

	// Wait for results and count if the election wins
	successNum := 0
	electionResultGenerated := false
	// For each endpoint, either receive a vote result or a message to abandon the election (when the endpoint has higher term than candidate)
	for i := 0; i < len(svr.PeerIdAddresses); i++ {
		select {
		// If the candidate receive a vote result
		case success := <-VoteResults:
			if success {
				successNum += 1
			}
			// If the election is not abandoned, the candidate is still a candidate, and the number of success votes just got over majority
			if !electionResultGenerated && svr.State.VotedFor == svr.Id && successNum == int(len(svr.PeerIdAddresses)/2) {
				// Become a leader and update corresponding states
				fmt.Println("You become a leader with term", svr.State.CurrentTerm)
				var newNextIndex = make(map[int]int)
				var newMatchIndex = make(map[int]int)
				var newConnection = make(map[int]bool)
				for id, _ := range svr.PeerIdAddresses {
					newNextIndex[id] = 0
					newMatchIndex[id] = 0
					newConnection[id] = false
				}
				svr.RWMutex.Lock()
				svr.State.IsLeader = true
				// Refresh the timeout countdown of the candidate
				svr.HeartBeatReceived <- true
				svr.State.NextIndex = newNextIndex
				svr.State.MatchIndex = newMatchIndex
				svr.State.Connection = newConnection
				svr.RWMutex.Unlock()
				// Send HeartBeat informing all endpoints
				go svr.HeartBeat(FOOARGS, &FOOREPLY)
				electionResultGenerated = true
			}
			break
		// If the election is abandoned, become a follower without known leader
		case <-AbandonElection:
			svr.State.VotedFor = -1
			electionResultGenerated = true
			break
		}
	}
	return nil
}

// Method to send RequestVote to targeted endpoint
func (svr *Server) SendRequestVote(args SendRequestVoteArguments, reply *EmptyReply) error {
	// Set up arguments and call
	var requestVoteReply RequestVoteReply
	var requestVoteArguments RequestVoteArguments
	requestVoteArguments.Term = svr.State.CurrentTerm
	requestVoteArguments.CandidateId = svr.Id
	requestVoteArguments.LastLogIndex = len(svr.State.Logs) - 1
	requestVoteArguments.LastLogTerm = svr.State.Logs[len(svr.State.Logs)-1].Term
	connected := call(args.Address, "Server.RequestVote", requestVoteArguments, &requestVoteReply)
	// After receiving the result
	// If the endpoint receiving RequestVote has higher term than candidate, abandon the election
	// Else respond the vote result through channel
	if connected && requestVoteReply.Term > svr.State.CurrentTerm {
		fmt.Println("Abandon Election")
		args.AbandonElection <- true
	} else {
		fmt.Println("VoteGranted!", requestVoteReply.VoteGranted)
		args.VoteResults <- requestVoteReply.VoteGranted
	}
	return nil
}

// Method for leader to request all followers with Concensus call
// If a follower is alive, its content would be consisten with the leader after each HeartBeat
func (svr *Server) HeartBeat(EmptyArgument, *EmptyReply) error {
	// Create arguments for Concensus and call
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
	// Used to catch concensus result and avoid blocking
	for _, _ = range svr.PeerIdAddresses {
		<-ConsensusResult
	}
	return nil
}

// Run the Server
// If the Server is a leader, create a new goroutine and perform HeartBeat to all followers every 50 ms
// If the Server is not a leader, after a random amount of time between 150ms to 300ms without receiving
// a HeartBeat, start an election.
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

// Send the termination signal to stop the server
func (svr *Server) TerminateServer(EmptyArgument, *EmptyReply) error {
	svr.Terminate <- true
	return nil
}
