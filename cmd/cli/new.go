package main

import (
	"log"
	"strings"
)

func doNew(appName string) {
	appName = strings.ToLower(appName)

	if strings.Contains(appName, "/") {
		exploded := strings.SplitAfter(appName, "/")
		appName = exploded[(len(exploded) - 1)]
	}

	log.Println("App name is:", appName)
}
