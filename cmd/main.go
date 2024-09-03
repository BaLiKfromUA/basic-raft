package main

import (
	"basic-raft/internal/server"
	"log"
)

func main() {
	srv := server.NewNodeServer()
	log.Printf("Start serving HTTP at %s", srv.Addr)
	log.Fatal(srv.ListenAndServe())
}
