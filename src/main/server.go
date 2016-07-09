package main

import (
    "fmt"
    "net/http"
    "log"
    "github.com/gorilla/mux"
)

func serverHandler(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi there, I am serverHandler!", r.URL.Path[1:])
}

//TODO:Add authentication

////DISTRIBUTOR

//ConnectionID + StreamID
//Extension server IP:PORT
func extendAggStream(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi there, I am extendAggStream !", r.URL.Path[1:])
}

////CLIENT

//ConnectionID
func createConnection(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi there, I am createConnection !", r.URL.Path[1:])
}

//ConnectionID + StreamID
//Extension servers[] IP:PORT
func addStream(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi there, I am addEndPoint !", r.URL.Path[1:])
}

//ConnectionID + StreamID
func deleteStream(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi there, I am deleteStream !", r.URL.Path[1:])
}


//CLIENT & DISTRIBUTOR
//ConnectionID
func terminateConnection(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi there, I am terminateStream !", r.URL.Path[1:])
}

////GENERAL

//ConnectionID + StreamID
//Extension server IP:PORT
func extendStream(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi there, I am extendStream !", r.URL.Path[1:])
}

//ConnectionID + StreamID
func terminateStream(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi there, I am terminateStream !", r.URL.Path[1:])
}

//Open and wait for incoming stream
//ConnectionID + StreamID
func openIncoming(w http.ResponseWriter, r *http.Request) {
    fmt.Fprintf(w, "Hi there, I am openIncoming !", r.URL.Path[1:])
}

func StartServer() {
	router := mux.NewRouter().StrictSlash(true)
    router.HandleFunc("/", serverHandler)
    router.HandleFunc("/openIncoming", openIncoming)
    
    log.Fatal(http.ListenAndServe(":8080", router))
}