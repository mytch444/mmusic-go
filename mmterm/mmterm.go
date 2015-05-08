package main

import (
	"fmt"
	"os"
	"flag"
	"sync"
	"time"
	"io/ioutil"
	"github.com/nsf/termbox-go"
	"github.com/mytch444/mmusic/lib"
)

var ctrlKeys = map[termbox.Key](func()) {
	termbox.KeyEnter: playCursor,
	termbox.KeyArrowDown: moveNext,
	termbox.KeyArrowUp: movePrev,
	termbox.KeyArrowRight: next,
	termbox.KeyPgdn: pageDown,
	termbox.KeyPgup: pageUp,
	termbox.KeyCtrlF: pageDown,
	termbox.KeyCtrlB: pageUp,
	termbox.KeyHome: gotoTop,
	termbox.KeyEnd: gotoBottom,
	termbox.KeyCtrlC: exit,
}

var normKeys = map[rune](func()) {
	'q': exit,
	'a': addUpcoming,
	'A': addTopUpcoming,
	'l': next,
	'p': togglePause,
	' ': togglePause,
	'r': toggleRandom,
	'/': searchForward,
	'?': searchBackward,
	'n': searchNext,
	'N': searchNextInverse,
	'1': viewPlaylists,
	'2': viewPlaylist,
	'3': viewUpcoming,
	'j': moveNext,
	'k': movePrev,
	'g': gotoTop,
	'G': gotoBottom,
	'+': increaseVolume,
	'-': decreaseVolume,
}

var lock *sync.Mutex
var tmp string

var width, height, bottom int

var songs *mmusic.Song
var cursor *mmusic.Song
var fromTop int

func exit() {
	termbox.Close()
	os.Exit(0)
}

func playCursor() {
	addTopUpcoming()
	next()
}

func addUpcoming() {
	upcoming, err := os.OpenFile(tmp + mmusic.SuffixUpcoming, os.O_WRONLY, 0666)
	if err != nil {
		termbox.Close()
		panic(err)
	}
	
	upcoming.Seek(0, 2)
	upcoming.Write([]byte(cursor.Path + "\n"))
	upcoming.Close()
	
	moveNext()
}

func addTopUpcoming() {
	f, err := ioutil.TempFile(os.TempDir(), ".mmusic")
	if err != nil {
		termbox.Close()
		panic(err)
	}
	
	tmpName := f.Name()
	f.Write([]byte(cursor.Path + "\n"))
	
	upcomingData, err := ioutil.ReadFile(tmp + mmusic.SuffixUpcoming)
	if err != nil {
		termbox.Close()
		panic(err)
	}
	
	f.Write(upcomingData)
	f.Close()
	os.Rename(tmpName, tmp + mmusic.SuffixUpcoming)
	
	moveNext()
}

func next() {
	writeToIn("next")
}

func togglePause() {
	_, err := os.Stat(tmp + mmusic.SuffixIspaused)
	if err == nil {
		writeToIn("resume")
	} else {
		writeToIn("pause")
	}
}

func toggleRandom() {
	_, err := os.Stat(tmp + mmusic.SuffixIsrandom)
	if err == nil {
		writeToIn("normal")
	} else {
		writeToIn("random")
	}
}

func increaseVolume() {
	writeToIn("increase")
}

func decreaseVolume() {
	writeToIn("decrease")
}

func searchForward() {
	
}

func searchBackward() {

}

func searchNext() {

}

func searchNextInverse() {

}

func viewPlaylists() {

}

func viewPlaylist() {

}

func viewUpcoming() {

}

func moveNext() {
	if cursor.Next == nil {
		return
	}
	
	fromTop++
	if fromTop >= bottom {
		fromTop = 0
	}
	cursor = cursor.Next
}

func movePrev() {
	if cursor.Prev == songs {
		return
	}
	
	fromTop--
	if fromTop < 0 {
		fromTop = bottom - 1
	}
	cursor = cursor.Prev
}

func gotoTop() {
	fromTop = 0
	cursor = songs.Next
}

func gotoBottom() {
	fromTop = bottom - 1
	for cursor = songs.Next; cursor.Next != nil; cursor = cursor.Next {}
}

func pageDown() {
	i := 0
	for ; cursor.Next != nil && i < bottom; cursor = cursor.Next {
		i++
	}
	if i < bottom {
		fromTop = bottom -1
	}
}

func pageUp() {
	i := 0
	for ; cursor.Prev != songs && i < bottom; cursor = cursor.Prev {
		i++
	}
	if i < bottom {
		fromTop = 0
	}
}

