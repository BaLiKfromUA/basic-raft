package message

import "basic-raft/internal/state"

type VoteRequest struct {
	Term         uint64 `json:"term"`
	CandidateId  uint64 `json:"candidate_id"`
	LastLogIndex uint64 `json:"last_log_index"`
	LastLogTerm  uint64 `json:"last_log_term"`
}

type VoteResponse struct {
	Term        uint64 `json:"term"`
	VoteGranted bool   `json:"vote_granted"`
}

func NewVoteRequest(candidateId uint64, s state.State) *VoteRequest {
	return &VoteRequest{
		Term:         uint64(s.GetCurrentTerm()),
		CandidateId:  candidateId,
		LastLogIndex: s.GetLastLogIndex(),
		LastLogTerm:  uint64(s.GetLastLogTerm()),
	}
}

type AppendEntriesRequest struct {
	Term         uint64     `json:"term"`
	LeaderId     uint64     `json:"leader_id"`
	PrevLogIndex uint64     `json:"prev_log_index"`
	PrevLogTerm  uint64     `json:"prev_log_term"`
	LeaderCommit uint64     `json:"leader_commit"`
	Entries      []LogEntry `json:"entries"`
}

type AppendEntriesResponse struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
}

type LogEntry struct {
	Term    uint64 `json:"term"`
	Command string `json:"command"`
}

func NewAppendEntriesRequest(leaderId uint64, peerId state.NodeId, s state.State) *AppendEntriesRequest {
	entries := make([]LogEntry, 0)
	for _, element := range s.GetNewLog(peerId) {
		entries = append(entries, LogEntry{
			Term:    uint64(element.Term),
			Command: string(element.Command),
		})
	}

	return &AppendEntriesRequest{
		Term:         uint64(s.GetCurrentTerm()),
		LeaderId:     leaderId,
		LeaderCommit: s.GetCommitIndex(),
		Entries:      entries,
		PrevLogIndex: s.GetPrevLogIndex(peerId),
		PrevLogTerm:  uint64(s.GetPrevLogTerm(peerId)),
	}
}
