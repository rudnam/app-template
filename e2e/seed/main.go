// Command seed fills an account with sample data through the API, so a
// preview has something on its screens. preview.sh runs it against its fresh
// database:
//
//	go run ./e2e/seed -url http://server:8080 -email preview@example.com -password preview-password
//
// It goes through the API, as a user would, so it needs no access to the
// database and keeps working as the schema changes. When you replace the
// example feature, replace what this adds too.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/cookiejar"
	"time"
)

// notes are added oldest first, so the list shows the last one at the top.
var notes = []string{
	"Welcome! This preview starts with a few sample notes.",
	"Groceries: eggs, rice, miso, spring onions",
	"Book the dentist for next month",
	"Idea: a weekly summary email",
	"Call the bank about the card replacement",
}

func main() {
	base := flag.String("url", "http://localhost:8080", "the app's address")
	email := flag.String("email", "preview@example.com", "account to fill")
	password := flag.String("password", "preview-password", "its password")
	flag.Parse()

	jar, _ := cookiejar.New(nil)
	c := &client{base: *base, http: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
	c.must("POST", "/api/login", map[string]string{"email": *email, "password": *password})
	for _, body := range notes {
		c.must("POST", "/api/notes", map[string]string{"body": body})
	}
	fmt.Printf("Added %d sample notes for %s\n", len(notes), *email)
}

type client struct {
	base string
	http *http.Client
}

// must sends body as JSON and stops the program on anything but a 2xx. The
// Origin header passes the app's cross-site check, as a browser's would.
func (c *client) must(method, path string, body any) {
	b, err := json.Marshal(body)
	if err != nil {
		log.Fatal(err)
	}
	req, err := http.NewRequest(method, c.base+path, bytes.NewReader(b))
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", c.base)
	res, err := c.http.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 != 2 {
		var e struct{ Error string }
		_ = json.NewDecoder(res.Body).Decode(&e)
		log.Fatalf("%s %s: %s %s", method, path, res.Status, e.Error)
	}
}
