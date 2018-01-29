package logger

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Config is the configuration point of this package
type Config struct {
	// Input file to read from. Often uses os.Stdin
	Input io.Reader

	// Output file to write to. Often uses os.Stdout
	Output io.Writer

	// UpdateInterval is how often to print a status message. Set to zero to
	// never print status updates.
	UpdateInterval time.Duration

	// HighTrafficInterval is how often to check for high traffic. Set to zero
	// to never print high traffic warnings
	HighTrafficInterval time.Duration

	// HighTrafficThreshold is the maximum number of hits in
	// HighTrafficInterval before we print a warning
	HighTrafficThreshold int
}

// Run the logging daemon. This function will return when config.Input is
// closed. Make sure to set all the fields in Config appropriately.
func Run(config Config) {
	logger := logger{
		Config:      config,
		sectionHits: map[string]int{},
	}
	reader := bufio.NewReader(config.Input)

	stopWatcher := logger.watch()
	defer stopWatcher()

	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return
		}
		line = bytes.TrimSpace(line)

		entry, err := parseLineEntry(string(line))
		if err != nil {
			fmt.Fprintf(logger.Output,
				"WARNING: %s\n",
				err,
			)
		}

		logger.totalTraffic++
		logger.sectionHitsLock.Lock()
		logger.sectionHits[entry.Section]++
		logger.sectionHitsLock.Unlock()
	}
}

const commonLogFormatPattern = `^(\S+)` + // client-id: matches[0]
	` \S+` + // user-identifier: ignored
	` (\S+)` + // user-id: matches[1]
	` \[([^\]]+)\]` + // time-stamp: matches[3]
	` "([A-Z]+)` + // http-method: matches[4]
	` ((/?[^/ "]+)[^ "]*)? HTTP/[0-9.]+"` + // resource and section: matches[5] and matches[6]
	` ([0-9]{3})` + // http-status: matches[7]
	` ([0-9]+|-)` // message-size: matches[8]

const commonLogDateFormat = "02/Jan/2006:15:04:05 -0700"

var commonLogFormatRegexp = regexp.MustCompile(commonLogFormatPattern)

type commonLogEntry struct {
	ClientIP string
	UserID   string
	Time     time.Time
	Method   string
	Section  string
	Resource string
	Status   int
	Size     int
}

type logger struct {
	Config
	sectionHitsLock        sync.Mutex
	sectionHits            map[string]int
	totalTraffic           int
	isHighTrafficTriggered bool
}

// watch calls handler functions for each of our periodic actions. Make sure to
// call the stop function when ending the watching process.
func (logger *logger) watch() (stop func()) {
	updateTicker := time.Tick(logger.UpdateInterval)
	highTrafficTicker := time.Tick(logger.HighTrafficInterval)
	end := make(chan struct{})
	go func() {
		for {
			select {
			case <-updateTicker:
				logger.printUpdate()

			case <-highTrafficTicker:
				logger.checkHighTraffic()

			case <-end:
				return
			}
		}
	}()

	return func() {
		end <- struct{}{}
	}
}

// printUpdate prints a message with the busiest section during the last
// UpdateInterval period.
func (logger *logger) printUpdate() {
	logger.sectionHitsLock.Lock()
	oldSectionHits := logger.sectionHits
	logger.sectionHits = map[string]int{}
	logger.sectionHitsLock.Unlock()

	maxHits := 0
	var activeSections []string
	for section, hits := range oldSectionHits {
		if hits > maxHits {
			maxHits = hits
			activeSections = nil
		}
		if maxHits == hits {
			activeSections = append(activeSections, section)
		}
	}

	hitsPerSecond := float64(maxHits) *
		float64(time.Second) / float64(logger.UpdateInterval)

	if maxHits == 0 {
		fmt.Fprintln(logger.Output,
			"no hits to server",
		)
	} else {
		sort.Strings(activeSections)
		fmt.Fprintf(logger.Output,
			"busiest sections: %s (%0.2f hits per second)\n",
			strings.Join(activeSections, ", "),
			hitsPerSecond,
		)
	}
}

// checkHighTraffic checks if we've recieved more hits than
// HighTrafficThreshold. If we have, then print a warning.
// We also print a warning when this high traffic state is resolved.
func (logger *logger) checkHighTraffic() {
	if logger.totalTraffic > logger.HighTrafficThreshold &&
		!logger.isHighTrafficTriggered {

		hitsPerSecond := float64(logger.totalTraffic) *
			float64(time.Second) / float64(logger.HighTrafficInterval)

		fmt.Fprintf(logger.Output,
			"WARNING: high traffic of %0.2f hits per second, triggered at %s\n",
			hitsPerSecond,
			time.Now().Format(time.RFC3339),
		)

		logger.isHighTrafficTriggered = true
	}

	if logger.totalTraffic <= logger.HighTrafficThreshold &&
		logger.isHighTrafficTriggered {

		fmt.Fprintf(logger.Output,
			"WARNING: high traffic condition resolved at %s\n",
			time.Now().Format(time.RFC3339),
		)

		logger.isHighTrafficTriggered = false
	}

	logger.totalTraffic = 0
}

// parseLineEntry parses a common log formatted line, and return a struct
// containing the data we are interested in.
func parseLineEntry(line string) (*commonLogEntry, error) {
	matches := commonLogFormatRegexp.FindStringSubmatch(line)
	if matches == nil {
		return nil, fmt.Errorf("could not parse line '%s'", line)
	}

	time, err := time.Parse(commonLogDateFormat, matches[3])
	if err != nil {
		return nil, err
	}

	status, err := strconv.Atoi(matches[7])
	if err != nil {
		return nil, err
	}

	size, err := strconv.Atoi(matches[8])
	if err != nil {
		return nil, err
	}

	entry := commonLogEntry{
		ClientIP: matches[1],
		UserID:   matches[2],
		Time:     time,
		Method:   matches[4],
		Section:  matches[6],
		Resource: matches[5],
		Status:   status,
		Size:     size,
	}

	return &entry, nil
}
