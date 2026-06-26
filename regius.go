package regius

import (
	"fmt"
	"log"
	"net"
	"net/rpc"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/CloudyKit/jet/v6"
	"github.com/alexedwards/scs/v2"
	"github.com/dgraph-io/badger/v3"
	"github.com/go-chi/chi/v5"
	"github.com/gomodule/redigo/redis"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"

	"github.com/hbarral/regius/cache"
	"github.com/hbarral/regius/filesystems/miniofilesystem"
	"github.com/hbarral/regius/filesystems/s3filesystem"
	"github.com/hbarral/regius/filesystems/sftpfilesystem"
	"github.com/hbarral/regius/filesystems/webdavfilesystem"
	"github.com/hbarral/regius/hash"
	"github.com/hbarral/regius/mailer"
	"github.com/hbarral/regius/render"
	"github.com/hbarral/regius/session"
)

const version = "1.3.0"

var (
	myRedisCache  *cache.RedisCache
	myBadgerCache *cache.BadgerCache
	redisPool     *redis.Pool
	badgerConn    *badger.DB
)

var maintenanceMode bool

type Regius struct {
	AppName       string
	Debug         bool
	Version       string
	ErrorLog      *log.Logger
	InfoLog       *log.Logger
	RootPath      string
	Routes        *chi.Mux
	Render        *render.Render
	JetViews      *jet.Set
	config        config
	Session       *scs.SessionManager
	DB            Database
	EncryptionKey string
	Cache         cache.Cache
	Hash          hash.Hasher
	Scheduler     *cron.Cron
	Mail          mailer.Mail
	Server        Server
	FileSystems   map[string]interface{}
	S3            s3filesystem.S3
	SFTP          sftpfilesystem.SFTP
	WebDAV        webdavfilesystem.WebDAV
	Minio         miniofilesystem.Minio
}

type Server struct {
	ServerName string
	Port       string
	Secure     bool
	URL        string
}

type config struct {
	port             string
	renderer         string
	cookie           cookieConfig
	sessionType      string
	database         databaseConfig
	redis            redisConfig
	uploads          uploadConfig
	cors             CORSConfig
	securityHeaders  SecurityHeadersConfig
	apiKeyAuth       APIKeyAuthConfig
	requestID        RequestIDConfig
	requestSanitizer RequestSanitizerConfig
	ipFilter         IPFilterConfig
	hash             hashConfig
}

type uploadConfig struct {
	allowedTypes []string
}

