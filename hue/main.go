package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/jason0x43/go-hue"
)

func main() {
	var username string
	var newSession bool

	flag.StringVar(&username, "user", "", "hub username")
	flag.BoolVar(&newSession, "new", false, "create new user?")

	flag.Parse()

	var hubs []hue.Hub
	var err error
	if hubs, err = hue.GetHubs(); err != nil {
		log.Fatal("error: %s", err)

	}
	fmt.Printf("Hubs\n")
	fmt.Printf("----\n")
	for _, h := range hubs {
		fmt.Printf("%s\n", h)
	}
	fmt.Printf("\n")

	var session hue.Session
	if newSession {
		log.Printf("Press the Connect button on your hub, then press enter to continue...")
		bio := bufio.NewReader(os.Stdin)
		bio.ReadLine()
		if session, err = hue.NewSession(hubs[0].IpAddress, username); err != nil {
			log.Fatal("error: %s", err)
		}
	} else {
		session = hue.OpenSession(hubs[0].IpAddress, username)
	}

	lights, _ := session.Lights()
	fmt.Printf("Lights\n")
	fmt.Printf("------\n")
	for _, l := range lights {
		fmt.Printf("%s\n", l)
	}
	fmt.Printf("\n")

	scenes, _ := session.Scenes()
	fmt.Printf("Scenes\n")
	fmt.Printf("------\n")
	for _, s := range scenes {
		fmt.Printf("%s\n", s)
	}
	fmt.Printf("\n")
}
