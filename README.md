The server reads 1K bytes from /dev/urandom and encodes them in base64, generating a unique random string for each request.
The essence of the work is to calculate the MD5 hash of the received random string for a specified period of time (in milliseconds), simulating work that requires a lot of CPU resources.
The server accepts CPU time, synchronous wait parameters in the request URL. If parameters are not specified, default values are used.

## Build
```bash
go build idler
```

## Run
```bash
./idler
```

## Use
```bash
curl -X GET "http://localhost:8080/payload/2000/0"
```

Url `payload/{cpuMsec}/{waitMsec}`

`cpuMsec` - CPU time that the service spends on calculating hash sums

`waitMsec` - Waiting after execution

Default parameters - `payload/10/0`






