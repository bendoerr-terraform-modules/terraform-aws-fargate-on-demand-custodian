package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"

	//nolint:depguard // TODO: Replace with log/slog
	"log"
	"os/exec"
	"strings"
	"time"
)

//nolint:gochecknoglobals // Configuration variables
var (
	port             string
	timeout          int64
	eventsEnabled    bool
	eventsTopic      string
	eventActive      string
	eventInactive    string
	eventEmitTimeout int64
)

//nolint:gochecknoinits // Used for configuration initialization
func init() {
	flag.StringVar(&port, "port", "443", "The TCP port to watch")
	flag.Int64Var(&timeout, "timeout", 300, "The timeout when watching")
	flag.BoolVar(&eventsEnabled, "events", true, "Should events be emitted or not")
	flag.StringVar(&eventsTopic, "events-topic", "", "ARN of the SNS Topic")
	flag.StringVar(&eventActive, "event-type-active", "active", "The event type to emit when 'active'")
	flag.StringVar(&eventInactive, "event-type-inactive", "inactive", "The event type to emit when 'inactive'")
	flag.Int64Var(&eventEmitTimeout, "event-emit-timeout", 10, "Timeout in seconds when emitting an event")
}

func monitorUnconn() (<-chan interface{}, error) {
	notify := make(chan interface{})

	// ss needs a fake tty so wrap it in script
	//nolint:gosec // G204: Command runs with trusted input from configuration
	cmd := exec.Command("script", "--quiet", "--flush", "--return", "--command",
		fmt.Sprintf("ss --no-header --numeric --oneline --events sport = %s", port))

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
			log.Printf("DEBUG monitorUnconn: text: %s\n", scanner.Text())
			notify <- struct{}{}
		}

		err = cmd.Wait()
		if err != nil {
			log.Fatalf("FATAL monitorUnconn: Wait(): %s\n", err)
		}

		log.Println("WARN monitorUnconn: done")
	}()

	return notify, nil
}

func countEstab() (int, error) {
	// ss needs a fake tty so wrap it in script
	//nolint:gosec // G204: Command runs with trusted input from configuration
	cmd := exec.Command("script", "--quiet", "--flush", "--return", "--command",
		fmt.Sprintf("ss --no-header --numeric --oneline sport = %s", port))

	// Run the command
	output, err := cmd.CombinedOutput()
	if err != nil {
		return -1, fmt.Errorf("countEstab: CombinedOutput(): %w", err)
	}

	count := strings.Count(string(output), "\n")
	if count > 0 {
		for _, l := range strings.Split(string(output), "\n") {
			log.Printf("DEBUG countEstab: text: %s\n", l)
		}
	}

	return count, nil
}

func emitEvent(eventType string) {
	if !eventsEnabled {
		return
	}

	ctx, cancel := context.WithTimeout(context.TODO(),
		time.Duration(eventEmitTimeout)*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, "./event-emitter",
		"--type", eventType, "--topic", eventsTopic)
	command.Stderr = command.Stdout

	stdout, err := command.StdoutPipe()
	if err != nil {
		log.Printf("ERROR emitEvent: StdoutPipe(): %s\n", err)

		return
	}

	err = command.Start()
	if err != nil {
		log.Printf("ERROR emitEvent: Start(): %s\n", err)

		return
	}

	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		// Propagate the 'event-emitter' output without decoration
		//nolint:forbidigo // This is not logging
		fmt.Println(scanner.Text())
	}

	err = command.Wait()
	if err != nil {
		log.Printf("ERROR emitEvent: Wait(): %s\n", err)

		return
	}
}

func monitor() {
	// Start monitoring disconnects
	unconn, err := monitorUnconn()
	if err != nil {
		log.Fatalf("ERROR main: monitorUnconn(): %s\n", err)
	}

	// Start the main loop
	active := false
	waiting := true

	log.Println("INFO main: watching")

	for waiting {
		timer := time.NewTimer(time.Duration(timeout) * time.Second)

		select {
		case <-timer.C:
			log.Printf("INFO main: timeout, active=%t\n", active)

			//nolint:govet // Intentional shadowing of err variable
			count, err := countEstab()
			if err != nil {
				log.Fatalf("ERROR main: countEstab(): %s\n", err)
			}

			log.Printf("INFO main: established connections=%d\n", count)

			if count < 1 {
				if active {
					emitEvent("inactive")
				} else {
					waiting = false
				}

				active = false
			}

		case <-unconn:
			log.Printf("INFO main: unconn, active=%t\n", active)

			if !active {
				emitEvent("active")
			}

			active = true
		}

		log.Printf("INFO main: active=%t, waiting=%t\n", active, waiting)

		// Stop the timer
		timer.Stop()

		// Make sure the channel was read from, so it can be gc'd
		select {
		case <-timer.C:
		default:
		}
	}

	log.Println("WARN main: done watching")
}

func main() {
	log.SetPrefix("[watcher-tcp] ")
	log.SetFlags(0)

	flag.Parse()

	monitor()
}
