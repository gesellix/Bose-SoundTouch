package setup

import (
	"reflect"
	"sync"
	"testing"
	"time"
)

type serializedTelnetFactory struct {
	mu                   sync.Mutex
	nextID               int
	urlsByID             map[int]telnetURLs
	commandsByConnection []int
	firstCommandStarted  chan struct{}
	releaseFirstCommand  chan struct{}
	secondDialed         chan struct{}
}

type serializedTelnetClient struct {
	factory      *serializedTelnetFactory
	id           int
	commandCount int
}

func (f *serializedTelnetFactory) newClient(string) TelnetClient {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.nextID++

	return &serializedTelnetClient{factory: f, id: f.nextID}
}

func (c *serializedTelnetClient) Dial() error {
	if c.id == 2 {
		close(c.factory.secondDialed)
	}

	return nil
}

func (c *serializedTelnetClient) Probe() (string, error) { return "", nil }
func (c *serializedTelnetClient) Close() error           { return nil }

func (c *serializedTelnetClient) SendCommand(command string) (string, error) {
	c.commandCount++

	c.factory.mu.Lock()
	c.factory.commandsByConnection = append(c.factory.commandsByConnection, c.id)
	c.factory.mu.Unlock()

	if c.id == 1 && c.commandCount == 1 {
		close(c.factory.firstCommandStarted)
		<-c.factory.releaseFirstCommand
	}

	urls := c.factory.urlsByID[c.id]
	if command == "getpdo CurrentSystemConfiguration" {
		return flatGetpdoResponse(urls), nil
	}

	if command == "envswitch boseurls set "+urls.Marge+" "+urls.SwUpdate {
		return "Setting Bose Server URLs to " + urls.Marge + " and " + urls.SwUpdate + " ->\n", nil
	}

	return "OK\n", nil
}

func TestTelnetURLMutationsSameSpeakerAreSerialized(t *testing.T) {
	firstURLs := canonicalBoseTelnetURLs()
	secondURLs := defaultTelnetURLs("http://next.example:8000")
	factory := &serializedTelnetFactory{
		urlsByID: map[int]telnetURLs{
			1: firstURLs,
			2: secondURLs,
		},
		firstCommandStarted: make(chan struct{}),
		releaseFirstCommand: make(chan struct{}),
		secondDialed:        make(chan struct{}),
	}
	m := &Manager{NewTelnet: factory.newClient}
	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)

	go func() {
		_, err := m.RevertTelnetURLs("192.0.2.1", nil)
		firstDone <- err
	}()

	<-factory.firstCommandStarted
	secondStarted := make(chan struct{})

	go func() {
		close(secondStarted)
		_, err := m.setAllBoseURLsViaTelnet("192.0.2.1", secondURLs)
		secondDone <- err
	}()

	<-secondStarted

	select {
	case <-factory.secondDialed:
		close(factory.releaseFirstCommand)
		t.Fatal("second mutation dialed before the first sequence completed")
	case <-time.After(100 * time.Millisecond):
	}

	close(factory.releaseFirstCommand)

	if err := <-firstDone; err != nil {
		t.Fatalf("first mutation: %v", err)
	}

	if err := <-secondDone; err != nil {
		t.Fatalf("second mutation: %v", err)
	}

	factory.mu.Lock()
	got := append([]int(nil), factory.commandsByConnection...)
	factory.mu.Unlock()

	want := []int{1, 1, 1, 1, 1, 1, 2, 2, 2, 2, 2}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("connection command order = %v, want contiguous blocks %v", got, want)
	}
}
