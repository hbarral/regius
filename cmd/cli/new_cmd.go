package main

import (
	"crypto/rand"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/fatih/color"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
)

var appURL string

func init() {
	rootCmd.AddCommand(newCmd)
}

var newCmd = &cobra.Command{
	Use:   "new [application-name]",
	Short: "Create a new Regius application",
	Long: `Create a new Regius application by cloning the starter template
and setting up the initial configuration.`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		doNew(args[0])
	},
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, n)
	rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = letters[b%byte(len(letters))]
	}
	return string(bytes)
}

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
	env = strings.ReplaceAll(env, "${KEY}", randomString(32))

	err = copyDataToFile([]byte(env), fmt.Sprintf("./%s/.env", appName))
	if err != nil {
		exitGracefully(err)
	}

	if runtime.GOOS == "windows" {
		source, err := templateFS.ReadFile("templates/windows/Makefile")
		if err != nil {
			exitGracefully(err)
		}
		err = copyDataToFile(source, fmt.Sprintf("./%s/Makefile.windows", appName))
		if err != nil {
			exitGracefully(err)
		}
	} else {
		source, err := templateFS.ReadFile("templates/unix/Makefile")
		if err != nil {
			exitGracefully(err)
		}
		err = copyDataToFile(source, fmt.Sprintf("./%s/Makefile", appName))
		if err != nil {
			exitGracefully(err)
		}
	}

	color.Yellow("\tCreating go.mod file...")
	_ = os.Remove(fmt.Sprintf("./%s/go.mod", appName))

	data, err = templateFS.ReadFile("templates/go.mod")
	if err != nil {
		exitGracefully(err)
	}

	mod := string(data)
	mod = strings.ReplaceAll(mod, "${APP_NAME}", appURL)

	err = copyDataToFile([]byte(mod), fmt.Sprintf("./%s/go.mod", appName))
	if err != nil {
		exitGracefully(err)
	}

	color.Yellow("\tUpdating source files...")
	os.Chdir("./" + appName)
	updateSource()
	os.Chdir("..")

	color.Yellow("\tRunning go mod tidy...")
	cmd := exec.Command("go", "mod", "tidy")
	err = cmd.Start()
	if err != nil {
		exitGracefully(err)
	}

	color.Green("Done!")
	color.Yellow("Go into the " + appName + " directory and check the .env file.")
}
