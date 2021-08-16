package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Program struct {
	TempFile  string        // Temp file location
	Functions string        // The code that is pulled out of the main function
	Filename  string        // The name of the most recently executed cell
	Cells     map[int]*Cell // Represents a cell from VS Code notebook
}

type Cell struct {
	Fragment  int    // What index the cell was at, at time of execution
	Index     int    // Current index by order in VS Code
	Contents  string // What's inside the cell
	Executing bool   // The cell that is currently being executed
	Filename  string // What file the executing cell is from
}

// Fixes the imports of p.TempFile, runs it, and returns only the outputs of the executing cell
func (p *Program) run(executingFragment int) ([]byte, error) {
	gopath, err := exec.Command("go", "env", "GOPATH", filepath.Join(p.File)).CombinedOutput()
	gopls := strings.ReplaceAll(string(gopath), "\n", "") + "/bin/gopls"
	if err != nil {
		return gopath, err
	}
	// Adds package imports and removes anything unused
	err = exec.Command(gopls, "imports", "-w", filepath.Join(p.File)).Run()
	if err != nil {
		return nil, err
	}

	// Use the go run too to run the program and return the result
	out, err := exec.Command("go", "run", filepath.Join(p.File)).CombinedOutput()
	if err != nil {
		// If cell doesn't run due to error, clear it
		p.Cells[executingFragment].Contents = ""
		return out, err
	}
	// Format the temp file, as some uses will check the source code
	err = exec.Command("go", "fmt", filepath.Join(p.TempFile)).Run()

	if err != nil {
		return nil, err
	}
	// Determine where the output for executing cell starts and ends
	start, _ := regexp.Compile("gobook-output-start")
	end, _ := regexp.Compile("gobook-output-end")
	s := start.FindIndex(out)
	if s == nil {
		return []byte{}, nil
	}
	e := end.FindIndex(out)
	// Return just the output of the executing cell
	return out[s[1]:e[0]], nil
}

// Determines what order the file should be written based on existing
// execution order (fragments) and new execution order (index)
// Fragment is a VS Code notebook term that identifies a cell at the time
// Of execution, so if it changes order we can still modify the same cell
func (p *Program) writeFile(input Cell) error {
	// Overwrites cell if it already existed
	p.Cells[input.Fragment] = &input

	// The keys are used to determine current order of the notebook cells
	// The fragments are the original order
	keys := [][]int{}
	for _, v := range p.Cells {
		keys = append(keys, []int{v.Index, v.Fragment})
	}

	// Sort keys by the new order
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] == keys[j][0] {
			return true
		}
		return keys[i][0] < keys[j][0]
	})

	// Two buffers, one for code going in main and one for everything else
	var programBuf bytes.Buffer
	var mainFuncBuf bytes.Buffer

	for _, key := range keys {
		c := p.Cells[key[1]]
		// If cell contains a function, don't write cell to mainBuf
		reFunc, _ := regexp.Compile(`\s*func.*\(\).*{`)
		if reFunc.MatchString(c.Contents) {
			// Add it instead the the functions string
			p.Functions += "\n" + c.Contents
			// Also stop any output
			c.Executing = false
		} else {
			if c.Executing {
				// This output is used to limit returned text to the executing cell
				mainFuncBuf.Write([]byte(`println("gobook-output-start")`))
			}
			mainFuncBuf.Write([]byte("\n" + c.Contents + "\n"))
			if c.Executing {
				mainFuncBuf.Write([]byte(`println("gobook-output-end") `))
			}
		}
	}
	// Write the rest of the program
	p.Cells[input.Fragment].Executing = false
	programBuf.Write([]byte("package main\n\n"))
	programBuf.Write([]byte(p.Functions))
	programBuf.Write([]byte("\n\nfunc main() {"))
	programBuf.Write(mainFuncBuf.Bytes())
	programBuf.Write([]byte("\n}"))
	err := os.WriteFile(p.TempFile, programBuf.Bytes(), 0600)
	if err != nil {
		return err
	}
	return nil
}

func (p *Program) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.Functions = ""
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

	// If filename different to last run, reset data
	if input.Filename != p.Filename && p.Filename != "" {
		fmt.Println("New file detected resetting input")
		p.Functions = ""
		p.Cells = make(map[int]*Cell)
	}
	p.Filename = input.Filename

	checkMain, _ := regexp.MatchString(`\s*func\s+main\s*\(\s*\)\s*{`, input.Contents)
	checkImport, _ := regexp.MatchString(`\s*import\s+[("]`, input.Contents)
	if checkMain {
		w.Write([]byte("exit status 3\nMain function is generated automatically. Please remove func main()"))
	} else if checkImport {
		w.Write([]byte("exit status 3\nImports are done automatically. Please remove import statement"))
	} else {
		err := p.writeFile(input)
		if err != nil {
			message := "exit status 3\n" + err.Error() + "\nMake sure the directory exists and you have permission to write there"
			w.Write([]byte(message))
		} else {
			result, err := p.run(input.Fragment)
			if err != nil {
				w.Write([]byte(err.Error()))
			}
			w.Write([]byte(result))
		}
	}
}

func main() {
	cells := make(map[int]*Cell)
	http.Handle("/", &Program{TempFile: os.TempDir() + "/main.go", Cells: cells, Filename: ""})
	log.Println("Kernel running on port 5250")
	fmt.Println("ctrl + click to view generated go code:", os.TempDir()+"/main.go")
	log.Fatal(http.ListenAndServe(":5250", nil))
}
