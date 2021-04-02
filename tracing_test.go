package tracing

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

type TestAction struct {
	Foo string
}

type TestAction2 struct {
	Foo *string
}

func readTraceOutputFile(t *testing.T, fileName string) (outputs []interface{}) {
	outF, err := os.Open(fileName)
	if err != nil {
		t.Fatal(err)
	}
	defer outF.Close()
	decoder := json.NewDecoder(outF)
	decoder.UseNumber()
	outputs = []interface{}{}
	for decoder.More() {
		var output interface{}
		err = decoder.Decode(&output)
		if err != nil {
			t.Fatal(err)
		}
		outputs = append(outputs, output)
	}
	return
}

func readShivizOutputFile(t *testing.T, fileName string) (outputs []string) {
	outF, err := os.Open(fileName)
	if err != nil {
		t.Fatal(err)
	}
	defer outF.Close()

	scanner := bufio.NewScanner(outF)
	scanner.Split(bufio.ScanLines)
	for scanner.Scan() {
		outputs = append(outputs, scanner.Text())
	}
	return
}

func intToJSONNubmer(x int) json.Number {
	return json.Number(strconv.Itoa(x))
}

func traceIDtoJSONNumber(id uint64) json.Number {
	return json.Number(strconv.FormatUint(id, 10))
}

func TestOneRecord(t *testing.T) {
	outputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outputFile.Name())

	shivizOutputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(shivizOutputFile.Name())

	var traceID uint64
	(func() {
		server := NewTracingServer(TracingServerConfig{
			ServerBind:       ":0",
			Secret:           []byte{},
			OutputFile:       outputFile.Name(),
			ShivizOutputFile: shivizOutputFile.Name(),
		})

		err = server.Open()
		if err != nil {
			t.Fatal(err)
		}
		serverBind := server.Listener.Addr().String()
		defer server.Close()
		go server.Accept()

		client1 := NewTracer(TracerConfig{
			ServerAddress:  serverBind,
			TracerIdentity: "client1",
			Secret:         []byte{},
		})
		defer client1.Close()

		trace := client1.CreateTrace()
		traceID = trace.ID
		trace.RecordAction(TestAction{Foo: "foo"})

		bar := "bar"
		trace.RecordAction(TestAction2{Foo: &bar})
		trace.RecordAction(TestAction2{Foo: nil})
	})()

	outputs := readTraceOutputFile(t, outputFile.Name())
	expected := []interface{}{
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(traceID),
			"Tag":            "CreateTrace",
			"Body":           map[string]interface{}{},
			"VectorClock": map[string]interface{}{
				"client1": intToJSONNubmer(1),
			},
		},
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(traceID),
			"Tag":            "TestAction",
			"Body":           map[string]interface{}{"Foo": "foo"},
			"VectorClock": map[string]interface{}{
				"client1": intToJSONNubmer(2),
			},
		},
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(traceID),
			"Tag":            "TestAction2",
			"Body":           map[string]interface{}{"Foo": "bar"},
			"VectorClock": map[string]interface{}{
				"client1": intToJSONNubmer(3),
			},
		},
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(traceID),
			"Tag":            "TestAction2",
			"Body":           map[string]interface{}{"Foo": nil},
			"VectorClock": map[string]interface{}{
				"client1": intToJSONNubmer(4),
			},
		},
	}

	if !cmp.Equal(outputs, expected) {
		t.Fatalf("expected trace %v did not equal actual trace %v", expected, outputs)
	}
}

