package main

import (
	"craeft/cmd"
	"os"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetOutput(os.Stdout)
}

func main() {
	cmd.Execute()
}
