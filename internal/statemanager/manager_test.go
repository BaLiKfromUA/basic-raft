package statemanager

import (
	"basic-raft/internal/client"
	"basic-raft/internal/state"
	"github.com/stretchr/testify/require"
	"testing"
)

type NodeClientMock struct {
	voteGranted bool
}

func (c *NodeClientMock) RequestVote(_ uint64, state state.State) (bool, state.Term, error) {
	return c.voteGranted, state.GetCurrentTerm(), nil
}

func NewTestManager(t *testing.T, nodes []client.NodeClient) *Manager {
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "10")
	t.Setenv("HEARTBEAT_PERIOD_MILLISECONDS", "5")
	manager := NewManager(0, nodes)
	return manager
}

func NewTestManagerDefault(t *testing.T) *Manager {
	mockClient := &NodeClientMock{voteGranted: true}
	return NewTestManager(t, []client.NodeClient{mockClient, mockClient})
}

func TestLeaderBecomesFollowerWhenReceivesVoteWithBiggerTerm(t *testing.T) {
	// GIVEN
	manager := NewTestManagerDefault(t)
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
	manager := NewTestManagerDefault(t)
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
	manager := NewTestManagerDefault(t)
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
	manager := NewTestManagerDefault(t)
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
	mockClient := &NodeClientMock{voteGranted: false} // set False to prevent becoming a leader
	manager := NewTestManager(t, []client.NodeClient{mockClient, mockClient})
	defer manager.CloseGracefully()

	initialTerm := state.Term(0)

	manager.state.SetCurrentStatus(state.FOLLOWER)
	manager.state.SetVotedFor(nil)
	manager.state.SetCurrentTerm(initialTerm)

	// WHEN
	manager.runElectionTimer()

	// THEN
	require.Equal(t, manager.state.GetCurrentStatus(), state.CANDIDATE)
	require.Equal(t, *manager.state.GetVotedFor(), manager.id)
	require.Equal(t, manager.state.GetCurrentTerm(), initialTerm+1)
}

func TestNodeDoesntStartsElectionIfLeader(t *testing.T) {
	// GIVEN
	mockClient := &NodeClientMock{voteGranted: false} // set False to prevent becoming a leader
	manager := NewTestManager(t, []client.NodeClient{mockClient, mockClient})
	defer manager.CloseGracefully()

	initialTerm := state.Term(0)

	manager.state.SetCurrentStatus(state.LEADER)
	manager.state.SetVotedFor(&manager.id)
	manager.state.SetCurrentTerm(initialTerm)

	// WHEN
	manager.runElectionTimer()

	// THEN
	require.Equal(t, manager.state.GetCurrentStatus(), state.LEADER)
	require.Equal(t, *manager.state.GetVotedFor(), manager.id)
	require.Equal(t, manager.state.GetCurrentTerm(), initialTerm)
}
