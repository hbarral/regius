package regius

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

const version = "1.0.0"

type Regius struct {
	AppName  string
	Debug    bool
	Version  string
	ErrorLog *log.Logger
	InfoLog  *log.Logger
	RootPath string
}

func (c *Regius) New(rootPath string) error {
	pathConfig := initPath{
		rootPath:    rootPath,
		folderNames: []string{"handlers", "migratios", "views", "data", "public", "tmp", "logs", "middleware"},
	}

	err := c.Init(pathConfig)

	if err != nil {
		return err
	}

	err = c.checkDotEnv(rootPath)
	if err != nil {
		return nil
	}

	err = godotenv.Load(rootPath + "/.env")
	if err != nil {
		return err
	}

	infoLog, errorLog := c.startLoggers()
	c.InfoLog = infoLog
	c.ErrorLog = errorLog
	c.Debug, _ = strconv.ParseBool(os.Getenv("DEBUG"))
	c.Version = version

	return nil
}

func (c *Regius) Init(p initPath) error {
	root := p.rootPath
	for _, path := range p.folderNames {
		err := c.CreateDirIfNotExist(root + "/" + path)

		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Regius) checkDotEnv(path string) error {
	err := c.CreateFileIfNotExists(fmt.Sprintf("%s/.env", path))

	if err != nil {
		return err
	}

	return nil
}

func (c *Regius) startLoggers() (*log.Logger, *log.Logger) {
	var infoLog *log.Logger
	var errorLog *log.Logger

	infoLog = log.New(os.Stdout, "INFO\t", log.Ldate|log.Ltime)
	errorLog = log.New(os.Stdout, "ERROR\t", log.Ldate|log.Ltime|log.Lshortfile)

	return infoLog, errorLog
}
