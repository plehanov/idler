package pkg

import (
	"crypto/aes"
	"crypto/md5"
	"crypto/sha256"
	"math/rand"
	//    "fmt"
	"os"
	"runtime"
	"syscall"
)

const (
	bufSize = 16 * 1024 // 16K
	//who     = syscall.RUSAGE_SELF
	who = syscall.RUSAGE_THREAD
)

type getrusagePayload struct {
	data []byte
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

func (p getrusagePayload) CPULoadLow(msec int64) (cycles uint, elapsedMsec float64, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var startUsage syscall.Rusage // Стартовые показатели CPU
	if err = syscall.Getrusage(who, &startUsage); err != nil {
		return 0, 0, err
	}

	durationMs := float64(msec)
	for elapsedMsec <= durationMs {
		md5Work(p.data)

		cycles++
		if elapsedMsec, err = elapsedUsageMsec(startUsage); err != nil {
			return 0, 0, err
		}
	}

	return cycles, elapsedMsec, nil
}

func (p getrusagePayload) CPULoadHeavy(msec int64) (cycles uint, elapsedMsec float64, err error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var startUsage syscall.Rusage
	if err = syscall.Getrusage(who, &startUsage); err != nil {
		return 0, 0, err
	}

	durationMs := float64(msec)
	for elapsedMsec <= durationMs {
		heavyWork(p.data, 32)

		cycles++
		if elapsedMsec, err = elapsedUsageMsec(startUsage); err != nil {
			return 0, 0, err
		}
	}

	return cycles, elapsedMsec, nil
}

func randomUserID() uint {
	return uint(1 + rand.Intn(1000))
}

func md5Work(data []byte) {
	_ = md5.Sum(data)
}

func heavyWork(data []byte, size int) {
	h := sha256.New()
	h.Write(data)
	_ = h.Sum(nil)

	cipher, _ := aes.NewCipher(data[:size])
	dst := make([]byte, size)
	cipher.Encrypt(dst, data[:size])
}
