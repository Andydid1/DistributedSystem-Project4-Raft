DistributedSystem-Project4-Raft

Creater: Guandi Wang
Description:
    This is the repo of the implementation of a reliable, redundant, distributed web application. The application
is separated into front end and back end. No data is stored in the front end. All data is stored in memory-based 
data structures in the back end. The back end ensures consistency of multiple replicas with Raft Algorithm. 
Instruction for building, running, and testing the program:
    For backend:
        Command Arguments: 
            --listen     The port this server is running on (e.g 8090)
            --backend    A comma-separated list of endpoints (e.g localhost:8091,:8092)
            --id         A comma-separated list of endpoints' corresponding ids. The first input should be the id 
                         for this endpoint (e.g 1,2,3)
        Instruction for building and running:
            Assume you are in this directory where README.txt is located, use instruction:
                cd backend
                go run ./ --listen 8090 --backend localhost:8091,localhost:8092 --id 1,2,3
                go run ./ --listen 8091 --backend localhost:8090,localhost:8092 --id 2,1,3
                go run ./ --listen 8092 --backend localhost:8090,localhost:8092 --id 3,1,2
            Notice:
                *** remember to use go run ./ instead of go run ./backend.go, otherwise compliation would fail.
                The sequence of ids needs to be corresponding to "(id for this endpoint),(id for endpoint of first 
                backend argument),(id for endpoint of second backend argument)"
                You should open a new command prompt for each backend setup.
                The system is capable of operating on more backend endpoints as long as the arguments are set up properly
    For frontend:
        Command Arguments: 
            --listen     The port this server is running on (e.g 8080)
            --backend    A comma-separated list of endpoints (e.g localhost:8090,:8091,:8092)
        Instruction for building and running:
            Assume you are in this directory where README.txt is located, use instruction:
                cd frontend
                go run ./frontend.go --listen 8080 --backend localhost:8090,localhost:8091,localhost:8092
            Notice:
                The sequence of backends arguments is arbitrary
    Instruction for testing:
        After setting up backends and front end, access the port of frontend (localhost:8080 in the example above), and play with it.
        Currently, a Monitor function is implemented to print the current backend state to stdout ever second. This can be referred to
        when testing.
The state of work:
    The assignment is completed. The backend operates strictly with Raft Algorithm according to related papers.
