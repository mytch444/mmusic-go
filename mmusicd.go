package main

import (
	"fmt"
	"os"
	"os/signal"
	"io/ioutil"
	"syscall"
	"flag"
	"github.com/ziutek/gst"
	"strings"
	"math/rand"
)

var codes = map[string](func(*Player)) {
	"next": playNext,
	"scan": scan,
	"random": setRandom,
	"normal": setNormal,
	"pause": pause,
	"resume": resume,
	"increase": increaseVolume,
	"decrease": decreaseVolume,
	"mute": muteVolume,
}

type Song struct {
	path string
	next *Song
}

type Player struct {
	snd *gst.Element
	bus *gst.Bus
	
	size int64 /* for convinience */
	songs *Song
	
	current *Song

	volume float64
	random bool
	
	playingFile *os.File
	volumeFile *os.File
	
	tmpDir string
}

/* helper functions */

func writeStringToPath(path string, s string) {
	os.Remove(path)
	file, err := os.Create(path)
	if err == nil {
		file.WriteString(s)
		file.Close()
	}
}

/* TODO: improve this so it's not so random.
 * http://keyj.emphy.de/balanced-shuffle/
 */
func randomSong(p *Player) *Song {
	n := rand.Int63n(p.size)
	s := p.songs.next
	for i := int64(0); i < n && s != nil ; i++ {
		s = s.next
	}
	return s
}

func popLine(data []byte, n int) (string, int) {
	var i, le int
	for i = 0; i < n; i++ {
		if (data[i] == '\n') {
			break
		} else {
			le++
		}
	}
	
	return string(data[:le]), le + 1
}

func makeURI(str string) string {
	if strings.HasPrefix(str, "file://") ||
		strings.HasPrefix(str, "http://") ||
		strings.HasPrefix(str, "https://") {
		return str
	} else if strings.HasPrefix(str, "/") {
		return "file://" + str
	} else {
		return "file://" + os.Getenv("PWD") + "/" + str
	}
}

/* user functions */

func scan(p *Player) {
	var seek int64 = 0
	var n int
	var s *Song
	
	fmt.Println("Scanning")
	f, err := os.Open(p.tmpDir + "/playlist")
	defer f.Close()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	
	p.size = 0
	p.songs = new(Song)
	p.songs.next = nil
	s = p.songs
	
	bad := new(Song)
	bad.next = nil
	
	data := make([]byte, 2048)
	err = nil

	for ; ; {
		n, err = f.Read(data)
		if err != nil || n == 0 {
			break
		}
		
		line, le := popLine(data, n)
		
		seek = int64(le - n)
		if seek >= 0 {
			break
		}
		f.Seek(seek, 1)
		
		if line[0] == '!' {
			bad.next = new(Song)
			bad = bad.next
			bad.path = line
			bad.next = nil
		} else {
			p.size++
			s.next = new(Song)
			s = s.next
			s.path = line
			s.next = nil
		}
	}
	
	/* TODO: go through bad and remove all matchs in p.songs 
	 * actually, no.
	 * go through p.songs and check if it matches a bad, remove
	 * if it does, otherwise check if it is a dir, if it is 
	 * remove it and find all files in it.
	 */
}

func setRandom(p *Player) {
	p.random = true
	os.Create(p.tmpDir + "/state/israndom")
}

func setNormal(p *Player) {
	p.random = false
	os.Remove(p.tmpDir + "/state/israndom")
}

func pause(p *Player) {
	p.snd.SetState(gst.STATE_PAUSED)
	os.Create(p.tmpDir + "/state/ispaused")
}

func resume(p *Player) {
	p.snd.SetState(gst.STATE_PLAYING)
	os.Remove(p.tmpDir + "/state/ispaused")
}

func increaseVolume(p *Player) {
	p.volume += 0.01
	updateVolume(p)
}

func decreaseVolume(p *Player) {
	p.volume -= 0.01
	updateVolume(p)
}

func muteVolume(p *Player) {
	p.volume = 0.0
	updateVolume(p)
}

func updateVolume(p *Player) {
	if (p.volume > 1.0) {
		p.volume = 1.0
	} else if (p.volume < 0.0) {
		p.volume = 0.0
	}

	p.snd.SetProperty("volume", p.volume)
	
	writeStringToPath(p.tmpDir + "/state/volume",
		fmt.Sprintf("%d\n", int(p.volume * 100)))
}

func popUpcoming(p *Player) string {
	var data []byte = make([]byte, 2048)
	
	upcomingFile, err := os.Open(p.tmpDir + "/upcoming")
	if err != nil {
		fmt.Println(err)
		return ""
	}
	
	/* hopefully 2048 is enough bytes for the first line */
	n, err := upcomingFile.Read(data)
	if err != nil {
		return ""
	}
	
	tmpFile, err := os.Create(p.tmpDir + "/.upcoming.tmp")
	
	ret, le := popLine(data, n)
	tmpFile.Write(data[le:n])
	for ; ; {
		n, err := upcomingFile.Read(data)
		if err != nil {
			break
		}
		
		tmpFile.Write(data[:n])
	}
	
	upcomingFile.Close()
	tmpFile.Close()
	os.Remove(p.tmpDir + "/upcoming")
	os.Rename(p.tmpDir + "/.upcoming.tmp", p.tmpDir + "/upcoming")
	return ret
}

