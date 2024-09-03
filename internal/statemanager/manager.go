package statemanager

import (
	"basic-raft/internal/state"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"
)

type Manager struct {
	mu               *sync.Mutex
	routineGroup     *sync.WaitGroup
	state            *state.State
	id               state.CandidateId
	lastElectionTime time.Time
}

func NewManager(id state.CandidateId) *Manager {
	return &Manager{mu: &sync.Mutex{}, routineGroup: &sync.WaitGroup{}, state: state.NewState(), id: id, lastElectionTime: time.Now()}
}

func (m *Manager) Start() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.becomeFollower(m.state.GetCurrentTerm())
}

func (m *Manager) getRandomElectionTimeout() time.Duration {
	electionTimeout := 150 // default lower bound recommended by paper
	electionTimeoutStr, ok := os.LookupEnv("ELECTION_TIMEOUT_MILLISECONDS")
	if ok {
		electionTimeout, _ = strconv.Atoi(electionTimeoutStr)
	}

	return time.Duration(rand.Intn(electionTimeout)+electionTimeout) * time.Millisecond
}

func (m *Manager) runElectionTimer() {
	m.routineGroup.Add(1)
	defer m.routineGroup.Done()

	timeout := m.getRandomElectionTimeout()
	m.mu.Lock()
	termStarted := m.state.GetCurrentTerm()
	m.mu.Unlock()

	log.Printf("[current term: %v] Election timer started with timeout: %v\n", termStarted, timeout)
	defer fmt.Printf("[current term: %v]. Finished election timer.\n", termStarted)

	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()

	for {
		<-ticker.C // wait 1 tick

		m.mu.Lock()

		if m.state.GetCurrentStatus() == state.LEADER {
			log.Printf("[current term: %v] Node %d is leader. Stop election timer", termStarted, m.id)
			m.mu.Unlock()
			return
		}

		if termStarted != m.state.GetCurrentTerm() {
			log.Printf("[current term: %v] outdated term, new term: %v. Stop election timer", termStarted, m.state.GetCurrentTerm())
			m.mu.Unlock()
			return
		}

		if elapsed := time.Since(m.lastElectionTime); elapsed >= timeout {
			log.Printf("[current term: %v] Election timer elapsed: %v", termStarted, elapsed)
			m.mu.Unlock()
			m.startElection()
			return
		} else {
			m.mu.Unlock()
		}

	}
}

func (m *Manager) startElection() {
	m.mu.Lock()

	m.state.SetCurrentStatus(state.CANDIDATE)
	m.state.SetCurrentTerm(m.state.GetCurrentTerm() + 1)
	m.state.SetVotedFor(&m.id)
	m.lastElectionTime = time.Now()

	log.Printf("[current term: %v] Start election, candidate is %d", m.state.GetCurrentTerm(), m.id)
	m.mu.Unlock()

	// todo:
}

// becomeFollower: not thread-safe and private
func (m *Manager) becomeFollower(term state.Term) {
	log.Printf("Becoming follower for term: %v", term)
	m.state.SetCurrentTerm(term)
	m.state.SetCurrentStatus(state.FOLLOWER)
	m.state.SetVotedFor(nil) // unset vote for current term
	m.lastElectionTime = time.Now()

	go m.runElectionTimer()
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
		m.lastElectionTime = time.Now()
	} else {
		log.Printf("[current term: %v] Rejected candidate proposal of %v", m.state.GetCurrentTerm(), candidateId)
		acceptedVote = false
	}

	return acceptedVote, m.state.GetCurrentTerm()
}

func (m *Manager) CloseGracefully() {
	// wait for all goroutines to finish
	log.Print("Gracefully closing all goroutines...\n")
	m.routineGroup.Wait()
}
