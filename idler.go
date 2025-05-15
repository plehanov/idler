package main

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func payloadHandler(w http.ResponseWriter, r *http.Request) {
	cpuMsec, waitMsec, pathParts := params(r)

	startTime := time.Now()
	processStartTime := time.Now()

	// Читаем 1K случайных байт из /dev/urandom
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

	// CPU-intensive work
	startCPU := time.Now()
	for time.Since(startCPU).Milliseconds() < int64(cpuMsec) {
		hash := md5.Sum([]byte(randomString))
		_ = hash          // Используем hash чтобы компилятор не оптимизировал
		runtime.Gosched() // Даем возможность работать другим горутинам
	}

	// Sync wait
	if waitMsec > 0 {
		time.Sleep(time.Duration(waitMsec) * time.Millisecond)
	}

	// Формируем ответ
	response := map[string]interface{}{
		"status":          "completed",
		"cpu_time_ms":     cpuMsec,
		"wait_time_ms":    waitMsec,
		"total_time_ms":   time.Since(startTime).Milliseconds(),
		"process_time_ms": time.Since(processStartTime).Milliseconds(),
		"used_defaults":   len(pathParts) < 2, // Показывает, использовались ли значения по умолчанию
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func params(r *http.Request) (int, int, []string) {
	// Устанавливаем значения по умолчанию
	cpuMsec := 10
	waitMsec := 0

	// Извлекаем параметры из URL, если они есть
	pathParts := strings.Split(r.URL.Path[len("/payload/"):], "/")

	if len(pathParts) >= 1 && pathParts[0] != "" {
		if val, err := strconv.Atoi(pathParts[0]); err == nil {
			cpuMsec = val
		}
	}
	if len(pathParts) >= 2 && pathParts[1] != "" {
		if val, err := strconv.Atoi(pathParts[1]); err == nil {
			waitMsec = val
		}
	}
	return cpuMsec, waitMsec, pathParts

	//	// Извлекаем параметры из URL
	//	params := r.URL.Path[len("/payload/"):]
	//	var cpuMsec, waitMsec, waitAsyncMsec int
	//	_, err := fmt.Sscanf(params, "%d/%d/%d", &cpuMsec, &waitMsec, &waitAsyncMsec)
	//	if err != nil {
	//		http.Error(w, "Invalid parameters", http.StatusBadRequest)
	//		panic(err)
	//	}
	//  return cpuMsec, waitMsec, waitAsyncMsec, pathParts
}

func main() {
	server := &http.Server{
		Addr: ":8080",
		//ReadTimeout:  10 * time.Second,
		//WriteTimeout: 10 * time.Second,
		//IdleTimeout:  30 * time.Second,
	}

	http.HandleFunc("/payload/", payloadHandler)
	http.HandleFunc("/payload", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/payload/10/0", http.StatusFound)
	})

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		log.Println("Server started on :8080")
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v\n", err)
		}
	}()

	<-stop
	log.Println("Server is shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Printf("Server shutdown error: %v\n", err)
	}

	log.Println("Server stopped")
}
