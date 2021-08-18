package main

import (
	"log"
	"net/http"
	"regexp"
)

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
