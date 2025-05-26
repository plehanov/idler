package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

const (
	bufSize = 16 * 1024 // 16K
	//who     = syscall.RUSAGE_SELF
	who                      = syscall.RUSAGE_THREAD
	defaultPostgresTableName = "key_value"
	defaultPostgresCount     = 1000
	defaultRedisDB           = 0
	defaultRedisPassword     = ""
	defaultRedisTTL          = 30 * time.Second
	defaultRedisCount        = 1000
	defaultConfigFile        = "config.json"
	defaultRedisAddress      = "localhost:6379"

	// Настраиваем параметры пула
	poolMaxConns          = 20        // Максимальное количество соединений
	poolMinConns          = 2         // Минимальное количество соединений
	poolMaxConnLifetime   = time.Hour // Максимальное время жизни соединения
	poolHealthCheckPeriod = 1 * time.Minute
)

type Config struct {
	Redis    RedisConfig    `json:"redis"`
	Postgres PostgresConfig `json:"postgres"`
}

type RedisConfig struct {
	Addr     string        `json:"addr"` // localhost:6379
	Password string        `json:"password"`
	DB       int           `json:"db"`
	TTL      time.Duration `json:"ttl"` // in seconds
	Count    int           `json:"count"`
}

type PostgresConfig struct {
	DSN         string        `json:"dns"` // "postgres://username:password@localhost:5432/database_name"
	TableName   string        `json:"table_name"`
	Count       int           `json:"count"`
	MinConns    int           `json:"min_conns"`
	MaxConns    int           `json:"max_conns"`
	LifeTime    time.Duration `json:"life_time"`    // in seconds
	HealthCheck time.Duration `json:"health_check"` // in seconds
}

var (
	configFile       string
	warmupRedis      bool
	initRedis        bool
	initRedisKeys    bool
	initPostgres     bool
	initPostgresKeys bool
	clearPostgres    bool
	rdClient         *redisClient
	pgClient         *postgresClient
	config           Config
)

// ./idler -maxprocs 4 -port 8078 -config config.json -init_redis=true -init_redis_keys=true
// ./idler -maxprocs 4 -port 8078 -config config.json -init_redis_keys -init_redis -init_postgres -init_postgres_keys
func main() {
	maxProcs := flag.Int("maxprocs", runtime.NumCPU(), "maximum number of CPUs that can be used")
	httpPort := flag.Int("port", 8078, "HTTP server port")
	flag.StringVar(&configFile, "config", defaultConfigFile, "path to config file")

	flag.BoolVar(&warmupRedis, "warmup_redis", false, "Warmup Redis with initial data")
	flag.BoolVar(&initRedis, "init_redis", false, "Initialize Redis")
	flag.BoolVar(&initRedisKeys, "init_redis_keys", false, "Initialize Redis values")
	flag.BoolVar(&initPostgres, "init_postgres", false, "Initialize Postgres")
	flag.BoolVar(&initPostgresKeys, "init_postgres_keys", false, "Initialize Postgres values")
	flag.BoolVar(&clearPostgres, "clear_postgres", false, "Clear Postgres data before initialization")
	flag.Parse()

	if err := loadConfig(configFile); err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	runtime.GOMAXPROCS(*maxProcs)
	fmt.Println("set GOMAXPROCS =", *maxProcs)
	fmt.Println("set port =", *httpPort)

	jsonBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fmt.Println("Error marshaling JSON:", err)
		return
	}
	fmt.Println("Loaded config: ", string(jsonBytes))

	ctx := context.Background()

	if initRedis {
		rdClient = NewRedisClient(config.Redis)

		if initRedisKeys {
			fmt.Println("Initializing Redis with new data...")
			if !rdClient.ManySave(ctx) {
				fmt.Println("Failed to initialize Redis")
				os.Exit(1)
			}
			fmt.Println("Successfully initialized Redis")
		}
		if warmupRedis {
			fmt.Println("Warming up Redis...")
			for i := 0; i < 100; i++ {
				_, _ = rdClient.GetRandomValue(ctx)
			}
		}
	}

	if initPostgres {
		var err error
		pgClient, err = NewPostgresClient(ctx, config.Postgres)
		if err != nil {
			fmt.Printf("Failed to connect to Postgres: %v\n", err)
			os.Exit(1)
		}
		defer pgClient.Close()

		if clearPostgres {
			fmt.Println("Clearing Postgres data...")
			err := pgClient.CleanDB(ctx)
			if err != nil {
				fmt.Printf("Failed to clear Postgres data: %v\n", err)
				os.Exit(1)
			} else {
				fmt.Println("Successfully cleared Postgres data")
				os.Exit(0)
			}
		}

		if initPostgresKeys {
			fmt.Println("Initializing Postgres with new data...")
			if err := pgClient.ReinitializeData(ctx); err != nil {
				fmt.Println("Failed to initialize Postgres", err)
				os.Exit(1)
			}
			fmt.Println("Successfully initialized Postgres")
		}
	}

	http.HandleFunc("/payload", payloadHandler)

	http.HandleFunc("/health", healthHandler)

	http.HandleFunc("/hello", helloHandler)

	if rdClient != nil {
		http.HandleFunc("/redis", redisHandler)
	}

	if pgClient != nil {
		http.HandleFunc("/postgres", postgresHandler)
	}

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./public"))))

	http.ListenAndServe(":"+strconv.Itoa(*httpPort), nil)
}

