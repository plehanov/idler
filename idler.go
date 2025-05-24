package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"plehanov/idler/pkg"
	"runtime"
	"strconv"
	"time"
)

func main() {
	maxProcs := flag.Int("maxprocs", runtime.NumCPU(), "maximum number of CPUs that can be used")
	httpPort := flag.Int("port", 8078, "HTTP server port")
	flag.Parse()

	runtime.GOMAXPROCS(*maxProcs)
	fmt.Println("set GOMAXPROCS =", *maxProcs)
	fmt.Println("set port =", *httpPort)

	http.HandleFunc("/payload", func(w http.ResponseWriter, r *http.Request) {
		startUsage := time.Now()
		response := map[string]interface{}{
			"cycles":        0,
			"total_time_ms": 0,
		}

		if cpuMsecStr := r.URL.Query().Get("cpu_ms"); cpuMsecStr != "" {
			cpuMsec, err := strconv.ParseInt(cpuMsecStr, 10, 64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			//log.Print("CPU msec =", cpuMsec)
			worker := pkg.NewGetrusagePayload()
			cycles, _, err := worker.CPULoadHeavy(cpuMsec)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			response["cycles"] = cycles
			//response["cpu_time_ms"] = elapsedMsec
		}

		if ioMsecStr := r.URL.Query().Get("io_ms"); ioMsecStr != "" {
			ioMsec, err := strconv.ParseInt(ioMsecStr, 10, 64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			//log.Print("I/O msec =", ioMsec)
			time.Sleep(time.Duration(ioMsec) * time.Millisecond)
			//response["io_time_ms"] = ioMsec
		}

		response["total_time_ms"] = float64(time.Since(startUsage).Nanoseconds()) / 1e6

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("OK"))
	})

	http.ListenAndServe(":"+strconv.Itoa(*httpPort), nil)
}
