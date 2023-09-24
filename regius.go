package regius

const version = "1.0.0"

type Regius struct {
	AppName string
	Debug   bool
	Version string
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
