/*
Package tracing provides a tracing library, in order to allow for precise
automatic grading in CPSC 416.
A trace provides a precise, ordered representation of what your assignment code
is doing (well, what it says it's doing), which can be used to assess some
things that are unclear from either unit testing or code inspection.
These include correct concurrency management, as well as properly following
any sequencing/causality rules required by the protocol you are implementing.

The tracing library is split into two parts: the tracing server TracingServer,
and the tracing client Tracer.
You should have one instance of Tracer per network node, and you should first
get access to a Trace and then you can record an action by calling Trace.RecordAction(action).
A Trace is a set of recorded actions that are associated with a unique trace ID.
With traces, actions are recorded as part of traces.

Each report will be defined as a struct type, whose fields will list the details
of a given action.
These reports generally double as logging statements, which can be turned
off and on with Tracer.SetShouldPrint.

The TracingServer will aggregate all recorded actions and write them out to
a JSON file, which can be used both for grading and for debugging via
external processing. Moreover, tracing server generates a ShiViz-compatible
log that can be used with ShiViz to visualize the execution of the system.
*/
package tracing
