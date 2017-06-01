package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
)

type reqRslt struct {
	App        string
	Accessible bool
	Status     int
	Notes      string
}

// fill is a method on reqRslt that populates the object with data
func (r *reqRslt) fill(app string, acc bool, st int, nts string) {
	r.Accessible = acc
	r.App = app
	r.Notes = nts
	r.Status = st
	return
}

var (
	err     error
	args    []string
	apps    []string
	inFile  *os.File
	scnr    *bufio.Scanner
	results []reqRslt
	wg      sync.WaitGroup
	elm     *reqRslt
)

// isHTML indicates if the response body is a web page
func isHTML(b []byte) bool {
	// Does the body contain an <html> and <body> tag?
	if bytes.Contains(b, []byte("<html")) || bytes.Contains(b, []byte("<HTML")) {
		// Return true if the body tag is present
		return bytes.Contains(b, []byte("<body")) || bytes.Contains(b, []byte("<BODY"))
	}

	// Did not find an <html> tag; return false
	return false
}

// isHerokuPage indicates if the response body is a Heroku error page or welcome page
func isHerokuPage(b []byte) bool {

	if bytes.Contains(b, []byte("www.herokucdn.com/error-pages/application-error.html")) {
		// Assume the response is the Heroku Application Error page
		return true
	}

	if bytes.Contains(b, []byte("Welcome to your new app")) {
		// Assume the response is the Heroku Welcome page
		return true
	}

	// No Heroku page "tell" was found
	return false
}

func procApp(site string, c chan *reqRslt) {
	var (
		ferr       error
		resp       *http.Response
		tReslt     reqRslt
		tURL, tStr string
		body       []byte
		tLen       = 50
	)

	// Remember to decrement the goroutine counter
	defer wg.Done()

	// Have a non-empty string parameter?
	if len(site) == 0 {
		// No site is being requested
		tReslt.fill(site, false, 999, "no site name was provided")
		goto WrapUp
	}

	// Execute a GET on the Heroku app domain
	tURL = fmt.Sprintf("http://%v.herokuapp.com", site)
	if resp, ferr = http.Get(tURL); ferr != nil {
		// Error occurred during the request
		log.Printf("ERROR: error occurred fetching: %v. See: %v", tURL, ferr)
		tReslt.fill(site, false, 999, fmt.Sprintf("error getting site response: %v", ferr))
		goto WrapUp
	}

	// Inspect the response body
	defer resp.Body.Close() // Ensure the response body gets closed

	// Read the response body
	if body, ferr = ioutil.ReadAll(resp.Body); ferr != nil {
		// Error occurred reading a response body
		log.Printf("ERROR: error occurred reading a response body for site: %v. See: %v\n", site, ferr)
		tReslt.fill(site, false, 999, fmt.Sprintf("error reading the site response: %v", ferr))
		goto WrapUp
	}

	// Is the response the Heroku Application Error?
	if isHerokuPage(body) {
		// Found a Heroku page
		tReslt.fill(site, false, resp.StatusCode, "Heroku welcome or application error page")
		goto WrapUp
	}

	// Find a web page?
	if isHTML(body) {
		// Found an HTML page
		tReslt.fill(site, true, resp.StatusCode, "HTML page")
		goto WrapUp
	}

	// Found something - not a Heroku page or generic web page
	if len(body) < tLen {
		tLen = len(body)
	}
	tStr = fmt.Sprintf("found this (first %v bytes): %v...", tLen, string(body[:tLen]))
	tReslt.fill(site, true, resp.StatusCode, tStr)

WrapUp:
	c <- &tReslt // Put a pointer to the result object on the channel

	return
}

func main() {
	log.Printf("INFO: start processing...\n")

	// Grab file information to be processed
	if args = os.Args; len(args) != 2 {
		// Missing filename parameter
		log.Fatalln("END: Missing filename parameter")
	}

	// Read file of Heroku apps into a slice
	if inFile, err = os.Open(args[1]); err != nil {
		// Error occurred opening file of heroku apps
		log.Fatalf("ERROR: error occurred opening input file. See: %v\n", err)
	}
	defer inFile.Close()

	// Read the file contents into a slice of strings
	scnr = bufio.NewScanner(inFile)
	for scnr.Scan() {
		apps = append(apps, scnr.Text())
	}

	// Determine if there are any read errors
	if scnr.Err() != nil {
		log.Fatalf("ERROR: error reading the input file. See: %v\n", err)
	}

	chn := make(chan *reqRslt, len(apps))

	// Iterate through the list of apps
	for i := 0; i < len(apps); i++ {
		wg.Add(1)                // Account for a new goroutine
		go procApp(apps[i], chn) // Spin up a goroutine to process an app reference
	}

	// Wait for all goroutines to stop processing
	log.Printf("INFO: wait for goroutines to complete\n")
	wg.Wait()

	log.Printf("INFO: the number of elements in the channel is: %v\n", len(chn))

	// Print out results
	fmt.Printf("Application,Accessible,HTTP Status,Notes\n")
	for i := 0; i < len(apps); i++ {
		v := <-chn
		fmt.Printf("%v,%v,%v,%v\n", v.App, v.Accessible, v.Status, v.Notes)
	}

	log.Printf("INFO: complete processing\n")
}