// config

func loadConfig(configFile string) error {
	absPath, err := filepath.Abs(configFile)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	file, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %v", err)
	}

	if err := json.Unmarshal(file, &config); err != nil {
		return fmt.Errorf("failed to parse config: %v", err)
	}

	if config.Redis.Addr == "" {
		config.Redis.Addr = defaultRedisAddress
	}
	if config.Redis.TTL == 0 {
		config.Redis.TTL = defaultRedisTTL
	} else {
		config.Redis.TTL *= time.Second
	}
	if config.Redis.DB == 0 {
		config.Redis.DB = defaultRedisDB
	}
	if config.Redis.Password == "" {
		config.Redis.Password = defaultRedisPassword
	}
	if config.Redis.Count == 0 {
		config.Redis.Count = defaultRedisCount
	}
	if config.Postgres.Count == 0 {
		config.Postgres.Count = defaultPostgresCount
	}
	if config.Postgres.TableName == "" {
		config.Postgres.TableName = defaultPostgresTableName
	}

	if config.Postgres.MinConns == 0 {
		config.Postgres.MinConns = poolMinConns
	}
	if config.Postgres.MaxConns == 0 {
		config.Postgres.MaxConns = poolMaxConns
	}
	if config.Postgres.LifeTime == 0 {
		config.Postgres.LifeTime = poolMaxConnLifetime
	} else {
		config.Postgres.LifeTime *= time.Second
	}
	if config.Postgres.HealthCheck == 0 {
		config.Postgres.HealthCheck = poolHealthCheckPeriod
	} else {
		config.Postgres.HealthCheck *= time.Second
	}

	return nil
}

// HTTP handlers

func payloadHandler(w http.ResponseWriter, r *http.Request) {
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
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("OK"))
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.Write([]byte("Hello, world!"))
}

