// This is a http server that starts from VS Code, only one request is ever running at a time,
// the VS Code notebook API runs cells synchronously based on execution order.
package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type FragmentKey []int

func main() {
	cells := make(map[int]*Cell)
	http.Handle("/", &Program{TempFile: os.TempDir() + "/main.go", Cells: cells, ExecutedFilename: ""})
	log.Println("Kernel running on port 5250")
	fmt.Println("ctrl + click to view generated go code:", os.TempDir()+"/main.go")
	s := &http.Server{
		Addr:           "127.0.0.1:5250",
		Handler:        nil,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20,
	}
	log.Fatal(s.ListenAndServe())
}
