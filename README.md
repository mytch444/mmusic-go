#mmusic

Music managment daemon written in go, it uses gstreamer. So you will
need to get [some bindings](github.com/ziutek/gst) first.

Try `mmusic -h` for some options.

To set what songs `mmusic` plays add filenames of playlist files
(explaned further down) as arguments and/or to stdin. And no, `mmusic`
does not fork to the background if you want that do it with your shell.

When run it will create and populate the following directory (you can
change the path with the `-t` option). If this already exists `mmusic`
will give a warning and exit.

    $tmp                        # defaults to /tmp/mmusic-$USER_ID

        in                      # fifo that listens that you can control
                                  mmusic with.

        playlist                # files that are in the lineup, these are
                                  added by looking through the playlist
                                  files given at startup.

        upcoming                # add file paths (or uri's) and they
                                  will be played next.

        playing                 # contains the uri currently playing.

        volume                  # contains the volume percentage.

        ispaused                # if this file exists, playback has been
                                  paused. No creating it does not pause
                                  playback... yet.

        israndom                # same as above but for randomness.

The `in` fifo will listen for the following commands.

    exit                # exits

    next                # goes to next song.

    random              # sets mode to random

    normal              # sets mode to normal

    pause               # pauses playback

    resume              # resumes playback

    increase            # increase the volume by 5%

    decrease            # decrease the volume by 5%

Once the current stream ends or you write `next` to `in` `mmusic` will
reads the upcoming file to find if there is anything it should play,
if upcoming is empty depending on mode selects a random song or the next
alphanumericaly in it's library.

In playlist files you can list uri's or paths (absolute or relative)
to directories or files. When `mmusic` scans the playlist lines that
are directories will be searched and any music files (and subdirs)
will be added to the library.

If `mmusic` comes accross a line that begins with a '!' all files that
begin with the remainder of the line will be ignored. This is so you
can for example add "/media/music" then add "!/media/music/Katy Perry"
to exclude Katy Perry, not that I have anything against Katy Perry.

In terms of playlist managment `mmusic` doesn't really do anything. When
you run it, give playlist files either as arguments or piped to stdin
(with `-stdin` option) and it will populate `$tmp/playlist` with the files
it found in subdirectories of paths given.

Sending SIGTERM to `mmusic` has the same effect as writing `exit` to the
fifo.

Works with any sort of files gstreamer can play (so yes, can play network
streams).

#mmterm/mmterm

Note: Not fully functional yet.

A termbox-go controller for `mmusic`. From it you can choose playlists,
manage their contents, select songs to play, add to upcoming (start and
end), toggle random, pause, change volume and view what is playing.

In otherwords, an interface that makes everything easier to see as well
as adding better playlist controls.

It stores it's playlists in `$XDG_CONFIG/mmterm/`

#Notes

Note: There seems to be some problems with the fifo. Don't write things
to it too quickly. ie: if you do a loop in bash put a delay of a few
milliseconds in between each iteration.

