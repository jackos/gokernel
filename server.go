package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
)

type Program struct {
	TempFile         string        // Temp file location
	Functions        string        // The code that is pulled out of the main function
	ExecutedFilename string        // The most recently executed filename
	Cells            map[int]*Cell // Represents a cell from VS Code notebook
}

type Cell struct {
	Fragment  int    // What index the cell was at, at time of execution
	Index     int    // Current index by order in VS Code
	Contents  string // What's inside the cell
	Executing bool   // The cell that is currently being executed
	Filename  string // What file the executing cell is from
}

// Runs for every execution, not like a traditional server in that this will
// only ever run synchronously, as the execution is controlled by VS Code nodejs
// code which awaits each cell to finish one at a time
func (p *Program) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Reset the data in the functions outside the main block
	p.Functions = ""
	// Unmarshal data into the Cell data structure
	input := p.Unmarshal(w, r)
	// If filename different to last run, reset data
	if input.Filename != p.ExecutedFilename && p.ExecutedFilename != "" {
		fmt.Println("New file detected resetting input")
		p.Functions = ""
		p.Cells = make(map[int]*Cell)
	}
	p.ExecutedFilename = input.Filename

	if ok := p.checkErrors(input, w); ok {
		err := p.writeFile(input)
		// If error writing file return error
		if err != nil {
			message := "exit status 3\n" + err.Error() + "\nMake sure the directory exists and you have permission to write there"
			_, err := w.Write([]byte(message))
			if err != nil {
				log.Println(err)
			}
		} else {
			// If successful up to this point, run the program and return result
			result, err := p.run(input.Fragment)
			if err != nil {
				_, err := w.Write([]byte(err.Error()))
				if err != nil {
					fmt.Println(err)
				}
			}
			_, err = w.Write(result)
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

// Unmarshal the JSON data and return it
func (p *Program) Unmarshal(w http.ResponseWriter, r *http.Request) Cell {
	b := make([]byte, r.ContentLength)
	_, err := r.Body.Read(b)
	if err != io.EOF && err != nil {
		_, err := w.Write([]byte(err.Error()))
		if err != nil {
			log.Println(err)
		}
	}
	var input Cell
	_ = json.Unmarshal([]byte(b), &input)
	return input
}

// Runs the program and only returns outputs from the executing cell
func (p *Program) run(executingFragment int) ([]byte, error) {
	err := p.fixFile(executingFragment)
	if err != nil {
		return []byte(err.Error()), err
	}

	// Use the go run tool to run the program and return the result
	out, err := exec.Command("go", "run", filepath.Join(p.TempFile)).CombinedOutput()
	if err != nil {
		// If cell doesn't run due to error, clear it
		p.Cells[executingFragment].Contents = ""
		return out, err
	}

	// Determine where the output for executing cell starts and ends
	start := strings.Index(string(out), "gobook-output-start")
	end := strings.Index(string(out), "gobook-output-end")
	if start == -1 || end == -1 {
		return []byte{}, nil
	}
	s := start + 20
	e := end - 1
	if s >= len(out) || e >= len(out) {
		log.Println("Warning: output slice out of array bounds")
		return []byte{}, nil
	}
	// Return just the output of the executing cell
	return out[s:e], nil
}
