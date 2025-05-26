The server reads 16K bytes from /dev/urandom and encodes them in base64, generating a unique random string for each request.
The essence of the work is to calculate the MD5 hash of the received random string for a specified period of time (in milliseconds), simulating work that requires a lot of CPU resources.
The server accepts CPU time, synchronous wait parameters in the request URL. If parameters are not specified, default values are used.

## Build
```bash
go mod tidy
go mod vendor
go build idler.go
```

## Run

*variants*
```bash
./idler -maxprocs 4 -port 8078 # only payload, health and hello handlers
./idler -maxprocs 4 -port 8078 -config config.json -init_redis_keys -init_redis -init_postgres -init_postgres_keys # run with redis and postgres handlers
./idler -init_postgres -clear_postgres #clean dataset in pg and stop application
```

### Options:

1) `maxprocs` - default: 4

2) `port` - default: 8078

3) `config` sample file
```json
{
  "redis": {
    "addr": "localhost:6379", // default: localhost:6379
    "password": "",           // default: empty
    "db": 0,                  // default: 0
    "ttl": 30,                // default: 30 (seconds)
    "count": 1000             // default: 1000 (test data pool)
  },
  "postgres": {
    "dns": "postgres://postgres:password@localhost:5432/dbname",
    "count": 1000,             // default: 1000 (test data pool)
    "table_name": "key_value", // default: key_value
    "min_conns": 1,            // default: 2 
    "max_conns": 10,           // default: 20
    "life_time": 60,           // default: 60*60 (seconds)
    "health_check": 60         // default: 60 (seconds)
  }
}
```

4) `init_redis` - activate redis handler

`/redis` - get random keys

`/redis?id=10` - get current keys

5) `init_postgres` - activate postgres handler

`/postgres` - get random keys

`/postgres?id=10` - get current keys

6) `init_redis_keys` and `init_postgres_keys` - load keys and values to storage, range [1 - <count>]

## Use
```bash
curl -X GET "http://localhost:8078/health"
curl -X GET "http://localhost:8078/payload?cpu_ms=100&io_ms=500"
curl -X GET "http://localhost:8078/health" # return OK
curl -X GET "http://localhost:8078/hello"  # return Hello, world!
```

`cpu_ms` - CPU time that the service spends on calculating hash sums

`io_ms` - Waiting after execution

### Remark
> for use on mac os need change line 26 to 25, current version is prepared for linux. 
> syscall.RUSAGE_THREAD to syscall.RUSAGE_SELF 