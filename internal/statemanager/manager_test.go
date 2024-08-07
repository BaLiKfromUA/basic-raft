package statemanager

import (
	"basic-raft/internal/state"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLeaderBecomesFollowerWhenReceivesVoteWithBiggerTerm(t *testing.T) {
	// GIVEN
	manager := NewManager()
	defer manager.CloseGracefully()

	currentNodeId := state.CandidateId(1)
	manager.state.SetCurrentStatus(state.LEADER)
	manager.state.SetVotedFor(&currentNodeId)

	newTerm := manager.state.GetCurrentTerm() + 1
	newLeaderId := state.CandidateId(2)

	// WHEN
	voteGranted, currentTerm := manager.GrantVote(newTerm, newLeaderId)

	// THEN
	// check call results
	require.True(t, voteGranted)
	require.Equal(t, currentTerm, newTerm)
	// check state
	require.Equal(t, manager.state.GetCurrentTerm(), newTerm)
	require.Equal(t, manager.state.GetVotedFor(), &newLeaderId)
	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
}

func TestFollowerGrantsVoteIfNoVotedFor(t *testing.T) {
	// GIVEN
	manager := NewManager()
	defer manager.CloseGracefully()

	manager.state.SetCurrentStatus(state.FOLLOWER)
	manager.state.SetVotedFor(nil) // no vote yet

	newTerm := manager.state.GetCurrentTerm() // same term
	newLeaderId := state.CandidateId(2)

	// WHEN
	voteGranted, currentTerm := manager.GrantVote(newTerm, newLeaderId)

	// THEN
	// check call results
	require.True(t, voteGranted)
	require.Equal(t, currentTerm, newTerm)
	// check state
	require.Equal(t, manager.state.GetCurrentTerm(), newTerm)
	require.Equal(t, manager.state.GetVotedFor(), &newLeaderId)
	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
}

func TestFollowerDoesntGrantVoteIfAlreadyVoted(t *testing.T) {
	// Given
	manager := NewManager()
	defer manager.CloseGracefully()

	oldLeaderId := state.CandidateId(1)
	manager.state.SetCurrentStatus(state.FOLLOWER)
	manager.state.SetVotedFor(&oldLeaderId)

	newTerm := manager.state.GetCurrentTerm() // same term
	newLeaderId := state.CandidateId(2)

	// WHEN
	voteGranted, currentTerm := manager.GrantVote(newTerm, newLeaderId)

	// THEN
	// check call results
	require.False(t, voteGranted)
	require.Equal(t, currentTerm, newTerm)

	// check state
	require.Equal(t, manager.state.GetCurrentTerm(), newTerm)
	require.Equal(t, manager.state.GetVotedFor(), &oldLeaderId)
	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
}

func TestFollowerDoesntGrantVoteIfNewTermIsOutdated(t *testing.T) {
	// Given
	manager := NewManager()
	defer manager.CloseGracefully()

	manager.state.SetCurrentStatus(state.FOLLOWER)
	manager.state.SetVotedFor(nil)
	manager.state.SetCurrentTerm(state.Term(2))

	newTerm := manager.state.GetCurrentTerm() - 1 // previous term
	newLeaderId := state.CandidateId(2)

	// WHEN
	voteGranted, currentTerm := manager.GrantVote(newTerm, newLeaderId)

	// THEN
	// check call results
	require.False(t, voteGranted)
	require.Equal(t, currentTerm, manager.state.GetCurrentTerm())

	// check state
	require.NotEqual(t, manager.state.GetCurrentTerm(), newTerm)
	require.Nil(t, manager.state.GetVotedFor())
	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
}

func TestFollowerStartsElectionIfTimeoutExceeded(t *testing.T) {
	// GIVEN
	t.Setenv("ELECTION_TIMEOUT", "10")
	manager := NewManager()

	initialTerm := state.Term(0)

	manager.state.SetCurrentStatus(state.FOLLOWER)
	manager.state.SetVotedFor(nil)
	manager.state.SetCurrentTerm(initialTerm)

	// WHEN
	manager.runElectionTimer()
	manager.CloseGracefully() // wait timer to finish

	// THEN
	require.Equal(t, manager.state.GetCurrentStatus(), state.CANDIDATE)
	require.Equal(t, *manager.state.GetVotedFor(), manager.id)
	require.Equal(t, manager.state.GetCurrentTerm(), initialTerm+1)
}

func TestNodeDoesntStartsElectionIfLeader(t *testing.T) {
	// GIVEN
	t.Setenv("ELECTION_TIMEOUT", "10")
	manager := NewManager()
	initialTerm := state.Term(0)

	manager.state.SetCurrentStatus(state.LEADER)
	manager.state.SetVotedFor(&manager.id)
	manager.state.SetCurrentTerm(initialTerm)

	// WHEN
	manager.runElectionTimer()
	manager.CloseGracefully() // wait timer to finish

	// THEN
	require.Equal(t, manager.state.GetCurrentStatus(), state.LEADER)
	require.Equal(t, *manager.state.GetVotedFor(), manager.id)
	require.Equal(t, manager.state.GetCurrentTerm(), initialTerm)
}