func (r *Regius) New(rootPath string) error {
	pathConfig := initPath{
		rootPath: rootPath,
		folderNames: []string{
			"handlers",
			"migrations",
			"views",
			"mail",
			"data",
			"public",
			"tmp",
			"logs",
			"middleware",
			"screenshots",
		},
	}

	err := r.Init(pathConfig)
	if err != nil {
		return err
	}

	err = r.checkDotEnv(rootPath)
	if err != nil {
		return err
	}

	err = godotenv.Load(rootPath + "/.env")
	if err != nil {
		return err
	}

	infoLog, errorLog := r.startLoggers()

	if os.Getenv("DATABASE_TYPE") != "" {
		db, err := r.OpenDB(os.Getenv("DATABASE_TYPE"), r.BuildDSN())
		if err != nil {
			return fmt.Errorf("failed to connect to database: %w", err)
		}

		r.DB = Database{
			DataType: os.Getenv("DATABASE_TYPE"),
			Pool:     db,
		}
	}

	if os.Getenv("CACHE") == "redis" || os.Getenv("SESSION_TYPE") == "redis" {
		myRedisCache = r.createClientRedisCache()
		r.Cache = myRedisCache
		redisPool = myRedisCache.Conn
	}

	scheduler := cron.New()
	r.Scheduler = scheduler

	if os.Getenv("CACHE") == "badger" {
		myBadgerCache = r.createClientBadgerCache()
		r.Cache = myBadgerCache
		badgerConn = myBadgerCache.Conn

		_, err = r.Scheduler.AddFunc("@daily", func() {
			_ = myBadgerCache.Conn.RunValueLogGC(0.7)
		})
		if err != nil {
			return err
		}
	}

	r.InfoLog = infoLog
	r.ErrorLog = errorLog
	r.Debug, _ = strconv.ParseBool(os.Getenv("DEBUG"))
	r.Version = version
	r.RootPath = rootPath
	r.Mail = r.createMailer()

	exploded := strings.Split(os.Getenv("ALLOWED_FILETYPES"), ",")
	var allowedTypes []string
	for _, at := range exploded {
		allowedTypes = append(allowedTypes, strings.TrimSpace(at))
	}

	corsEnabled := true
	if os.Getenv("CORS_ENABLED") != "" {
		corsEnabled, _ = strconv.ParseBool(os.Getenv("CORS_ENABLED"))
	}
	corsMaxAge, _ := strconv.Atoi(os.Getenv("CORS_MAX_AGE"))
	if corsMaxAge == 0 {
		corsMaxAge = 300
	}
	corsCredentials := true
	if os.Getenv("CORS_ALLOW_CREDENTIALS") != "" {
		corsCredentials, _ = strconv.ParseBool(os.Getenv("CORS_ALLOW_CREDENTIALS"))
	}
	corsDebug, _ := strconv.ParseBool(os.Getenv("CORS_DEBUG"))
	corsPassthrough, _ := strconv.ParseBool(os.Getenv("CORS_OPTIONS_PASSTHROUGH"))

	securityHeadersEnabled := false
	if os.Getenv("SECURITY_HEADERS_ENABLED") != "" {
		securityHeadersEnabled, _ = strconv.ParseBool(os.Getenv("SECURITY_HEADERS_ENABLED"))
	}
	hstsMaxAge, _ := strconv.Atoi(os.Getenv("HSTS_MAX_AGE"))
	hstsIncludeSubDomains := true
	if os.Getenv("HSTS_INCLUDE_SUBDOMAINS") != "" {
		hstsIncludeSubDomains, _ = strconv.ParseBool(os.Getenv("HSTS_INCLUDE_SUBDOMAINS"))
	}
	hstsPreload, _ := strconv.ParseBool(os.Getenv("HSTS_PRELOAD"))

	apiKeyAuthEnabled := false
	if os.Getenv("API_KEY_AUTH_ENABLED") != "" {
		apiKeyAuthEnabled, _ = strconv.ParseBool(os.Getenv("API_KEY_AUTH_ENABLED"))
	}

	requestIDEnabled := true
	if os.Getenv("REQUEST_ID_ENABLED") != "" {
		requestIDEnabled, _ = strconv.ParseBool(os.Getenv("REQUEST_ID_ENABLED"))
	}

	requestSanitizerEnabled := false
	if os.Getenv("REQUEST_SANITIZATION_ENABLED") != "" {
		requestSanitizerEnabled, _ = strconv.ParseBool(os.Getenv("REQUEST_SANITIZATION_ENABLED"))
	}
	requestSanitizerQuery := true
	if v := os.Getenv("REQUEST_SANITIZATION_QUERY"); v != "" {
		requestSanitizerQuery, _ = strconv.ParseBool(v)
	}
	requestSanitizerForm := true
	if v := os.Getenv("REQUEST_SANITIZATION_FORM"); v != "" {
		requestSanitizerForm, _ = strconv.ParseBool(v)
	}

	ipFilterEnabled := false
	if os.Getenv("IP_FILTER_ENABLED") != "" {
		ipFilterEnabled, _ = strconv.ParseBool(os.Getenv("IP_FILTER_ENABLED"))
	}
	ipFilterTrustProxy, _ := strconv.ParseBool(os.Getenv("IP_FILTER_TRUST_PROXY"))
	ipFilterStatusCode, _ := strconv.Atoi(os.Getenv("IP_FILTER_STATUS_CODE"))

	r.config = config{
		port:     os.Getenv("PORT"),
		renderer: os.Getenv("RENDERER"),
		cookie: cookieConfig{
			name:     os.Getenv("COOKIE_NAME"),
			lifetime: os.Getenv("COOKIE_LIFETIME"),
			persist:  os.Getenv("COOKIE_PERSISTS"),
			secure:   os.Getenv("COOKIE_SECURE"),
			domain:   os.Getenv("COOKIE_DOMAIN"),
		},
		sessionType: os.Getenv("SESSION_TYPE"),
		database: databaseConfig{
			database: os.Getenv("DATABASE_TYPE"),
			dsn:      r.BuildDSN(),
		},
		redis: redisConfig{
			host:     os.Getenv("REDIS_HOST"),
			password: os.Getenv("REDIS_PASSWORD"),
			prefix:   os.Getenv("REDIS_PREFIX"),
		},
		uploads: uploadConfig{
			allowedTypes: allowedTypes,
		},
		cors: CORSConfig{
			Enabled:            corsEnabled,
			AllowedOrigins:     parseStringSliceEnv("CORS_ALLOWED_ORIGINS", "*"),
			AllowedMethods:     parseStringSliceEnv("CORS_ALLOWED_METHODS", "GET,POST,PUT,DELETE,OPTIONS,PATCH,HEAD"),
			AllowedHeaders:     parseStringSliceEnv("CORS_ALLOWED_HEADERS", "Accept,Authorization,Content-Type,X-CSRF-Token"),
			ExposedHeaders:     parseStringSliceEnv("CORS_EXPOSED_HEADERS", ""),
			MaxAge:             corsMaxAge,
			AllowCredentials:   corsCredentials,
			OptionsPassthrough: corsPassthrough,
			Debug:              corsDebug,
		},
		securityHeaders: SecurityHeadersConfig{
			Enabled:                       securityHeadersEnabled,
			ContentSecurityPolicy:         os.Getenv("CONTENT_SECURITY_POLICY"),
			HSTSMaxAge:                    hstsMaxAge,
			HSTSIncludeSubDomains:         hstsIncludeSubDomains,
			HSTSPreload:                   hstsPreload,
			ReferrerPolicy:                os.Getenv("REFERRER_POLICY"),
			XFrameOptions:                 os.Getenv("X_FRAME_OPTIONS"),
			XPermittedCrossDomainPolicies: os.Getenv("X_PERMITTED_CROSS_DOMAIN_POLICIES"),
			CrossOriginOpenerPolicy:       os.Getenv("CROSS_ORIGIN_OPENER_POLICY"),
			CrossOriginResourcePolicy:     os.Getenv("CROSS_ORIGIN_RESOURCE_POLICY"),
			XDNSPrefetchControl:           os.Getenv("X_DNS_PREFETCH_CONTROL"),
		},
		apiKeyAuth: APIKeyAuthConfig{
			Enabled:    apiKeyAuthEnabled,
			Keys:       parseStringSliceEnv("API_KEYS", ""),
			Header:     os.Getenv("API_KEY_HEADER"),
			Scheme:     os.Getenv("API_KEY_SCHEME"),
			AltHeader:  os.Getenv("API_KEY_ALT_HEADER"),
			QueryParam: os.Getenv("API_KEY_QUERY_PARAM"),
			Realm:      os.Getenv("API_KEY_REALM"),
		},
		requestID: RequestIDConfig{
			Enabled:        requestIDEnabled,
			Header:         os.Getenv("REQUEST_ID_HEADER"),
			ResponseHeader: os.Getenv("REQUEST_ID_RESPONSE_HEADER"),
			Format:         os.Getenv("REQUEST_ID_FORMAT"),
		},
		requestSanitizer: RequestSanitizerConfig{
			Enabled: requestSanitizerEnabled,
			Policy:  os.Getenv("REQUEST_SANITIZATION_POLICY"),
			Query:   BoolPtr(requestSanitizerQuery),
			Form:    BoolPtr(requestSanitizerForm),
			Headers: parseStringSliceEnv("REQUEST_SANITIZATION_HEADERS", "Referer,User-Agent"),
			Exempt:  os.Getenv("REQUEST_SANITIZATION_EXEMPT"),
		},
		ipFilter: IPFilterConfig{
			Enabled:    ipFilterEnabled,
			Allow:      parseStringSliceEnv("IP_FILTER_ALLOW", ""),
			Deny:       parseStringSliceEnv("IP_FILTER_DENY", ""),
			TrustProxy: ipFilterTrustProxy,
			StatusCode: ipFilterStatusCode,
			Message:    os.Getenv("IP_FILTER_MESSAGE"),
		},
		hash: r.createHashConfig(),
	}

	r.Routes = r.routes().(*chi.Mux)

	secure := true
	if strings.ToLower(os.Getenv("SECURE")) == "false" {
		secure = false
	}
	r.Server = Server{
		ServerName: os.Getenv("SERVER_NAME"),
		Port:       os.Getenv("PORT"),
		Secure:     secure,
		URL:        os.Getenv("APP_URL"),
	}

	sess := session.Session{
		CookieLifetime: r.config.cookie.lifetime,
		CookiePersist:  r.config.cookie.persist,
		CookieName:     r.config.cookie.name,
		CookieDomain:   r.config.cookie.domain,
		SessionType:    r.config.sessionType,
	}

	switch r.config.sessionType {
	case "redis":
		sess.RedisPool = myRedisCache.Conn
	case "mysql", "postgres", "postgresql", "mariadb":
		sess.DBPool = r.DB.Pool
	}

	r.Session = sess.InitSession()
	r.EncryptionKey = os.Getenv("KEY")
	r.Hash = r.createHasher()

	if r.Debug {
		views := jet.NewSet(
			jet.NewOSFileSystemLoader(fmt.Sprintf("%s/views/", rootPath)),
			jet.InDevelopmentMode(),
		)
		r.JetViews = views
	} else {
		views := jet.NewSet(
			jet.NewOSFileSystemLoader(fmt.Sprintf("%s/views/", rootPath)),
		)
		r.JetViews = views
	}

	r.createRenderer()
	r.FileSystems = r.createFileSystems()
	go r.Mail.ListenForMail()

	return nil
}

