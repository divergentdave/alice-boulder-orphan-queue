package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"

	"github.com/beeker1121/goque"
)

var pattern = regexp.MustCompile(`^(?:Writing orphan with ID (\d+)|Wrote orphan with ID (\d+)|Integrated orphan with ID (\d+)|Error .*)$`)

type orphanState struct {
	// Whether we've seen a message indicating an attempt was made to write
	// an item to the queue.
	written bool
	// Whether we've seen a message indicating an item should have been durably
	// written to the queue.
	flushed bool
	// Whether we've seen a message indicating an item has been read from the
	// queue in the normal orphan integration loop, or the item could still be
	// read from the queue during the next run of the program.
	wasRead bool
}

type orphanedCert struct {
	DER      []byte
	OCSPResp []byte
	RegID    int64
	Precert  bool
	IssuerID int64
}

func parseNumber(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Fatalf("Invalid ID number: %v\n", s)
	}
	return n
}

func main() {
	if len(os.Args) < 3 {
		log.Fatal("This program should be called with two arguments, the paths to the crashed state directory and the reconstructed stdout output.")
	}
	directory := os.Args[1]
	stdoutFilename := os.Args[2]

	file, err := os.Open(stdoutFilename)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	states := make(map[int64]orphanState)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		submatches := pattern.FindStringSubmatch(line)
		if submatches == nil {
			log.Fatalf("Invalid output line: %v\n", line)
		} else if len(submatches[1]) > 0 {
			regID := parseNumber((submatches[1]))
			state := states[regID]
			if state.written {
				log.Fatalf("ID collision on %v in orphan write attempts\n", regID)
			}
			state.written = true
			states[regID] = state
		} else if len(submatches[2]) > 0 {
			regID := parseNumber((submatches[2]))
			state := states[regID]
			if state.flushed {
				log.Fatalf("ID collision on %v in orphan write confirmations\n", regID)
			}
			state.flushed = true
			states[regID] = state
		} else if len(submatches[3]) > 0 {
			regID := parseNumber((submatches[3]))
			state := states[regID]
			if !state.written {
				log.Fatalf("ID %v was read before/without being written", regID)
			}
			state.wasRead = true
			states[regID] = state
		} else {
			log.Fatalln(line)
		}
	}

	queueDirectory := fmt.Sprintf("%s/orphanqueue", directory)
	orphanQueue, err := goque.OpenQueue(queueDirectory)
	if err != nil {
		log.Fatalf("Error opening queue: %v\n", err)
	}
	length := orphanQueue.Length()
	for offset := uint64(0); offset < length; offset++ {
		item, err := orphanQueue.PeekByOffset(offset)
		if err != nil {
			log.Fatalf("Error reading queue: %v\n", err)
		}
		var orphan orphanedCert
		if err = item.ToObject(&orphan); err != nil {
			log.Fatalf("Error unmarshaling orphan: %v\n", err)
		}

		state := states[orphan.RegID]
		if !state.written {
			log.Fatalf("ID %v was read without being written", orphan.RegID)
		}
		state.wasRead = true
		states[orphan.RegID] = state
	}

	for regID, state := range states {
		if state.flushed && !state.wasRead {
			log.Fatalf("ID %v was not durable", regID)
		}
	}
}
