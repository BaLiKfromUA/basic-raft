package state

type NodeStatus string

const (
	FOLLOWER  NodeStatus = "FOLLOWER"
	LEADER    NodeStatus = "LEADER"
	CANDIDATE NodeStatus = "CANDIDATE"
	DEAD      NodeStatus = "DEAD"
)

type Term uint64
type NodeId uint64
type Command string

type LogEntry struct {
	Term    Term
	Command Command
}

// State WARNING: not thread-safe. Synchronization is implemented by state manager
type State struct {
	// persistent state on ALL SERVERS
	currentTerm Term       // latest term server has seen (initialized to 0 on first boot, increases monotonically)
	votedFor    *NodeId    //candidateId that received vote in current term (or null if none)
	log         []LogEntry // first index is 1

	// volatile state on ALL SERVERS
	currentStatus NodeStatus
	commitIndex   uint64
	// TODO: add concept of 'Apply' operation + lastApplied index

	// volatile state on LEADERS
	nextIndex  []uint64
	matchIndex []uint64
}

func NewState() *State {
	return &State{
		currentTerm:   0,
		votedFor:      nil,
		log:           make([]LogEntry, 0),
		currentStatus: FOLLOWER,
		commitIndex:   0,
	}
}

func (s *State) GetCurrentTerm() Term {
	return s.currentTerm
}

func (s *State) SetCurrentTerm(term Term) {
	s.currentTerm = term
}

func (s *State) GetVotedFor() *NodeId {
	return s.votedFor
}

func (s *State) SetVotedFor(votedFor *NodeId) {
	s.votedFor = votedFor
}

func (s *State) GetLastLogIndex() uint64 {
	if len(s.log) == 0 {
		return 0
	}

	return uint64(len(s.log))
}

func (s *State) GetLastLogTerm() Term {
	return s.GetTermOfLog(s.GetLastLogIndex())
}

func (s *State) GetCurrentStatus() NodeStatus {
	return s.currentStatus
}

func (s *State) SetCurrentStatus(status NodeStatus) {
	s.currentStatus = status
}

func (s *State) Submit(cmd Command) uint64 {
	s.log = append(s.log, LogEntry{s.currentTerm, cmd})
	return uint64(len(s.log))
}

func (s *State) GetCommittedLog() []LogEntry {
	if s.commitIndex == 0 {
		return make([]LogEntry, 0)
	}

	return s.log[0:s.commitIndex] // todo: double-check and test
}

func (s *State) SetCommitIndex(commitIndex uint64) {
	s.commitIndex = commitIndex
}

func (s *State) GetCommitIndex() uint64 {
	return s.commitIndex
}

func (s *State) GetTermOfLog(index uint64) Term {
	if int(index) >= len(s.log) {
		return 0
	}

	return s.log[index-1].Term
}

func (s *State) ResetReplicationStatus(numberOfNodes int) {
	s.nextIndex = make([]uint64, numberOfNodes)
	s.matchIndex = make([]uint64, numberOfNodes)

	for i := 0; i < numberOfNodes; i++ {
		s.matchIndex[i] = 0
		s.nextIndex[i] = uint64(len(s.log)) + 1
	}
}

func (s *State) GetPrevLogIndex(id NodeId) uint64 {
	return s.nextIndex[id] - 1
}

func (s *State) GetPrevLogTerm(id NodeId) Term {
	return s.GetTermOfLog(s.GetPrevLogIndex(id))
}

func (s *State) GetNewLog(id NodeId) []LogEntry {
	start := s.nextIndex[id]
	if start == 0 {
		return make([]LogEntry, 0)
	}

	return s.log[start-1:]
}

func (s *State) SetNextIndexForNode(id NodeId, ind int) {
	s.nextIndex[id] = uint64(max(ind, 1))
}

func (s *State) SetMatchIndexForNode(id NodeId, ind int) {
	s.matchIndex[id] = uint64(max(ind, 0))
}

func (s *State) GetMatchIndexForNode(id NodeId) uint64 {
	return s.matchIndex[id]
}

func (s *State) GetNextIndexForNode(id NodeId) uint64 {
	return s.nextIndex[id]
}
