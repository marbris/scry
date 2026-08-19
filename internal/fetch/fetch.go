// Package fetch is the one place scry talks to the network.
//
// Four services are involved — Scryfall for cards, Moxfield for decks,
// MTGJSON for printed text, and Wizards for the rules — and they all want the
// same courtesy: a User-Agent that says who is calling. Keeping that in one
// place is the only way it stays true of all four.
package fetch

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// UserAgent identifies scry to every service it calls.
const UserAgent = "scry/2.1"

// NotFound is a 404, which several callers treat as an answer rather than a
// failure: no such card, no such deck, no results.
type NotFound struct{}

func (e NotFound) Error() string { return "no results found" }

// Get fetches a URL as JSON.
func Get(u string) ([]byte, error) {
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "application/json;q=0.9,*/*;q=0.8")
	return do(http.DefaultClient, req, u)
}

// Post sends a JSON body and reads a JSON reply.
func Post(u string, payload []byte) ([]byte, error) {
	req, err := http.NewRequest("POST", u, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	return do(http.DefaultClient, req, u)
}

// GetFile fetches something that isn't JSON — the comprehensive rules are a
// plain text file, served through a redirect that drops headers unless they
// are set again on the way.
func GetFile(u string) ([]byte, error) {
	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			req.Header.Set("User-Agent", UserAgent)
			req.Header.Set("Accept", "*/*")
			return nil
		},
	}
	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "*/*")
	return do(client, req, u)
}

func do(client *http.Client, req *http.Request, u string) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode == 404 {
		return nil, NotFound{}
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s (%d): %s", Host(u), resp.StatusCode, string(body))
	}
	return body, nil
}

// Host labels an error with the service that produced it — cards and rulings
// come from Scryfall, decks from Moxfield.
func Host(u string) string {
	parsed, err := url.Parse(u)
	if err != nil || parsed.Host == "" {
		return "request"
	}
	return strings.TrimPrefix(parsed.Host, "api.")
}
