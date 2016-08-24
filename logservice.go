package main

import (
	"io"
	"log"
	"os"
)

var emitterPath = "/var/run/sd/emitter"

func scanFile(output io.Writer, path, buildID string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}

	if _, err := io.Copy(output, file); err != nil {
		return err
	}

	return nil
}

func main() {
	if len(os.Args) != 1 {
		log.Fatal("No buildID specified.")
		os.Exit(1)
	}

	buildID := os.Args[0]

	if err := scanFile(os.Stdout, emitterPath, buildID); err != nil {
		log.Fatal(err)
		os.Exit(1)
	}

}
