package LoadBalance

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"time"
)

type Proxy struct {
	Port string `json:"port"`
}

type Backend struct {
	URL    string `json:"url"`
	IsDead bool
	mu     sync.Mutex
}

type Config struct {
	Proxy    Proxy     `json:"proxy"`
	Backends []Backend `json:"backends"`
}

func (backend *Backend) SetDead(set bool) {
	backend.mu.Lock()
	backend.IsDead = set
	backend.mu.Unlock()
}

func (backend *Backend) IsWorking() bool {
	backend.mu.Lock()
	isWorking := backend.IsDead
	backend.mu.Unlock()
	return isWorking
}

/*
we need to check the current backend is working or not
other wise our request will be goinf to the dead backend
*/
var cfg Config

/*
implementation of round robin algorithm
*/

var mu sync.Mutex
var current int = 0

/*
mutex lock is for locking the server backend for multiple goroutines access
the same backend at the same time to avoid race conditions
*/
/*
TODO: Need to implement retries or support persistence
*/
func lbHandler(w http.ResponseWriter, r *http.Request) {
	mx := len(cfg.Backends)

	mu.Lock()
	backend := cfg.Backends[current%mx]
	if backend.IsWorking() {
		current++
	}
	url, err := url.Parse(cfg.Backends[current%mx].URL)

	if err != nil {
		log.Fatal(err.Error())
	}
	current++
	mu.Unlock()
	rp := httputil.NewSingleHostReverseProxy(url)
	rp.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {

		log.Printf("%v is not working", url)
		backend.SetDead(true)
		lbHandler(w, r)
	}
	rp.ServeHTTP(w, r)
}

func isAlive(url *url.URL) bool {
	connection, err := net.DialTimeout("tcp", url.Host, time.Minute*1)
	if err != nil {
		log.Printf("Unreachable to %v, error: ", url.Host, err.Error())

	}
	defer connection.Close()
	return true
}

func HealthCheck() {
	t := time.NewTicker(time.Minute * 1)

	for {
		select {
		case <-t.C:
			for _, backend := range cfg.Backends {
				pingURL, err := url.Parse(backend.URL)
				if err != nil {
					log.Fatal(err.Error())
				}
				isAlive := isAlive(pingURL)
				backend.SetDead(!isAlive)
				msg := "working"
				if !isAlive {
					msg = "dead"
				}
				log.Printf("%v checked %v", backend.URL, msg)

			}
		}
	}
}

func Serve() {
	data, err := ioutil.ReadFile("./setting.json")

	if err != nil {
		log.Fatal(err.Error())
	}
	json.Unmarshal(data, &cfg)

	go HealthCheck()
	app := http.Server{
		Addr:    ":" + cfg.Proxy.Port,
		Handler: http.HandlerFunc(lbHandler),
	}

	if err := app.ListenAndServe(); err != nil {
		log.Fatal(err.Error())
	}
}
