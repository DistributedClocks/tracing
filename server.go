package tracing

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/rpc"
	"os"

	"github.com/DistributedClocks/GoVector/govec/vclock"
)

// TracingServerConfig contains the necessary configuration options for a
// tracing server.
type TracingServerConfig struct {
	ServerBind       string // the ip:port pair to which the server should bind, as one might pass to net.Listen
	Secret           []byte
	OutputFile       string // the output filename, where the tracing records JSON will be written
	ShivizOutputFile string // the shiviz-compatible output filename
}

// TracingServer should be used with rpc.Register, as an RPC target.
type TracingServer struct {
	Listener         net.Listener
	acceptDone       chan struct{}
	rpcServer        *rpc.Server
	recordFile       *os.File
	recordEncoder    *json.Encoder
	Config           *TracingServerConfig
	shivizRecordFile *os.File
	shivizLogger     *shivizLogger
}

// ActionRecorder is an abstraction to prevent registering non-RPC functions
// in the RPC server. ActionRecorder should be used with rpc.Register, as an
// RPC target.
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

// Open creates the related files for the tracing server and starts an RPC server
// on the specified address.
func (tracingServer *TracingServer) Open() error {
	if tracingServer.recordFile == nil {
		recordFile, err := os.Create(tracingServer.Config.OutputFile)
		if err != nil {
			return err
		}
		tracingServer.recordFile = recordFile
		tracingServer.recordEncoder = json.NewEncoder(recordFile)
	}
	if tracingServer.shivizRecordFile == nil {
		shivizRecordFile, err := os.Create(tracingServer.Config.ShivizOutputFile)
		if err != nil {
			return err
		}
		shivizLogger, err := newShivizLogger(shivizRecordFile)
		if err != nil {
			return err
		}
		tracingServer.shivizRecordFile = shivizRecordFile
		tracingServer.shivizLogger = shivizLogger
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

// Accept accepts connections on the listener and serves requests for each incoming
// connection. Accept blocks until the listener returns a non-nil error.
// This implementation matches exactly the implementation of `rpc.Accept` from
// https://golang.org/src/net/rpc/server.go?s=18334:18380#L613,
// except it does not log the listner.Accept error.
func (tracingServer *TracingServer) Accept() {
	for {
		conn, err := tracingServer.Listener.Accept()
		if err != nil {
			break
		}
		go tracingServer.rpcServer.ServeConn(conn)
	}
	tracingServer.acceptDone <- struct{}{}
}

// Close closes the related opened files and the RPC server.
func (tracingServer *TracingServer) Close() error {
	if err := tracingServer.Listener.Close(); err != nil {
		return err
	}
	<-tracingServer.acceptDone

	// close the output files, once the request loop is fully complete
	if err := tracingServer.recordFile.Close(); err != nil {
		return err
	}
	tracingServer.recordFile = nil

	if err := tracingServer.shivizRecordFile.Close(); err != nil {
		return err
	}
	tracingServer.shivizRecordFile = nil

	return nil
}

// RecordActionArg indicates RecordAction RPC argument.
type RecordActionArg struct {
	TracerIdentity string
	TraceID        uint64
	RecordName     string
	Record         []byte
	VectorClock    vclock.VClock
}

// RecordActionResult indicates RecordActionRPC output.
type RecordActionResult struct{}

// TraceRecord indicates the structure of each recorded trace
type TraceRecord struct {
	TracerIdentity string
	TraceID        uint64
	Tag            string
	Body           json.RawMessage
	VectorClock    vclock.VClock
}

// RecordAction writes the Record field of the argument as a JSON-encoded record,
// tagging the record with its type name.
// It also tags the result with TracerIdentity, which tracks the identity given
// to the tracer reporting the event.
func (actionRecorder *ActionRecorder) RecordAction(arg RecordActionArg, result *RecordActionResult) error {
	wrappedRecord := TraceRecord{
		TracerIdentity: arg.TracerIdentity,
		TraceID:        arg.TraceID,
		Tag:            arg.RecordName,
		Body:           arg.Record,
		VectorClock:    arg.VectorClock,
	}
	if err := actionRecorder.server.recordEncoder.Encode(wrappedRecord); err != nil {
		return err
	}
	if err := actionRecorder.server.shivizLogger.log(wrappedRecord); err != nil {
		return err
	}
	return nil
}