func TestTwoClients(t *testing.T) {
	outputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outputFile.Name())

	shivizOutputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(shivizOutputFile.Name())

	var trace1ID, trace2ID uint64
	(func() {
		server := NewTracingServer(TracingServerConfig{
			ServerBind:       ":0",
			Secret:           []byte{},
			OutputFile:       outputFile.Name(),
			ShivizOutputFile: shivizOutputFile.Name(),
		})

		err = server.Open()
		if err != nil {
			t.Fatal(err)
		}
		serverBind := server.Listener.Addr().String()
		defer server.Close()
		go server.Accept()

		client1 := NewTracer(TracerConfig{
			ServerAddress:  serverBind,
			TracerIdentity: "client1",
			Secret:         []byte{},
		})
		defer client1.Close()
		client2 := NewTracer(TracerConfig{
			ServerAddress:  serverBind,
			TracerIdentity: "client2",
			Secret:         []byte{},
		})
		defer client2.Close()

		trace1 := client1.CreateTrace()
		trace1ID = trace1.ID
		trace1.RecordAction(TestAction{Foo: "foo"})

		trace2 := client2.CreateTrace()
		trace2ID = trace2.ID
		trace2.RecordAction(TestAction{Foo: "bar"})
	})()

	outputs := readTraceOutputFile(t, outputFile.Name())
	expected := []interface{}{
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(trace1ID),
			"Tag":            "CreateTrace",
			"Body":           map[string]interface{}{},
			"VectorClock": map[string]interface{}{
				"client1": intToJSONNubmer(1),
			},
		},
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(trace1ID),
			"Tag":            "TestAction",
			"Body":           map[string]interface{}{"Foo": "foo"},
			"VectorClock": map[string]interface{}{
				"client1": intToJSONNubmer(2),
			},
		},
		map[string]interface{}{
			"TracerIdentity": "client2",
			"TraceID":        traceIDtoJSONNumber(trace2ID),
			"Tag":            "CreateTrace",
			"Body":           map[string]interface{}{},
			"VectorClock": map[string]interface{}{
				"client2": intToJSONNubmer(1),
			},
		},
		map[string]interface{}{
			"TracerIdentity": "client2",
			"TraceID":        traceIDtoJSONNumber(trace2ID),
			"Tag":            "TestAction",
			"Body":           map[string]interface{}{"Foo": "bar"},
			"VectorClock": map[string]interface{}{
				"client2": intToJSONNubmer(2),
			},
		},
	}

	if !cmp.Equal(outputs, expected) {
		t.Fatalf("expected trace %v did not equal actual trace %v", expected, outputs)
	}

	shivizOutputs := readShivizOutputFile(t, shivizOutputFile.Name())
	shivizExpected := []string{
		"(?<host>\\S*) (?<clock>{.*})\\n(?<event>.*)",
		"",
		"client1 {\"client1\":1}",
		fmt.Sprintf("%d CreateTrace {}", trace1ID),
		"client1 {\"client1\":2}",
		fmt.Sprintf("%d TestAction {\"Foo\":\"foo\"}", trace1ID),
		"client2 {\"client2\":1}",
		fmt.Sprintf("%d CreateTrace {}", trace2ID),
		"client2 {\"client2\":2}",
		fmt.Sprintf("%d TestAction {\"Foo\":\"bar\"}", trace2ID),
	}

	if !cmp.Equal(shivizOutputs, shivizExpected) {
		t.Fatalf("expected shiviz output %v did not equal actual shiviz output %v", shivizExpected, shivizOutputs)
	}
}

