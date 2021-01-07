// In order to allow for precise automatic grading in CPSC 416, the course provides a tracing library.
// A trace provides a precise, ordered representation of what your assignment code is doing
// (well, what it says it's doing), which can be used to assess some things that are
// unclear from either unit testing or code inspection.
// These include correct concurrency management, as well as properly following
// any sequencing/causality rules required by the protocol you are implementing.
//
// The tracing library is split into two parts: the tracing server TracingServer,
// and the tracing client Tracer.
// You should one instance of Tracer per network node, and you should report
// any relevant actions that node takes via Tracer.RecordAction.
// Each report will be defined as a struct type, whose fields will list the details
// of a given action.
// These reports generally double as logging statements, which can be turned
// off and on with Tracer.SetShouldPrint.
//
// The TracingServer will aggregate all recorded actions and write them out to
// a JSON file, which can be used both for grading and for debugging via
// external processing.
package tracing

import (
	"log"
	"os"
	"reflect"
	"sync"

	"encoding/json"
	"io/ioutil"
	"net/rpc"
)

// TracingToken is an abstract token to be used when tracing
// message passing between network nodes.
//
// A one-time-use token can be retrieved using Tracer.GenerateToken,
// and the "reception" of that token can be recorded using
// Tracer.ReceiveToken.
type TracingToken []byte

type TracingServerConfig struct {
	ServerBind string // the ip:port pair to which the server should bind, as one might pass to net.Listen
	Secret     []byte
	OutputFile string // the output filename, where the tracing JSON will be written
}

// TracingServer should be used with rpc.Register, as an RPC target.
type TracingServer struct {
	recordFile    *os.File
	recordEncoder *json.Encoder
	Config        *TracingServerConfig
}

// NewTracingServer instantiates a new tracing server.
//
// Configuration is loaded from the JSON-formatted configFile, whose fields correspond to
// the TracingServerConfig struct.
//
// Note that each instance of Tracer is thread-safe.
//
// Note also that this function does not actually set up any RPC/server binding, it handles
// everything up to that point (opening output files, setting up internals).
func NewTracingServer(configFile string) *TracingServer {
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatal("reading config file: ", err)
	}

	config := new(TracingServerConfig)
	err = json.Unmarshal(configData, config)
	if err != nil {
		log.Fatal("parsing config data: ", err)
	}

	recordFile, err := os.Create(config.OutputFile)
	if err != nil {
		log.Fatal("opening output file: ", err)
	}
	tracingServer := &TracingServer{
		recordFile:    recordFile,
		recordEncoder: json.NewEncoder(recordFile),
		Config:        config,
	}
	return tracingServer
}

type RecordActionArg struct {
	TracerIdentity string
	Record         interface{}
}
type RecordActionResult struct{}

// RecordAction writes the Record field of the argument as a JSON-encoded record, tagging the record with its type name.
// It also tags the result with TracerIdentity, which tracks the identity given to the tracer reporting the event
func (tracingServer *TracingServer) RecordAction(arg RecordActionArg, result *RecordActionResult) error {
	type TraceRecord struct {
		TracerIdentity string
		Name           string
		Body           interface{}
	}
	record := arg.Record
	wrappedRecord := TraceRecord{
		TracerIdentity: arg.TracerIdentity,
		Name:           reflect.TypeOf(record).String(),
		Body:           record,
	}
	return tracingServer.recordEncoder.Encode(wrappedRecord)
}

type Tracer struct {
	lock        sync.Mutex
	identity    string
	client      *rpc.Client
	secret      []byte
	shouldPrint bool
}

// NewTracer instantiates a fresh tracer client.
//
// Configuration is loaded from the JSON-formatted configFile, which should specify:
// 	- ServerAddress, an ip:port pair identifying a tracing server, as one might pass to rpc.Dial
// 	- TracerIdentity, a unique string giving the tracer an identity that tracks which tracer reported which action
// 	- Secret [TODO]
//
// Note that each instance of Tracer is thread-safe.
func NewTracer(configFile string) *Tracer {
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatal("reading config file: ", err)
	}

	type Config struct {
		ServerAddress  string
		TracerIdentity string
		Secret         []byte
	}
	config := new(Config)
	err = json.Unmarshal(configData, config)
	if err != nil {
		log.Fatal("parsing config data: ", err)
	}

	client, err := rpc.Dial("tcp", config.ServerAddress)
	if err != nil {
		log.Fatal("dialing server: ", err)
	}

	tracer := &Tracer{
		client:      client,
		identity:    config.TracerIdentity,
		shouldPrint: true,
	}

	return tracer
}

// RecordAction ensures that the record is recorded by the tracing server,
// and optionally logs the record's contents. record can be any struct value; its contents will be extracted via reflection.
//
// For example, consider (with tracer id "id"):
// 	struct MyRecord { Foo string; Bar string }
// and the call:
// 	RecordAction(MyRecord{ Foo: "foo", Bar: "bar" })
//
// This will result in a log (and relevant tracing data) that contains the following:
// 	[id] MyRecord Foo="foo", Bar="bar"
func (tracer *Tracer) RecordAction(record interface{}) {
	tracer.lock.Lock()
	defer tracer.lock.Unlock()

	if tracer.shouldPrint {
		recVal := reflect.ValueOf(record)
		recType := reflect.TypeOf(record)
		numFields := recVal.NumField()

		// log a human-readable representation, of the form "[identity] StructType field1=val1, field2=val2, ..."
		logStr := "[%s] %s "
		logParams := []interface{}{tracer.identity, recType.Name()}
		{
			isFirst := true
			for i := 0; i < numFields; i++ {
				if !isFirst {
					logStr += ", "
				} else {
					isFirst = false
				}
				logStr += recType.Field(i).Name + "=%v"
				logParams = append(logParams, recVal.Field(i).Interface())
			}
		}

		log.Printf(logStr, logParams...)
	}

	// send data to tracer server
	err := tracer.client.Call("TracingServer.RecordAction", &record, nil)
	if err != nil {
		log.Fatal("recording action to remote: ", err)
	}
}

type GenerateTokenTrace struct {
	Token TracingToken // the generated tracing token
}

// Produces a fresh TracingToken, and records the event via RecordAction.
// This allows analysis of the resulting trace to correlate token generation
// and token reception.
func (tracer *Tracer) GenerateToken() TracingToken {
	token := []byte{} // TODO: actually interesting, identifying data
	tracer.RecordAction(GenerateTokenTrace{Token: token})
	return token
}

type ReceiveTokenTrace struct {
	Token TracingToken // the token that was received. Has some secret, internal meaning
}

// ReceiveToken records the token by calling RecordAction with
// ReceiveTokenTrace.
func (tracer *Tracer) ReceiveToken(token TracingToken) {
	tracer.RecordAction(ReceiveTokenTrace{Token: token})
}

// Close cleans up the connection to the tracing server.
// To allow for tracing long-running processes and Ctrl^C, this call is unnecessary, as
// there is no connection state.
func (tracer *Tracer) Close() error {
	tracer.lock.Lock()
	defer tracer.lock.Unlock()
	return tracer.client.Close()
}

// SetShouldPrint determines whether RecordAction should log the action being recorded as
// it sends the action to the tracing server.
// For more complex applications which have long, involved traces, it may be helpful to
// silence trace logging.
func (tracer *Tracer) SetShouldPrint(shouldPrint bool) {
	tracer.shouldPrint = shouldPrint
}
