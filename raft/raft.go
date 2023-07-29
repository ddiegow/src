package raft

//
// this is an outline of the API that raft must expose to
// the service (or tester). see comments below for
// each of these functions for more details.
//
// rf = Make(...)
//   create a new Raft server.
// rf.Start(command interface{}) (index, term, isleader)
//   start agreement on a new log entry
// rf.GetState() (term, isLeader)
//   ask a Raft for its current term, and whether it thinks it is leader
// ApplyMsg
//   each time a new entry is committed to the log, each Raft peer
//   should send an ApplyMsg to the service (or tester)
//   in the same server.
//

import (
	"math/rand"
	"mit_distributed_systems/labrpc"
	"sync"
	"sync/atomic"
	"time"
)

// import "bytes"
// import "../labgob"

// as each Raft peer becomes aware that successive log entries are
// committed, the peer should send an ApplyMsg to the service (or
// tester) on the same server, via the applyCh passed to Make(). set
// CommandValid to true to indicate that the ApplyMsg contains a newly
// committed log entry.
//
// in Lab 3 you'll want to send other kinds of messages (e.g.,
// snapshots) on the applyCh; at that point you can add fields to
// ApplyMsg, but set CommandValid to false for these other uses.
type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

const (
	FOLLOWER  = 0
	CANDIDATE = 1
	LEADER    = 2
)

type LogEntry struct {
	Command interface{}
	Term    int
}

// A Go object implementing a single Raft peer.
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	// Your data here (2A, 2B, 2C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	state       int
	currentTerm int
	votedFor    int
	numVotes    int
	commitIndex int
	lastApplied int
	nextIndex   []int
	matchIndex  []int
	log         []LogEntry
	// Channels
	votedChan       chan bool
	heartBeatChan   chan bool
	electionWonChan chan bool
	stepDownChan    chan bool
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.currentTerm, rf.state == LEADER
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
func (rf *Raft) persist() {
	// Your code here (2C).
	// Example:
	// w := new(bytes.Buffer)
	// e := labgob.NewEncoder(w)
	// e.Encode(rf.xxx)
	// e.Encode(rf.yyy)
	// data := w.Bytes()
	// rf.persister.SaveRaftState(data)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 { // bootstrap without any state?
		return
	}
	// Your code here (2C).
	// Example:
	// r := bytes.NewBuffer(data)
	// d := labgob.NewDecoder(r)
	// var xxx
	// var yyy
	// if d.Decode(&xxx) != nil ||
	//    d.Decode(&yyy) != nil {
	//   error...
	// } else {
	//   rf.xxx = xxx
	//   rf.yyy = yyy
	// }
}

// example RequestVote RPC arguments structure.
// field names must start with capital letters!
type RequestVoteArgs struct {
	// Your data here (2A, 2B).
	Term         int
	CandidateId  int
	LastLogIndex int
	LastLogTerm  int
}

// example RequestVote RPC reply structure.
// field names must start with capital letters!
type RequestVoteReply struct {
	// Your data here (2A).
	Term        int
	VoteGranted bool
}

// example RequestVote RPC handler.
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (2A, 2B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	reply.Term = rf.currentTerm

	if args.Term < rf.currentTerm { // if candidate's term lower
		reply.VoteGranted = false // don't grant vote
		return
	}
	if args.Term > rf.currentTerm { // if the request vote has a higher term, become a follower
		rf.toFollower(args.Term)
	}
	reply.VoteGranted = false // initially don't give vote
	if rf.votedFor == -1 || rf.votedFor == args.CandidateId {
		// Could possibly do this in one if statement, but it seems more readable this way
		if args.LastLogTerm > rf.getLastTerm() { // candidate's last log term is higher
			reply.VoteGranted = true
			rf.votedFor = args.CandidateId
			rf.sendToNonBlockChan(rf.votedChan, true)
			return
		}
		if args.LastLogTerm == rf.getLastTerm() { // if the logs end with the same term
			if args.LastLogIndex >= rf.getLastIndex() { // if candidate's log is longer
				reply.VoteGranted = true // give it the vote
				rf.votedFor = args.CandidateId
				rf.sendToNonBlockChan(rf.votedChan, true)
				return
			}
		}
	}

}

