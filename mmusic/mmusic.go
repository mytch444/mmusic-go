package main

import (
	"fmt"
	"os"
	"io"
	"os/signal"
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
var SuffixPlaying string    = "/playing"
var SuffixIsRandom string   = "/israndom"
var SuffixIsPaused string   = "/ispaused"

type Song struct {
	Value string
	Next *Song
}

type Player struct {
	snd *gst.Element
	bus *gst.Bus
	
	size int64 
	songs *Song
	
	current *Song

	random bool
	
	playingFile *os.File
	
	tmpDir string
}

func PopLine(file *os.File) (string, error) {
	var i int 
	data := make([]byte, 80)
	line := ""
	
	for {
		n, err := file.Read(data)
		if err != nil {
			return line, err
		}
		
		for i = 0; i < n; i++ {
			if (data[i] == '\n') {
				break
			}
		}
		
		if i == 0 {
			break
		} else if i < n {
			line += string(data[:i])
			file.Seek(int64(i-n+1), 1)
			break
		} else {
			line += string(data[:n])
		}
	}
	
	return line, nil
}

func fillSubDirs(songs *Song) {
	var prev, next, t, s *Song
	prev = songs
	for s = songs.Next; s != nil; s = s.Next {
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

func scan(file *os.File) (songs *Song) {
	var s *Song
	
	songs = new(Song)
	s = songs
	
	for {
		line, err := PopLine(file)
		if err != nil {
			break
		} else if line == "" {
			continue
		}

		s.Next = new(Song)
		s = s.Next
		s.Value = line
	}
	
	fillSubDirs(songs)
	
	return songs.Next
}

func writeStringToValue(path string, s string) {
	file, err := os.Create(path)
	if err == nil {
		file.WriteString(s)
		file.Close()
	}
}

func (p *Player) Exit() {
	os.RemoveAll(p.tmpDir)
	os.Exit(0)
}

func (p *Player) SetModeRandom() {
	p.random = true
	f, err := os.Create(p.tmpDir + SuffixIsRandom)
	if err == nil {
		f.Close()
	}
}

func (p *Player) SetModeNormal() {
	p.random = false
	os.Remove(p.tmpDir + SuffixIsRandom)
}

func (p *Player) Pause() {
	p.snd.SetState(gst.STATE_PAUSED)
	f, err := os.Create(p.tmpDir + SuffixIsPaused)
	if err == nil {
		f.Close()
	}
}

func (p *Player) Resume() {
	p.snd.SetState(gst.STATE_PLAYING)
	os.Remove(p.tmpDir + SuffixIsPaused)
}

func (p *Player) PopUpcoming() error {
	var s *Song
	var data []byte = make([]byte, 2048)
	var top string
	
	upcomingFile, err := os.Open(p.tmpDir + SuffixUpcoming)
	if err != nil {
		panic(err)
	}
	
	top, err = PopLine(upcomingFile)
	if err != nil {
		return err
	}
	
	if top == "" {
		return io.EOF
	}
	
	tmpFile, err := os.Create(p.tmpDir + "/.tmp")
	if err != nil {
		panic(err)
	}
	
	for {
		n, err := upcomingFile.Read(data)
		if err != nil {
			break
		}
		
		tmpFile.Write(data[:n])
	}
	
	upcomingFile.Close()
	tmpFile.Close()

	os.Rename(p.tmpDir + "/.tmp", p.tmpDir + SuffixUpcoming)

	for s = p.songs.Next; s != nil; s = s.Next {
		if strings.Contains(s.Value, top) {
			break
		}
	}
	
	/* Not in the playlist so make a new song for it */
	if s == nil {
		s = new(Song)
		s.Next = nil
		s.Value = top
	}
	
	p.current = s
	return nil
}

/* TODO: improve this so it's not so random.
 * http://keyj.emphy.de/balanced-shuffle/
 */
func (p *Player) PickRandom() {
	n := int(rand.Int63n(p.size))
	p.current = p.songs.Next
	for i := 0; i < n && p.current != nil; i++ {
		p.current = p.current.Next
	}
}

func (p *Player) PickNormal() {
	if p.current != nil {
		p.current = p.current.Next
	}

	if p.current == nil {
		p.current = p.songs.Next
	}
}

func (p *Player) PickNext() {
	err := p.PopUpcoming()
	if err == nil {
		return
	} else if p.size == 0 {
		p.Exit()
	} else if p.random {
		p.PickRandom()
	} else {
		p.PickNormal()
	}
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

func (p *Player) PlayNext() {
	p.PickNext()
	uri := makeURI(p.current.Value)
	p.snd.SetState(gst.STATE_NULL)
	p.snd.SetProperty("uri", uri)
	p.snd.SetState(gst.STATE_PLAYING)

	os.Remove(p.tmpDir + SuffixIsPaused)
	writeStringToValue(p.tmpDir + SuffixPlaying, uri + "\n")
}

func listenFifo(p *Player, c chan string) {
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
		
		in.Close()
		
		str := string(data[:n])
		for _, name := range strings.Fields(str) {
			c <- name
		}
	}
}

func listenBus(p *Player, c chan *gst.Message) {
	for {
		time.Sleep(time.Second)
		mesg := p.bus.TimedPop(100000000)
		if mesg != nil {
			c <- mesg
		}
	}
}

func doFunction(p *Player, mesg string) {
	if mesg == "exit" {
		p.Exit()
	} else if mesg == "next" {
		p.PlayNext()
	} else if mesg == "random" {
		p.SetModeRandom()
	} else if mesg == "normal" {
		p.SetModeNormal()
	} else if mesg == "pause" {
		p.Pause()
	} else if mesg == "resume" {
		p.Resume()
	}
}

func (p *Player) Run() {
	sigChan := make(chan os.Signal)
	fifoChan := make(chan string)
	busChan := make(chan *gst.Message)
	
	signal.Notify(sigChan, syscall.SIGTERM)
	signal.Notify(sigChan, syscall.SIGINT)
	
	go listenFifo(p, fifoChan)
	go listenBus(p, busChan)
	
	for {
		select {
		case _ = <- sigChan:
			p.Exit()
		case mesg := <- fifoChan:
			doFunction(p, mesg)
		case mesg := <- busChan:
			t := mesg.GetType()
			if t == gst.MESSAGE_EOS || t == gst.MESSAGE_ERROR {
				p.PlayNext()
			}
		}
	}
}

func (p *Player) populateTmp() {
	var f *os.File
	_, err := os.Stat(p.tmpDir)
	if err == nil { /* File exits? */
		fmt.Println("tmp dir:", p.tmpDir, "exists.")
		os.Exit(1)
	}
	
	/* Just hope there are no errors. */
	os.Mkdir(p.tmpDir, 0700)
	syscall.Mkfifo(p.tmpDir + SuffixIn, 0700)
	
	f, _ = os.Create(p.tmpDir + SuffixUpcoming)
	f.Close()
	f, _ = os.Create(p.tmpDir + SuffixPlaying)
	f.Close()
}

func (p *Player) initGst(nsink string) {
	p.snd = gst.ElementFactoryMake("playbin", "mmusic")
	if p.snd == nil {
		fmt.Println("Failed to initialize gst: snd")
		os.Exit(1)
	}
	
	sink := gst.ElementFactoryMake(nsink, "Sink")
	if sink == nil {
		fmt.Println("Failed to initilize gst: ", nsink)
		os.Exit(1)
	}
	p.snd.Link(sink)
	
	p.bus = p.snd.GetBus()
	if p.bus == nil {
		fmt.Println("Failed to open gstreamer bus!")
		os.Exit(1)
	}
	
	p.snd.SetProperty("volume", 1.0)
}

func main () {
	rand.Seed(int64(time.Now().Nanosecond()))
	defaultTmp := fmt.Sprintf("%s/mmusic-%d", os.TempDir(), os.Getuid())
	
	tmpDir	:= flag.String("t", defaultTmp, "Set tmp directory.")
	nsink	:= flag.String("l", "alsasink", "Change gstreamer sink.")
	random	:= flag.Bool("r", true, "Set starting randomness.")

	flag.Parse()
	
	p := new(Player)
	p.tmpDir = *tmpDir
	p.songs = new(Song)
	p.initGst(*nsink)
	p.populateTmp()
	if *random {
		p.SetModeRandom()
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

	p.PlayNext()
	p.Run()
}
