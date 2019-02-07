package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func getMinioPeersAddr() []string {
	cmd := "ps -e -o args | grep 'minio server' | grep -v grep | head -n1"
	out, err := exec.Command("bash", "-c", cmd).Output()
	if err != nil {
		log.Fatal(err)
	}
	minioCmd := string(out)
	minioCmd = strings.TrimSpace(minioCmd)

	var peersAddr []string

	args := strings.Split(minioCmd, " ")
	for _, a := range args {
		u, err := url.Parse(a)
		if err != nil {
			continue
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme != "http" && scheme != "https" {
			continue
		}
		host := u.Hostname()
		if host == "" {
			continue
		}
		peersAddr = append(peersAddr, host)
	}

	return peersAddr
}

func startPingHandler() {
	ping := func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("pong"))
	}

	go func() {
		http.HandleFunc("/ping", ping)
		if err := http.ListenAndServe(":9080", nil); err != nil {
			panic(err)
		}
	}()
}

func configureLogging() {
	logBaseDir := "/tmp/minio-healthcheck"
	logFilename := filepath.Join(logBaseDir, fmt.Sprintf("%v", time.Now().UTC().Unix()))
	if err := os.MkdirAll("/tmp/minio-healthcheck/", 0744); err != nil {
		panic(err)
	}
	f, err := os.Create(logFilename)
	if err != nil {
		panic(err)
	}
	log.SetOutput(f)
}

func startPingPeersNLog() {
	peersAddr := getMinioPeersAddr()
	if len(peersAddr) < 4 {
		log.Println("Found peers:", peersAddr)
		os.Exit(1)
	}

	var lastOpsMap = sync.Map{}

	log := func(key string, err error, v ...interface{}) {
		var shouldPrint bool
		if s, ok := lastOpsMap.Load(key); !ok {
			shouldPrint = true
		} else {
			var peerErr error
			if s != nil {
				peerErr = s.(error)
			}
			if (err == nil && peerErr != nil) || (err != nil && peerErr == nil) {
				shouldPrint = true
			}
		}

		lastOpsMap.Store(key, err)

		if shouldPrint {
			log.Printf("error: <%v> %v ", err, fmt.Sprintln(v...))
		}
	}

	for {
		for _, peerAddr := range peersAddr {
			resp, err := http.Get("http://" + peerAddr + ":9080" + "/ping")
			log(peerAddr, err)
			if err != nil {
				continue
			}
			defer resp.Body.Close()
			body, err := ioutil.ReadAll(resp.Body)
			log(peerAddr, err)
			if err != nil {
				continue
			}
			if string(body) != "pong" {
				log(peerAddr, fmt.Errorf("unexpected pong response: `%s`", string(body)))
				continue
			}
			log(peerAddr, nil, "first success!")
		}

		time.Sleep(5 * time.Second)
	}
}

func main() {
	startPingHandler()
	configureLogging()
	startPingPeersNLog()
}