// example code to send a RequestVote RPC to a server.
// server is the index of the target server in rf.peers[].
// expects RPC arguments in args.
// fills in *reply with RPC reply, so caller should
// pass &reply.
// the types of the args and reply passed to Call() must be
// the same as the types of the arguments declared in the
// handler function (including whether they are pointers).
//
// The labrpc package simulates a lossy network, in which servers
// may be unreachable, and in which requests and replies may be lost.
// Call() sends a request and waits for a reply. If a reply arrives
// within a timeout interval, Call() returns true; otherwise
// Call() returns false. Thus Call() may not return for a while.
// A false return can be caused by a dead server, a live server that
// can't be reached, a lost request, or a lost reply.
//
// Call() is guaranteed to return (perhaps after a delay) *except* if the
// handler function on the server side does not return.  Thus there
// is no need to implement your own timeouts around Call().
//
// look at the comments in ../labrpc/labrpc.go for more details.
//
// if you're having trouble getting RPC to work, check that you've
// capitalized all field names in structs passed over RPC, and
// that the caller passes the address of the reply struct with &, not
// the struct itself.
func (rf *Raft) sendRequestVote(server int, args *RequestVoteArgs, reply *RequestVoteReply) {
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)

	if !ok { // if we didn't get a reply
		return
	}

	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != CANDIDATE { // if we're not the candidate
		return // skip the vote
	}
	if args.Term != rf.currentTerm { // if the term we sent is different from our term now
		return // skip the vote
	}
	if reply.Term > rf.currentTerm { // if the follower's term is higher than ours (there's another leader)
		return // skip the vote
	}

	if reply.VoteGranted {
		rf.numVotes++
		if rf.numVotes == len(rf.peers)/2+1 {
			time.Sleep(time.Millisecond)
			rf.sendToNonBlockChan(rf.electionWonChan, true) // this happens too soon
		}
	}
}

type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
}

func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.Success = false
		return
	}
	if args.Term > rf.currentTerm {
		reply.Term = args.Term
		rf.toFollower(args.Term)

	}
	rf.sendToNonBlockChan(rf.heartBeatChan, true)
}

func (rf *Raft) sendAppendEntries(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	if !ok {
		return
	}
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != LEADER {
		return
	}
	if args.Term != rf.currentTerm {
		return
	}
	if reply.Term < rf.currentTerm {
		return
	}
	if reply.Term > rf.currentTerm {
		rf.sendToNonBlockChan(rf.stepDownChan, true)
		return
	}

}

// the service using Raft (e.g. a k/v server) wants to start
// agreement on the next command to be appended to Raft's log. if this
// server isn't the leader, returns false. otherwise start the
// agreement and return immediately. there is no guarantee that this
// command will ever be committed to the Raft log, since the leader
// may fail or lose an election. even if the Raft instance has been killed,
// this function should return gracefully.
//
// the first return value is the index that the command will appear at
// if it's ever committed. the second return value is the current
// term. the third return value is true if this server believes it is
// the leader.
func (rf *Raft) Start(command interface{}) (int, int, bool) {
	index := -1
	term := -1
	isLeader := true

	// Your code here (2B).

	return index, term, isLeader
}

// the tester doesn't halt goroutines created by Raft after each test,
// but it does call the Kill() method. your code can use killed() to
// check whether Kill() has been called. the use of atomic avoids the
// need for a lock.
//
// the issue is that long-running goroutines use memory and may chew
// up CPU time, perhaps causing later tests to fail and generating
// confusing debug output. any goroutine with a long-running loop
// should call killed() to check whether it should stop.
func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

// this function
func (rf *Raft) handleServer() {
	for !rf.killed() {
		rf.mu.Lock()
		serverState := rf.state
		rf.mu.Unlock()
		switch serverState {
		// Followers:
		// 		reset election timer on heartbeat
		//		reset election timer on vote
		// 		handle election timeouts
		case FOLLOWER:
			select {
			case <-rf.votedChan: // skip re-election if we voted
			case <-rf.heartBeatChan: // skip re-election timer if we got a heart-beat
			case <-time.After(rf.getElectionTimeout()):
				// convert from follower to candidate and start election
				rf.toCandidate(FOLLOWER)
			}
		// Candidates need to handle:
		case CANDIDATE:
			select {
			case <-rf.stepDownChan: // we are already a follower, so next select iteration it will go to the follower case
			case <-rf.electionWonChan:
				rf.toLeader()
			case <-time.After(rf.getElectionTimeout()):
				rf.toCandidate(CANDIDATE)
			}

		// Leaders need to handle:
		case LEADER:
			select {
			case <-rf.stepDownChan: // we are already a follower, so next select iteration it won't send the heartbeat
			case <-time.After(120 * time.Millisecond):
				rf.mu.Lock()
				rf.heartBeat()
				rf.mu.Unlock()
			}
		}
	}
}
func (rf *Raft) toLeader() {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != CANDIDATE { // if we're not the candidate (maybe somebody else won the election before us)
		return // return
	}
	rf.cleanUpChans()
	rf.state = LEADER
	rf.nextIndex = make([]int, len(rf.peers))
	rf.matchIndex = make([]int, len(rf.peers))
	lastLogIndex := rf.getLastIndex() + 1
	for i := 0; i < len(rf.peers); i++ {
		rf.nextIndex[i] = lastLogIndex
	}
	rf.heartBeat()
}
func (rf *Raft) toCandidate(originalState int) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != originalState {
		return
	}
	rf.cleanUpChans()
	// change state
	rf.state = CANDIDATE
	// increase current term
	rf.currentTerm++
	// vote for self
	rf.votedFor = rf.me
	rf.numVotes = 1
	// clean-up all the channels

	rf.startElection()
}

