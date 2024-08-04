package statemanager

import (
	"basic-raft/internal/state"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLeaderBecomesFollowerWhenReceivesVoteWithBiggerTerm(t *testing.T) {
	// GIVEN
	currentNodeId := state.CandidateId(1)
	manager := NewManager()
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
	oldLeaderId := state.CandidateId(1)
	manager := NewManager()
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
	oldLeaderId := state.CandidateId(1)
	manager := NewManager()
	manager.state.SetCurrentStatus(state.FOLLOWER)
	manager.state.SetVotedFor(&oldLeaderId)
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
	require.Equal(t, manager.state.GetVotedFor(), &oldLeaderId)
	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
}