func redisHandler(w http.ResponseWriter, r *http.Request) {
	startUsage := time.Now()
	response := map[string]interface{}{
		"total_time_ms": 0,
	}
	ctx := context.Background()

	if id := r.URL.Query().Get("id"); id != "" {
		value, err := rdClient.GetByID(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		response["value"] = value
	} else {
		value, err := rdClient.GetRandomValue(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		response["value"] = value
	}

	response["total_time_ms"] = float64(time.Since(startUsage).Nanoseconds()) / 1e6
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func postgresHandler(w http.ResponseWriter, r *http.Request) {
	startUsage := time.Now()
	response := map[string]interface{}{
		"total_time_ms": 0,
	}
	ctx := context.Background()

	if idkey := r.URL.Query().Get("id"); idkey != "" {
		id, err := strconv.Atoi(idkey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		value, err := pgClient.GetByID(ctx, id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		response["value"] = value
	} else {
		value, err := pgClient.GetRandomValue(ctx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		response["value"] = value
	}

	response["total_time_ms"] = float64(time.Since(startUsage).Nanoseconds()) / 1e6
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetrusagePayload

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

// Redis

type redisClient struct {
	rdb    *redis.Client
	config RedisConfig
}

func NewRedisClient(config RedisConfig) *redisClient {
	rdb := redis.NewClient(&redis.Options{
		Addr:     config.Addr,
		Password: config.Password,
		DB:       config.DB,
	})

	return &redisClient{
		rdb:    rdb,
		config: config,
	}
}

func (r *redisClient) ManySave(ctx context.Context) bool {
	for i := 1; i <= r.config.Count; i++ {
		key := fmt.Sprintf("%d", i)
		value := fmt.Sprintf("random text %d", i)
		err := r.rdb.Set(ctx, key, value, r.config.TTL).Err()
		if err != nil {
			return false
		}
	}

	return true
}

func (r *redisClient) GetRandomValue(ctx context.Context) (string, error) {
	randomKeyNum := rand.Intn(r.config.Count)
	key := fmt.Sprintf("key%d", randomKeyNum)

	return r.rdb.Get(ctx, key).Result()
}

func (r *redisClient) GetByID(ctx context.Context, key string) (string, error) {
	return rdClient.rdb.Get(ctx, key).Result()
}

// Postgres

type postgresClient struct {
	pool   *pgxpool.Pool
	config PostgresConfig
}

func NewPostgresClient(ctx context.Context, config PostgresConfig) (*postgresClient, error) {
	poolConfig, err := pgxpool.ParseConfig(config.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	poolConfig.MaxConns = int32(poolMaxConns)
	poolConfig.MinConns = int32(poolMinConns)
	poolConfig.MaxConnLifetime = poolMaxConnLifetime
	poolConfig.HealthCheckPeriod = poolHealthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	if err := ensureTableExists(ctx, pool, config.TableName); err != nil {
		return nil, err
	}

	return &postgresClient{pool: pool, config: config}, nil
}

func (p *postgresClient) ReinitializeData(ctx context.Context) error {
	if err := p.CleanDB(ctx); err != nil {
		return fmt.Errorf("failed to clean DB: %w", err)
	}
	fmt.Println("  cleaned DB")

	if err := p.ManySave(ctx); err != nil {
		return fmt.Errorf("failed to save many: %w", err)
	}
	fmt.Println("  saved new test dataset")

	return nil
}

func ensureTableExists(ctx context.Context, pool *pgxpool.Pool, tableName string) error {
	sql := fmt.Sprintf(`
        CREATE TABLE IF NOT EXISTS %s (
            id SERIAL PRIMARY KEY,
            value TEXT NOT NULL
        );
        
        CREATE INDEX IF NOT EXISTS idx_%s_id ON %s(id);
    `, tableName, tableName, tableName)

	_, err := pool.Exec(ctx, sql)
	if err != nil {
		return fmt.Errorf("failed to create table: %w", err)
	}
	return nil
}

func (p *postgresClient) ManySave(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, p.config.DSN)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s", p.config.TableName))
	if err != nil {
		return fmt.Errorf("failed to truncate table: %w", err)
	}

	rows := make([][]interface{}, p.config.Count)
	for i := 0; i < p.config.Count; i++ {
		rows[i] = []interface{}{fmt.Sprintf("random text %d", i+1)}
	}

	_, err = tx.CopyFrom(ctx,
		pgx.Identifier{p.config.TableName},
		[]string{"value"},
		pgx.CopyFromRows(rows),
	)
	if err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (p *postgresClient) GetRandomValue(ctx context.Context) (value string, err error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	randomID := rand.Intn(p.config.Count) + 1
	err = conn.QueryRow(ctx,
		fmt.Sprintf("SELECT value FROM %s WHERE id = $1", p.config.TableName),
		randomID,
	).Scan(&value)
	return value, err
}

func (p *postgresClient) GetByID(ctx context.Context, id int) (value string, err error) {
	conn, err := p.pool.Acquire(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to acquire connection: %w", err)
	}
	defer conn.Release()

	sql := fmt.Sprintf("SELECT value FROM %s WHERE id = $1", p.config.TableName)
	err = conn.QueryRow(ctx, sql, id).Scan(&value)
	return value, nil
}

func (p *postgresClient) Close() {
	p.pool.Close()
}

func (p *postgresClient) CleanDB(ctx context.Context) error {
	_, err := p.pool.Exec(ctx, fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY", p.config.TableName))
	return err
}
