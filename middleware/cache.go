package middleware

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/go-redis/redis"
)

type DurationSettings struct {
	TTLSeconds   int64 `json:"ttl"`
	StaleSeconds int64 `json:"stale"`
}

func (r *DurationSettings) TTL() time.Duration {
	return time.Duration(r.TTLSeconds) * time.Second
}

func (r *DurationSettings) Stale() time.Duration {
	return time.Duration(r.StaleSeconds) * time.Second
}

type AsyncCache struct {
	client   *redis.Client
	settings map[string]DurationSettings
}

func NewAsyncCache() *AsyncCache {

	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})

	settings := make(map[string]DurationSettings)

	jsonFile, err := ioutil.ReadFile("route-config.json")
	if err != nil {
		log.Fatal(err)
	}
	err = json.Unmarshal(jsonFile, &settings)

	if err != nil {
		log.Fatal(err)
	}

	return &AsyncCache{
		client:   client,
		settings: settings,
	}
}

func AsyncCacheMiddleware(handler http.Handler, cache *AsyncCache) http.Handler {

	return http.HandlerFunc(func(writer http.ResponseWriter, req *http.Request) {

		// Cache Key is Host/Path
		key := fmt.Sprintf("%s%s", req.Host, req.URL.Path)

		// Get the route Stale/TTL settings
		durationSettings, ok := cache.settings[req.URL.Path]

		if !ok {
			// Default settings
			durationSettings = DurationSettings{TTLSeconds: 600, StaleSeconds: 60}
		}

		// Attempt to retrieve from the cache
		content, err := cache.client.Get(key).Result()

		if err != nil {
			log.Printf("Cache Miss")

			rec := cache.FetchAndCache(handler, req, durationSettings, key)

			for key, value := range rec.Header() {
				writer.Header()[key] = value
			}
			writer.Write([]byte(content))

		} else {
			log.Printf("Cache Hit")

			writer.Write([]byte(content))

			// Check the stale threshold of the data to determine
			// if we need to asynchronously refresh it
			remainingTTL, err := cache.client.TTL(key).Result()
			age := durationSettings.TTL() - remainingTTL

			if err == nil && age > durationSettings.Stale() {
				log.Printf("Cache Stale")
				// Set the stale data, back into the cache for the
				// stale duration, to stop multiple requests.
				cache.client.Set(key, content, durationSettings.Stale())
				go cache.FetchAndCache(handler, req, durationSettings, key)
			}
		}
	})
}

func (cache *AsyncCache) FetchAndCache(handler http.Handler, req *http.Request, durationSettings DurationSettings,
	key string) httptest.ResponseRecorder {
	// Record the request
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	content := rec.Body.Bytes()

	// Cache the 200's
	if rec.Code == 200 {
		go cache.client.Set(key, content, durationSettings.TTL())
	}
	return *rec
}
