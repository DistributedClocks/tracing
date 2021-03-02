package tracing

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"
)

type TracingServerConfig struct {
	ServerBind string // the ip:port pair to which the server should bind, as one might pass to net.Listen
	Secret     []byte
	OutputFile string // the output filename, where the tracing JSON will be written
}

// TracingServer should be used with rpc.Register, as an RPC target.
type TracingServer struct {
	Listener      net.Listener
	acceptDone    chan struct{}
	rpcServer     *rpc.Server
	recordFile    *os.File
	recordEncoder *json.Encoder
	Config        *TracingServerConfig
}

// ActionRecorder is an abstraction to prevent registering
// non-action recording functions in the rpc server
type ActionRecorder struct {
	server *TracingServer
}

// NewTracingServerFromFile instantiates a new tracing server from a configuration file.
//
// Configuration is loaded from the JSON-formatted configFile, whose fields correspond to
// the TracingServerConfig struct.
//
// Note that each instance of Tracer is thread-safe.
//
// Note also that this function does not actually set up any RPC/server binding, it handles
// everything up to that point (opening output files, setting up internals).
func NewTracingServerFromFile(configFile string) *TracingServer {
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatal("reading config file: ", err)
	}

	config := new(TracingServerConfig)
	err = json.Unmarshal(configData, config)
	if err != nil {
		log.Fatal("parsing config data: ", err)
	}

	return NewTracingServer(*config)
}

// NewTracingServer instantiates a new tracing server.
func NewTracingServer(config TracingServerConfig) *TracingServer {
	tracingServer := &TracingServer{
		acceptDone: make(chan struct{}),
		Config:     &config,
	}
	return tracingServer
}

func (tracingServer *TracingServer) Open() error {
	if tracingServer.recordFile == nil {
		recordFile, err := os.Create(tracingServer.Config.OutputFile)
		if err != nil {
			return err
		}
		tracingServer.recordFile = recordFile
		tracingServer.recordEncoder = json.NewEncoder(recordFile)
	}

	tracingServer.rpcServer = rpc.NewServer()
	actionRecorder := &ActionRecorder{server: tracingServer}
	err := tracingServer.rpcServer.Register(actionRecorder)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", tracingServer.Config.ServerBind)
	if err != nil {
		return err
	}
	tracingServer.Listener = listener

	return nil
}

func (tracingServer *TracingServer) Accept() {
	// This matches exactly the implementation of `rpc.Accept`
	// https://golang.org/src/net/rpc/server.go?s=18334:18380#L613
	// except it does not log the listner.Accept error
	for {
		conn, err := tracingServer.Listener.Accept()
		if err != nil {
			break
		}
		go tracingServer.rpcServer.ServeConn(conn)
	}
	tracingServer.acceptDone <- struct{}{}
}

func (tracingServer *TracingServer) Close() error {
	err := tracingServer.Listener.Close()
	if err != nil {
		return err
	}
	<-tracingServer.acceptDone
	// close the output file, once the request loop is fully complete
	err = tracingServer.recordFile.Close()
	tracingServer.recordFile = nil
	return err
}

type RecordActionArg struct {
	TracerIdentity string
	RecordName     string
	Record         []byte
}
type RecordActionResult struct{}

// RecordAction writes the Record field of the argument as a JSON-encoded record, tagging the record with its type name.
// It also tags the result with TracerIdentity, which tracks the identity given to the tracer reporting the event
func (actionRecorder *ActionRecorder) RecordAction(arg RecordActionArg, result *RecordActionResult) error {
	type TraceRecord struct {
		TracerIdentity string
		Tag            string
		Body           json.RawMessage
	}
	wrappedRecord := TraceRecord{
		TracerIdentity: arg.TracerIdentity,
		Tag:            arg.RecordName,
		Body:           arg.Record,
	}
	return actionRecorder.server.recordEncoder.Encode(wrappedRecord)
}
