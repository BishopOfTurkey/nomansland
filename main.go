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

	ctx context.Context
)

func main() {
	var err error
	flag.Parse()

	secrets = loadSecrets()

	ctx = context.Background()

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

	conn, err = pgx.Connect(ctx, *dbURI)
	if err != nil {
		log.Fatal("Unable to connect to database:", err)
	}
	defer conn.Close(ctx)

	jsonBlob, err := ioutil.ReadFile("strava_token.json")
	if os.IsNotExist(err) {
		log.Println("strava token file doesn't exist")
	} else if err != nil {
		log.Fatal("Failed to read strava token file: ", err)
	} else {
		var tok oauth2.Token
		err = json.Unmarshal(jsonBlob, &tok)
		_, ok := err.(*json.SyntaxError)
		if ok {
			log.Println("Strava token error:", err)
		} else if err != nil {
			log.Fatal("Failed to parse strava token file: ", err)
		} else {
			stravaClient = conf.Client(ctx, &tok)
		}
	}

	timeout, cancel := context.WithTimeout(ctx, time.Second)
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
	mux.HandleFunc("/data", serveData)

	err = http.ListenAndServe(*addr, mux)
	if err != nil {
		log.Fatal(err)
	}
}

func index(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello!")
}

type Activity struct {
	Name     string    `sql:"name" json:"name"`
	Date     time.Time `sql:"date" json:"date"`
	Distance float64   `sql:"distance" json:"distance"`
	Duration int       `sql:"duration" json:"duration"`
	Title    string    `sql:"title" json:"title"`
	Hall     string    `sql:"hall" json:"hall"`
}

func serveData(w http.ResponseWriter, r *http.Request) {
	timeout, cancel := context.WithTimeout(r.Context(), time.Second)
	defer cancel()
	rows, err := conn.Query(timeout, `SELECT name, date, distance, duration, title, hall FROM activities ORDER BY date DESC;`)
	if err != nil {
		log.Println("Failed to get activities:", err)
	}

	var activities []Activity

	for rows.Next() {
		a := Activity{}
		err = rows.Scan(&a.Name, &a.Date, &a.Distance, &a.Duration, &a.Title, &a.Hall)
		if err != nil {
			log.Println("Can't parse row: ", err)
			return
		}
		activities = append(activities, a)
	}

	jsonBlob, err := json.Marshal(activities)
	if err != nil {
		log.Println("Failed to marshal acivities", err)
		return
	}
	w.Header().Add("Content-type", "application/json")
	_, err = w.Write(jsonBlob)
	if err != nil {
		log.Println("Failed to send data", err)
	}
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
		log.Printf("Invalid user attempted to authenticate: %v\n", athlete)
		return
	}

	jsonBlob, err := json.Marshal(tok)
	if err != nil {
		io.WriteString(w, "Failed to marshal token")
		log.Printf("Failed to marshal token: %v\n", err)
		return
	}

	stravaClient = conf.Client(ctx, tok)

	err = ioutil.WriteFile("strava_token.json", jsonBlob, 0400)
	if err != nil {
		io.WriteString(w, "Failed to save token")
		log.Printf("Failed to save token: %v\n", err)
		return
	}

	io.WriteString(w, "Strava oauth2 token successfully added.")
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
