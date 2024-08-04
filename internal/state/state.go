package state

type NodeStatus string

const (
	FOLLOWER  NodeStatus = "FOLLOWER"
	LEADER    NodeStatus = "LEADER"
	CANDIDATE NodeStatus = "CANDIDATE"
)

type Term uint64
type CandidateId uint64

type LogEntry struct {
	Term    Term
	Message string // todo: replace?
}

// State WARNING: not thread-safe. Synchronization is implemented by state manager
type State struct {
	currentTerm   Term         // latest term server has seen (initialized to 0 on first boot, increases monotonically)
	votedFor      *CandidateId //candidateId that received vote in current term (or null if none)
	log           []LogEntry
	currentStatus NodeStatus
}

func NewState() *State {
	return &State{currentTerm: 0, votedFor: nil, log: make([]LogEntry, 0), currentStatus: FOLLOWER}
}

func (s *State) GetCurrentTerm() Term {
	return s.currentTerm
}

func (s *State) SetCurrentTerm(term Term) {
	s.currentTerm = term
}

func (s *State) GetVotedFor() *CandidateId {
	return s.votedFor
}

func (s *State) SetVotedFor(votedFor *CandidateId) {
	s.votedFor = votedFor
}

func (s *State) GetLastLogIndex() uint64 {
	// todo: revisit
	return uint64(len(s.log))
}

func (s *State) GetLastLogTerm() Term {
	currentLen := len(s.log)

	if currentLen == 0 {
		return 0
	}

	// todo: revisit
	return s.log[currentLen-1].Term
}

func (s *State) GetCurrentStatus() NodeStatus {
	return s.currentStatus
}

func (s *State) SetCurrentStatus(status NodeStatus) {
	s.currentStatus = status
}
