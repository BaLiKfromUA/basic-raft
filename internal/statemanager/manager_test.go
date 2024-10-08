package statemanager

import (
	"basic-raft/internal/client"
	"basic-raft/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sync"
	"testing"
	"time"
)

type NodeClientMock struct {
	returnValue bool
	term        *state.Term
}

func (c *NodeClientMock) RequestVote(_ state.NodeId, state state.State) (bool, state.Term, error) {
	termToReturn := state.GetCurrentTerm()
	if c.term != nil {
		termToReturn = *c.term
	}

	return c.returnValue, termToReturn, nil
}

func (c *NodeClientMock) AppendEntries(_ state.NodeId, _ state.NodeId, state state.State) (bool, state.Term, error) {
	termToReturn := state.GetCurrentTerm()
	if c.term != nil {
		termToReturn = *c.term
	}

	return c.returnValue, termToReturn, nil
}

func NewTestManager(t *testing.T, nodes []client.NodeClient) *Manager {
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "10")
	t.Setenv("HEARTBEAT_PERIOD_MILLISECONDS", "5")

	mutex := &sync.Mutex{}
	return &Manager{
		mu:               mutex,
		routineGroup:     &sync.WaitGroup{},
		state:            state.NewState(),
		id:               0,
		lastElectionTime: time.Now(),
		nodes:            nodes,
		waitForCommit:    &sync.Cond{L: mutex},
	}
}

func NewTestManagerDefault(t *testing.T) *Manager {
	mockClient := &NodeClientMock{returnValue: true}
	return NewTestManager(t, []client.NodeClient{mockClient, mockClient})
}

func TestNodeIsInDeadStateAfterClosing(t *testing.T) {
	// GIVEN
	manager := NewTestManagerDefault(t)
	manager.Start()

	// WHEN
	manager.CloseGracefully()

	//  THEN
	require.Equal(t, manager.state.GetCurrentStatus(), state.DEAD)
}

func TestNodeIsInFollowerStateAfterStarting(t *testing.T) {
	// GIVEN
	manager := NewTestManagerDefault(t)
	defer manager.CloseGracefully()
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "100000") // long timeout

	var expectedVote *state.NodeId = nil

	// WHEN
	manager.Start()

	// THEN
	require.Equal(t, manager.state.GetCurrentTerm(), state.Term(0))
	require.Equal(t, manager.state.GetVotedFor(), expectedVote)
	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
}

func TestLeaderBecomesFollowerWhenReceivesVoteWithBiggerTerm(t *testing.T) {
	// GIVEN
	manager := NewTestManagerDefault(t)
	defer manager.CloseGracefully()

	currentNodeId := state.NodeId(1)
	manager.state.SetCurrentStatus(state.LEADER)
	manager.state.SetVotedFor(&currentNodeId)

	newTerm := manager.state.GetCurrentTerm() + 1
	newLeaderId := state.NodeId(2)

	// WHEN
	voteGranted, currentTerm := manager.GrantVote(newTerm, newLeaderId, 0, 0)

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
	newLeaderId := state.NodeId(2)

	// WHEN
	voteGranted, currentTerm := manager.GrantVote(newTerm, newLeaderId, 0, 0)

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

	oldLeaderId := state.NodeId(1)
	manager.state.SetCurrentStatus(state.FOLLOWER)
	manager.state.SetVotedFor(&oldLeaderId)

	newTerm := manager.state.GetCurrentTerm() // same term
	newLeaderId := state.NodeId(2)

	// WHEN
	voteGranted, currentTerm := manager.GrantVote(newTerm, newLeaderId, 0, 0)

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
	newLeaderId := state.NodeId(2)

	// WHEN
	voteGranted, currentTerm := manager.GrantVote(newTerm, newLeaderId, 0, 9)

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
	mockClient := &NodeClientMock{returnValue: false} // set False to prevent becoming a leader
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
	mockClient := &NodeClientMock{returnValue: false} // set False to prevent becoming a leader
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

func TestNodeBecomesLeaderIfReceivesEnoughVotes(t *testing.T) {
	// GIVEN
	mockClient := &NodeClientMock{returnValue: true} // set False to become a leader
	manager := NewTestManager(t, []client.NodeClient{mockClient, mockClient})
	defer manager.CloseGracefully()
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "100000") // long timeout to prevent another election

	initialTerm := state.Term(0)

	manager.state.SetCurrentStatus(state.FOLLOWER)
	manager.state.SetVotedFor(nil)
	manager.state.SetCurrentTerm(initialTerm)

	// WHEN
	manager.startElection()
	time.Sleep(time.Millisecond * 100)

	// THEN
	require.Equal(t, manager.state.GetCurrentStatus(), state.LEADER)
	require.Equal(t, *manager.state.GetVotedFor(), manager.id)
	require.Equal(t, manager.state.GetCurrentTerm(), initialTerm+1)
}

