package tracing

import "github.com/DistributedClocks/GoVector/govec"

type Trace struct {
	ID     uint64
	Tracer *Tracer
}

// RecordAction ensures that the record is recorded by the tracing server,
// and optionally logs the record's contents. record can be any struct value;
// its contents will be extracted via reflection.
// RecordAction implementation is thread-safe.
//
// For example, consider (with tracer id "id"):
// 	struct MyRecord { Foo string; Bar string }
// and the call:
// 	RecordAction(MyRecord{ Foo: "foo", Bar: "bar" })
//
// This will result in a log (and relevant tracing data) that contains the following:
//  [id] MyRecord Foo="foo", Bar="bar"
func (trace *Trace) RecordAction(record interface{}) {
	trace.Tracer.recordAction(trace, record, true)
}

type PrepareTokenTrace struct{}

type GenerateTokenTrace struct {
	Token TracingToken // the generated tracing token
}

// GenerateToken produces a fresh TracingToken, and records the event via RecordAction.
// This allows analysis of the resulting trace to correlate token generation
// and token reception.
func (trace *Trace) GenerateToken() TracingToken {
	token := trace.Tracer.logger.PrepareSend(trace.Tracer.getLogString(trace, PrepareTokenTrace{}),
		trace.ID, govec.GetDefaultLogOptions())
	trace.Tracer.recordAction(trace, GenerateTokenTrace{Token: token}, false)
	return token
}
