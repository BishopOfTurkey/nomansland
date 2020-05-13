package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v4"
	"golang.org/x/oauth2"
)

var (
	addr  = flag.String("addr", "localhost:8080", "address to host the server on")
	dbURI = flag.String("db", "", "uri to access postgres database")
)

var (
	secrets      StravaAPIKeys
	conn         *pgx.Conn
	conf         *oauth2.Config
	stravaClient *http.Client

	cxt context.Context
)

func main() {
	var err error
	flag.Parse()

	secrets = loadSecrets()

	cxt = context.Background()

	conf = &oauth2.Config{
		ClientID:     secrets.ClientID,
		ClientSecret: secrets.ClientSecret,
		Scopes:       []string{"read"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  "http://www.strava.com/oauth/authorize",
			TokenURL: "http://www.strava.com/oauth/token",
		},
		RedirectURL: fmt.Sprintf("%v%v%v", "http://", *addr, "/strava_token"),
	}

	conn, err = pgx.Connect(cxt, *dbURI)
	if err != nil {
		log.Fatal("Unable to connect to database:", err)
	}
	defer conn.Close(cxt)

	timeout, cancel := context.WithTimeout(cxt, time.Second)
	err = conn.Ping(timeout)
	if err != nil {
		log.Fatal(err)
	}
	cancel()

	log.Println("Connected to DB successfully.")

	mux := http.NewServeMux()

	mux.HandleFunc("/auth", stravaOAuth)
	mux.HandleFunc("/strava_token", handleStravaToken)

	mux.HandleFunc("/", index)

	err = http.ListenAndServe(*addr, mux)
	if err != nil {
		log.Fatal(err)
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello!")
}

func handleStravaToken(w http.ResponseWriter, r *http.Request) {
	code, ok := r.URL.Query()["code"]
	if !ok || len(code) == 0 {
		io.WriteString(w, "Failed to authenticate with Strava")
		log.Printf("Keys in request: %+v\n", r.URL.RawQuery)
		return
	}

	// Use the authorization code that is pushed to the redirect
	// URL. Exchange will do the handshake to retrieve the
	// initial access token. The HTTP Client returned by
	// conf.Client will refresh the token as necessary.
	ctx := context.Background()
	tok, err := conf.Exchange(ctx, code[0])
	if err != nil {
		log.Println(err)
		return
	}

	athlete, ok := tok.Extra("athlete").(map[string]interface{})
	if !ok {
		io.WriteString(w, "Failed to authenticate with Strava")
		log.Printf("No athelete field in request.\n")
		return
	}
	createdAt, ok := athlete["created_at"].(string)
	if !ok {
		io.WriteString(w, "Failed to authenticate with Strava")
		log.Printf("created_at field not string\n")
		return
	}
	if createdAt != secrets.ClientCreatedAt {
		io.WriteString(w, "Unauthorised")
		log.Printf("Invalid used attempted to authenticate: %v\n", athlete)
		return
	}

	http.Redirect(w, r, "/", http.StatusFound)
}

type StravaAPIKeys struct {
	ClientSecret    string `json:"client_secret"`
	ClientID        string `json:"client_id"`
	ClientCreatedAt string `json:"created_at"`
}

func loadSecrets() StravaAPIKeys {
	file, err := os.Open("config.json")
	if err != nil {
		log.Fatal(err)
	}
	jsonBlob, err := ioutil.ReadAll(file)
	if err != nil {
		log.Fatal(err)
	}

	json.Unmarshal(jsonBlob, &secrets)

	return secrets
}

func stravaOAuth(w http.ResponseWriter, r *http.Request) {
	// Redirect user to consent page to ask for permission
	// for the scopes specified above.
	url := conf.AuthCodeURL("state", oauth2.AccessTypeOnline, oauth2.SetAuthURLParam("approval_prompt", "force"))

	http.Redirect(w, r, url, http.StatusFound)
}

func 