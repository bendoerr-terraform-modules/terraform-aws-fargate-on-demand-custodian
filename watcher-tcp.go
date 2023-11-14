package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func monitorUnconn(port string) (<-chan interface{}, error) {
	notify := make(chan interface{})

	// ss needs a fake tty so wrap it in script
	cmd := exec.Command("script", "--quiet", "--flush", "--command",
		fmt.Sprintf("/usr/sbin/ss --no-header --numeric --oneline --events sport = %s", port))

	// Redirect stderr to stdout
	cmd.Stderr = cmd.Stdout

	outReader, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("monitorUnconn: StdoutPipe(): %w", err)
	}

	// Start the command
	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("monitorUnconn: Start(): %w", err)
	}

	// Build the scanner
	scanner := bufio.NewScanner(outReader)
	scanner.Split(bufio.ScanLines)

	// Go and watch the output
	go func() {
		for scanner.Scan() {
			notify <- struct{}{}
		}

		err = cmd.Wait()
		if err != nil {
			log.Fatal(fmt.Errorf("monitorUnconn: Wait(): %w", err))
		}

		log.Println("monitorUnconn: done")
	}()

	return notify, nil
}

func countEstab(port string) (int, error) {
	// ss needs a fake tty so wrap it in script
	cmd := exec.Command("script", "--quiet", "--flush", "--command",
		fmt.Sprintf("/usr/sbin/ss --no-header --numeric --oneline sport = %s", port))

	// Run the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return -1, fmt.Errorf("countEstab: CombinedOutput(): %w", err)
	}

	return strings.Count(string(output), "\n"), nil
}

func main() {
	if len(os.Args) < 2 {
		log.Fatal("missing tcp port")
	}

	if len(os.Args) < 3 {
		log.Fatal("missing timeout in seconds")
	}

	port := os.Args[1]
	timeoutSeconds, err := strconv.ParseInt(os.Args[2], 10, 64)
	if err != nil {
		log.Fatal(err)
	}

	unconn, err := monitorUnconn(port)
	if err != nil {
		log.Fatal(err)
	}

	log.Println("watching")

	waiting := true
	for waiting {
		timer := time.NewTimer(time.Duration(timeoutSeconds) * time.Second)

		select {
		case <-timer.C:
			log.Println("event: timeout")
			estab, err := countEstab(port)
			if err != nil {
				log.Fatal(err)
			}

			log.Println("found established connections: " + strconv.Itoa(estab))
			waiting = estab > 0
		case <-unconn:
			log.Println("event: unconn")
		}

		// Stop the timer
		timer.Stop()

		// Make sure the channel was read from, so it can be gc'd
		select {
		case <-timer.C:
		default:
		}
	}

	log.Println("done watching")
}
