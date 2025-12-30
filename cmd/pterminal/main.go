package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/ankouros/pterminal/internal/app"
	"github.com/ankouros/pterminal/internal/buildinfo"
)

var (
	showVersion = flag.Bool("version", false, "print version and exit")
)

func main() {
	flag.Parse()
	if *showVersion {
		fmt.Println(buildinfo.String())
		os.Exit(0)
	}

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
