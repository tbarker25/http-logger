package logger

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"sync"
	"testing"
	"time"
)

func writeTestLine(w io.Writer, section string) {
	fmt.Fprintf(w,
		`238.129.94.11 - - [%s] "PUT /%s/posts HTTP/1.0" 200 5034 "http://sanchez-malone.com/faq.php" "Mozilla/5.0 (Macintosh; PPC Mac OS X 10_6_8; rv:1.9.6.20) Gecko/2015-06-28 12:08:37 Firefox/3.6.12"`+"\n",
		time.Now().Format(commonLogDateFormat),
		section,
	)
}

func TestReportingSections(t *testing.T) {
	t.Parallel()
	readIn, writeIn := io.Pipe()
	var out syncBuffer

	var config = Config{
		Input:          readIn,
		Output:         &out,
		UpdateInterval: 1 * time.Second,
	}

	go Run(config)
	defer writeIn.Close()
	time.Sleep(100 * time.Millisecond)

	writeTestLine(writeIn, "section-a")
	writeTestLine(writeIn, "section-a")
	writeTestLine(writeIn, "section-b")
	time.Sleep(config.UpdateInterval)

	want := "busiest sections: /section-a (2.00 hits per second)\n"
	if want != out.String() {
		t.Fatalf("want: `%s`\ngot: `%s`\n", want, out.String())
	}
	out.Reset()

	writeTestLine(writeIn, "section-a")
	writeTestLine(writeIn, "section-b")
	writeTestLine(writeIn, "section-c")
	time.Sleep(config.UpdateInterval)

	want = "busiest sections: /section-a, /section-b, /section-c (1.00 hits per second)\n"
	if want != out.String() {
		t.Fatalf("want: `%s`\ngot: `%s`\n", want, out.String())
	}
	out.Reset()

	time.Sleep(config.UpdateInterval)
	want = "no hits to server\n"
	if want != out.String() {
		t.Fatalf("want: `%s`\ngot: `%s`\n", want, out.String())
	}
	out.Reset()
}

func TestPrintingAlert(t *testing.T) {
	t.Parallel()
	readIn, writeIn := io.Pipe()
	var out syncBuffer

	var config = Config{
		Input:                readIn,
		Output:               &out,
		HighTrafficThreshold: 10,
		HighTrafficInterval:  1 * time.Second,
	}

	go Run(config)
	defer writeIn.Close()

	time.Sleep(100 * time.Millisecond)

	// We shouldn't write anything when below threshold
	for i := 0; i < config.HighTrafficThreshold; i++ {
		writeTestLine(writeIn, "section-a")
	}
	time.Sleep(config.HighTrafficInterval + time.Millisecond)
	if out.String() != "" {
		t.Fatalf("want: `%s`\ngot: `%s`\n", "", out.String())
	}
	out.Reset()

	// We shouldn't write anything when below threshold
	// trying this twice to ensure that the counter is reset after waiting for
	// HighTrafficInterval to elapse
	for i := 0; i < config.HighTrafficThreshold; i++ {
		writeTestLine(writeIn, "section-a")
	}
	time.Sleep(config.HighTrafficInterval)
	if out.String() != "" {
		t.Fatalf("want: `%s`\ngot: `%s`\n", "", out.String())
	}
	out.Reset()

	// This time lets breach the threshold and check that an error is logged
	for i := 0; i < config.HighTrafficThreshold+1; i++ {
		writeTestLine(writeIn, "section-a")
	}
	time.Sleep(config.HighTrafficInterval)
	want := regexp.MustCompile(`^WARNING: high traffic of \d+.\d+ hits per second, triggered at \S+\n$`)
	if !want.MatchString(out.String()) {
		t.Fatalf("want: `%s`\ngot: `%s`\n", want, out.String())
	}
	out.Reset()

	// If we breach again we don't log anything, since we already logged an
	// error earlier
	for i := 0; i < config.HighTrafficThreshold+1; i++ {
		writeTestLine(writeIn, "section-a")
	}
	time.Sleep(config.HighTrafficInterval)
	if out.String() != "" {
		t.Fatalf("want: `%s`\ngot: `%s`\n", "", out.String())
	}
	out.Reset()

	// Now lets not breach the threshold and make sure we log that the high
	// traffic condition is resolved
	time.Sleep(config.HighTrafficInterval + time.Millisecond)
	want = regexp.MustCompile(`^WARNING: high traffic condition resolved at \S+\n$`)
	if !want.MatchString(out.String()) {
		t.Fatalf("want: `%s`\ngot: `%s`\n", want, out.String())
	}
}

// Just a simple sync-safe bytes.Buffer
type syncBuffer struct {
	b bytes.Buffer
	m sync.Mutex
}

func (b *syncBuffer) Read(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Read(p)
}

func (b *syncBuffer) Write(p []byte) (n int, err error) {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.m.Lock()
	defer b.m.Unlock()
	return b.b.String()
}

func (b *syncBuffer) Reset() {
	b.m.Lock()
	defer b.m.Unlock()
	b.b.Reset()
}
