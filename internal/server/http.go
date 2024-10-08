package server

import (
	"basic-raft/internal/client"
	"basic-raft/internal/message"
	"basic-raft/internal/state"
	"basic-raft/internal/statemanager"
	"encoding/json"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Node struct {
	state statemanager.StateManager
}

type ClientAppendEntryRequest struct {
	Command string `json:"command"`
}

type GetNodeStateResponse struct {
	Term        uint64             `json:"term"`
	State       string             `json:"state"`
	Log         []message.LogEntry `json:"log"`
	Uncommitted []message.LogEntry `json:"uncommitted"`
}

func (s *Node) GetNodeStateHandler(w http.ResponseWriter, _ *http.Request) {
	term, logs, status, uncommited := s.state.GetState()

	respLog := make([]message.LogEntry, len(logs))
	for i, l := range logs {
		respLog[i] = message.LogEntry{
			Term:    uint64(l.Term),
			Command: string(l.Command),
		}
	}

	respUncommitted := make([]message.LogEntry, len(uncommited))
	for i, l := range uncommited {
		respUncommitted[i] = message.LogEntry{
			Term:    uint64(l.Term),
			Command: string(l.Command),
		}
	}

	response := GetNodeStateResponse{
		Term:        uint64(term),
		State:       string(status),
		Log:         respLog,
		Uncommitted: respUncommitted,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	rawResponse, _ := json.Marshal(response)
	_, _ = w.Write(rawResponse)
}

func (s *Node) AppendEntryHandler(w http.ResponseWriter, r *http.Request) {
	var appendEntryRequest ClientAppendEntryRequest

	err := json.NewDecoder(r.Body).Decode(&appendEntryRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// todo: reject if no quorum or not leader

	isAppended := s.state.AppendEntry(state.Command(appendEntryRequest.Command))

	if isAppended {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusConflict)
	}
}

func (s *Node) AppendEntriesHandler(w http.ResponseWriter, r *http.Request) {
	var appendEntriesRequest message.AppendEntriesRequest

	err := json.NewDecoder(r.Body).Decode(&appendEntriesRequest)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var newEntries []state.LogEntry
	for _, entry := range appendEntriesRequest.Entries {
		newEntries = append(newEntries, state.LogEntry{
			Term:    state.Term(entry.Term),
			Command: state.Command(entry.Command),
		})
	}

	success, currentTerm := s.state.AppendEntries(
		state.Term(appendEntriesRequest.Term),
		state.NodeId(appendEntriesRequest.LeaderId),
		appendEntriesRequest.PrevLogIndex,
		state.Term(appendEntriesRequest.Term),
		appendEntriesRequest.LeaderCommit,
		newEntries)

	response := message.AppendEntriesResponse{
		Success: success,
		Term:    uint64(currentTerm),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	rawResponse, _ := json.Marshal(response)
	_, _ = w.Write(rawResponse)
}

func (s *Node) RequestVoteHandler(w http.ResponseWriter, r *http.Request) {
	var voteMessage message.VoteRequest

	err := json.NewDecoder(r.Body).Decode(&voteMessage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	voteGranted, currentTerm := s.state.GrantVote(
		state.Term(voteMessage.Term),
		state.NodeId(voteMessage.CandidateId),
		voteMessage.LastLogIndex,
		state.Term(voteMessage.LastLogTerm),
	)
	response := message.VoteResponse{
		Term:        uint64(currentTerm),
		VoteGranted: voteGranted,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	rawResponse, _ := json.Marshal(response)
	_, _ = w.Write(rawResponse)
}

func createRouter(handler *Node) *mux.Router {
	r := mux.NewRouter()

	r.HandleFunc("/api/v1/append", handler.AppendEntryHandler).Methods("POST")
	r.HandleFunc("/api/v1/state", handler.GetNodeStateHandler).Methods("GET")

	r.HandleFunc("/api/v1/internal/append", handler.AppendEntriesHandler).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/internal/vote", handler.RequestVoteHandler).Methods(http.MethodPost)

	return r
}

// isValidUrl tests a string to determine if it is a well-structured url or not.
func isValidUrl(toTest string) bool {
	_, err := url.ParseRequestURI(toTest)
	if err != nil {
		log.Printf("'%s' is an invalid URL", toTest)
		return false
	}

	u, err := url.Parse(toTest)
	if err != nil || u.Scheme == "" || u.Host == "" {
		log.Printf("'%s' is an invalid URL", toTest)
		return false
	}

	return true
}

func NewNodeServer() *http.Server {
	idStr, ok := os.LookupEnv("NODE_INDEX")
	if !ok {
		log.Fatalf("node id is not set! Use `NODE_INDEX` env variable")
	}

	nodesUrlToken, ok := os.LookupEnv("NODES_URLS")
	if !ok {
		log.Fatalf("node urls is not set! Use `NODE_URLS` env variable")
	}

	nodesUrls := strings.Split(nodesUrlToken, ",")
	if len(nodesUrls) == 0 {
		log.Fatalf("Given `NODE_URLS` token is empty")
	}

	isValid := true
	for _, nodeUrl := range nodesUrls {
		isValid = isValidUrl(nodeUrl) && isValid
	}
	if !isValid {
		log.Fatalf("Given `NODE_URLS` token is invalid: '%s'", nodesUrlToken)

	}

	nodes := make([]client.NodeClient, 0)
	for _, nodeUrl := range nodesUrls {
		nodes = append(nodes, client.NewNodeClient(nodeUrl))
	}

	id, _ := strconv.Atoi(idStr)
	server := &Node{
		state: statemanager.NewManager(state.NodeId(id), nodes),
	}

	port, ok := os.LookupEnv("NODE_PORT")
	if !ok {
		port = "8080"
	}

	headersOk := handlers.AllowedHeaders([]string{"X-Requested-With", "Authorization", "Content-Type"})
	originsOk := handlers.AllowedOrigins([]string{"*"})
	methodsOk := handlers.AllowedMethods([]string{http.MethodGet, http.MethodPost, http.MethodPut})

	srv := &http.Server{
		Handler:      handlers.CORS(originsOk, headersOk, methodsOk)(createRouter(server)),
		Addr:         "0.0.0.0:" + port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}

	server.state.Start()
	srv.RegisterOnShutdown(func() {
		server.state.CloseGracefully()
	})

	return srv
}
