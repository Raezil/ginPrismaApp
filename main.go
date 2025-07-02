package main

import (
	"db"
	"router"
)

func main() {

	database := db.NewClient()
	if err := database.Connect(); err != nil {
		panic(err)
	}
	defer func() {
		if err := database.Disconnect(); err != nil {
			panic(err)
		}
	}()

	// Public group

	r := router.SetupRouter(database)
	r.Run() // default :8080
}
