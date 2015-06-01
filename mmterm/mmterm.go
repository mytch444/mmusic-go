package main

import (
	"fmt"
	"os"
	"flag"
	"sync"
	"time"
	"strings"
	"io/ioutil"
	"unicode/utf8"
	"regexp"
	"github.com/nsf/termbox-go"
)

var SuffixIn string       = "/in"
var SuffixPlaylist string = "/playlist"
var SuffixUpcoming string = "/upcoming"
var SuffixVolume string   = "/volume"
var SuffixPlaying string  = "/playing"
var SuffixIsRandom string = "/israndom"
var SuffixIsPaused string = "/ispaused"

type Line struct {
	Value string
	Prev *Line
	Next *Line
}

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
	's': gotoPlaying,
	'+': increaseVolume,
	'-': decreaseVolume,
}

var lock *sync.Mutex
var tmp string

var width, height, bottom int

var gettingInput bool
var input []byte
var inputPrefix string
var inputCursor int
var inputFinish (func())

var searchingForward bool
var searchReg *regexp.Regexp

var lines *Line
var cursor *Line
var fromTop int

func getPlaying() string {
	f, err := os.Open(tmp + SuffixPlaying)
	if err != nil {
		termbox.Close()
		panic(err)
	}
	defer f.Close()
	
	data := make([]byte, 2048)
	n, err := f.Read(data)
	
	if n == 0 {
		return ""
	}
	
	return string(data[:n-1])
}

func exit() {
	termbox.Close()
	os.Exit(0)
}

func playCursor() {
	addTopUpcoming()
	next()
}

func addUpcoming() {
	upcoming, err := os.OpenFile(tmp + SuffixUpcoming, os.O_WRONLY, 0666)
	if err != nil {
		termbox.Close()
		panic(err)
	}
	
	upcoming.Seek(0, 2)
	upcoming.Write([]byte(cursor.Value + "\n"))
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
	f.Write([]byte(cursor.Value + "\n"))
	
	upcomingData, err := ioutil.ReadFile(tmp + SuffixUpcoming)
	if err != nil {
		termbox.Close()
		panic(err)
	}
	
	f.Write(upcomingData)
	f.Close()
	os.Rename(tmpName, tmp + SuffixUpcoming)
	
	moveNext()
}

func next() {
	writeToIn("next")
}

func togglePause() {
	_, err := os.Stat(tmp + SuffixIsPaused)
	if err == nil {
		writeToIn("resume")
	} else {
		writeToIn("pause")
	}
}

func toggleRandom() {
	_, err := os.Stat(tmp + SuffixIsRandom)
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

func search() {
	var err error
	var i int
	for i = 0; i < len(input); i++ {
		if input[i] == 0 {
			break
		}
	}
	
	searchReg, err = regexp.Compile(string(input[:i]))
	if err != nil {
		panic(err)
	}
	
	searchNext()
}

func searchForward() {
	searchingForward = true
	getInput("/", search)
}

func searchBackward() {
	searchingForward = false
	getInput("?", search)
}

func searchNext() {
	var l *Line
	if searchReg == nil {
		return
	}
	
	l = cursor
	for l != nil {
		if searchingForward {
			l = l.Next
		} else {
			l = l.Prev
		}
	
		if l != nil && searchReg.MatchString(l.Value) {
			cursor = l
			fromTop = bottom / 2
			break
		}
	}
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
		fromTop = bottom / 2
	}
	cursor = cursor.Next
}

func movePrev() {
	if cursor.Prev == nil {
		return
	}
	
	fromTop--
	if fromTop < 0 {
		fromTop = bottom / 2
	}
	cursor = cursor.Prev
}

func gotoTop() {
	fromTop = 0
	cursor = lines
}

func gotoBottom() {
	fromTop = bottom - 1
	for cursor = lines; cursor.Next != nil; cursor = cursor.Next {}
}

func gotoPlaying() {
	playing := getPlaying()
	
	for l := lines; l != nil; l = l.Next {
		if strings.HasSuffix(playing, l.Value) {
			fromTop = bottom / 2
			cursor = l
			break
		}
	}
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
	for ; cursor.Prev != nil && i < bottom; cursor = cursor.Prev {
		i++
	}
	if i < bottom {
		fromTop = 0
	}
}

func writeToIn(code string) {
	in, err := os.OpenFile(tmp + SuffixIn, os.O_WRONLY, os.ModeNamedPipe)
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
	
	f, err = os.Open(tmp + SuffixIsPaused)
	if err == nil {
		termbox.SetCell(1, bottom, 'P', fg, bg)
		f.Close()
	}
		
	f, err = os.Open(tmp + SuffixIsRandom)
	if err == nil {
		termbox.SetCell(0, bottom, 'R', fg, bg)
		f.Close()
	}
	
	playing := getPlaying()
	putString(playing, 4, bottom, fg, bg)
		
	f, err = os.Open(tmp + SuffixVolume)
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
	
	y = fromTop - 1
	for s := cursor.Prev; s != nil && y >= 0; s = s.Prev {
		putString(s.Value, 0, y, fg, bg)
		y--
	}
	
	putString(cursor.Value, 0, fromTop, termbox.ColorWhite, termbox.ColorBlack)
	
	y = fromTop+1
	for s := cursor.Next; s != nil && y < bottom; s = s.Next {
		putString(s.Value, 0, y, fg, bg)
		y++
	}
}