// always check that lock is being held before calling this function!
func (rf *Raft) toFollower(term int) {
	initialState := rf.state
	// need to change state before asking leader (if server is the leader) to step down
	// so that in the next switch iteration it will go right to the follower case
	rf.state = FOLLOWER
	rf.currentTerm = term
	rf.votedFor = -1
	// now that our state is the follower, we can check the initial state before the change
	if initialState != FOLLOWER { // if we are the leader
		rf.sendToNonBlockChan(rf.stepDownChan, true) // step down
	}
}

// always check that lock is being held before calling this function!
func (rf *Raft) startElection() {
	if rf.state != CANDIDATE {
		return
	}
	args := RequestVoteArgs{
		Term:         rf.currentTerm,
		CandidateId:  rf.me,
		LastLogIndex: rf.getLastIndex(),
		LastLogTerm:  rf.getLastTerm(),
	}
	for server := range rf.peers {
		if server == rf.me {
			continue
		}
		go rf.sendRequestVote(server, &args, &RequestVoteReply{})
	}
}

// always check that lock is being held before calling this function!
func (rf *Raft) heartBeat() {
	if rf.state != LEADER {
		return
	}
	for server := range rf.peers {
		if server == rf.me {
			continue
		}
		entries := rf.log[rf.nextIndex[server]:]
		prevLogIndex := rf.nextIndex[server] - 1
		prevLogTerm := -1
		if prevLogIndex > -1 {
			prevLogTerm = rf.log[prevLogIndex].Term
		}
		args := AppendEntriesArgs{
			Term:         rf.currentTerm,
			LeaderId:     rf.me,
			PrevLogIndex: prevLogIndex,
			PrevLogTerm:  prevLogTerm,
			Entries:      make([]LogEntry, len(entries)),
			LeaderCommit: rf.commitIndex,
		}
		copy(args.Entries, entries)
		go rf.sendAppendEntries(server, &args, &AppendEntriesReply{})
	}

}

// always check that lock is being held before calling this function!
func (rf *Raft) getLastIndex() int {
	return len(rf.log) - 1
}

// always check that lock is being held before calling this function!
func (rf *Raft) getLastTerm() int {
	if rf.getLastIndex() == -1 {
		return -1
	}
	return rf.log[rf.getLastIndex()].Term
}

// always check that lock is being held before calling this function!
func (rf *Raft) cleanUpChans() {
	rf.heartBeatChan = make(chan bool)
	rf.votedChan = make(chan bool)
	rf.stepDownChan = make(chan bool)
	rf.electionWonChan = make(chan bool)
}
func (rf *Raft) getElectionTimeout() time.Duration {
	time := time.Duration(350+rand.Intn(250)) * time.Millisecond
	return time
}

// this allows us to send a message to the channel without blocking
// it avoids a lot of stalling and slowing down of servers which would lead to
// timing problems
func (rf *Raft) sendToNonBlockChan(c chan bool, x bool) {
	select {
	case c <- x:
	default:
	}
}

// the service or tester wants to create a Raft server. the ports
// of all the Raft servers (including this one) are in peers[]. this
// server's port is peers[me]. all the servers' peers[] arrays
// have the same order. persister is a place for this server to
// save its persistent state, and also initially holds the most
// recent saved state, if any. applyCh is a channel on which the
// tester or service expects Raft to send ApplyMsg messages.
// Make() must return quickly, so it should start goroutines
// for any long-running work.
func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me

	// Your initialization code here (2A, 2B, 2C).
	rf.state = FOLLOWER
	rf.currentTerm = 0
	rf.log = make([]LogEntry, 0)
	rf.currentTerm = 0
	rf.heartBeatChan = make(chan bool)
	rf.votedChan = make(chan bool)
	rf.electionWonChan = make(chan bool)
	rf.stepDownChan = make(chan bool)
	rf.votedFor = -1
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.matchIndex = make([]int, len(rf.peers))
	rf.nextIndex = make([]int, len(rf.peers))
	// initialize from state persisted before a crash
	rf.readPersist(persister.ReadRaftState())
	go rf.handleServer()
	return rf
}
