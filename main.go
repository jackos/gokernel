package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
)

type Program struct {
	File      string        // Temp file location
	Cells     map[int]*Cell // Represents a cell from VS Code notebook
	Functions string
}

type Cell struct {
	Fragment  int    // What index the cell was at, at time of execution
	Index     int    // Current index by order in VS Code
	Contents  string // What's inside the cell
	Executing bool   // The cell that is currently being executed
}

func (p *Program) run(executingFragment int) ([]byte, error) {
	err := exec.Command("gopls", "imports", "-w", filepath.Join(p.File)).Run()
	if err != nil {
		return nil, err
	}
	out, err := exec.Command("go", "run", filepath.Join(p.File)).CombinedOutput()
	if err != nil {
		// If cell doesn't run due to error, clear it
		p.Cells[executingFragment].Contents = ""
		return out, err
	}
	err = exec.Command("go", "fmt", filepath.Join(p.File)).Run()
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
	return out[s[1]:e[0]], nil
}

func (p *Program) writeFile(input Cell) error {
	p.Cells[input.Fragment] = &input

	keys := [][]int{}
	for _, v := range p.Cells {
		keys = append(keys, []int{v.Index, v.Fragment})
	}

	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] == keys[j][0] {
			return true
		}
		return keys[i][0] < keys[j][0]
	})
	var buf bytes.Buffer
	var bufBody bytes.Buffer

	for _, key := range keys {
		c := p.Cells[key[1]]

		reFunc, _ := regexp.Compile(`\s*func.*\(\)\s*{`)
		if reFunc.MatchString(c.Contents) {
			p.Functions += "\n" + c.Contents
			c.Executing = false
		} else {
			if c.Executing {
				bufBody.Write([]byte(`println("gobook-output-start")`))
			}
			bufBody.Write([]byte("\n" + c.Contents + "\n"))
			if c.Executing {
				bufBody.Write([]byte(`println("gobook-output-end") `))
			}
		}
	}
	p.Cells[input.Fragment].Executing = false
	buf.Write([]byte("package main\n\n"))
	buf.Write([]byte(p.Functions))
	buf.Write([]byte("\n\nfunc main() {"))
	buf.Write(bufBody.Bytes())
	buf.Write([]byte("\n}"))
	err := os.WriteFile(p.File, buf.Bytes(), 0600)
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

	checkMain, _ := regexp.MatchString(`\s*func\s+main\s*\(\s*\)\s*{`, input.Contents)
	checkImport, _ := regexp.MatchString(`\s*import\s+[("]`, input.Contents)
	if checkMain {
		_, err := w.Write([]byte("Main function is generated automatically. Please remove func main()"))
		if err != nil {
			log.Println(err)
		}
	} else if checkImport {
		_, err = w.Write([]byte("Imports are done automatically. Please remove import statement"))
		if err != nil {
			log.Println(err)
		}
	} else {
		err := p.writeFile(input)
		if err != nil {
			message := err.Error() + "\nMake sure the directory exists and you have permission to write there"
			_, err = w.Write([]byte(message))
			if err != nil {
				log.Println(message)
			}
		}
		result, err := p.run(input.Fragment)
		if err != nil {
			_, err = w.Write([]byte(err.Error()))
			if err != nil {
				log.Println("Failed to run program:", err)
			}
		}
		_, err = w.Write([]byte(result))
		if err != nil {
			log.Println(err)
		}
	}
}

func main() {
	cells := make(map[int]*Cell)
	http.Handle("/", &Program{File: os.TempDir() + "/main.go", Cells: cells})
	log.Println("Kernel running on port 5250")
	log.Fatal(http.ListenAndServe(":5250", nil))
}
