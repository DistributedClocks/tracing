package tracing

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

type TestAction struct {
	Foo string
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

func readGoVectorOutputFile(t *testing.T, fileName string) (outputs []string) {
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

func traceIDtoJSONNumber(id uint64) json.Number {
	return json.Number(strconv.FormatUint(id, 10))
}

func TestOneRecord(t *testing.T) {
	outputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outputFile.Name())

	var traceID uint64
	(func() {
		server := NewTracingServer(TracingServerConfig{
			ServerBind: ":0",
			Secret:     []byte{},
			OutputFile: outputFile.Name(),
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
	})()

	outputs := readTraceOutputFile(t, outputFile.Name())
	expected := []interface{}{
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(traceID),
			"Tag":            "CreateTrace",
			"Body":           map[string]interface{}{},
		},
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(traceID),
			"Tag":            "TestAction",
			"Body":           map[string]interface{}{"Foo": "foo"},
		},
	}

	if !reflect.DeepEqual(outputs, expected) {
		t.Fatalf("expected trace %v did not equal actual trace %v", expected, outputs)
	}
}

func TestTwoClients(t *testing.T) {
	outputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outputFile.Name())

	var trace1ID, trace2ID uint64
	(func() {
		server := NewTracingServer(TracingServerConfig{
			ServerBind: ":0",
			Secret:     []byte{},
			OutputFile: outputFile.Name(),
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
		},
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(trace1ID),
			"Tag":            "TestAction",
			"Body":           map[string]interface{}{"Foo": "foo"},
		},
		map[string]interface{}{
			"TracerIdentity": "client2",
			"TraceID":        traceIDtoJSONNumber(trace2ID),
			"Tag":            "CreateTrace",
			"Body":           map[string]interface{}{},
		},
		map[string]interface{}{
			"TracerIdentity": "client2",
			"TraceID":        traceIDtoJSONNumber(trace2ID),
			"Tag":            "TestAction",
			"Body":           map[string]interface{}{"Foo": "bar"},
		},
	}

	if !reflect.DeepEqual(outputs, expected) {
		t.Fatalf("expected trace %v did not equal actual trace %v", expected, outputs)
	}

	goVecOutputFiles := []string{"GoVector-client1-Log.txt", "GoVector-client2-Log.txt"}
	goVecExpected := [][]string{
		{
			"client1 {\"client1\":1}",
			"Initialization Complete",
			"client1 {\"client1\":2}",
			fmt.Sprintf("INFO [client1] TraceID=%d CreateTrace", trace1ID),
			"client1 {\"client1\":3}",
			fmt.Sprintf("INFO [client1] TraceID=%d TestAction Foo=foo", trace1ID),
		},
		{
			"client2 {\"client2\":1}",
			"Initialization Complete",
			"client2 {\"client2\":2}",
			fmt.Sprintf("INFO [client2] TraceID=%d CreateTrace", trace2ID),
			"client2 {\"client2\":3}",
			fmt.Sprintf("INFO [client2] TraceID=%d TestAction Foo=bar", trace2ID),
		},
	}
	for i, goVecOutputFile := range goVecOutputFiles {
		goVecOutputs := readGoVectorOutputFile(t, goVecOutputFile)
		if !reflect.DeepEqual(goVecOutputs, goVecExpected[i]) {
			t.Fatalf("expected go vector output %v did not equal actual go vector output %v", goVecExpected[i], goVecOutputs)
		}
	}

}

func TestTokenActions(t *testing.T) {
	outputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outputFile.Name())

	var token TracingToken
	var trace1ID, trace2ID uint64
	(func() {
		server := NewTracingServer(TracingServerConfig{
			ServerBind: ":0",
			Secret:     []byte{},
			OutputFile: outputFile.Name(),
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
		},
		map[string]interface{}{
			"TracerIdentity": "client1",
			"TraceID":        traceIDtoJSONNumber(trace1ID),
			"Tag":            "GenerateTokenTrace",
			"Body":           map[string]interface{}{"Token": string(bToken)},
		},
		map[string]interface{}{
			"TracerIdentity": "client2",
			"TraceID":        traceIDtoJSONNumber(trace2ID),
			"Tag":            "ReceiveTokenTrace",
			"Body":           map[string]interface{}{"Token": string(bToken)},
		},
	}

	if !reflect.DeepEqual(outputs, expected) {
		t.Fatalf("expected trace %v did not equal actual trace %v", expected, outputs)
	}

	if trace1ID != trace2ID {
		t.Fatalf("trace1.ID and trace2.ID are not equal")
	}

	goVecOutputFiles := []string{"GoVector-client1-Log.txt", "GoVector-client2-Log.txt"}
	goVecExpected := [][]string{
		{
			"client1 {\"client1\":1}",
			"Initialization Complete",
			"client1 {\"client1\":2}",
			fmt.Sprintf("INFO [client1] TraceID=%d CreateTrace", trace1ID),
			"client1 {\"client1\":3}",
			fmt.Sprintf("INFO [client1] TraceID=%d PrepareTokenTrace", trace1ID),
		},
		{
			"client2 {\"client2\":1}",
			"Initialization Complete",
			"client2 {\"client1\":3, \"client2\":2}",
			fmt.Sprintf("INFO [client2] ReceiveTokenTrace Token=%d", token),
		},
	}
	for i, goVecOutputFile := range goVecOutputFiles {
		goVecOutputs := readGoVectorOutputFile(t, goVecOutputFile)
		if len(goVecOutputs) != len(goVecExpected[i]) {
			t.Fatalf("expected go vector output %v did not equal actual go vector output %v", goVecExpected[i], goVecOutputs)
		}
		eq := true
		for j := 0; j < len(goVecOutputs); j++ {
			if j%2 == 1 {
				if goVecExpected[i][j] != goVecOutputs[j] {
					eq = false
					break
				}
			} else {
				p1 := strings.SplitN(goVecExpected[i][j], " ", 2)
				p2 := strings.SplitN(goVecOutputs[j], " ", 2)
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
				if !reflect.DeepEqual(vc1, vc2) {
					eq = false
					break
				}
			}
		}
		if !eq {
			t.Fatalf("expected go vector output %v did not equal actual go vector output %v", goVecExpected[i], goVecOutputs)
		}
	}
}
