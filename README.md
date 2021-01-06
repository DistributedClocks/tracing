# tracing
A light-weight library for manual distributed system tracing.

In order to allow for precise automatic grading in CPSC 416, the course provides a tracing library.
A trace provides a precise, ordered representation of what your assignment code is doing
(well, what it says it's doing), which can be used to assess some things that are
unclear from either unit testing or code inspection.
These include correct concurrency management, as well as properly following
any sequencing/causality rules required by the protocol you are implementing.

The tracing library is split into two parts: the tracing server TracingServer,
and the tracing client Tracer.
You should one instance of Tracer per network node, and you should report
any relevant actions that node takes via Tracer.RecordAction.
Each report will be defined as a struct type, whose fields will list the details
of a given action.
These reports generally double as logging statements, which can be turned
off and on with Tracer.SetShouldPrint.

The TracingServer will aggregate all recorded actions and write them out to
a JSON file, which can be used both for grading and for debugging via
external processing.