func TestTokenActions(t *testing.T) {
	outputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outputFile.Name())

	shivizOutputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(shivizOutputFile.Name())

	var token TracingToken
	var trace1ID, trace2ID uint64
	(func() {
		server := NewTracingServer(TracingServerConfig{
			ServerBind:       ":0",
			Secret:           []byte{},
			OutputFile:       outputFile.Name(),
			ShivizOutputFile: shivizOutputFile.Name(),
		})

		err = server.Open()
		if err != nil {
			t.Fatal(err)
		}
		serverBind := server.Listener.Addr().String()
		defer server.Close()
		go server.Accept()

		client1 := NewTracer(TracerConfig{
			ServerAddress:  serverBind,
			TracerIdentity: "client1",
			Secret:         []byte{},
		})
		defer client1.Close()
		trace1 := client1.CreateTrace()
		trace1ID = trace1.ID

		client2 := NewTracer(TracerConfig{
			ServerAddress:  serverBind,
			TracerIdentity: "client2",
			Secret:         []byte{},
		})
		defer client2.Close()

		token = trace1.GenerateToken()

		trace2 := client2.ReceiveToken(token)
		trace2ID = trace2.ID
	})()

	bToken := make([]byte, base64.StdEncoding.EncodedLen(len(token)))
	base64.StdEncoding.Encode(bToken, token)

	outputs := readTraceOutputFile(t, outputFile.Name())
	expected := []interface{}{
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(trace1ID),
			"Tag":            "CreateTrace",
			"Body":           map[string]interface{}{},
			"VectorClock": map[string]interface{}{
				"client1": intToJSONNubmer(1),
			},
		},
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(trace1ID),
			"Tag":            "GenerateTokenTrace",
			"Body":           map[string]interface{}{"Token": string(bToken)},
			"VectorClock": map[string]interface{}{
				"client1": intToJSONNubmer(2),
			},
		},
		map[string]interface{}{
			"TracerIdentity": "client2",
			"TraceID":        traceIDtoJSONNumber(trace2ID),
			"Tag":            "ReceiveTokenTrace",
			"Body":           map[string]interface{}{"Token": string(bToken)},
			"VectorClock": map[string]interface{}{
				"client1": intToJSONNubmer(2),
				"client2": intToJSONNubmer(1),
			},
		},
	}

	if !cmp.Equal(expected, outputs) {
		t.Fatalf("expected trace %v did not equal actual trace %v", expected, outputs)
	}

	if trace1ID != trace2ID {
		t.Fatalf("trace1.ID and trace2.ID are not equal")
	}

	shivizOutputs := readShivizOutputFile(t, shivizOutputFile.Name())
	shivizExpected := []string{
		"(?<host>\\S*) (?<clock>{.*})\\n(?<event>.*)",
		"",
		"client1 {\"client1\":1}",
		fmt.Sprintf("%d CreateTrace {}", trace1ID),
		"client1 {\"client1\":2}",
		fmt.Sprintf("%d GenerateTokenTrace {\"Token\":\"%s\"}", trace1ID, bToken),
		"client2 {\"client2\":1, \"client1\":2}",
		fmt.Sprintf("%d ReceiveTokenTrace {\"Token\":\"%s\"}", trace1ID, bToken),
	}
	eq := true
	for i := 0; i < len(shivizOutputs); i++ {
		if i%2 == 1 || i == 0 {
			if shivizExpected[i] != shivizOutputs[i] {
				eq = false
				break
			}
		} else {
			p1 := strings.SplitN(shivizExpected[i], " ", 2)
			p2 := strings.SplitN(shivizOutputs[i], " ", 2)
			if len(p1) != len(p2) || p1[0] != p2[0] {
				eq = false
				break
			}
			var vc1, vc2 map[string]interface{}
			if err := json.Unmarshal([]byte(p1[1]), &vc1); err != nil {
				eq = false
				break
			}
			if err := json.Unmarshal([]byte(p2[1]), &vc2); err != nil {
				eq = false
				break
			}
			if !cmp.Equal(vc1, vc2) {
				eq = false
				break
			}
		}
	}
	if !eq {
		t.Fatalf("expected go vector output %v did not equal actual go vector output %v", shivizExpected, shivizOutputs)
	}
}

func TestTracerRejoin(t *testing.T) {
	outputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outputFile.Name())

	shivizOutputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(shivizOutputFile.Name())

	(func() {
		server := NewTracingServer(TracingServerConfig{
			ServerBind:       ":0",
			Secret:           []byte{},
			OutputFile:       outputFile.Name(),
			ShivizOutputFile: shivizOutputFile.Name(),
		})

		err = server.Open()
		if err != nil {
			t.Fatal(err)
		}
		serverBind := server.Listener.Addr().String()
		defer server.Close()
		go server.Accept()

		tracerIdentity := "client1"
		c := NewTracer(TracerConfig{
			ServerAddress:  serverBind,
			TracerIdentity: tracerIdentity,
			Secret:         []byte{},
		})
		defer c.Close()
		trace := c.CreateTrace()
		trace.RecordAction(TestAction{Foo: "foo"})
		trace.RecordAction(TestAction{Foo: "bar"})

		cRejoined := NewTracer(TracerConfig{
			ServerAddress:  serverBind,
			TracerIdentity: tracerIdentity,
			Secret:         []byte{},
		})
		defer cRejoined.Close()

		vc := c.logger.GetCurrentVC()
		rejoinedVC := cRejoined.logger.GetCurrentVC()

		if !cmp.Equal(vc, rejoinedVC) {
			t.Fatal("rejoined tracer clock value is not equal to intial tracer clock value")
		}
	})()

}
