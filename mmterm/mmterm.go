package main

import (
	"fmt"
	"os"
	"os/exec"
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

type View struct {
	Cursor *Line
	Lines *Line
	FromTop int
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
	termbox.KeySpace: togglePause,
}

var normKeys = map[rune](func()) {
	'q': exit,
	'a': addUpcoming,
	'A': addTopUpcoming,
	'l': next,
	'p': togglePause,
	'r': toggleRandom,
	'R': refresh,
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
	'c': gotoPlaying,
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
var currentView int

var ViewNothing   int = 0
var ViewPlaylists int = 1
var ViewPlaylist  int = 2
var ViewUpcoming  int = 3
var views [4]*View

var playlistDir string

func exit() {
	termbox.Close()
	os.Exit(0)
}

func startMMusic(playlist string) {
	cmd := exec.Command("mmusic", playlist)
	err := cmd.Start()
	if err != nil {
		termbox.Close()
		fmt.Println("Failed to start mmusic")
		os.Exit(1)
	}
}

func playCursor() {
	if cursor != nil {
		if currentView == ViewPlaylists {
			writeToIn("exit")
			startMMusic(playlistDir + "/" + cursor.Value)
			time.Sleep(100000000)
			refresh()
			viewPlaylist()
			gotoPlaying()
		} else {
			addTopUpcoming()
			next()
		}
	}
}

func addUpcoming() {
	if cursor == nil || currentView == ViewPlaylists {
		return
	}
	
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
	if cursor == nil || currentView == ViewPlaylists {
		return
	}
	
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
	if cursor == nil || searchReg == nil {
		return
	}
	
	l = cursor
	for l != nil {
		if searchingForward {
			l = l.Next
		} else {
			l = l.Prev
		}
	
		if l == nil {
			break;
		} else if searchReg.MatchString(l.Value) {
			cursor = l
			fromTop = bottom / 2
			break
		}
	}
}

func searchNextInverse() {
	searchingForward = !searchingForward
	searchNext()
	searchingForward = !searchingForward
}

func saveCurrentView() {
	if currentView < 0 {
		return
	}
	
	views[currentView].FromTop = fromTop
	views[currentView].Cursor = cursor
	views[currentView].Lines = lines
}

func loadView(view int) {
	fromTop = views[view].FromTop
	cursor = views[view].Cursor
	lines = views[view].Lines
	currentView = view
}

func viewPlaylists() {
	saveCurrentView()
	loadView(ViewPlaylists)
}

func viewPlaylist() {
	saveCurrentView()
	loadView(ViewPlaylist)
}

func viewUpcoming() {
	saveCurrentView()
	loadView(ViewUpcoming)
	
	go func() {
		for currentView == ViewUpcoming {
			lock.Lock()
			
			saveCurrentView()
			views[ViewUpcoming].Lines = scan(tmp + SuffixUpcoming)
			updateCursor(views[ViewUpcoming])
			loadView(ViewUpcoming)
			
			redraw()
			lock.Unlock()
			time.Sleep(1000000000)
		}
	}()
}

func moveNext() {
	if cursor == nil || cursor.Next == nil {
		return
	}
	
	fromTop++
	if fromTop >= bottom {
		fromTop = bottom / 2
	}
	cursor = cursor.Next
}

func movePrev() {
	if cursor == nil || cursor.Prev == nil {
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
	if cursor == nil {
		return
	}
	
	fromTop = bottom - 1
	for cursor = lines; cursor.Next != nil; cursor = cursor.Next {}
}

func findLine(value string, l *Line) *Line {
	for ; l != nil; l = l.Next {
		if strings.HasSuffix(value, l.Value) {
			return l
		}
	}
	return nil
}

func gotoPlaying() {
	if currentView == ViewPlaylists {
		return
	}
	
	playing := getPlaying()
	if playing == "" {
		return
	}
	l := findLine(playing, lines)
	if l != nil {
		fromTop = bottom / 2
		cursor = l
	}
}

func pageDown() {
	if cursor == nil {
		return
	}
	
	i := 0
	for ; cursor.Next != nil && i < bottom; cursor = cursor.Next {
		i++
	}
	if i < bottom {
		fromTop = bottom -1
	}
}

func pageUp() {
	if cursor == nil {
		return
	}
	
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
	if err == nil {
		in.WriteString(code)
		in.Close()
	}
}

func putString(str string, x, y int, fg, bg termbox.Attribute) {
	len := 0
	i := 0
	for r, s := utf8.DecodeRuneInString(str[len:]);
	    s > 0;
	    r, s = utf8.DecodeRuneInString(str[len:]) {
		termbox.SetCell(x+i, y, r, fg, bg)
		len += s
		i++
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
	if err == nil {
		n, err = f.Read(data)
	
		termbox.SetCell(width-1, bottom, '%', fg, bg)
		for i := 2; i <= n; i++ {
			termbox.SetCell(width-i, bottom,
			                rune(data[n-i]), fg, bg)
		}
		f.Close()
	}
	
	termbox.Flush()
}

func drawMain() {
	var y int
	fg := termbox.ColorBlack
	bg := termbox.ColorWhite
	
	if cursor == nil {
		return
	}
	
	y = fromTop - 1
	for s := cursor.Prev; s != nil && y >= 0; s = s.Prev {
		putString(s.Value, 0, y, fg, bg)
		y--
	}
	
	putString(cursor.Value, 0, fromTop,
	          termbox.ColorWhite, termbox.ColorBlack)
	
	y = fromTop+1
	for s := cursor.Next; s != nil && y < bottom; s = s.Next {
		putString(s.Value, 0, y, fg, bg)
		y++
	}
}

func redraw() {
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	drawMain()
	drawBar()
		
	if gettingInput {
		fg := termbox.ColorBlack
		bg := termbox.ColorWhite

		putString(inputPrefix, 0, bottom + 1, fg, bg)
		putString(string(input), len(inputPrefix),
		          bottom + 1, fg, bg)
			
		termbox.SetCursor(len(inputPrefix) +
		                  len(string(input[:inputCursor])),
		                  bottom + 1)
	}
		
	termbox.Flush()
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
			_, s := utf8.DecodeLastRuneInString(
			          string(input[:inputCursor]))
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
			_, s := utf8.DecodeLastRuneInString(
			          string(input[:inputCursor]))
			if s > 0 {
				inputCursor -= s
			}

		case termbox.KeyArrowRight:
			fallthrough
		case termbox.KeyCtrlF:
			_, s := utf8.DecodeRuneInString(
			           string(input[inputCursor:]))
			if input[inputCursor] != 0 {
				inputCursor += s
			}
		}
	} else {
		insertRune(ev.Ch)
	}
}

func getPlaying() string {
	f, err := os.Open(tmp + SuffixPlaying)
	if err != nil {
		return ""
	}
	defer f.Close()
	
	data := make([]byte, 2048)
	n, err := f.Read(data)
	
	if n == 0 {
		return ""
	}
	
	return string(data[:n-1])
}

func scan(path string) *Line {
	var f, l *Line
	var n, i int
	var err error
	
	file, err := os.Open(path)
	if err != nil {
		return nil
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
			fmt.Println("2048 was not enough bytes to hold a line!")
			panic(nil)
		}
		
		l.Next = new(Line)
		l.Next.Prev = l
		l = l.Next
		l.Value = string(data[:i])
		
		file.Seek(int64(1 + i - n), 1)
	}
	
	if f.Next != nil {
		f.Next.Prev = nil
		l.Next = nil
	}
	return f.Next
}

func updateCursor(view *View) {
	if view.Cursor != nil {
		l := findLine(view.Cursor.Value, view.Lines)
		if l == nil {
			view.Cursor = view.Lines
			view.FromTop = 0
		} else {
			view.Cursor = l
		}
	} else {
		view.Cursor = view.Lines
	}
}

func readPlaylists() *Line {
	playlistDir = os.Getenv("HOME") + "/.config/mmusic"
	dir, err := os.Open(playlistDir)
	if err != nil {
		termbox.Close()
		fmt.Println("Failed to open", playlistDir)
		os.Exit(1)
	}
	
	playlists, err := dir.Readdirnames(0)
	
	lines := new(Line)
	l := lines
	for _, playlist := range playlists {
		l.Next = new(Line)
		l.Next.Prev = l
		l = l.Next
		l.Value = playlist
	}
	
	if lines.Next != nil {
		lines.Next.Prev = nil
		l.Next = nil
	}
	return lines.Next
}

func refresh() {
	views[ViewPlaylists].Lines = readPlaylists()
	updateCursor(views[ViewPlaylists])
	
	views[ViewPlaylist].Lines = scan(tmp + SuffixPlaylist)
	updateCursor(views[ViewPlaylist])
	
	views[ViewUpcoming].Lines = scan(tmp + SuffixUpcoming)
	updateCursor(views[ViewUpcoming])
	
	loadView(currentView)
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
	
	termbox.SetInputMode(termbox.InputEsc)
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	termbox.HideCursor()
	
	width, height = termbox.Size()
	bottom = height - 1
	fromTop = 0
	
	for i := 0; i < len(views); i++ {
		views[i] = new(View)
	}
	
	refresh()
	currentView = -1
	
	_, err = os.Stat(tmp)
	if err != nil {
		viewPlaylists()
	} else {
		viewPlaylist()
		gotoPlaying()
	}
	
	input = make([]byte, 2048)
	gettingInput = false
	
	drawMain()
	go func() {
		for {
			lock.Lock()
			drawBar()
			lock.Unlock()
			time.Sleep(10000000)
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
		
		redraw()

		lock.Unlock()
	}
	
	termbox.Close()
}
