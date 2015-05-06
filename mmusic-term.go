package main

import (
	"github.com/nsf/termbox-go"
	"fmt"
)

func handleKey(ev *termbox.Event) {
	if ev.Mod & termbox.ModAlt != 0 {
		fmt.Println("alt!")
	}
	termbox.SetCell(10, 10, rune(ev.Ch), termbox.ColorWhite, termbox.ColorCyan)
}

func main () {
	err := termbox.Init()
	if err != nil {
		panic(err)
	}
	defer termbox.Close()
	
	termbox.SetInputMode(termbox.InputEsc)
	
	termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
	
	for {
		ev := termbox.PollEvent()
		switch ev.Type {
		case termbox.EventKey:
			if ev.Ch == 'q' {
				return
			}
			handleKey(&ev)
			
		case termbox.EventResize:
			termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)
		case termbox.EventError:
			panic(ev.Err)
		}
		termbox.Flush()
	}
}
