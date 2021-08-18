package main

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

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
		reType, _ := regexp.Compile(`\s*type\s+\w+\s+.*`)
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