func TestNodeBecomesFollowerIfVoteResponseTermIsBigger(t *testing.T) {
	// GIVEN
	expectedTerm := state.Term(2)
	var expectedVote *state.NodeId = nil

	mockClient := &NodeClientMock{returnValue: false, term: &expectedTerm} // set bigger term then initial

	manager := NewTestManager(t, []client.NodeClient{mockClient, mockClient})
	defer manager.CloseGracefully()
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "100000") // long timeout to prevent another election

	initialTerm := state.Term(0)

	manager.state.SetCurrentStatus(state.FOLLOWER)
	manager.state.SetVotedFor(nil)
	manager.state.SetCurrentTerm(initialTerm)

	// WHEN
	manager.startElection()
	time.Sleep(time.Millisecond * 100)

	// THEN
	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
	require.Equal(t, manager.state.GetVotedFor(), expectedVote)
	require.Equal(t, manager.state.GetCurrentTerm(), expectedTerm)
}

func TestNodeBecomesFollowerIfHeartbeatResponseTermIsBigger(t *testing.T) {
	// GIVEN
	expectedTerm := state.Term(2)
	var expectedVote *state.NodeId = nil

	mockClient := &NodeClientMock{returnValue: false, term: &expectedTerm} // set bigger term then initial

	manager := NewTestManager(t, []client.NodeClient{mockClient, mockClient})
	defer manager.CloseGracefully()
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "100000") // long timeout to prevent another election

	initialTerm := state.Term(0)
	nodeId := state.NodeId(0)

	manager.state.SetCurrentStatus(state.LEADER)
	manager.state.SetVotedFor(&nodeId)
	manager.state.SetCurrentTerm(initialTerm)

	// WHEN
	manager.syncStateWithOtherNodes()
	time.Sleep(time.Millisecond * 100)

	// THEN
	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
	require.Equal(t, manager.state.GetVotedFor(), expectedVote)
	require.Equal(t, manager.state.GetCurrentTerm(), expectedTerm)
}

func TestLeaderBecomesFollowerIfReceivesHeartbeatWithNewerTerm(t *testing.T) {
	// GIVEN
	expectedTerm := state.Term(2)
	var expectedVote *state.NodeId = nil

	manager := NewTestManagerDefault(t)
	defer manager.CloseGracefully()
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "100000") // long timeout to prevent another election

	initialTerm := state.Term(0)
	nodeId := state.NodeId(0)

	manager.state.SetCurrentStatus(state.LEADER)
	manager.state.SetVotedFor(&nodeId)
	manager.state.SetCurrentTerm(initialTerm)

	// WHEN
	success, term := manager.AppendEntries(expectedTerm, 1, 0, 0, 0, []state.LogEntry{})

	// THEN
	require.Equal(t, true, success)
	require.Equal(t, expectedTerm, term)

	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
	require.Equal(t, manager.state.GetVotedFor(), expectedVote)
	require.Equal(t, manager.state.GetCurrentTerm(), expectedTerm)
}

func TestCandidateBecomesFollowerIfReceivesHeartbeatFromNewLeader(t *testing.T) {
	// GIVEN
	var expectedVote *state.NodeId = nil

	manager := NewTestManagerDefault(t)
	defer manager.CloseGracefully()
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "100000") // long timeout to prevent another election

	initialTerm := state.Term(0)
	nodeId := state.NodeId(0)

	manager.state.SetCurrentStatus(state.CANDIDATE)
	manager.state.SetVotedFor(&nodeId)
	manager.state.SetCurrentTerm(initialTerm)

	// WHEN
	success, term := manager.AppendEntries(initialTerm, 1, 0, 0, 0, []state.LogEntry{})

	// THEN
	require.Equal(t, true, success)
	require.Equal(t, initialTerm, term)

	require.Equal(t, manager.state.GetCurrentStatus(), state.FOLLOWER)
	require.Equal(t, manager.state.GetVotedFor(), expectedVote)
	require.Equal(t, manager.state.GetCurrentTerm(), initialTerm)
}

func TestAppendEntryWaitLoopIsTerminatedIfNodeDead(t *testing.T) {
	// GIVEN
	manager := NewTestManagerDefault(t)
	manager.state.SetCurrentStatus(state.LEADER)

	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "100000") // long timeout to prevent another election

	var wg sync.WaitGroup

	// WHEN
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			manager.AppendEntry("test")
		}()
	}
	manager.CloseGracefully() // terminate
	wg.Wait()

	// THEN
	require.Equal(t, manager.state.GetCurrentStatus(), state.DEAD)
}

