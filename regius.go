package regius

import (
	"fmt"
	"log"
	"net/http"
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

	"gitlab.com/hbarral/regius/cache"
	"gitlab.com/hbarral/regius/mailer"
	"gitlab.com/hbarral/regius/render"
	"gitlab.com/hbarral/regius/session"
)

const version = "1.0.0"

var (
	myRedisCache  *cache.RedisCache
	myBadgerCache *cache.BadgerCache
	redisPool     *redis.Pool
	badgerConn    *badger.DB
)

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
	Scheduler     *cron.Cron
	Mail          mailer.Mail
	Server        Server
}

type Server struct {
	ServerName string
	Port       string
	Secure     bool
	URL        string
}

type config struct {
	port        string
	renderer    string
	cookie      cookieConfig
	sessionType string
	database    databaseConfig
	redis       redisConfig
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
		},
	}

	err := r.Init(pathConfig)
	if err != nil {
		return err
	}

	err = r.checkDotEnv(rootPath)
	if err != nil {
		return nil
	}

	err = godotenv.Load(rootPath + "/.env")
	if err != nil {
		return err
	}

	infoLog, errorLog := r.startLoggers()

	if os.Getenv("DATABASE_TYPE") != "" {
		db, err := r.OpenDB(os.Getenv("DATABASE_TYPE"), r.BuildDSN())
		if err != nil {
			errorLog.Println(err)
			os.Exit(1)
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
	r.Routes = r.routes().(*chi.Mux)
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
	}

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

func (r *Regius) ListenAndServe() {
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", os.Getenv("PORT")),
		ErrorLog:     r.ErrorLog,
		Handler:      r.Routes,
		IdleTimeout:  30 * time.Second,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 600 * time.Second,
	}

	if r.DB.Pool != nil {
		defer r.DB.Pool.Close()
	}

	if redisPool != nil {
		defer redisPool.Close()
	}

	if badgerConn != nil {
		defer badgerConn.Close()
	}

	r.InfoLog.Printf("Listening on port %s", os.Getenv("PORT"))

	err := srv.ListenAndServe()
	r.ErrorLog.Fatal(err)
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
