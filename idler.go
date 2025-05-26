package main

import (
	"crypto/aes"
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

const (
	bufSize = 16 * 1024 // 16K
	//who     = syscall.RUSAGE_SELF
	who = syscall.RUSAGE_THREAD
)

type getrusagePayload struct {
	data []byte
}

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
			worker := NewGetrusagePayload()
			cycles, elapsedMsec, err := worker.CPULoad(cpuMsec)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			response["cycles"] = cycles
			response["cpu_time_ms"] = elapsedMsec
		}

		if ioMsecStr := r.URL.Query().Get("io_ms"); ioMsecStr != "" {
			ioMsec, err := strconv.ParseInt(ioMsecStr, 10, 64)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			//log.Print("I/O msec =", ioMsec)
			time.Sleep(time.Duration(ioMsec) * time.Millisecond)
			response["io_time_ms"] = ioMsec
		}

		response["total_time_ms"] = float64(time.Since(startUsage).Nanoseconds()) / 1e6

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	})

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("OK"))
	})

	http.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("Hello, world!"))
	})

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./public"))))

	http.ListenAndServe(":"+strconv.Itoa(*httpPort), nil)
}

func NewGetrusagePayload() getrusagePayload {
	file, err := os.Open("/dev/urandom")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	data := make([]byte, bufSize)
	file.Read(data)

	return getrusagePayload{
		data: data,
	}
}

func (p getrusagePayload) CPULoad(msec int64) (cycles uint, elapsedMsec float64, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var startUsage syscall.Rusage
	if err = syscall.Getrusage(who, &startUsage); err != nil {
		return 0, 0, err
	}

	durationMs := float64(msec)
	durationMs *= 0.99 // 1% overhead
	for elapsedMsec <= durationMs {
		md5Work(p.data)

		cycles++
		if elapsedMsec, err = elapsedUsageMsec(startUsage); err != nil {
			return 0, 0, err
		}
	}

	return cycles, elapsedMsec, nil
}

func md5Work(data []byte) {
	hash := md5.Sum(data)
	_ = hex.EncodeToString(hash[:])
}

func heavyWork(data []byte) {
	h := sha256.New()
	h.Write(data)
	_ = h.Sum(nil)

	cipher, _ := aes.NewCipher(data[:32])
	dst := make([]byte, 32)
	cipher.Encrypt(dst, data[:32])
}

func elapsedUsageMsec(startUsage syscall.Rusage) (float64, error) {
	usage := syscall.Rusage{}
	if err := syscall.Getrusage(who, &usage); err != nil {
		//zap.L().Error("getrusage error", zap.Error(err))
		return 0, err
	}

	elapsed := float64(usage.Utime.Nano()) - float64(startUsage.Utime.Nano()) + float64(usage.Stime.Nano()) - float64(startUsage.Stime.Nano())
	elapsed /= 1e6

	return elapsed, nil
}
