package middleware

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
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

func NewRedisClient() *redis.Client {
	redisHost, ok := os.LookupEnv("REDIS_HOST")
	if !ok {
		redisHost = "localhost"
	}
	redisPort, ok := os.LookupEnv("REDIS_PORT")
	if !ok {
		redisPort = "6379"
	}
	redisPass, ok := os.LookupEnv("REDIS_PASS")
	if !ok {
		redisPass = ""
	}
	redisDBString, ok := os.LookupEnv("REDIS_DB")
	redisDB, err := strconv.Atoi(redisDBString)
	if !ok || err != nil {
		redisDB = 0
	}
	urlString := fmt.Sprintf("%s:%s", redisHost, redisPort)
	return redis.NewClient(&redis.Options{
		Addr:     urlString,
		Password: redisPass,
		DB:       redisDB,
	})
}

func NewAsyncCache() *AsyncCache {
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
		client:   NewRedisClient(),
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

			rec := cache.FetchAndCache(handler, req, durationSettings, key)

			for key, value := range rec.Header() {
				writer.Header()[key] = value
			}
			writer.Write([]byte(rec.Body.Bytes()))

		} else {

			writer.Write([]byte(content))

			// Check the stale threshold of the data to determine
			// if we need to asynchronously refresh it
			remainingTTL, err := cache.client.TTL(key).Result()
			age := durationSettings.TTL() - remainingTTL

			if err == nil && age > durationSettings.Stale() {
				// Set the stale data, back into the cache for the
				// stale duration, to stop multiple requests.
				cache.client.Set(key, content, durationSettings.Stale())
				go cache.FetchAndCache(handler, req, durationSettings, key)
			}
			// No need to run the other middleware syncronously. Return early
			return
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
