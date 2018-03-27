package main

import (
	"flag"
	"log"
	"os"
	"strings"

	"github.com/fujiwara/sardine"
)

func main() {
	var config string

	// Set a default format. XXX mackerel-client modifies global flags.
	// https://github.com/mackerelio/mackerel-client-go/issues/57
	log.SetFlags(log.LstdFlags)

	flag.StringVar(&config, "config", "", "config file path")
	flag.BoolVar(&sardine.Debug, "debug", false, "enable debug logging")
	flag.VisitAll(envToFlag)
	flag.Parse()
	err := sardine.Run(config)
	if err != nil {
		log.Println(err)
		os.Exit(1)
	}
}

func envToFlag(f *flag.Flag) {
	names := []string{
		strings.ToUpper(strings.Replace(f.Name, "-", "_", -1)),
		strings.ToLower(strings.Replace(f.Name, "-", "_", -1)),
	}
	for _, name := range names {
		if s := os.Getenv(name); s != "" {
			f.Value.Set(s)
			break
		}
	}
}
