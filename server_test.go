// handlers_test.go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type Payload struct {
	Fragment  int
	Index     int
	Contents  string
	Executing bool
	Filename  string
}

func defaultPayload() Payload {
	return Payload{
		Fragment:  0,
		Index:     0,
		Contents:  "",
		Executing: true,
		Filename:  "test.md",
	}
}

func TestMultiply(t *testing.T) {
	p := defaultPayload()
	p.Contents = `fmt.Println(10 * 50)`
	pm, _ := json.Marshal(p)
	testHTTP(pm, t, "500")
}

func TestInlineFunc(t *testing.T) {
	p := defaultPayload()
	p.Contents = `fmt.Println(func() int {
		return 5 + 10
	}())`
	pm, _ := json.Marshal(p)
	testHTTP(pm, t, "15")
}

func TestMainFunc(t *testing.T) {
	p := defaultPayload()
	p.Contents = `
	func main() {
		fmt.Println("Should fail)
	}`
	pm, _ := json.Marshal(p)
	testHTTP(pm, t, "exit status 3\nMain function is generated automatically. Please remove func main()")
}

func TestType(t *testing.T) {
	p := defaultPayload()
	p.Contents = `
	type TestType struct {
		x string
		y int
	}`
	pm, _ := json.Marshal(p)
	testHTTP(pm, t, "")
}

func testHTTP(requestBody []byte, t *testing.T, expected string) {
	cells := make(map[int]*Cell)
	p := &Program{TempFile: os.TempDir() + "/main.go", Cells: cells, ExecutedFilename: ""}
	// Create a request to pass to our handler. We don't have any query parameters for now, so we'll
	// pass 'nil' as the third parameter.
	req, err := http.NewRequest("GET", "/", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatal(err)
	}

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(p.ServeHTTP)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
	if rr.Body.String() != expected {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}
