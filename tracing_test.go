package tracing

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"
	"testing"
)

type TestAction struct {
	Foo string
}

func readOutputFile(t *testing.T, fileName string) (outputs []interface{}) {
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

	outputs := readOutputFile(t, outputFile.Name())
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

	outputs := readOutputFile(t, outputFile.Name())
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
}
