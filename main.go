package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/sholmesy/asynccache/middleware"
)

func handler(w http.ResponseWriter, r *http.Request) {
	log.Printf("Retrieving slow data...")
	time.Sleep(3 * time.Second)
	log.Printf("Done!")
	fmt.Fprintf(w, "Response for route:", r.URL.Path[1:])
}

func main() {

	log.Printf("Setting up...")
	cache := middleware.NewAsyncCache()

	http.Handle("/", middleware.AsyncCacheMiddleware(http.HandlerFunc(handler), cache))
	log.Printf("Good to go")

	http.ListenAndServe(":8080", nil)
}
