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

func NewVoteRequest(candidateId uint64, state *state.State) *VoteRequest {
	return &VoteRequest{Term: uint64(state.GetCurrentTerm() + 1), CandidateId: candidateId, LastLogIndex: state.GetLastLogIndex(), LastLogTerm: uint64(state.GetLastLogTerm())}
}
