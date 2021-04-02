package tracing

import (
	"fmt"
	"log"
	"math/rand"
	"reflect"
	"sync"
	"time"

	"encoding/json"
	"io/ioutil"
	"net/rpc"

	"github.com/DistributedClocks/GoVector/govec"
	"github.com/DistributedClocks/GoVector/govec/vclock"
)

// TracingToken is an abstract token to be used when tracing
// message passing between network nodes.
//
// A one-time-use token can be retrieved using Trace.GenerateToken,
// and the "reception" of that token can be recorded using
// Tracer.ReceiveToken.
type TracingToken []byte

// TracerConfig contains the necessary configuration options for a tracer.
type TracerConfig struct {
	ServerAddress  string // address of the server to send traces to
	TracerIdentity string // a unique string identifying the tracer
	Secret         []byte // TODO
}

// Tracer is the tracing client.
type Tracer struct {
	lock        sync.Mutex
	identity    string
	client      *rpc.Client
	secret      []byte
	shouldPrint bool
	logger      *govec.GoLog
}

// NewTracerFromFile instantiates a fresh tracer client from a configuration file.
//
// Configuration is loaded from the JSON-formatted configFile, which should specify:
// 	- ServerAddress, an ip:port pair identifying a tracing server, as one might pass to rpc.Dial
// 	- TracerIdentity, a unique string giving the tracer an identity that tracks which tracer reported which action
// 	- Secret [TODO]
//
// Note that each instance of Tracer is thread-safe.
func NewTracerFromFile(configFile string) *Tracer {
	configData, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatal("reading config file: ", err)
	}

	config := new(TracerConfig)
	err = json.Unmarshal(configData, config)
	if err != nil {
		log.Fatal("parsing config data: ", err)
	}

	return NewTracer(*config)
}

// NewTracer instantiates a fresh tracer client.
func NewTracer(config TracerConfig) *Tracer {
	client, err := rpc.Dial("tcp", config.ServerAddress)
	if err != nil {
		log.Fatal("dialing server: ", err)
	}

	goLogConfig := govec.GetDefaultConfig()
	goLogConfig.LogToFile = false

	// TODO: make this call optional
	var initialVC vclock.VClock
	err = client.Call("RPCProvider.GetLastVC", config.TracerIdentity, &initialVC)
	if err == nil {
		goLogConfig.InitialVC = initialVC.Copy()
	}

	tracer := &Tracer{
		client:      client,
		identity:    config.TracerIdentity,
		shouldPrint: true,
		logger: govec.InitGoVector(config.TracerIdentity,
			"GoVector-"+config.TracerIdentity, goLogConfig),
	}

	return tracer
}

var (
	seededIDGen = rand.New(rand.NewSource(time.Now().UnixNano()))
	// NewSource returns a new pseudo-random Source seeded with the given value.
	// Unlike the default Source used by top-level functions, this source is not
	// safe for concurrent use by multiple goroutines. Hence the need for a mutex.
	seededIDLock sync.Mutex
)

// CreateTrace is an action that indicates creation of a trace.
type CreateTrace struct{}

// CreateTrace creates a new trace object with a unique ID. Also, it records a
// CreateTrace action.
func (tracer *Tracer) CreateTrace() *Trace {
	seededIDLock.Lock()
	traceID := seededIDGen.Int63()
	seededIDLock.Unlock()

	trace := &Trace{
		ID:     uint64(traceID),
		Tracer: tracer,
	}
	trace.RecordAction(CreateTrace{})
	return trace
}

// getLogString returns a human-readable representation,
// of the form:
//  [TracerID] TraceID=ID StructType field1=val1, field2=val2, ...
// Note that we are not logging vector clock, but we send it to the
// tracing server.
func (tracer *Tracer) getLogString(trace *Trace, record interface{}) string {
	recVal := reflect.ValueOf(record)
	recType := reflect.TypeOf(record)
	numFields := recVal.NumField()

	logFormat := "[%s] %s"
	logParams := []interface{}{tracer.identity, recType.Name()}
	if trace != nil {
		logFormat = "[%s] TraceID=%d %s"
		logParams = []interface{}{tracer.identity, trace.ID, recType.Name()}
	}
	{
		isFirst := true
		for i := 0; i < numFields; i++ {
			if !isFirst {
				logFormat += ", "
			} else {
				logFormat += " "
				isFirst = false
			}
			logFormat += recType.Field(i).Name + "=%v"
			// strip all pointer types (when not nil), so we log the pointed-to value
			valueToLog := recVal.Field(i)
			for valueToLog.Kind() == reflect.Ptr && !valueToLog.IsNil() {
				valueToLog = reflect.Indirect(valueToLog)
			}
			logParams = append(logParams, valueToLog.Interface())
		}
	}
	return fmt.Sprintf(logFormat, logParams...)
}

func (tracer *Tracer) recordAction(trace *Trace, record interface{}, isLocalEvent bool) {
	if isLocalEvent {
		tracer.logger.LogLocalEvent(tracer.getLogString(trace, record), govec.GetDefaultLogOptions())
	}
	if tracer.shouldPrint {
		log.Print(tracer.getLogString(trace, record))
	}

	// send data to tracer server
	marshaledRecord, err := json.Marshal(record)
	if err != nil {
		log.Print("error marshaling record: ", err)
	}
	err = tracer.client.Call("RPCProvider.RecordAction", RecordActionArg{
		TracerIdentity: tracer.identity,
		TraceID:        trace.ID,
		RecordName:     reflect.TypeOf(record).Name(),
		Record:         marshaledRecord,
		VectorClock:    tracer.logger.GetCurrentVC(),
	}, nil)
	if err != nil {
		log.Print("error recording action to remote: ", err)
	}
}

// ReceiveTokenTrace is an action that indicated receiption of a token.
type ReceiveTokenTrace struct {
	Token TracingToken // the token that was received.
}

// ReceiveToken records the token by calling RecordAction with
// ReceiveTokenTrace.
func (tracer *Tracer) ReceiveToken(token TracingToken) *Trace {
	tracer.lock.Lock()
	defer tracer.lock.Unlock()

	record := ReceiveTokenTrace{Token: token}
	var traceID uint64
	tracer.logger.UnpackReceive(tracer.getLogString(nil, record),
		token, &traceID, govec.GetDefaultLogOptions())
	trace := &Trace{
		ID:     traceID,
		Tracer: tracer,
	}
	tracer.recordAction(trace, record, false)
	return trace
}

// Close cleans up the connection to the tracing server.
// To allow for tracing long-running processes and Ctrl^C, this call is
// unnecessary, as there is no connection state. After this call, the use of
// any previously generated local Trace instances leads to undefined behavior.
func (tracer *Tracer) Close() error {
	tracer.lock.Lock()
	defer tracer.lock.Unlock()
	return tracer.client.Close()
}

// SetShouldPrint determines whether RecordAction should log the action being
// recorded as it sends the action to the tracing server. In other words, it
// indicates that the Tracer instance should log (print to stdout) the recorded
// actions or not.
// For more complex applications which have long, involved traces, it may be
// helpful to silence trace logging.
func (tracer *Tracer) SetShouldPrint(shouldPrint bool) {
	tracer.lock.Lock()
	defer tracer.lock.Unlock()

	tracer.shouldPrint = shouldPrint
}
