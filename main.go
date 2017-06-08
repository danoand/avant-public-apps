package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
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

// isHerokuErrPage indicates if the response body is a Heroku error page or welcome page
func isHerokuErrPage(b []byte) bool {

	if bytes.Contains(b, []byte("www.herokucdn.com/error-pages/application-error.html")) {
		// Assume the response is the Heroku Application Error page
		return true
	}

	// No Heroku page "tell" was found
	return false
}

// isHerokuWelPage indicates if the response body is a Heroku error page or welcome page
func isHerokuWelPage(b []byte) bool {

	if bytes.Contains(b, []byte("Welcome to your new app")) {
		// Assume the response is the Heroku Welcome page
		return true
	}

	// No Heroku page "tell" was found
	return false
}

// isAvantErrPage indicates if the response body is a Heroku error page or welcome page
func isAvantErrPage(b []byte) bool {

	if bytes.Contains(b, []byte("We have been notified, please try again later. Have a question? Call us at 800-712-5407")) {
		// Assume the response is the Heroku Welcome page
		return true
	}

	if bytes.Contains(b, []byte("We have been notified, please try again later. Have a question? Call us at 0800 610 1516")) {
		// Assume the response is the Heroku Welcome page
		return true
	}

	// No Heroku page "tell" was found
	return false
}

// procApp executes an HTTP GET on a passed Heroku app and does some rudimentary analysis
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
	resp, ferr = http.Get(tURL)

	// General error when executing the HTTP GET?
	if ferr != nil {
		// EOF error? (no data in the response)
		if strings.Contains(ferr.Error(), "EOF") {
			// No data returned in the response
			tReslt.fill(site, false, 999, fmt.Sprintf("no response data: %v", ferr))
			goto WrapUp
		}

		// Certificate error?
		if strings.Contains(ferr.Error(), "x509: certificate is valid for") {
			// No data returned in the response
			tReslt.fill(site, false, 200, fmt.Sprintf("SSL certificate error: %v", ferr))
			goto WrapUp
		}

		// Error occurred during the request
		fmt.Printf("ERROR: error occurred fetching: %v. See: %v", tURL, ferr)
		tReslt.fill(site, false, 999, fmt.Sprintf("error getting site response: %v", ferr))
		goto WrapUp
	}

	// Inspect the response body
	defer resp.Body.Close() // Ensure the response body gets closed

	// Read the response body
	if body, ferr = ioutil.ReadAll(resp.Body); ferr != nil {
		// Error occurred reading a response body
		fmt.Printf("ERROR: error occurred reading a response body for site: %v. See: %v\n", site, ferr)
		tReslt.fill(site, false, 999, fmt.Sprintf("error reading the site response: %v", ferr))
		goto WrapUp
	}

	// Is the response the Avant Application Error?
	if isAvantErrPage(body) {
		// Found a Heroku page
		tReslt.fill(site, false, resp.StatusCode, "Avant error page")
		goto WrapUp
	}

	// Is the response the Heroku Error page?
	if isHerokuErrPage(body) {
		// Found a Heroku page
		tReslt.fill(site, false, resp.StatusCode, "Heroku error page")
		goto WrapUp
	}

	// Is the response the Heroku Welcome page?
	if isHerokuWelPage(body) {
		// Found a Heroku page
		tReslt.fill(site, false, resp.StatusCode, "Heroku welcome page")
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
	fmt.Printf("\n\n*********************************\nINFO: Start processing job\n---------------------------------\n\n")

	// Grab file information to be processed
	if args = os.Args; len(args) != 2 {
		// Missing filename parameter
		fmt.Printf("ERROR: Missing filename parameter\n")
		os.Exit(1)
	}

	// Read file of Heroku apps into a slice
	if inFile, err = os.Open(args[1]); err != nil {
		// Error occurred opening file of heroku apps
		fmt.Printf("ERROR: error occurred opening input file. See: %v\n", err)
		os.Exit(1)
	}
	defer inFile.Close() // NOTE: defer keywork

	// Read the file contents into a slice of strings
	scnr = bufio.NewScanner(inFile)
	for scnr.Scan() {
		apps = append(apps, scnr.Text())
	}

	// Determine if there are any read errors
	if scnr.Err() != nil {
		fmt.Printf("ERROR: error reading the input file. See: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("*********************************\nINFO: Read in %v URLS to fetch\n---------------------------------\n\n", len(apps))

	chn := make(chan *reqRslt, len(apps))

	fmt.Printf("**********************************************************\nINFO: Starting %v goroutines to get responses from %v URLs\n-------------------------------------------------------------\n", len(apps), len(apps))

	// Iterate through the list of apps
	for i := 0; i < len(apps); i++ {
		wg.Add(1)                // Account for a new goroutine
		go procApp(apps[i], chn) // Spin up a goroutine to process an app reference
		fmt.Printf("go routine: starting #%v of %v for site: %v\n", i+1, len(apps), apps[i])
	}

	// Wait for all goroutines to stop processing
	fmt.Printf("\n\n*********************************\nINFO: All goroutines have been fired. Now wait for them to complete. %v\n---------------------------------\n\n", time.Now())
	wg.Wait()

	fmt.Printf("*********************************\nINFO: All goroutines have now ended. %v\n---------------------------------\n\n", time.Now())

	// Take a pause and wait on user input to proceed
	fmt.Printf("******************************************************************\nINFO: Taking a quick pause. Type something and hit enter to resume the program:\n\n")
	reader := bufio.NewReader(os.Stdin)
	var text string
	for {
		text, _ = reader.ReadString('\n')
		if len(text) != 0 {
			break
		}
	}

	fmt.Printf("\n\n*********************************\nINFO: Printing out the results.\n---------------------------------\n")

	// Print out results
	fmt.Printf("Application,Accessible,HTTP Status,Notes\n")
	for i := 0; i < len(apps); i++ {
		v := <-chn
		fmt.Printf("%v,%v,%v,%v\n", v.App, v.Accessible, v.Status, v.Notes)
	}

	fmt.Printf("\n\n*********************************\nINFO: complete processing\n---------------------------------\n\n")
}
