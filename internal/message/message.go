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
		Term:        uint64(s.GetCurrentTerm()),
		CandidateId: candidateId,
		// todo: add more
	}
}

type AppendEntriesRequest struct {
	Term         uint64           `json:"term"`
	LeaderId     uint64           `json:"leader_id"`
	PrevLogIndex uint64           `json:"prev_log_index"`
	PrevLogTerm  uint64           `json:"prev_log_term"`
	LeaderCommit uint64           `json:"leader_commit"`
	Entries      []state.LogEntry `json:"entries"`
}

type AppendEntriesResponse struct {
	Term    uint64 `json:"term"`
	Success bool   `json:"success"`
}

func NewAppendEntriesRequest(leaderId uint64, s state.State) *AppendEntriesRequest {
	return &AppendEntriesRequest{
		Term:     uint64(s.GetCurrentTerm()),
		LeaderId: leaderId,
		Entries:  make([]state.LogEntry, 0), // empty if healthcheck
		// todo: add more
	}
}
