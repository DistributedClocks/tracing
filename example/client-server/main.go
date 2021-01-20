package main

import (
	"fmt"
	"log"
	"net"
	"net/rpc"

	"github.com/DistributedClocks/tracing"
)

type Person struct {
	name   string
	tracer *tracing.Tracer
}

type Args struct {
	Token tracing.TracingToken
}

type Reply struct {
	Name  string
	Token tracing.TracingToken
}

func (p *Person) GetName(args Args, reply *Reply) error {
	p.tracer.ReceiveToken(args.Token)
	reply.Name = p.name
	reply.Token = p.tracer.GenerateToken()
	return nil
}

var serverPort = ":1234"

type ServerStart struct {
	Port string
}

func server(done chan int) {
	tracer := tracing.NewTracerFromFile("server_config.json")
	defer tracer.Close()

	person := &Person{name: "John Doe", tracer: tracer}
	rpc.Register(person)

	tcpAddr, err := net.ResolveTCPAddr("tcp", serverPort)
	if err != nil {
		log.Fatal(err)
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		log.Fatal(err)
	}

	tracer.RecordAction(ServerStart{Port: serverPort})
	done <- 1

	rpc.Accept(listener)
}

type ClientStart struct {
	ServerPort string
}

type ClientFinish struct {
	ServerPort string
}

func client(done chan int) {
	tracer := tracing.NewTracerFromFile("client_config.json")
	defer tracer.Close()

	client, err := rpc.Dial("tcp", serverPort)
	if err != nil {
		log.Fatal("dialing:", err)
	}
	tracer.RecordAction(ClientStart{ServerPort: serverPort})

	args := Args{Token: tracer.GenerateToken()}
	var reply *Reply
	err = client.Call("Person.GetName", args, &reply)
	if err != nil {
		log.Fatal("person error:", err)
	}
	fmt.Printf("GetName: %s\n", reply.Name)
	tracer.ReceiveToken(reply.Token)

	tracer.RecordAction(ClientFinish{ServerPort: serverPort})
	done <- 1
}

func main() {
	tracingServer := tracing.NewTracingServerFromFile("tracing_server_config.json")
	err := tracingServer.Open()
	if err != nil {
		log.Fatal(err)
	}
	defer tracingServer.Close()
	go tracingServer.Accept()

	done := make(chan int, 1)
	go server(done)
	<-done
	go client(done)
	<-done
}