func playNext(p *Player) {
	var path string
	
	p.snd.SetState(gst.STATE_NULL)
	
	path = popUpcoming(p)
	
	if path == "" {
		if p.random && p.size > 0 {
			p.current = randomSong(p)
		} else {
			if p.current != nil {
				p.current = p.current.next
			} else {
				p.current = p.songs.next
			}
			if p.current == nil {
				p.current = p.songs.next
			}
		}
		
		if p.current == nil {
			fmt.Println("There are no songs to play!")
			return
		}
		
		path = p.current.path
	} else {
		p.current = nil
	}
	
	fmt.Println("playing", makeURI(path))
	
	p.snd.SetProperty("uri", makeURI(path))
	p.snd.SetState(gst.STATE_PLAYING)

	writeStringToPath(p.tmpDir + "/state/playing", path + "\n")
}

/* init functions */

/* NOT WORKING */
func (p *Player) onMessage(bus *gst.Bus, msg *gst.Message) {
	fmt.Println("Got a message from bus")
	switch msg.GetType() {
	case gst.MESSAGE_EOS:
		playNext(p)
	case gst.MESSAGE_ERROR:
		err, debug := msg.ParseError()
		fmt.Printf("GST ERROR: %s (debug: %s)\n", err, debug)
		os.Exit(1)
	}
}

func populateTmp(path string) {
	_, err := os.Stat(path)
	if err == nil { /* File exits? */
		fmt.Println(path, "exits. Is mmusicd already running?")
		os.Exit(1)
	}
	
	/*
	 * Just going to ignore everything.
	 * There really shouldn't be any errors.
	 */
	os.Mkdir(path, 0700)
	os.Mkdir(path + "/state", 0700)
	syscall.Mkfifo(path + "/in", 0700)
	os.Create(path + "/upcoming")
	os.Create(path + "/state/playing")
	os.Create(path + "/state/volume")
}

func listenFifo(p *Player) {
	var data = make([]byte, 512)

	for ; ; {
		in, err := os.Open(p.tmpDir + "/in")
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		defer in.Close()
		n, err := in.Read(data)
		if n > 0 && err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		
		str := string(data[:n])
		for _, name := range strings.Fields(str) {
			code := codes[name]
			if code != nil {
				code(p)
			}
		}
	}
}

func main () {
	var tmpDir *string
	var nsink *string
	var random *bool
	var volume *int64
	var readstdin *bool
	
	var sigChan chan os.Signal = make(chan os.Signal, 1)
	
	defaultTmp := fmt.Sprintf("%s/mmusic-%d", os.TempDir(), os.Getuid())
	
	tmpDir = flag.String("t", defaultTmp, "Set tmp directory.")
	nsink = flag.String("l", "alsasink", "Change gstreamer sink.")
	random = flag.Bool("r", true, "Set starting randomness.")
	volume = flag.Int64("v", 50, "Set starting volume [0-100].")
	readstdin = flag.Bool("stdin", false, "Read stdin as a playlist file.")

	flag.Parse()

	signal.Notify(sigChan, syscall.SIGTERM)
	signal.Notify(sigChan, syscall.SIGINT)
	
	p := new(Player)

	p.snd = gst.ElementFactoryMake("playbin", "mmusicd")
	if (p.snd == nil) {
		fmt.Println("Failed to initialize gst: snd")
		os.Exit(1)
	}
	
	sink := gst.ElementFactoryMake(*nsink, "Sink")
	if (sink == nil) {
		fmt.Println("Failed to initilize gst: ", *nsink)
		os.Exit(1)
	}
	
	p.snd.Link(sink)
	
	populateTmp(*tmpDir)
	
	p.tmpDir = *tmpDir
	p.volume = float64(*volume) / 100
	p.random = *random
	
	if p.random {
		os.Create(p.tmpDir + "/state/israndom")
	}
	
	p.bus = p.snd.GetBus()
	p.bus.AddSignalWatch()
	p.bus.Connect("message", (*Player).onMessage, p)
	p.bus.EnableSyncMessageEmission()
	
	file, err := os.Create(p.tmpDir + "/playlist")
	if err != nil {
		fmt.Println(err)
		os.RemoveAll(p.tmpDir)
		os.Exit(1)
	}
	
	if *readstdin {
		fmt.Println("Adding stdin")
		data := make([]byte, 512)
		for ; ; {
			n, err := os.Stdin.Read(data)
			if err != nil {
				break
			}
			file.Write(data[:n])
		}
	}

	fmt.Println("reading playlists")
	for _, name := range flag.Args() {
		fmt.Println("Adding playlist", name)
		bs, err := ioutil.ReadFile(name)
		if err != nil {
			fmt.Println(err)
		} else {
			file.Write(bs)
		}
	}
	file.Close()
	
	fmt.Println("Starting  mmusicd")
	
	updateVolume(p)
	scan(p)
	
	go listenFifo(p)
	playNext(p)
	
	/* Wait for SIGTERM/INT signal */
	_ = <-sigChan
	os.RemoveAll(p.tmpDir)
	os.Exit(0)
}
