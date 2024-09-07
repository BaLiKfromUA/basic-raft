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
	state *statemanager.Manager
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
		state: statemanager.NewManager(state.CandidateId(id), nodes),
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
