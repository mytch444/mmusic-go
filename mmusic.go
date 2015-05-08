package mmusic

import (
	"os"
	"strings"
)

type Song struct {
	Path string
	Next *Song
}

func PopLine(data []byte, n int) (string, int) {
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

func songInBad(bad *Song, song *Song) bool {
	for bad = bad.Next; bad != nil; bad = bad.Next {
		if (strings.Contains(song.Path, bad.Path)) {
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
			file, err := os.Open(s.Path)
			if err != nil {
				continue
			}
			subs, err := file.Readdirnames(0)
			if err != nil {
				continue
			}
			
			next = s.Next
			t = s
			for _, sub := range subs {
				t.Next = new(Song)
				t = t.Next
				t.Path = s.Path + "/" + sub
			}
			t.Next = nil 
			fillAndClean(s, bad)
			t.Next = next

			prev.Next = s.Next
		}
		
		prev = s
	}
}

func Scan(path string) (songs *Song, number int64, err error) {
	var seek int64 = 0
	var n int
	var s, b *Song
	
	f, err := os.Open(path)
	defer f.Close()
	if err != nil {
		return nil, 0, err
	}
	
	songs = new(Song)
	s = songs
	
	bad := new(Song)
	bad.Next = nil
	b = bad
	
	data := make([]byte, 2048)
	err = nil

	for ; ; {
		n, err = f.Read(data)
		if err != nil || n == 0 {
			break
		}
		
		line, le := PopLine(data, n)

		if line[0] == '!' {
			b.Next = new(Song)
			b = b.Next
			b.Path = line[1:]
		} else {
			s.Next = new(Song)
			s = s.Next
			s.Path = line
		}
		
		seek = int64(le - n)
		f.Seek(seek, 1)
	}
	
	fillAndClean(songs, bad)
	
	number = 0
	for s = songs.Next; s != nil; s = s.Next {
		number++
	}
	
	return songs, number, nil
}