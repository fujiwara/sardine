package main

import (
	"flag"
	"log"
	"os"

	"github.com/fujiwara/sardine"
)

func main() {
	var config string
	flag.StringVar(&config, "config", "", "config file path")
	flag.Parse()
	err := sardine.Run(config)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
}
