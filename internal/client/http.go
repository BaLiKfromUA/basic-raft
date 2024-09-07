package client

import (
	"basic-raft/internal/message"
	"basic-raft/internal/state"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"
)

type NodeClient interface {
	RequestVote(candidateId state.NodeId, state state.State) (bool, state.Term, error)
	AppendEntries(leaderId state.NodeId, state state.State) (bool, state.Term, error)
}

type httpClient struct {
	client *http.Client
	addr   string
}

func (c *httpClient) RequestVote(candidateId state.NodeId, s state.State) (bool, state.Term, error) {
	req := message.NewVoteRequest(uint64(candidateId), s)

	reqBytes, err := json.Marshal(&req)
	if err != nil {
		log.Printf("error during encoding to json: %v", err)
		return false, 0, err
	}

	resp, err := c.client.Post(c.addr+"/api/v1/vote", "application/json", bytes.NewReader(reqBytes))
	if err != nil {
		log.Printf("error during vote request to %s: %v", c.addr, err)
		return false, 0, err
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("unexpected status code after vote request to %s: %v", c.addr, resp.Status)
		return false, 0, errors.New(resp.Status)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error during decoding response body: %v", err)
		return false, 0, err
	}

	var voteResponse message.VoteResponse
	err = json.Unmarshal(responseBody, &voteResponse)
	if err != nil {
		log.Printf("error during decoding response body: %v", err)
		return false, 0, err
	}

	return voteResponse.VoteGranted, state.Term(voteResponse.Term), nil
}

func (c *httpClient) AppendEntries(leaderId state.NodeId, s state.State) (bool, state.Term, error) {
	req := message.NewAppendEntriesRequest(uint64(leaderId), s)

	reqBytes, err := json.Marshal(&req)
	if err != nil {
		log.Printf("error during encoding to json: %v", err)
		return false, 0, err
	}

	resp, err := c.client.Post(c.addr+"/api/v1/append", "application/json", bytes.NewReader(reqBytes))
	if err != nil {
		log.Printf("error during append request to %s: %v", c.addr, err)
		return false, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("unexpected status code after append request to %s: %v", c.addr, resp.Status)
		return false, 0, errors.New(resp.Status)
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("error during decoding response body: %v", err)
		return false, 0, err
	}

	var appendResponse message.AppendEntriesResponse
	err = json.Unmarshal(responseBody, &appendResponse)
	if err != nil {
		log.Printf("error during decoding response body: %v", err)
		return false, 0, err
	}

	return appendResponse.Success, state.Term(appendResponse.Term), nil
}

func NewNodeClient(addr string) NodeClient {
	requestTimeout := 150
	requestTimeoutStr, ok := os.LookupEnv("REQUEST_TIMEOUT_MILLISECONDS")
	if ok {
		requestTimeout, _ = strconv.Atoi(requestTimeoutStr)
	}

	return &httpClient{
		client: &http.Client{Timeout: time.Duration(requestTimeout) * time.Millisecond},
		addr:   addr,
	}
}
