package statemanager

import (
	"basic-raft/internal/client"
	"basic-raft/internal/state"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"sync"
	"time"
)

type StateManager interface {
	GrantVote(proposedTerm state.Term, candidateId state.NodeId, lastLogIndex uint64, lastLogTerm state.Term) (bool, state.Term)
	AppendEntries(leaderTerm state.Term, leaderId state.NodeId, prevLogIndex uint64, prevLogTerm state.Term, leaderCommit uint64, newEntries []state.LogEntry) (bool, state.Term)
	AppendEntry(command state.Command) bool

	GetState() (state.Term, []state.LogEntry, state.NodeStatus, []state.LogEntry)

	Start()
	CloseGracefully()
}

type Manager struct {
	mu               *sync.Mutex
	routineGroup     *sync.WaitGroup
	state            *state.State
	id               state.NodeId
	lastElectionTime time.Time
	nodes            []client.NodeClient
	waitForCommit    *sync.Cond
}

func NewManager(id state.NodeId, nodes []client.NodeClient) StateManager {
	if int(id) >= len(nodes) {
		log.Fatal("invalid candidate id, id > number of nodes")
	}

	mutex := new(sync.Mutex)
	return &Manager{
		mu:               mutex,
		routineGroup:     &sync.WaitGroup{},
		state:            state.NewState(),
		id:               id,
		lastElectionTime: time.Now(),
		nodes:            nodes,
		waitForCommit:    &sync.Cond{L: mutex},
	}
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

		if m.state.GetCurrentStatus() == state.LEADER || m.state.GetCurrentStatus() == state.DEAD {
			log.Printf("[current term: %v] Node %d is leader or dead. Stop election timer", termStarted, m.id)
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

	savedState := *m.state // copy of state
	candidateId := m.id
	log.Printf("[current term: %v] Start election, candidate is %d", savedState.GetCurrentTerm(), candidateId)

	m.mu.Unlock()

	votesReceived := 1
	for ind, peerNode := range m.nodes {
		if state.NodeId(ind) == candidateId {
			continue
		}

		go func(peerInd int, peer client.NodeClient) {
			m.routineGroup.Add(1)
			defer m.routineGroup.Done()

			voteGranted, voteTerm, err := peer.RequestVote(candidateId, savedState)
			if err == nil {
				m.mu.Lock()
				defer m.mu.Unlock()

				if m.state.GetCurrentStatus() != state.CANDIDATE {
					log.Printf("while waiting for vote reply, node %d status has been changed to %s", m.id, m.state.GetCurrentStatus())
					return
				}

				if voteTerm > savedState.GetCurrentTerm() {
					log.Printf("term in vote is out of date, node %d becomes follower for term %d", m.id, voteTerm)
					m.becomeFollower(voteTerm)
					return
				}

				if voteTerm == savedState.GetCurrentTerm() && voteGranted {
					votesReceived++
					log.Printf("[current term: %v] Candidate %d receives vote from node %d", voteTerm, candidateId, peerInd)

					// received the majority of votes --> (N + 1) // 2
					if votesReceived*2 >= len(m.nodes)+1 {
						log.Printf("[current term: %v] Candidate %d won an election and becomes leader", voteTerm, candidateId)
						m.becomeLeader()
					}
				}

			} else {
				log.Printf("error during call of peer node %d to vote: %v", peerInd, err)
			}

		}(ind, peerNode)

	}

	// run another election timer, in case this election is not successful
	go m.runElectionTimer()
}

// becomeLeader: not thread-safe and private
func (m *Manager) becomeLeader() {
	m.state.SetCurrentStatus(state.LEADER)
	log.Printf("Reset replication status for node %d", m.id)
	m.state.ResetReplicationStatus(len(m.nodes))

	go func() {
		m.routineGroup.Add(1)
		defer m.routineGroup.Done()

		heartbeatPeriod := 50
		heartbeatPeriodStr, ok := os.LookupEnv("HEARTBEAT_PERIOD_MILLISECONDS")
		if ok {
			heartbeatPeriod, _ = strconv.Atoi(heartbeatPeriodStr)
		}

		ticker := time.NewTicker(time.Duration(heartbeatPeriod) * time.Millisecond)
		defer ticker.Stop()

		for {
			m.syncStateWithOtherNodes()
			<-ticker.C

			m.mu.Lock()
			if m.state.GetCurrentStatus() != state.LEADER {
				m.mu.Unlock()
				return
			}
			m.mu.Unlock()
		}
	}()
}

func (m *Manager) syncStateWithOtherNodes() {
	// todo: optimize health check later
	m.mu.Lock()
	candidateId := m.id
	savedState := *m.state
	log.Printf("[current term: %d] Sending periodic AppendEntries from %d", m.state.GetCurrentTerm(), candidateId)
	m.mu.Unlock()

	for id, peerNode := range m.nodes {
		if state.NodeId(id) == candidateId {
			continue
		}

		go func(peerNode client.NodeClient, peerInd int) {
			m.routineGroup.Add(1)
			defer m.routineGroup.Done()

			success, peerTerm, err := peerNode.AppendEntries(candidateId, state.NodeId(peerInd), savedState)
			if err == nil {
				m.mu.Lock()
				defer m.mu.Unlock()
				if peerTerm > savedState.GetCurrentTerm() {
					log.Printf("term in append response is out of date, node %d becomes follower for term %d", m.id, peerTerm)
					m.becomeFollower(peerTerm)
					return
				}

				peerId := state.NodeId(peerInd)
				nextIndex := savedState.GetNextIndexForNode(peerId)
				delta := len(savedState.GetNewLog(peerId))
				if delta > 0 {
					log.Printf("[current term: %d] Leader has %d new message(s) for peer %d", savedState.GetCurrentTerm(), delta, peerId)
				} else {
					log.Printf("[current term: %d] Periodic healthcheck for peer %d", savedState.GetCurrentTerm(), peerId)
				}

				if m.state.GetCurrentStatus() == state.LEADER && peerTerm == savedState.GetCurrentTerm() {
					if success {
						// If successful: update nextIndex and matchIndex for follower
						nextIndexAfterReplication := int(nextIndex) + delta
						m.state.SetNextIndexForNode(peerId, nextIndexAfterReplication)
						m.state.SetMatchIndexForNode(peerId, nextIndexAfterReplication-1)

						// If there exists an N such that N > commitIndex, a majority
						// of matchIndex[i] ≥ N, and log[N].term == currentTerm:
						// set commitIndex = N
						savedCommitIndex := m.state.GetCommitIndex()
						for commitInd := m.state.GetCommitIndex() + 1; commitInd <= m.state.GetLastLogIndex(); commitInd++ {
							matchCount := 1
							// check all nodes
							for nodeId := range m.nodes {
								if nodeId == int(candidateId) {
									continue
								}

								if m.state.GetMatchIndexForNode(state.NodeId(nodeId)) >= commitInd {
									matchCount++
								}
							}

							// check majority
							if matchCount*2 >= len(m.nodes)+1 {
								log.Printf("Majority for log %d, commiting locally...", commitInd)
								m.state.SetCommitIndex(commitInd)
							}
						}

						if m.state.GetCommitIndex() != savedCommitIndex {
							// notify clients about commit
							m.waitForCommit.Broadcast()
						}

					} else {
						// If AppendEntries fails because of log inconsistency:
						// decrement nextIndex and retry
						m.state.SetNextIndexForNode(peerId, int(nextIndex)-1)
						log.Printf("AppendEntries !success from %d to %d. Retry later", peerId, m.id)
					}
				}

			} else {
				log.Printf("error during call of peer node %d to ping: %v", peerInd, err)
			}
		}(peerNode, id)
	}
}

// becomeFollower: not thread-safe and private
func (m *Manager) becomeFollower(term state.Term) {
	log.Printf("Becoming follower for term: %v", term)

	m.state.SetCurrentTerm(term)
	m.state.SetCurrentStatus(state.FOLLOWER)
	m.state.SetVotedFor(nil) // unset vote for current term
	m.lastElectionTime = time.Now()

	m.waitForCommit.Broadcast() // unblock all waiters for commit

	go m.runElectionTimer()
}

func (m *Manager) GrantVote(proposedTerm state.Term, candidateId state.NodeId, lastLogIndex uint64, lastLogTerm state.Term) (bool, state.Term) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.GetCurrentStatus() == state.DEAD {
		return false, 0
	}

	log.Printf("Trial to grant vote for candidate %v during term %v", candidateId, proposedTerm)

	if proposedTerm > m.state.GetCurrentTerm() {
		log.Printf("Current term %v is out of date, new term: %v", m.state.GetCurrentTerm(), proposedTerm)
		m.becomeFollower(proposedTerm)
	}

	var acceptedVote bool
	candidateIsValid := m.state.GetVotedFor() == nil || *m.state.GetVotedFor() == candidateId
	// TODO: cover logsIsValid logic with tests
	logIsValid := lastLogTerm > m.state.GetLastLogTerm() || (lastLogTerm == m.state.GetLastLogTerm() && lastLogIndex >= m.state.GetLastLogIndex())
	if proposedTerm == m.state.GetCurrentTerm() && candidateIsValid && logIsValid {
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

func (m *Manager) AppendEntries(leaderTerm state.Term, leaderId state.NodeId, prevLogIndex uint64, prevLogTerm state.Term, leaderCommit uint64, newEntries []state.LogEntry) (bool, state.Term) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.state.GetCurrentStatus() == state.DEAD {
		return false, 0
	}

	log.Printf("[current term: %d] Trial to append entries from node %v for term %d", m.state.GetCurrentTerm(), leaderId, leaderTerm)

	if leaderTerm > m.state.GetCurrentTerm() {
		log.Printf("current term %d is out of date, new term: %v", m.state.GetCurrentTerm(), leaderTerm)
		m.becomeFollower(leaderTerm)
	}

	responseSuccess := false
	if leaderTerm == m.state.GetCurrentTerm() {
		if m.state.GetCurrentStatus() != state.FOLLOWER {
			m.becomeFollower(leaderTerm)
		}
		m.lastElectionTime = time.Now()

		if prevLogIndex == 0 ||
			(prevLogIndex <= m.state.GetLastLogIndex() && prevLogTerm == m.state.GetTermOfLog(prevLogIndex)) {
			responseSuccess = true

			logInsertIndex := prevLogIndex + 1
			newEntriesIndex := 0

			// todo: cover this loop with tests
			for {
				if logInsertIndex > m.state.GetLastLogIndex() || newEntriesIndex >= len(newEntries) {
					break
				}

				if m.state.GetTermOfLog(logInsertIndex) != newEntries[newEntriesIndex].Term {
					break
				}

				logInsertIndex++
				newEntriesIndex++
			}

			// At the end of the loop above:
			// - logInsertIndex points at the end of the log, or an index where the
			//   term mismatches with an entry from the leader
			// - newEntriesIndex points at the end of Entries, or an index where the
			//   term mismatches with the corresponding log entry
			if newEntriesIndex < len(newEntries) {
				log.Printf("Inserting %v starting from index %d", newEntries[newEntriesIndex:], logInsertIndex)
				m.state.AppendStartingFromIndex(int(logInsertIndex), newEntries[newEntriesIndex:])
			}

			// If leaderCommit > commitIndex, set commitIndex =
			// min(leaderCommit, index of last new entry)
			if leaderCommit > m.state.GetCommitIndex() {
				m.state.SetCommitIndex(min(leaderCommit, m.state.GetLastLogIndex()))
				log.Printf("Committed log now: %v", m.state.GetCommittedLog())
			}
		} else {
			log.Printf("Rejected append entries from node %v during term %v. [PrevLogIndex: %d, PrevLogTerm: %d, LocalLastLogIndex: %d, LocalPrevLogTerm: %d]", leaderId, leaderTerm, prevLogIndex, prevLogTerm, m.state.GetLastLogIndex(), m.state.GetTermOfLog(prevLogIndex))
		}
	}

	responseTerm := m.state.GetCurrentTerm()
	return responseSuccess, responseTerm
}

func (m *Manager) AppendEntry(command state.Command) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.state.GetCurrentStatus() == state.LEADER {
		// append entry to local log
		indexToCommit := m.state.Submit(command)
		savedTerm := m.state.GetCurrentTerm()

		for indexToCommit > m.state.GetCommitIndex() && m.state.GetCurrentStatus() == state.LEADER {
			// respond after entry applied to state machine
			m.waitForCommit.Wait()
		}

		// todo: double-check condition
		return m.state.GetCurrentStatus() == state.LEADER && m.state.GetTermOfLog(indexToCommit) == savedTerm
	}

	return false
}

func (m *Manager) GetState() (state.Term, []state.LogEntry, state.NodeStatus, []state.LogEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.state.GetCurrentTerm(), m.state.GetCommittedLog(), m.state.GetCurrentStatus(), m.state.GetUncommittedLog()
}

func (m *Manager) CloseGracefully() {
	m.mu.Lock()
	// stop heartbeats and elections
	m.state.SetCurrentStatus(state.DEAD)
	m.waitForCommit.Broadcast() // unblock all waiters for commit
	m.mu.Unlock()
	// wait for all goroutines to finish
	log.Print("Gracefully closing all goroutines...\n")
	m.routineGroup.Wait()
}