func (r *Regius) Init(p initPath) error {
	root := p.rootPath
	for _, path := range p.folderNames {
		err := r.CreateDirIfNotExist(root + "/" + path)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Regius) checkDotEnv(path string) error {
	err := r.CreateFileIfNotExists(fmt.Sprintf("%s/.env", path))
	if err != nil {
		return err
	}

	return nil
}

func (r *Regius) startLoggers() (*log.Logger, *log.Logger) {
	var infoLog *log.Logger
	var errorLog *log.Logger

	infoLog = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog = log.New(os.Stdout, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	return infoLog, errorLog
}

func (r *Regius) createRenderer() {
	myrenderer := render.Render{
		Renderer: r.config.renderer,
		RootPath: r.RootPath,
		Port:     r.config.port,
		JetViews: r.JetViews,
		Session:  r.Session,
	}

	r.Render = &myrenderer
}

func (r *Regius) createHashConfig() hashConfig {
	cost, _ := strconv.Atoi(os.Getenv("HASH_COST"))
	scryptN, _ := strconv.Atoi(os.Getenv("HASH_SCRYPT_N"))
	scryptR, _ := strconv.Atoi(os.Getenv("HASH_SCRYPT_R"))
	scryptP, _ := strconv.Atoi(os.Getenv("HASH_SCRYPT_P"))

	argon2Memory, _ := strconv.ParseUint(os.Getenv("HASH_ARGON2_MEMORY"), 10, 32)
	argon2Iterations, _ := strconv.ParseUint(os.Getenv("HASH_ARGON2_ITERATIONS"), 10, 32)
	argon2Parallelism, _ := strconv.ParseUint(os.Getenv("HASH_ARGON2_PARALLELISM"), 10, 8)

	return hashConfig{
		algorithm:         os.Getenv("HASH_ALGORITHM"),
		cost:              cost,
		scryptN:           scryptN,
		scryptR:           scryptR,
		scryptP:           scryptP,
		argon2Memory:      uint32(argon2Memory),
		argon2Iterations:  uint32(argon2Iterations),
		argon2Parallelism: uint8(argon2Parallelism),
	}
}

func (r *Regius) createHasher() hash.Hasher {
	return hash.New(hash.Config{
		Algorithm:         r.config.hash.algorithm,
		Cost:              r.config.hash.cost,
		ScryptN:           r.config.hash.scryptN,
		ScryptR:           r.config.hash.scryptR,
		ScryptP:           r.config.hash.scryptP,
		Argon2Memory:      r.config.hash.argon2Memory,
		Argon2Iterations:  r.config.hash.argon2Iterations,
		Argon2Parallelism: r.config.hash.argon2Parallelism,
	})
}

func (r *Regius) createMailer() mailer.Mail {
	port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
	m := mailer.Mail{
		Domain:      os.Getenv("MAIL_DOMAIN"),
		Templates:   r.RootPath + "/mail",
		Host:        os.Getenv("SMTP_HOST"),
		Port:        port,
		Username:    os.Getenv("SMTP_USERNAME"),
		Password:    os.Getenv("SMTP_PASSWORD"),
		Encryption:  os.Getenv("SMTP_ENCRYPTION"),
		FromName:    os.Getenv("FROM_NAME"),
		FromAddress: os.Getenv("FROM_ADDRESS"),
		Jobs:        make(chan mailer.Message, 20),
		Results:     make(chan mailer.Result, 20),
		API:         os.Getenv("MAILER_API"),
		APIKey:      os.Getenv("MAILER_KEY"),
		APIUrl:      os.Getenv("MAILER_URL"),
	}

	return m
}

func (r *Regius) createClientRedisCache() *cache.RedisCache {
	cacheClient := cache.RedisCache{
		Conn:   r.createRedisPool(),
		Prefix: r.config.redis.prefix,
	}

	return &cacheClient
}

func (r *Regius) createClientBadgerCache() *cache.BadgerCache {
	cacheClient := cache.BadgerCache{
		Conn: r.createBadgerConn(),
	}

	return &cacheClient
}

func (r *Regius) createRedisPool() *redis.Pool {
	return &redis.Pool{
		MaxIdle:     50,
		MaxActive:   10000,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			return redis.Dial("tcp",
				r.config.redis.host,
				redis.DialPassword(r.config.redis.password))
		},

		TestOnBorrow: func(conn redis.Conn, t time.Time) error {
			_, err := conn.Do("PING")
			return err
		},
	}
}

func (r *Regius) createBadgerConn() *badger.DB {
	db, err := badger.Open(badger.DefaultOptions(r.RootPath + "/tmp/badger"))
	if err != nil {
		return nil
	}

	return db
}

func parseStringSliceEnv(key, defaultValue string) []string {
	val := os.Getenv(key)
	if val == "" {
		val = defaultValue
	}
	if val == "" {
		return []string{}
	}
	parts := strings.Split(val, ",")
	var result []string
	for _, p := range parts {
		s := strings.TrimSpace(p)
		if s != "" {
			result = append(result, s)
		}
	}
	return result
}

func (r *Regius) BuildDSN() string {
	var dsn string

	switch os.Getenv("DATABASE_TYPE") {
	case "postgres", "postgresql":
		dsn = fmt.Sprintf(
			"host=%s port=%s user=%s dbname=%s sslmode=%s timezone=UTC connect_timeout=5",
			os.Getenv("DATABASE_HOST"),
			os.Getenv("DATABASE_PORT"),
			os.Getenv("DATABASE_USER"),
			os.Getenv("DATABASE_NAME"),
			os.Getenv("DATABASE_SSL_MODE"),
		)

		if os.Getenv("DATABASE_PASS") != "" {
			dsn = fmt.Sprintf("%s password=%s", dsn, os.Getenv("DATABASE_PASS"))
		}
	default:

	}

	return dsn
}

func (r *Regius) createFileSystems() map[string]interface{} {
	fileSystems := make(map[string]interface{})

	if os.Getenv("MINIO_SECRET") != "" {
		useSSL := false

		if strings.ToLower(os.Getenv("MINIO_USESSL")) == "true" {
			useSSL = true
		}

		minio := miniofilesystem.Minio{
			Endpoint: os.Getenv("MINIO_ENDPOINT"),
			Key:      os.Getenv("MINIO_KEY"),
			Secret:   os.Getenv("MINIO_SECRET"),
			UseSSL:   useSSL,
			Region:   os.Getenv("MINIO_REGION"),
			Bucket:   os.Getenv("MINIO_BUCKET"),
		}
		fileSystems["MINIO"] = minio
		r.Minio = minio
	}

	if os.Getenv("SFTP_HOST") != "" {
		sftp := sftpfilesystem.SFTP{
			Host: os.Getenv("SFTP_HOST"),
			Port: os.Getenv("SFTP_PORT"),
			User: os.Getenv("SFTP_USER"),
			Pass: os.Getenv("SFTP_PASS"),
		}

		fileSystems["SFTP"] = sftp
		r.SFTP = sftp
	}

	if os.Getenv("WEBDAV_HOST") != "" {
		useSSL := false
		if strings.ToLower(os.Getenv("WEBDAV_USESSL")) == "true" {
			useSSL = true
		}

		webdav := webdavfilesystem.WebDAV{
			Host:   os.Getenv("WEBDAV_HOST"),
			Port:   os.Getenv("WEBDAV_PORT"),
			User:   os.Getenv("WEBDAV_USER"),
			Pass:   os.Getenv("WEBDAV_PASS"),
			UseSSL: useSSL,
		}

		fileSystems["WebDAV"] = webdav
		r.WebDAV = webdav
	}

	if os.Getenv("S3_KEY") != "" {
		s3 := s3filesystem.S3{
			Key:      os.Getenv("S3_KEY"),
			Secret:   os.Getenv("S3_SECRET"),
			Region:   os.Getenv("S3_REGION"),
			Bucket:   os.Getenv("S3_BUCKET"),
			Endpoint: os.Getenv("S3_ENDPOINT"),
		}
		fileSystems["S3"] = s3
		r.S3 = s3
	}

	return fileSystems
}

type RPCServer struct {
	Host string
	Port string
}

func (r *RPCServer) MaintenanceMode(inMaintenanceMode bool, resp *string) error {
	if inMaintenanceMode {
		maintenanceMode = true
		*resp = "Server in maintenance mode"
	} else {
		maintenanceMode = false
		*resp = "Server live!"
	}
	return nil
}

func (r *Regius) listenRPC() {
	if os.Getenv("RPC_PORT") != "" {
		port := os.Getenv("RPC_PORT")
		r.InfoLog.Println("Starting RPC server on port " + port)
		err := rpc.Register(new(RPCServer))
		if err != nil {
			r.ErrorLog.Println(err)
			return
		}

		listen, err := net.Listen("tcp", "127.0.0.1:"+port)
		if err != nil {
			r.ErrorLog.Println(err)
			return
		}

		for {
			rpcConn, err := listen.Accept()
			if err != nil {
				r.ErrorLog.Println(err)
				continue
			}

			go rpc.ServeConn(rpcConn)
		}

	}
}
