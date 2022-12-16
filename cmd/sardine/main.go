package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/fujiwara/sardine"
	"golang.org/x/sys/unix"
)

var trapSignals = []os.Signal{os.Interrupt, unix.SIGTERM}

func main() {
	var config string
	var sleep time.Duration
	var atOnce bool

	// Set a default format. XXX mackerel-client modifies global flags.
	// https://github.com/mackerelio/mackerel-client-go/issues/57
	log.SetFlags(log.LstdFlags)

	flag.StringVar(&config, "config", "", "config file path")
	flag.BoolVar(&sardine.Debug, "debug", false, "enable debug logging")
	flag.DurationVar(&sleep, "sleep", 0, "sleep duration at wake up")
	flag.BoolVar(&atOnce, "at-once", false, "run at once and exit")
	flag.VisitAll(envToFlag)
	flag.Parse()

	log.Println("starting sardine agent")
	if sleep > 0 {
		log.Printf("sleeping %s", sleep)
		time.Sleep(sleep)
	}

	ctx, stop := signal.NotifyContext(context.Background(), trapSignals...)
	defer stop()

	var err error
	if atOnce {
		log.Println("run at once")
		err = sardine.RunAtOnce(ctx, config)
	} else {
		log.Println("running daemon")
		err = sardine.Run(ctx, config)
	}
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