func getInput(p string, f (func())) {
	if gettingInput {
		return
	}
	
	inputPrefix = p
	inputFinish = f
	gettingInput = true
	for i := 0; i < len(input); i++ {
		input[i] = 0;
	}
	inputCursor = 0
	bottom--
}

func finishInput(good bool) {
	if good {
		inputFinish()
	}
	
	gettingInput = false
	bottom++
	
	termbox.HideCursor()
}

func insertRune(r rune) {
	var i int
	a := string(r)
	l := len(a)
	
	for i = len(input) - 1; i > l && i > inputCursor; i-- {
		input[i] = input[i-l];
	}
	
	for i = 0; i < l; i++ {
		input[inputCursor+i] = a[i]
	}
	
	inputCursor += l
}

func handleInput(ev termbox.Event) {
	if ev.Ch == 0 {
		switch ev.Key {
		case termbox.KeySpace:
			insertRune(' ')
		case termbox.KeyEnter:
			finishInput(true)
		case termbox.KeyEsc:
			finishInput(false)

		case termbox.KeyBackspace:
			fallthrough
		case termbox.KeyBackspace2:
			if inputCursor == 0 {
				return
			}
			_, s := utf8.DecodeLastRuneInString(string(input[:inputCursor]))
			if inputCursor < s {
				return
			}
			
			for i := inputCursor - s; i + s < len(input); i++ {
				input[i] = input[i+s]
			}
			
			inputCursor -= s

		case termbox.KeyArrowLeft:
			fallthrough
		case termbox.KeyCtrlB:
			_, s := utf8.DecodeLastRuneInString(string(input[:inputCursor]))
			if s > 0 {
				inputCursor -= s
			}

		case termbox.KeyArrowRight:
			fallthrough
		case termbox.KeyCtrlF:
			_, s := utf8.DecodeRuneInString(string(input[inputCursor:]))
			if input[inputCursor] != 0 {
				inputCursor += s
			}
		}
	} else {
		insertRune(ev.Ch)
	}
}

func scan(path string) *Line {
	var f, l *Line
	var n, i int
	var err error
	
	file, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer file.Close()
	
	data := make([]byte, 2048)
	f = new(Line)
	l = f
	for {
		n, err = file.Read(data)
		if err != nil || n == 0 {
			break
		}
		
		for i = 0; i < n; i++ {
			if data[i] == '\n' {
				break
			}
		}
		
		if i >= n {
			fmt.Println("2048 was not enough bytes to hold a line, go yell at mytchel")
			panic(nil)
		}
		
		l.Next = new(Line)
		l.Next.Prev = l
		l = l.Next
		l.Value = string(data[:i])
		
		file.Seek(int64(1 + i - n), 1)
	}
	
	f.Next.Prev = nil
	l.Next = nil
	return f.Next
}

func main () {
	var tmpDir *string
	var err error
	
	defaultTmp := fmt.Sprintf("%s/mmusic-%d", os.TempDir(), os.Getuid())
	tmpDir = flag.String("t", defaultTmp, "Set tmp directory.")
	
	flag.Parse()
	
	lock = new(sync.Mutex)
	tmp = *tmpDir

	lines = scan(tmp + SuffixPlaylist)

	cursor = lines
	if cursor == nil {
		fmt.Println("What is the point if there is nothing to manage?")
		os.Exit(1)
	}

	err = termbox.Init()
	if err != nil {
		panic(err)
	}
	
	termbox.SetInputMode(termbox.InputEsc)
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	termbox.HideCursor()
	
	width, height = termbox.Size()
	bottom = height - 1
	fromTop = 0
	
	searchReg = nil
	
	input = make([]byte, 2048)
	gettingInput = false
	
	drawMain()
	go func() {
		for {
			lock.Lock()
			drawBar()
			lock.Unlock()
			time.Sleep(1000 * 1000 * 100)
		}
	}()
	
	for {
		ev := termbox.PollEvent()
		lock.Lock()
		switch ev.Type {
		case termbox.EventKey:
			 /* getting input from bottom */
			if gettingInput {
				handleInput(ev)
			} else { /* just a normal key */
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
			}
		case termbox.EventResize:
			width = ev.Width
			height = ev.Height
			bottom = height - 1
		case termbox.EventError:
			panic(ev.Err)
		}
		
		termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
		drawMain()
		drawBar()
		
		if gettingInput {
			fg := termbox.ColorBlack
			bg := termbox.ColorWhite

			putString(inputPrefix, 0, bottom + 1, fg, bg)
			putString(string(input), len(inputPrefix), bottom + 1, fg, bg)
			
			termbox.SetCursor(len(inputPrefix) + len(string(input[:inputCursor])), bottom + 1)
		}
		
		termbox.Flush()
		lock.Unlock()
	}
	
	termbox.Close()
}
