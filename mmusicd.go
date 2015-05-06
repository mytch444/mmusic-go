package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"flag"
	"github.com/ziutek/gst"
	"io"
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

type Player struct {
	snd *gst.Element
	bus *gst.Bus
	
	size int64
	library []string
	
	index int64

	volume float64
	random bool
}

var tmpDir *string = nil

var playlistFile *os.File = nil
var upcomingFile *os.File = nil
var playingFile *os.File = nil
var volumeFile *os.File = nil

func exit(stat int) {
	fmt.Print("Clean...")
	os.RemoveAll(*tmpDir)
	fmt.Println("Exit.")
	os.Exit(stat)
}

func error(mesg string) {
	fmt.Println(mesg)
	exit(1)
}

func scan(p *Player) {
	p.size = 2
	p.library = []string{
		"/media/music/Moby/18/01 We Are All Made Of Stars.mp3",
		"/media/music/Tool/Opiate/05 Jerk-Off.mp3",
	}
}

func setRandom(p *Player) {
	p.random = true
}

func setNormal(p *Player) {
	p.random = false
}

func pause(p *Player) {
	p.snd.SetState(gst.STATE_PAUSED)
	os.Create(*tmpDir + "/state/ispaused")
}

func resume(p *Player) {
	p.snd.SetState(gst.STATE_PLAYING)
	os.Remove(*tmpDir + "/state/ispaused")
}

func increaseVolume(p *Player) {
	p.volume += 0.01
	if (p.volume > 1.0) {
		p.volume = 1.0
	}
	updateVolume(p)
}

func decreaseVolume(p *Player) {
	p.volume -= 0.01
	if (p.volume < 0.0) {
		p.volume = 0.0
	}
	updateVolume(p)
}

func muteVolume(p *Player) {
	p.volume = 0.0
	updateVolume(p)
}

func updateVolume(p *Player) {
	p.snd.SetProperty("volume", p.volume)
	
	volumeFile.Truncate(0)
	volumeFile.Seek(0, 0)
	volumeFile.WriteString(fmt.Sprintf("%d\n", int(p.volume * 100)))
	volumeFile.Sync()
}

func popUpcoming() string {
	var data []byte = make([]byte, 512)
	var ret string
	var n, le int
		
	/* hopefully 2048 is enough bytes for the first line */
	n, err := upcomingFile.Read(data)
	if err == io.EOF && n == 0 {
		return ""
	}
	
	tmpFile, err := os.Create(*tmpDir + "/.upcoming.tmp")
	
	for _, c := range data {
		if (c == '\n') {
			break
		} else {
			le++
		}
	}
	
	ret = string(data[:le])
	
	tmpFile.Write(data[le+1:n])
	for ; ; {
		n, err := upcomingFile.Read(data)
		if err != nil {
			break
		}
		
		tmpFile.Write(data[:n])
	}
	
	tmpFile.Close()
	os.Remove(*tmpDir + "/upcoming")
	os.Rename(*tmpDir + "/.upcoming.tmp", *tmpDir + "/upcoming")
	return ret
}

func playNext(p *Player) {
	var path string
	
	path = popUpcoming()
	
	if path == "" {
		if p.random {
			p.index = rand.Int63n(p.size)
		} else {
			p.index++
			if (p.index >= p.size) {
				p.index = 0
			}
		}
		
		path = p.library[p.index]
	} else {
		p.index = -1
	}
	
	p.snd.SetState(gst.STATE_NULL)
	p.snd.SetProperty("uri", "file://" + path)
	p.snd.SetState(gst.STATE_PLAYING)
	
	playingFile.Truncate(0)
	playingFile.Seek(0, 0)
	playingFile.WriteString(path + "\n")
	playingFile.Sync()
}

func (p *Player) onMessage(bus *gst.Bus, msg *gst.Message) {
	fmt.Println("Got a message from bus")
	switch msg.GetType() {
	case gst.MESSAGE_EOS:
		playNext(p)
	case gst.MESSAGE_ERROR:
		err, debug := msg.ParseError()
		fmt.Printf("GST ERROR: %s (debug: %s)\n", err, debug)
		error("DIE")
	}
}

func populateTmp(clean bool) {
	_, err := os.Stat(*tmpDir)
	if err == nil { /* File exits? */
		if clean {
			os.RemoveAll(*tmpDir)
		} else {
			error("mmusicd already started, try -r")
		}
	}
	
	/* Just going to ignore errors. There really shouldn't be any. */
	_                 = os.Mkdir(*tmpDir, 0700)
	_                 = os.Mkdir(*tmpDir + "/state", 0700)
	_                 = syscall.Mkfifo(*tmpDir + "/in", 0700)
	upcomingFile, _   = os.Create(*tmpDir + "/upcoming")
	playingFile, _    = os.Create(*tmpDir + "/state/playing")
	volumeFile, _     = os.Create(*tmpDir + "/state/volume")
}

func listenFifo(p *Player) {
	data := make([]byte, 512)

	for ; ; {
		in, err := os.Open(*tmpDir + "/in")
		defer in.Close()
		if err != nil {
			error("Error opening " + *tmpDir + "/in")
		}
			
		n, err := in.Read(data)
		if err == io.EOF && n == 0 {
			continue
		} else if err != nil {
			error("Error reading fifo!")
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
	var cleanTmp *bool
	var defaultTmp string
	var nsink *string
	var random *bool
	var volume *int64
	
	var sigChan chan os.Signal = make(chan os.Signal, 1)
	
	defaultTmp = fmt.Sprintf("%s/mmusic-%d", os.TempDir(), os.Getuid())
	
	cleanTmp = flag.Bool("-restart", false, "If mmusicd tmp dir exists delete it then start.")
	tmpDir = flag.String("t", defaultTmp, "Set tmp directory.")
	nsink = flag.String("l", "alsasink", "Change gstreamer sink.")
	random = flag.Bool("r", true, "Set starting randomness.")
	volume = flag.Int64("v", 50, "Set starting volume.")

	flag.Parse()

	signal.Notify(sigChan, syscall.SIGTERM)
	signal.Notify(sigChan, syscall.SIGINT)

	populateTmp(*cleanTmp)

	p := new(Player)
	
	p.volume = float64(*volume) / 100
	p.random = *random
	
	p.snd = gst.ElementFactoryMake("playbin", "mmusicd")
	if (p.snd == nil) {
		error("Failed to initialize gst: snd")
	}
	
	sink := gst.ElementFactoryMake(*nsink, "Sink")
	if (sink == nil) {
		error("Failed to initilize gst: " + *nsink)
	}
	
	p.snd.Link(sink)
	
	p.bus = p.snd.GetBus()
	p.bus.AddSignalWatch()
	p.bus.Connect("message", (*Player).onMessage, p)
	
	for _, playlist := range flag.Args() {
		fmt.Println("Adding playlist", playlist)
	}
	
	fmt.Println("Starting  mmusicd")
	
	updateVolume(p)
	scan(p)
	
	go listenFifo(p)
	playNext(p)
	
	/* Wait for SIGTERM/INT signal */
	_ = <-sigChan
	exit(0)
}
