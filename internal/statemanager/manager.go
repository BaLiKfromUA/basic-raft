package statemanager

import (
	"basic-raft/internal/state"
	"log"
	"sync"
)

type Manager struct {
	mu    *sync.Mutex
	state *state.State
}

func NewManager() *Manager {
	return &Manager{mu: &sync.Mutex{}, state: state.NewState()}
}

// becomeFollower: not thread-safe and private
func (m *Manager) becomeFollower(term state.Term) {
	log.Printf("Becoming follower for term: %v", term)
	m.state.SetCurrentTerm(term)
	m.state.SetCurrentStatus(state.FOLLOWER)
	m.state.SetVotedFor(nil) // unset vote for current term

	// todo: election timer
}

func (m *Manager) GrantVote(proposedTerm state.Term, candidateId state.CandidateId) (bool, state.Term) {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Printf("Trial to grant vote for candidate %v during term %v", candidateId, proposedTerm)

	if proposedTerm > m.state.GetCurrentTerm() {
		log.Printf("Current term %v is out of date, new term: %v", m.state.GetCurrentTerm(), proposedTerm)
		m.becomeFollower(proposedTerm)
	}

	var acceptedVote bool
	candidateIsValid := m.state.GetVotedFor() == nil || *m.state.GetVotedFor() == candidateId
	if proposedTerm == m.state.GetCurrentTerm() && candidateIsValid {
		log.Printf("[current term: %v] Give a vote for %v", m.state.GetCurrentTerm(), candidateId)
		acceptedVote = true
		m.state.SetVotedFor(&candidateId)
		// todo: election timer
	} else {
		acceptedVote = false
		log.Printf("[current term: %v] Rejected candidate proposal of %v", m.state.GetCurrentTerm(), candidateId)
	}

	return acceptedVote, m.state.GetCurrentTerm()
}
