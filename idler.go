package main

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		cpuMsec := 10
		waitMsec := 0

		if r.URL.Path != "/" {
			parts := strings.Split(r.URL.Path[1:], "/")
			if len(parts) >= 1 && parts[0] != "" {
				if val, err := strconv.Atoi(parts[0]); err == nil {
					cpuMsec = val
				}
			}
			if len(parts) >= 2 && parts[1] != "" {
				if val, err := strconv.Atoi(parts[1]); err == nil {
					waitMsec = val
				}
			}
		}

		startTime := time.Now()

		randomBytes := make([]byte, 1024)
		file, err := os.Open("/dev/urandom")
		if err != nil {
			http.Error(w, "Failed to open /dev/urandom", http.StatusInternalServerError)
			return
		}
		defer file.Close()

		_, err = io.ReadFull(file, randomBytes)
		if err != nil {
			http.Error(w, "Failed to read random bytes", http.StatusInternalServerError)
			return
		}

		randomString := base64.StdEncoding.EncodeToString(randomBytes)

		// CPU-bound work
		startCPU := time.Now()
		for time.Since(startCPU).Milliseconds() < int64(cpuMsec) {
			hash := md5.Sum([]byte(randomString))
			_ = hash
			runtime.Gosched()
		}

		// Wait (IO-bound simulation)
		if waitMsec > 0 {
			time.Sleep(time.Duration(waitMsec) * time.Millisecond)
		}

		response := map[string]interface{}{
			"status":        "completed",
			"cpu_time_ms":   cpuMsec,
			"wait_time_ms":  waitMsec,
			"total_time_ms": time.Now().Sub(startTime).Milliseconds(),
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	http.ListenAndServe(":8080", nil)
}