func writeToIn(code string) {
	in, err := os.OpenFile(tmp + mmusic.SuffixIn, os.O_WRONLY, os.ModeNamedPipe)
	if err != nil {
		termbox.Close()
		panic(err)
	}
	in.WriteString(code)
	in.Close()
}

func putString(str string, x, y int, fg, bg termbox.Attribute) {
	for i := 0; i < len(str) && x + i < width; i++ {
		termbox.SetCell(x+i, y, rune(str[i]), fg, bg)
	}
}

func drawBar() {
	var err error
	var n int
	var f *os.File
	data := make([]byte, 2048)
	fg := termbox.ColorWhite | termbox.AttrBold
	bg := termbox.ColorBlack

	for i := 0; i < width; i++ {
		termbox.SetCell(i, bottom, ' ', fg, bg)
	}
	
	f, err = os.Open(tmp + mmusic.SuffixIspaused)
	if err == nil {
		termbox.SetCell(1, bottom, 'P', fg, bg)
		f.Close()
	}
		
	f, err = os.Open(tmp + mmusic.SuffixIsrandom)
	if err == nil {
		termbox.SetCell(0, bottom, 'R', fg, bg)
		f.Close()
	}
	
	f, err = os.Open(tmp + mmusic.SuffixPlaying)
	if err != nil {
		termbox.Close()
		panic(err)
	}
	
	n, err = f.Read(data)
	
	if n > 0 {
		playing := string(data[:n-1])
		putString(playing, 4, bottom, fg, bg)
	}
	
	f.Close()
		
	f, err = os.Open(tmp + mmusic.SuffixVolume)
	if err != nil {
		termbox.Close()
		panic(err)
	}
	
	n, err = f.Read(data)
	
	if n > 1 {
		termbox.SetCell(width-1, bottom, '%', fg, bg)
		for i := 2; i <= n; i++ {
			termbox.SetCell(width-i, bottom, rune(data[n-i]), fg, bg)
		}
	}

	f.Close()
		
	termbox.Flush()
}

func drawMain() {
	var y int
	fg := termbox.ColorBlack
	bg := termbox.ColorWhite
	
	y = fromTop-1
	for s := cursor.Prev; s != nil && y >= 0; s = s.Prev {
		putString(s.Path, 0, y, fg, bg)
		y--
	}
	
	putString(cursor.Path, 0, fromTop, termbox.ColorWhite, termbox.ColorBlack)
	
	y = fromTop+1
	for s := cursor.Next; s != nil && y < bottom; s = s.Next {
		putString(s.Path, 0, y, fg, bg)
		y++
	}
}

func main () {
	var tmpDir *string
	var err error
	
	defaultTmp := fmt.Sprintf("%s/mmusic-%d", os.TempDir(), os.Getuid())
	tmpDir = flag.String("t", defaultTmp, "Set tmp directory.")
	
	flag.Parse()
	
	lock = new(sync.Mutex)
	tmp = *tmpDir

	err = termbox.Init()
	if err != nil {
		panic(err)
	}
	
	songs, err = mmusic.Scan(tmp + mmusic.SuffixPlaylist)
	if err != nil {
		termbox.Close()
		panic(err)
	}
	
	cursor = songs.Next
	if cursor == nil {
		termbox.Close()
		fmt.Println("What is the point if there is nothing to manage?")
		os.Exit(1)
	}
	
	termbox.SetInputMode(termbox.InputEsc)
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	termbox.HideCursor()
	
	width, height = termbox.Size()
	bottom = height - 1
	fromTop = 0
	
	drawMain()
	go func() {
		for {
			lock.Lock()
			drawBar()
			lock.Unlock()
			time.Sleep(1000 * 1000 * 10)
		}
	}()
			
	
	for {
		ev := termbox.PollEvent()
		lock.Lock()
		switch ev.Type {
		case termbox.EventKey:
			if ev.Ch > 0 {
				f := normKeys[ev.Ch]
				if f != nil {
					f()
				}
			} else {
				f := ctrlKeys[ev.Key]
				if f != nil {
					f()
				}
			}
		case termbox.EventResize:
			width = ev.Width
			height = ev.Height
		case termbox.EventError:
			termbox.Close() /* possible? */
			panic(ev.Err)
		}
		
		
		termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
		drawMain()
		drawBar()
		
		termbox.Flush()
		lock.Unlock()
	}
	
	termbox.Close()
}