Resources used in development:
    Referred to the principal of Raft Algorithm in related papers(https://pdos.csail.mit.edu/6.824/papers/raft-extended.pdf)
    Utilized iris web framework for frontend
    Utilized net/http and net/rpc for communication between backends and communication between frontend and backends
    Utilized multiple built-in Golang packages for implementation
Program's Resilience:

    TestCase 1: Given a cluster consisting of n back end replicas, your program should withstand failure of up to (n−1)/2 replicas. 
    Your program will be tested as follows: some number of back ends will be started, then data will be entered through the web 
    interface, then one or more of the back ends will be forcibly terminated, simulating a crash. The web interface should remain 
    responsive and the data previously entered should remain available, as long as a quorum of back ends remain functioning.

        After setting up the system with 3 backend endpoints, the system operates as intended. By terminating one of the backend
    endpoints, the system operates normally regardless of if the terminated endpoint was a leader or not. The test case passed.
        Two situation could have been generated after a backend endpoint terminated:
            1. The endpoint was a follower before terminated, the remaining number of endpoints still forms a forum, backend is able
            to operate without error since leader keeps sending heartbeat to the remaining follower and keeps syncronizing when accepted
            request from frontend. In my implementation, frontend would loop over the known backend endpoints and send one request at
            a time. If the endpoints requested failed(terminated), the frontend would send request to next backend endpoint until a successful
            reply. When the request is sent to a non-leader backend, that endpoint would redirect the request to known leader, thus a
            request can be completed by the leader.
            2. The endpoint was a leader before terminated, the remaining number of endpoints still forms a forum, a new leader could 
            be generated through election after timeout for any follower since they are not receiving HeartBeat message from a leader.
            After the election, the backend is able to operate normally. Notice that when frontend send a request to a candidate or follower
            when no leader is known, my implementation would block the goroutine for handling the request for 50ms and redirect the request to itself.
            The redirect would keep going until a new leader is known by the endpoint, when the endpoint would process the request as a new 
            leader or redirect the request to the new leader.
    
    TestCase 2: After terminating up to (n-1)/2 replicas, some of them will be restarted, using the same command that was used to start them 
    initially. The newly restarted replicas should synchronize data with the cluster, so that all running back ends have the same data.

        After terminating 1 of the 3 backend endpoints and restarting the terminated one, the re-connected endpoint would receive HeartBeat
    from leader in the existing backend cluster. Upon receiving HeartBeat, the endpoint would become the follower of that leader and start
    syncronizing with the leader. The endpoint cluster would come to consistent state quickly.
        It is also possible that the backend endpoint timeout before receiving a HeartBeat and start a election. The result of the election.
    However, the election would come to final result either by receiving a HeartBeat from existing leader to become a follower, or by receiving
    enough votes to become a new leader, or by restarting a new election after timeout, depending on the situation. Eventually, the cluster would
    come to a consistent state and process the requests properly.

    TestCase 3: After restarting all previously terminated replicas, disjoint sets of back end replicas will be forcibly terminated and restarted, 
    until all replicas have been terminated and restarted. Your application should continue working normally during this process.

        Since Election, HeartBeat, and Concensus (An RPC Method I implemented for a leader to enforce a fllower to come to concensus through multiple
    AppendEntries RPC call) are all operates in new goroutines, and that the frontend requests are redirected dynamically in backend endpoints as
    mentioned before, the application would work normally as long as (n-1)/2 replicas are online, although slight delay might be detected during elections,
    before a new leader is generated.
    
    TestCase 4: More than (n-1)/2 replicas will be forcibly terminated simultaneously. At that point, writes initiated from the front end should fail.

        My implementation of a leader maintains a map which records which nodes are alive. The information is updated every HeartBeat. Then, every time a
    leader tries to handle a write function, the leader first check if number of followers alive is enough. If the number is not sufficient to form a forum,
    the leader would respond with fail immediately. If no leader remain in the cluster after more than (n-1)/2 replicas eliminated, the frontend request would
    timeout and fail. 

    TestCase 5:
    Terminated replicas will be restarted, at which point they should synchronize and normal operation of the cluster should resume. A brief delay is acceptable
     while replicas synchronize.
    
        When a forum is not able to form due to the number of endpoints falls below (n-1)/2, two condition would occur:
            1. A leader is still running in the cluster, it will constantly tries to send HeartBeat to endpoints. Whenever a terminated endpoint restarted and
        receive the HeartBeat and responded, the leader would know. Whenever, the number of follower is sufficient to form a forum, the leader would resume 
        write services.
            2. No leader is left in the cluster, all endpoint would try to election but fails constantly and repeatedly, until the number of endpoints is sufficient
        to come up with a leader. The leader would thus start handling write requests.

Design Decision Exmplaination:
    1. I used Raft Algorithm as my replication strategy. The reason is that it is easier to understand and implement. Moreover, according to the papers, the performance
    of Raft Algorithm is similar to Paxos. In the end, the algorithm fits my application well.
    2. The pros of Raft Algorithm is that it is easy to understand and implement. The cons of Raft is availability exchanged for consistency.
    3. My application will not be blocked while waiting to receive messages. As mentioned before, most of my methods would create a new goroutine to avoid blocking. Moreover,
    all the RPC are served with http.Serve, which "accepts incoming HTTP connections on the listener l, creating a new service goroutine for each. The service goroutines read 
    requests and then call handler to reply to them". Therefore, connection from frontend to backend would not block the operation of backend. Moreover, my Server.Run() would
    also create new goroutine when trying to send a HeartBeat or start an Election.