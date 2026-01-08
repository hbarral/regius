package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
)

var appURL string

func doNew(appName string) {
	appName = strings.ToLower(appName)
	appURL = appName

	if strings.Contains(appName, "/") {
		exploded := strings.SplitAfter(appName, "/")
		appName = exploded[(len(exploded) - 1)]
	}

	log.Println("App name is:", appName)

	color.Green("\tCloning repository...")
	_, err := git.PlainClone("./"+appName, false, &git.CloneOptions{
		URL:      "https://github.com/hbarral/regius-app.git",
		Progress: os.Stdout,
		Depth:    1,
	})
	if err != nil {
		exitGracefully(err)
	}

	err = os.RemoveAll(fmt.Sprintf("./%s/.git", appName))
	if err != nil {
		exitGracefully(err)
	}

	color.Yellow("\tCreating .env file...")
	data, err := templateFS.ReadFile("templates/env")
	if err != nil {
		exitGracefully(err)
	}

	env := string(data)
	env = strings.ReplaceAll(env, "${APP_NAME}", appName)
	env = strings.ReplaceAll(env, "${KEY}", reg.RandomString(32))

	err = copyDataToFile([]byte(env), fmt.Sprintf("./%s/.env", appName))
	if err != nil {
		exitGracefully(err)
	}

	if runtime.GOOS == "linux" {
		color.Yellow("\tCreating Makefile for linux...")

		data, err := templateFS.ReadFile("templates/Makefile.linux")
		if err != nil {
			exitGracefully(err)
		}

		env := string(data)
		env = strings.ReplaceAll(env, "${NAME}", appName)
		env = strings.ReplaceAll(env, "${BINARY_APP_NAME}", appName)

		err = copyDataToFile([]byte(env), fmt.Sprintf("./%s/Makefile", appName))
		if err != nil {
			exitGracefully(err)
		}
	}

	if runtime.GOOS == "darwin" {
		color.Yellow("\tCreating Makefile for MacOS...")

		data, err := templateFS.ReadFile("templates/Makefile.mac")
		if err != nil {
			exitGracefully(err)
		}

		env := string(data)
		env = strings.ReplaceAll(env, "${NAME}", appName)
		env = strings.ReplaceAll(env, "${BINARY_APP_NAME}", appName)

		err = copyDataToFile([]byte(env), fmt.Sprintf("./%s/Makefile", appName))
		if err != nil {
			exitGracefully(err)
		}
	}

	if runtime.GOOS == "windows" {
		color.Yellow("\tCreating Makefile for Windows...")

		data, err := templateFS.ReadFile("templates/Makefile.windows")
		if err != nil {
			exitGracefully(err)
		}

		env := string(data)
		env = strings.ReplaceAll(env, "${NAME}", appName)
		env = strings.ReplaceAll(env, "${BINARY_APP_NAME}", appName+".exe")

		err = copyDataToFile([]byte(env), fmt.Sprintf("./%s/Makefile", appName))
		if err != nil {
			exitGracefully(err)
		}
	}

	color.Yellow("\tCreating go.mod file...")
	_ = os.Remove("./" + appName + "/go.mod")

	data, err = templateFS.ReadFile("templates/go_mod")
	if err != nil {
		exitGracefully(err)
	}

	mod := string(data)
	mod = strings.ReplaceAll(mod, "${APP_NAME}", appName)

	err = copyDataToFile([]byte(mod), fmt.Sprintf("./%s/go.mod", appName))
	if err != nil {
		exitGracefully(err)
	}

	color.Yellow("\tUpdating source files...")
	os.Chdir("./" + appName)
	updateSource()

	color.Yellow("\tRunning go mod tidy...")

	cmd := exec.Command("go", "get", "github.com/hbarral/regius")
	err = cmd.Start()
	if err != nil {
		exitGracefully(err)
	}

	cmd = exec.Command("go", "mod", "tidy")
	err = cmd.Start()
	if err != nil {
		exitGracefully(err)
	}

	color.Green("\tDone building " + appURL)
	color.Green("\tGo build something real!")
}
