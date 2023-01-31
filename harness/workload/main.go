package main

import (
	cryptorand "crypto/rand"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/beeker1121/goque"
)

const filename = "workload_dir/orphanqueue"

type Config struct {
	// Number of times to open and close the queue.
	restartCycleCount int
	// Number of times per restart to read from the queue.
	readLoopIterationCount int
	// Number of times to write to the queue, in parallel.
	writeGoroutinesCount int
	// Probability of not dequeueing an item after peeking it.
	integrateFailureChance float64
}

func parseConfig() Config {
	var restartCycleCount int
	var readLoopIterationCount int
	var writeGoroutinesCount int
	var integrateFailureChance float64
	flag.IntVar(&restartCycleCount, "restarts", 5, "number of times to open and close the queue")
	flag.IntVar(&readLoopIterationCount, "reads", 20, "number of times per restart to read from the queue")
	flag.IntVar(&writeGoroutinesCount, "writes", 5, "number of times to write to the queue, in parallel")
	flag.Float64Var(&integrateFailureChance, "probability", 0.25, "probability of not dequeueing an item after peeking it")
	flag.Parse()
	return Config{
		restartCycleCount,
		readLoopIterationCount,
		writeGoroutinesCount,
		integrateFailureChance,
	}
}

// writeTranscript formats and prints a message to stdout. It exits the program
// with an error if the underlying I/O option failed, including if it returned a
// short write error.
//
// The ALICE software will truncate the syscall transcript at every possible
// position, so a short write would result in the verifier getting a stdout
// file with a truncated message. For this workload/verifier pair, stdout
// messages are line-delimited, so truncated messages would be hard to work
// with. Thus, this method instead detects such conditions at workload run
// time. If such errors occur, the experimental setup or stdout message format
// may need to be changed.
func writeTranscript(format string, a ...any) {
	_, err := fmt.Printf(format, a...)
	if err != nil {
		log.Fatalf("Error writing to stdout: %s\n", err)
	}
}

type orphanedCert struct {
	DER      []byte
	OCSPResp []byte
	RegID    int64
	Precert  bool
	IssuerID int64
}

func newOrphanedCert() orphanedCert {
	der := make([]byte, 16)
	_, err := cryptorand.Read(der)
	if err != nil {
		log.Fatalf("urandom error: %s\n", err)
	}

	ocsp := make([]byte, 16)
	_, err = cryptorand.Read(ocsp)
	if err != nil {
		log.Fatalf("urandom error: %s\n", err)
	}

	registrationID := int64(rand.Intn(1000000000))

	issuerID := int64(rand.Intn(10))

	return orphanedCert{
		DER:      der,
		OCSPResp: ocsp,
		RegID:    registrationID,
		Precert:  true,
		IssuerID: issuerID,
	}
}

func main() {
	config := parseConfig()
	for i := 0; i < config.restartCycleCount; i++ {
		if i != 0 {
			writeTranscript("Restarting\n")
		}
		runOnce(config)
	}
}

func runOnce(config Config) {
	var orphanQueue *goque.Queue
	var err error
	orphanQueue, err = goque.OpenQueue(filename)
	writeTranscript("Opened queue\n")
	if err != nil {
		log.Fatalf("Opening queue failed: %s\n", err)
	}
	defer func() {
		writeTranscript("Closing queue\n")
		err := orphanQueue.Close()
		if err != nil {
			// Write to stderr for diagnostics.
			log.Printf("Closing queue failed: %s\n", err)
			// Write to stdout for ALICE's transcript and verifier.
			writeTranscript("Error closing queue")
		}
	}()

	var wg sync.WaitGroup
	wg.Add(config.writeGoroutinesCount + 1)

	go readQueueLoop(orphanQueue, &wg, config)
	for i := 0; i < config.writeGoroutinesCount; i++ {
		go writeQueue(orphanQueue, &wg)
	}

	wg.Wait()
}

func integrateOrphan(orphanQueue *goque.Queue, integrateFailureChance float64) error {
	item, err := orphanQueue.Peek()
	if err != nil {
		if err == goque.ErrEmpty {
			return goque.ErrEmpty
		}
		return fmt.Errorf("failed to peek into orphan queue: %s", err)
	}
	var orphan orphanedCert
	if err = item.ToObject(&orphan); err != nil {
		return fmt.Errorf("failed to unmarshal orphan: %s", err)
	}
	if rand.Float64() < integrateFailureChance {
		// synthetic failure to store orphaned item in SA
		return nil
	}
	writeTranscript("Integrated orphan with ID %v\n", orphan.RegID)
	if _, err = orphanQueue.Dequeue(); err != nil {
		return fmt.Errorf("failed to dequeue integrated orphaned certificate: %s", err)
	}
	return nil
}

func readQueueLoop(orphanQueue *goque.Queue, wg *sync.WaitGroup, config Config) {
	for i := 0; i < config.readLoopIterationCount; i++ {
		err := integrateOrphan(orphanQueue, config.integrateFailureChance)
		if err != nil {
			if err == goque.ErrEmpty {
				time.Sleep(time.Millisecond)
				continue
			}
			// Write to stderr for diagnostics.
			log.Printf("%s\n", err)
			// Write to stdout for ALICE's transcript and verifier.
			writeTranscript("Error reading queue")
		}
	}
	wg.Done()
}

func writeQueue(orphanQueue *goque.Queue, wg *sync.WaitGroup) {
	orphan := newOrphanedCert()
	// Write to stdout indicating this orphan may be present in the queue. This
	// corresponds to the "orphaning certificate" audit log.
	writeTranscript("Writing orphan with ID %v\n", orphan.RegID)

	if _, err := orphanQueue.EnqueueObject(orphan); err != nil {
		log.Fatalf("Writing to queue failed: %s\n", err)
	}

	// Write to stdout indicating this orphan must be durably stored.
	writeTranscript("Wrote orphan with ID %v\n", orphan.RegID)

	wg.Done()
}