func TestAppendEntryReturnsSuccessIfWeManagedToReplicateMessage(t *testing.T) {
	// GIVEN
	mockClient := &NodeClientMock{returnValue: true} // emulate success on AppendEntries
	manager := NewTestManager(t, []client.NodeClient{mockClient, mockClient})
	defer manager.CloseGracefully()
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "100000") // long timeout to prevent another election

	// WHEN
	manager.becomeLeader() // replicate message on background
	success := manager.AppendEntry("test")

	//  THEN
	assert.True(t, success)
	assert.Equal(t, len(manager.state.GetCommittedLog()), 1)
}

func TestAppendEntryWaitLoopIsTerminatedAfterSuccessfulReplication(t *testing.T) {
	// GIVEN
	mockClient := &NodeClientMock{returnValue: true} // emulate success on AppendEntries
	manager := NewTestManager(t, []client.NodeClient{mockClient, mockClient})
	defer manager.CloseGracefully()
	t.Setenv("ELECTION_TIMEOUT_MILLISECONDS", "100000") // long timeout to prevent another election

	var wg sync.WaitGroup

	// WHEN
	manager.becomeLeader() // replicate messages on background
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			manager.AppendEntry("test")
		}()
	}
	wg.Wait()

	// THEN
	assert.Equal(t, len(manager.state.GetCommittedLog()), 5)
}

func TestAppendEntriesNewEntriesToEmptyList(t *testing.T) {
	// GIVEN
	manager := NewTestManagerDefault(t)
	defer manager.CloseGracefully()
	// manager list is empty, mode is follower, term is 0
	expectedMessages := []state.LogEntry{
		{
			Term:    0,
			Command: "test 0",
		},
		{
			Term:    1,
			Command: "test 1",
		},
	}

	// WHEN
	successSubmit, _ := manager.AppendEntries(1, 42, 0, 0, 0, expectedMessages)
	successCommit, _ := manager.AppendEntries(1, 42, 2, 1, 2, []state.LogEntry{})

	// THEN
	require.True(t, successSubmit)
	require.True(t, successCommit)

	require.Equal(t, manager.state.GetCurrentTerm(), state.Term(1))
	require.Equal(t, manager.state.GetCommittedLog(), expectedMessages)
}

func TestPublicLogIsEmptyIfNoCommits(t *testing.T) {
	// GIVEN
	manager := NewTestManagerDefault(t)
	defer manager.CloseGracefully()
	// manager list is empty, mode is follower, term is 0
	input := []state.LogEntry{
		{
			Term:    0,
			Command: "test 0",
		},
		{
			Term:    1,
			Command: "test 1",
		},
	}

	// WHEN
	successSubmit, _ := manager.AppendEntries(1, 42, 0, 0, 0, input)

	// THEN
	require.True(t, successSubmit)

	require.Equal(t, manager.state.GetCurrentTerm(), state.Term(1))
	require.Empty(t, manager.state.GetCommittedLog())
}

func TestAppendEntriesIsRejectedIfPrevLogTermIsUnexpected(t *testing.T) {
	// GIVEN
	manager := NewTestManagerDefault(t)
	defer manager.CloseGracefully()

	manager.state.Submit("test")

	input := []state.LogEntry{
		{
			Term:    2,
			Command: "test 1",
		},
	}

	// WHEN
	successSubmit, _ := manager.AppendEntries(1, 42, 1, 1, 0, input)

	// THEN
	require.False(t, successSubmit)
}

func TestAppendEntriesResolvesConflictCorrectly(t *testing.T) {
	// GIVEN
	manager := NewTestManagerDefault(t)
	defer manager.CloseGracefully()

	manager.state.Submit("test")
	manager.state.Submit("override me")

	input := []state.LogEntry{
		{
			Term:    2,
			Command: "test 1",
		},
	}

	expectedMessages := []state.LogEntry{
		{
			Term:    0,
			Command: "test",
		},
		{
			Term:    2,
			Command: "test 1",
		},
	}

	// WHEN
	successSubmit, _ := manager.AppendEntries(2, 42, 1, 0, 0, input)
	successCommit, _ := manager.AppendEntries(2, 42, 2, 2, 2, []state.LogEntry{})

	// THEN
	require.True(t, successSubmit)
	require.True(t, successCommit)

	require.Equal(t, manager.state.GetCurrentTerm(), state.Term(2))
	require.Equal(t, manager.state.GetCommittedLog(), expectedMessages)

}
