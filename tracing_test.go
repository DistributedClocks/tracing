package tracing

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
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

func TestOneRecord(t *testing.T) {
	outputFile, err := ioutil.TempFile("", "")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(outputFile.Name())

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

		client1.RecordAction(TestAction{Foo: "foo"})
	})()

	outputs := readTraceOutputFile(t, outputFile.Name())
	expected := []interface{}{
		map[string]interface{}{
			"TracerIdentity": "client1",
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

		client1.RecordAction(TestAction{Foo: "foo"})
		client2.RecordAction(TestAction{Foo: "bar"})
	})()

	outputs := readTraceOutputFile(t, outputFile.Name())
	expected := []interface{}{
		map[string]interface{}{
			"TracerIdentity": "client1",
			"Tag":            "TestAction",
			"Body":           map[string]interface{}{"Foo": "foo"},
		},
		map[string]interface{}{
			"TracerIdentity": "client2",
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
			"INFO [client1] TestAction Foo=foo",
		},
		{
			"client2 {\"client2\":1}",
			"Initialization Complete",
			"client2 {\"client2\":2}",
			"INFO [client2] TestAction Foo=bar",
		},
	}
	for i, goVecOutputFile := range goVecOutputFiles {
		goVecOutputs := readGoVectorOutputFile(t, goVecOutputFile)
		fmt.Println(goVecOutputs[0])
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

		token = client1.GenerateToken()
		client2.ReceiveToken(token)
	})()

	bToken := make([]byte, base64.StdEncoding.EncodedLen(len(token)))
	base64.StdEncoding.Encode(bToken, token)

	outputs := readTraceOutputFile(t, outputFile.Name())
	expected := []interface{}{
		map[string]interface{}{
			"TracerIdentity": "client1",
			"Tag":            "GenerateTokenTrace",
			"Body":           map[string]interface{}{"Token": string(bToken)},
		},
		map[string]interface{}{
			"TracerIdentity": "client2",
			"Tag":            "ReceiveTokenTrace",
			"Body":           map[string]interface{}{"Token": string(bToken)},
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
			"INFO [client1] PrepareTokenTrace",
		},
		{
			"client2 {\"client2\":1}",
			"Initialization Complete",
			"client2 {\"client2\":2, \"client1\":2}",
			"INFO [client2] ReceiveTokenTrace Token=[167 99 108 105 101 110 116 49 192 129 167 99 108 105 101 110 116 49 2]",
		},
	}
	for i, goVecOutputFile := range goVecOutputFiles {
		goVecOutputs := readGoVectorOutputFile(t, goVecOutputFile)
		fmt.Println(goVecOutputs[0])
		if !reflect.DeepEqual(goVecOutputs, goVecExpected[i]) {
			t.Fatalf("expected go vector output %v did not equal actual go vector output %v", goVecExpected[i], goVecOutputs)
		}
	}
}
