package main

import (
	"fmt"
	"os"
	"os/signal"
	"io/ioutil"
	"sync"
	"syscall"
	"flag"
	"strings"
	"math/rand"
	"github.com/ziutek/gst"
	"github.com/mytch444/mmusic/lib"
)

var codes = map[string](func(*Player)) {
	"next": playNext,
	"scan": scan,
	"random": random,
	"normal": normal,
	"pause": pause,
	"resume": resume,
	"increase": increaseVolume,
	"decrease": decreaseVolume,
	"mute": muteVolume,
}

type Player struct {
	lock *sync.Mutex
	
	snd *gst.Element
	bus *gst.Bus
	
	size int64 /* for convinience */
	songs *mmusic.Song
	
	current *mmusic.Song

	volume float64
	random bool
	
	playingFile *os.File
	volumeFile *os.File
	
	tmpDir string
}

func writeStringToPath(path string, s string) {
	file, err := os.Create(path)
	if err == nil {
		file.WriteString(s)
		file.Close()
	}
}

/* TODO: improve this so it's not so random.
 * http://keyj.emphy.de/balanced-shuffle/
 */
func randomSong(p *Player) *mmusic.Song {
	if p.size == 0 {
		return nil
	}
	
	n := rand.Int63n(p.size)
	s := p.songs.Next
	for i := int64(0); i < n && s != nil ; i++ {
		s = s.Next
	}
	return s
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

func scan(p *Player) {
	var err error
	p.songs, err = mmusic.Scan(p.tmpDir + mmusic.SuffixPlaylist)
	if err != nil {
		panic(err)
	}
	
	p.size = 0
	for s := p.songs.Next; s != nil; s = s.Next {
		p.size++
	}
}

func random(p *Player) {
	p.random = true
	f, err := os.Create(p.tmpDir + mmusic.SuffixIsrandom)
	if err == nil {
		f.Close()
	}
}

func normal(p *Player) {
	p.random = false
	os.Remove(p.tmpDir + mmusic.SuffixIsrandom)
}

func pause(p *Player) {
	p.snd.SetState(gst.STATE_PAUSED)
	f, err := os.Create(p.tmpDir + mmusic.SuffixIspaused)
	if err == nil {
		f.Close()
	}
}

func resume(p *Player) {
	p.snd.SetState(gst.STATE_PLAYING)
	os.Remove(p.tmpDir + mmusic.SuffixIspaused)
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
	
	writeStringToPath(p.tmpDir + mmusic.SuffixVolume,
		fmt.Sprintf("%d\n", int(p.volume * 100)))
}

func popUpcoming(p *Player) string {
	var data []byte = make([]byte, 2048)
	
	upcomingFile, err := os.Open(p.tmpDir + mmusic.SuffixUpcoming)
	if err != nil {
		fmt.Println(err)
		return ""
	}

	/* hopefully 2048 is enough bytes for the first line */
	n, err := upcomingFile.Read(data)
	if err != nil {
		upcomingFile.Close()
		return ""
	}
	
	tmpFile, err := os.Create(p.tmpDir + "/.upcoming.tmp")
	
	ret, le := mmusic.PopLine(data, n)
	tmpFile.Write(data[le:n])
	for {
		n, err := upcomingFile.Read(data)
		if err != nil {
			break
		}
		
		tmpFile.Write(data[:n])
	}
	
	upcomingFile.Close()
	tmpFile.Close()
	os.Remove(p.tmpDir + mmusic.SuffixUpcoming)
	os.Rename(p.tmpDir + "/.upcoming.tmp", p.tmpDir + mmusic.SuffixUpcoming)
	return ret
}

func playNext(p *Player) {
	var path string
	
	p.snd.SetState(gst.STATE_NULL)
	
	path = popUpcoming(p)
	
	if path == "" {
		if p.random {
			p.current = randomSong(p)
		} else {
			if p.current != nil {
				p.current = p.current.Next
			} else {
				p.current = p.songs.Next
				if p.current == nil {
					p.current = p.songs.Next
				}
			}
		}
		
		if p.current == nil {
			fmt.Println("There are no songs to play!")
			return
		}
		
		path = p.current.Path
	} else {
		p.current = nil
	}
	
	uri := makeURI(path)
	p.snd.SetProperty("uri", uri)
	p.snd.SetState(gst.STATE_PLAYING)

	os.Remove(p.tmpDir + mmusic.SuffixIspaused)
	writeStringToPath(p.tmpDir + mmusic.SuffixPlaying, uri + "\n")
}

/* init functions */

func populateTmp(path string) {
	var f *os.File
	_, err := os.Stat(path)
	if err == nil { /* File exits? */
		fmt.Println(path, "exists. Is another instance already running? If you really want another set -t to something.")
		os.Exit(1)
	}
	
	/* Just hope there are no errors. */
	os.Mkdir(path, 0700)
	syscall.Mkfifo(path + mmusic.SuffixIn, 0700)
	
	f, _ = os.Create(path + mmusic.SuffixUpcoming)
	f.Close()
	f, _ = os.Create(path + mmusic.SuffixPlaying)
	f.Close()
	f, _ = os.Create(path + mmusic.SuffixVolume)
	f.Close()
}

func listenFifo(p *Player) {
	var data = make([]byte, 512)

	for {
		in, err := os.Open(p.tmpDir + mmusic.SuffixIn)
		if err != nil {
			panic(err)
		}

		n, err := in.Read(data)
		if n > 0 && err != nil {
			panic(err)
		}
		
		str := string(data[:n])
		for _, name := range strings.Fields(str) {
			code := codes[name]
			if code != nil {
				p.lock.Lock()
				code(p)
				p.lock.Unlock()
			}
		}
		in.Close()
	}
}

func listenBus(p *Player) {
	for {
		mesg := p.bus.TimedPop(100000000)
		if mesg != nil {
			t := mesg.GetType()
			if t == gst.MESSAGE_EOS || t == gst.MESSAGE_ERROR {
			 /* best to just move on with life. */
				p.lock.Lock()
				playNext(p)
				p.lock.Unlock()
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

	p.snd = gst.ElementFactoryMake("playbin", "mmusic")
	if p.snd == nil {
		fmt.Println("Failed to initialize gst: snd")
		os.Exit(1)
	}
	
	sink := gst.ElementFactoryMake(*nsink, "Sink")
	if sink == nil {
		fmt.Println("Failed to initilize gst: ", *nsink)
		os.Exit(1)
	}
	p.snd.Link(sink)
	
	p.bus = p.snd.GetBus()
	if p.bus == nil {
		fmt.Println("Failed to open gstreamer bus!")
		os.Exit(1)
	}
	populateTmp(*tmpDir)
	
	p.tmpDir = *tmpDir
	p.volume = float64(*volume) / 100
	p.random = *random
	p.lock = new(sync.Mutex)
	
	file, err := os.Create(p.tmpDir + mmusic.SuffixPlaylist)
	if err != nil {
		panic(err)
	}
	
	if *readstdin {
		data := make([]byte, 512)
		for {
			n, err := os.Stdin.Read(data)
			if err != nil {
				break
			}
			file.Write(data[:n])
		}
	}

	for _, name := range flag.Args() {
		bs, err := ioutil.ReadFile(name)
		if err != nil {
			fmt.Println(err)
		} else {
			file.Write(bs)
		}
	}
	file.Close()
	
	updateVolume(p)
	if p.random {
		file, err = os.Create(p.tmpDir + mmusic.SuffixIsrandom)
		if err == nil {
			file.Close()
		}
		
	}
	
	scan(p)
	playNext(p)
	
	go listenFifo(p)
	go listenBus(p)
	
	/* Wait for SIGTERM/INT signal */
	_ = <-sigChan
	os.RemoveAll(p.tmpDir)
	os.Exit(0)
}
