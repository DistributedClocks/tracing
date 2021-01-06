package main

import (
	"log"
	"net"
	"net/rpc"

	"github.com/bestchai/tracing"
)

func main() {
	tracingServer := tracing.NewTracingServer("config.json")

	err := rpc.Register(tracingServer)
	if err != nil {
		log.Fatal("registering tracing server: ", err)
	}

	listener, err := net.Listen("tcp", tracingServer.Config.ServerBind)
	if err != nil {
		log.Fatal("listening on tracing server bind: ", err)
	}

	rpc.Accept(listener) // serve requests forever
}
