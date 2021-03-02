package tracing

import (
	"fmt"
	"log"
	"reflect"
	"sync"

	"encoding/json"
	"io/ioutil"
	"net/rpc"

	"github.com/DistributedClocks/GoVector/govec"
)

// TracingToken is an abstract token to be used when tracing
// message passing between network nodes.
//
// A one-time-use token can be retrieved using Tracer.GenerateToken,
// and the "reception" of that token can be recorded using
// Tracer.ReceiveToken.
type TracingToken []byte

type TracerConfig struct {
	ServerAddress  string // address of the server to send traces to
	TracerIdentity string // a unique string identifying the tracer
	Secret         []byte // TODO
}

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

	tracer := &Tracer{
		client:      client,
		identity:    config.TracerIdentity,
		shouldPrint: true,
		logger: govec.InitGoVector(config.TracerIdentity,
			"GoVector-"+config.TracerIdentity, govec.GetDefaultConfig()),
	}

	return tracer
}

// getLogString returns a human-readable representation,
// of the form "[identity] StructType field1=val1, field2=val2, ..."
func (tracer *Tracer) getLogString(record interface{}) string {
	recVal := reflect.ValueOf(record)
	recType := reflect.TypeOf(record)
	numFields := recVal.NumField()

	logFormat := "[%s] %s"
	logParams := []interface{}{tracer.identity, recType.Name()}
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
			logParams = append(logParams, recVal.Field(i).Interface())
		}
	}
	return fmt.Sprintf(logFormat, logParams...)
}

func (tracer *Tracer) recordAction(record interface{}, isLocalEvent bool) {
	tracer.lock.Lock()
	defer tracer.lock.Unlock()

	if tracer.shouldPrint {
		log.Printf(tracer.getLogString(record))
	}
	if isLocalEvent {
		tracer.logger.LogLocalEvent(tracer.getLogString(record), govec.GetDefaultLogOptions())
	}

	// send data to tracer server
	marshaledRecord, err := json.Marshal(record)
	if err != nil {
		log.Print("error marshaling record: ", err)
	}
	err = tracer.client.Call("ActionRecorder.RecordAction", RecordActionArg{
		TracerIdentity: tracer.identity,
		RecordName:     reflect.TypeOf(record).Name(),
		Record:         marshaledRecord,
	}, nil)
	if err != nil {
		log.Print("error recording action to remote: ", err)
	}
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
	tracer.recordAction(record, true)
}

type PrepareTokenTrace struct{}

type GenerateTokenTrace struct {
	Token TracingToken // the generated tracing token
}

// GenerateToken produces a fresh TracingToken, and records the event via RecordAction.
// This allows analysis of the resulting trace to correlate token generation
// and token reception.
func (tracer *Tracer) GenerateToken() TracingToken {
	token := tracer.logger.PrepareSend(tracer.getLogString(PrepareTokenTrace{}),
		nil, govec.GetDefaultLogOptions())
	tracer.recordAction(GenerateTokenTrace{Token: token}, false)
	return token
}

type ReceiveTokenTrace struct {
	Token TracingToken // the token that was received.
}

// ReceiveToken records the token by calling RecordAction with
// ReceiveTokenTrace.
func (tracer *Tracer) ReceiveToken(token TracingToken) {
	record := ReceiveTokenTrace{Token: token}
	tracer.recordAction(record, false)
	tracer.logger.UnpackReceive(tracer.getLogString(record),
		token, nil, govec.GetDefaultLogOptions())
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
