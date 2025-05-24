The server reads 16K bytes from /dev/urandom and encodes them in base64, generating a unique random string for each request.
The essence of the work is to calculate the MD5 hash of the received random string for a specified period of time (in milliseconds), simulating work that requires a lot of CPU resources.
The server accepts CPU time, synchronous wait parameters in the request URL. If parameters are not specified, default values are used.

## Build
```bash
go build idler.go
```

## Run
```bash
./idler -maxprocs 4 -port 8078
```
`maxprocs` - default: 4

`port` - default: 8078

## Use
```bash
curl -X GET "http://localhost:8078/health"
curl -X GET "http://localhost:8078/payload?cpu_ms=100&io_ms=500"
```

`cpu_ms` - CPU time that the service spends on calculating hash sums

`io_ms` - Waiting after execution




