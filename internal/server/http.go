package server

import (
	"basic-raft/internal/message"
	"basic-raft/internal/state"
	"basic-raft/internal/statemanager"
	"encoding/json"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"net/http"
	"os"
	"time"
)

type Node struct {
	state *statemanager.Manager
	// todo: implement state management and add it into this entity as part of state
}

func (s *Node) AppendEntriesHandler(w http.ResponseWriter, r *http.Request) {

}

func (s *Node) RequestVoteHandler(w http.ResponseWriter, r *http.Request) {
	var voteMessage message.VoteRequest

	err := json.NewDecoder(r.Body).Decode(&voteMessage)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// todo: handle all fields from request
	voteGranted, currentTerm := s.state.GrantVote(state.Term(voteMessage.Term), state.CandidateId(voteMessage.CandidateId))
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

	r.HandleFunc("/api/v1/append", handler.AppendEntriesHandler).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/vote", handler.RequestVoteHandler).Methods(http.MethodPut)

	return r
}

func NewNodeServer() *http.Server {
	server := &Node{
		state: statemanager.NewManager(),
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

	srv.RegisterOnShutdown(func() {
		// TODO:
	})

	return srv
}
