// This is a http server that starts from VS Code, only one request is ever running
// at the same time, the VS Code notebook API runs cells synchronously based on execution order.
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
	"time"
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

type FragmentKey []int

// Fixes the imports of p.TempFile and formats the file
func (p *Program) fixFile(executingFragment int) error {
	// Find GOPATH
	gopath, err := exec.Command("go", "env", "GOPATH", filepath.Join(p.TempFile)).CombinedOutput()
	gopls := strings.ReplaceAll(string(gopath), "\n", "") + "/bin/gopls"
	if err != nil {
		return err
	}

	// Adds package imports and removes anything unused
	err = exec.Command(gopls, "imports", "-w", filepath.Join(p.TempFile)).Run()
	if err != nil {
		return err
	}
	go p.formatFile()
	return nil
}

// Format the temp file, as some users will check the source code
// Runs in a separate go routine as not required before returning output
func (p *Program) formatFile() {
	err := exec.Command("go", "fmt", filepath.Join(p.TempFile)).Run()
	if err != nil {
		log.Println(err)
	}
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
	s := start + 19
	e := end
	if s >= len(out) || e >= len(out) {
		log.Println("Warning: output slice out of array bounds")
		return []byte{}, nil
	}
	// Return just the output of the executing cell
	return out[s:e], nil
}

// Determines what order the file should be written based on existing
// execution order (fragments) and new execution order (index)
// Fragment is a VS Code notebook term that identifies a cell at the time
// of execution, so if it changes order we can still modify the same cell
func (p *Program) writeFile(input Cell) error {
	// Overwrites cell if it already existed
	p.Cells[input.Fragment] = &input

	// The keys are used to determine current order of the notebook cells
	// The fragments are the original order
	keys := []FragmentKey{}
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
		// If cell contains a function or type, don't write cell to mainBuf
		reType, _ := regexp.Compile(`\s*type.*{`)
		reFunc, _ := regexp.Compile(`\s*func\s+\w+\s*\(.*\).*{`)
		reRecFunc, _ := regexp.Compile(`\s*func \(.*\)\s+\w+\(.*\).*{`)
		if reFunc.MatchString(c.Contents) || reType.MatchString(c.Contents) || reRecFunc.MatchString(c.Contents) {
			// Add it instead to the functions string
			p.Functions += "\n" + c.Contents
			// Also stop any output returning to to client
			c.Executing = false
		} else {
			if c.Executing {
				// This output is used to limit returned text to the executing cell
				fmt.Fprint(&mainFuncBuf, `println("gobook-output-start")`)
			}
			fmt.Fprint(&mainFuncBuf, "\n"+c.Contents+"\n")
			if c.Executing {
				fmt.Fprint(&mainFuncBuf, `println("gobook-output-end") `)
			}
		}
	}
	p.Cells[input.Fragment].Executing = false
	// Write the program
	fmt.Fprintf(&programBuf, ("package main\n\n%s\n\nfunc main() {%s\n}"), p.Functions, &mainFuncBuf)
	err := os.WriteFile(p.TempFile, programBuf.Bytes(), 0600)
	if err != nil {
		return err
	}
	return nil
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

// Checks if the caller is trying to write a package or main function
// Returns appropriate error if they are.
// This will change later to still allow writing a main function or package
func (p *Program) checkErrors(input Cell, w http.ResponseWriter) (ok bool) {
	// If a main function exists return error
	checkMain, err := regexp.MatchString(`\s*func\s+main\s*\(\s*\)\s*{`, input.Contents)
	if err != nil {
		log.Println(err)
	}
	if checkMain {
		_, err := w.Write([]byte("exit status 3\nMain function is generated automatically. Please remove func main()"))
		if err != nil {
			log.Println(err)
		}
		return false
	}

	// If import statement exists return error
	checkImport, err := regexp.MatchString(`\s*import\s+[("]`, input.Contents)
	if err != nil {
		log.Println(err)
	}
	if checkImport {
		_, err := w.Write([]byte("exit status 3\nImports are done automatically. Please remove import statement"))
		if err != nil {
			log.Println(err)
		}
		return false
	}

	// Check if user declaring a package
	checkPackage, err := regexp.MatchString(`\s*package\s+\w*\n"]`, input.Contents)
	if err != nil {
		log.Println(err)
	}
	if checkPackage {
		_, err = w.Write([]byte("exit status 3\nAre package is generated automatically. Please remove package statement"))
		if err != nil {
			log.Println(err)
		}
		return false
	}

	return true

}

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
			_, err = w.Write([]byte(result))
			if err != nil {
				fmt.Println(err)
			}
		}
	}
}

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
