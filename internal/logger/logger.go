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

const commonLogFormatPattern = `^(\S+)` + // client-id: matches[0]
	` \S+` + // user-identifier: ignored
	` (\S+)` + // user-id: matches[1]
	` \[([^\]]+)\]` + // time-stamp: matches[3]
	` "([A-Z]+)` + // http-method: matches[4]
	` ((/?[^/ "]+)[^ "]*)? HTTP/[0-9.]+"` + // resource and section: matches[5] and matches[6]
	` ([0-9]{3})` + // http-status: matches[7]
	` ([0-9]+|-)` // message-size: matches[8]

var commonLogFormatRegexp = regexp.MustCompile(commonLogFormatPattern)

const commonLogDateFormat = "02/Jan/2006:15:04:05 -0700"

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

type Config struct {
	Input                io.ReadCloser
	Output               io.Writer
	UpdateInterval       time.Duration
	HighTrafficThreshold int
	HighTrafficInterval  time.Duration
}

type logger struct {
	Config
	sectionHitsLock        sync.Mutex
	sectionHits            map[string]int
	totalTraffic           int
	isHighTrafficTriggered bool
}

func Run(config Config) {
	logger := logger{
		Config:      config,
		sectionHits: map[string]int{},
	}
	reader := bufio.NewReader(config.Input)

	end := make(chan struct{})
	go logger.watch(end)
	defer func() { end <- struct{}{} }()

	for {
		line, err := reader.ReadBytes('\n')
		if err == io.EOF {
			return
		}
		line = bytes.TrimSpace(line)

		entry, err := parseLineEntry(line)
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

func (logger *logger) watch(end chan struct{}) {
	updateTicker := time.Tick(logger.UpdateInterval)
	highTrafficTicker := time.Tick(logger.HighTrafficInterval)
	for {
		select {
		case <-updateTicker:
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

		case <-highTrafficTicker:
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

		case <-end:
			break
		}
	}
}

func parseLineEntry(line []byte) (*commonLogEntry, error) {
	matches := commonLogFormatRegexp.FindStringSubmatch(string(line))
	if len(matches) < 8 {
		return nil, fmt.Errorf("could not parse line '%s'\n", line)
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
