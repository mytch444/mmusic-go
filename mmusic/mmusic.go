package main

import (
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"flag"
	"strings"
	"time"
	"math/rand"
	"sort"
	"github.com/ziutek/gst"
)

var SuffixIn string         = "/in"
var SuffixPlaylist string   = "/playlist"
var SuffixUpcoming string   = "/upcoming"
var SuffixVolume string     = "/volume"
var SuffixPlaying string    = "/playing"
var SuffixIsRandom string   = "/israndom"
var SuffixIsPaused string   = "/ispaused"

type Song struct {
	Value string
	Next *Song
}

var codes = map[string](func(*Player)) {
	"exit": exit,
	"next": playNext,
	"random": random,
	"normal": normal,
	"pause": pause,
	"resume": resume,
	"increase": increaseVolume,
	"decrease": decreaseVolume,
}

type Player struct {
	lock *sync.Mutex
	
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


/* Returns the first line found in data looking no further than
 * n as well as the position of the next line (so you can os.Seek
 * to it).
 */
func PopLine(data []byte, n int) (string, int) {
	var i int
	for i = 0; i < n; i++ {
		if (data[i] == '\n') {
			break
		}
	}
	
	return string(data[:i]), i + 1
}

func songInBad(bad *Song, song *Song) bool {
	for bad = bad.Next; bad != nil; bad = bad.Next {
		if (strings.HasPrefix(song.Value, bad.Value)) {
			return true
		}
	}
	return false
}

func fillAndClean(songs *Song, bad *Song) {
	var prev, next, t, s *Song
	prev = songs
	for s = songs.Next; s != nil; s = s.Next {
		if (songInBad(bad, s)) {
			prev.Next = s.Next
		} else {
			fi, err := os.Stat(s.Value)
			if err != nil || !fi.IsDir() {
				prev = s
				continue
			}
			
			
			file, err := os.Open(s.Value)
			if err != nil {
				prev = s
				file.Close()
				continue
			}
			
			subs, err := file.Readdirnames(0)
			if err != nil {
				panic(err)
			}
			
			sort.Strings(subs)
			
			next = s.Next
			t = s
			for _, sub := range subs {
				t.Next = new(Song)
				t = t.Next
				t.Value = s.Value + "/" + sub
			}
			
			t.Next = next
			
			prev.Next = s.Next
			file.Close()
		}
	}
}

func scan(file *os.File) (songs *Song) {
	var seek int64 = 0
	var s, b *Song
	var n int
	var err error
	
	songs = new(Song)
	s = songs
	
	bad := new(Song)
	bad.Next = nil
	b = bad
	
	data := make([]byte, 2048)
	err = nil

	for {
		n, err = file.Read(data)
		if err != nil || n == 0 {
			break
		}
		
		line, le := PopLine(data, n)

		if line[0] == '!' {
			b.Next = new(Song)
			b = b.Next
			b.Value = line[1:]
		} else {
			s.Next = new(Song)
			s = s.Next
			s.Value = line
		}
		
		seek = int64(le - n)
		file.Seek(seek, 1)
	}
	
	fillAndClean(songs, bad)
	
	return songs.Next
}

func writeStringToValue(path string, s string) {
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

func exit(p *Player) {
	os.RemoveAll(p.tmpDir)
	os.Exit(0)
}

func random(p *Player) {
	p.random = true
	f, err := os.Create(p.tmpDir + SuffixIsRandom)
	if err == nil {
		f.Close()
	}
}

func normal(p *Player) {
	p.random = false
	os.Remove(p.tmpDir + SuffixIsRandom)
}

func pause(p *Player) {
	p.snd.SetState(gst.STATE_PAUSED)
	f, err := os.Create(p.tmpDir + SuffixIsPaused)
	if err == nil {
		f.Close()
	}
}

func resume(p *Player) {
	p.snd.SetState(gst.STATE_PLAYING)
	os.Remove(p.tmpDir + SuffixIsPaused)
}

func increaseVolume(p *Player) {
	p.volume += 0.05
	updateVolume(p)
}

func decreaseVolume(p *Player) {
	p.volume -= 0.05
	updateVolume(p)
}

func updateVolume(p *Player) {
	if (p.volume > 1.0) {
		p.volume = 1.0
	} else if (p.volume < 0.0) {
		p.volume = 0.0
	}

	p.snd.SetProperty("volume", p.volume)
	
	writeStringToValue(p.tmpDir + SuffixVolume,
		fmt.Sprintf("%d\n", int(p.volume * 100)))
}

func popUpcoming(p *Player) *Song {
	var s *Song
	var data []byte = make([]byte, 2048)
	var top string
	
	upcomingFile, err := os.Open(p.tmpDir + SuffixUpcoming)
	if err != nil {
		panic(err)
	}

	/* hopefully 2048 is enough bytes for the first line */
	n, err := upcomingFile.Read(data)
	if err != nil {
		upcomingFile.Close()
		return nil
	}
	
	tmpFile, err := os.Create(p.tmpDir + "/.upcoming.tmp")
	
	top, le := PopLine(data, n)
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
	os.Remove(p.tmpDir + SuffixUpcoming)
	os.Rename(p.tmpDir + "/.upcoming.tmp",
	          p.tmpDir + SuffixUpcoming)

	for s = p.songs.Next; s != nil; s = s.Next {
		if strings.Contains(s.Value, top) {
			return s
		}
	}
	
	s = new(Song)
	s.Next = nil
	s.Value = top
	return s
}

func playNext(p *Player) {
	p.snd.SetState(gst.STATE_NULL)
	
	old := p.current
	p.current = popUpcoming(p)
	
	if p.current == nil {
		if p.random {
			p.current = randomSong(p)
		} else {
			if old != nil {
				p.current = old.Next
				if p.current == nil {
					p.current = p.songs.Next
				}
			} else {
				p.current = p.songs.Next
			}
		}
	
		if p.current == nil {
			fmt.Println("There are no songs to play!")
			return
		}
	}
	
	uri := makeURI(p.current.Value)
	p.snd.SetProperty("uri", uri)
	p.snd.SetState(gst.STATE_PLAYING)

	os.Remove(p.tmpDir + SuffixIsPaused)
	writeStringToValue(p.tmpDir + SuffixPlaying, uri + "\n")
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
	syscall.Mkfifo(path + SuffixIn, 0700)
	
	f, _ = os.Create(path + SuffixUpcoming)
	f.Close()
	f, _ = os.Create(path + SuffixPlaying)
	f.Close()
	f, _ = os.Create(path + SuffixVolume)
	f.Close()
}

func listenFifo(p *Player) {
	var data = make([]byte, 512)

	for {
		in, err := os.Open(p.tmpDir + SuffixIn)
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
		time.Sleep(1000 * 1000)
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
	
	rand.Seed(int64(time.Now().Nanosecond()))
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
	
	p.tmpDir = *tmpDir
	p.volume = float64(*volume) / 100
	p.random = *random
	p.lock = new(sync.Mutex)
	p.songs = new(Song)
	
	populateTmp(p.tmpDir)
	
	file, err := os.Create(p.tmpDir + SuffixPlaylist)
	if err != nil {
		panic(err)
	}
	
	if *readstdin {
		p.songs.Next = scan(os.Stdin)
	}
	
	for _, name := range flag.Args() {
		f, err := os.Open(name)
		if err != nil {
			panic(err)
		}
		
		var t *Song
		for t = p.songs; t != nil && t.Next != nil; t = t.Next {}
		
		t.Next = scan(f)
		
		f.Close()
	}
	
	playlist, err := os.Create(p.tmpDir + SuffixPlaylist)
	if err != nil {
		panic(err)
	}
	
	for s := p.songs.Next; s != nil; s = s.Next {
		p.size++
		playlist.WriteString(s.Value + "\n")
	}
	
	playlist.Close()

	updateVolume(p)
	if p.random {
		file, err = os.Create(p.tmpDir + SuffixIsRandom)
		if err == nil {
			file.Close()
		}
		
	}
	
	playNext(p)
	
	go listenFifo(p)
	go listenBus(p)
	
	/* Wait for SIGTERM/INT signal */
	_ = <-sigChan
	os.RemoveAll(p.tmpDir)
	os.Exit(0)
}
