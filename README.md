#mmusicd

Music managment daemon. Takes a playlist file (described later) as an argument
or from stdin. It uses gstreamer.

Try `mmusicd -h` for some options.

When run it will create and populate the following directory (you can change
the path with the `-t` option). If this already exists mmusicd will exit and
give a warning.

    $tmp # defaults to /tmp/mmusic-$USER_ID
    	in
    	playlist # symbolic link or copy of playlist file?
    	upcoming
    	state # change the values of these files will not change the daemon
    	        state as cool as that would be. Maybe latter on.

		ispaused 	# if this file exists, playback is paused.
    		israndom	# same as above but for randomness.
    		playing		# contains the path to the current
    				  playing file.
    		volume		# contains the volume level.

Reads upcoming file to find if there is anything it should play, else
depending on mode selects a random song or the next alphanumericaly in
it's library.

The `in` fifo will listen for the following commands.

    next		# goes to next song.

    scan		# rescans the playlist and builds its library of 
    			  music files.

    random		# sets mode to random

    normal		# sets mode to normal
    
    pause		# pauses playback

    resume		# resumes playback

    increase		# increase the volume by 1%

    decrease		# decrease the volume by 1%

    mute		# set volume to 0%

Note: There seems to be some problems with the fifo. Don't write 
things to it too quickly. ie: if you do a loop in bash put a delay
of a few milliseconds in between each iteration.

In playlist files you can list paths to directories or files. When
mmusicd scans the playlist lines that are directories will be searched
and any music files (and subdirs) will be added to the library.

If mmusicd comes accross a line that begins with a '!' all files that
match that path will be ignored. This is so you can for example add
"/media/music" then add "!/media/music/Katy Perry" to exclude Katy Perry,
not that I have anything against Katy Perry.

In terms of playlist managment `mmusicd` doesn't really do anything. When
you run it give playlist files as arguments and/or stdin (give `--`)
 lines to it and it will write everything you give it to `$tmp/playlist`.
You can add things to this as you go (the write `scan` to `$tmp/in`).

Send SIGTERM to mmusicd to stop it.

Works with any sort of files gstreamer can play (so it should be able
to play network streams, can't confirm yet).

Note; Do not need the ability to play a certain file as the user can
add said file to `upcoming` then `next` to play it.

#mmusic

mmusic library that is used by mmusicd and `insert controller name`
for common functions such as parsing playlist files and not much
else.

#`insert controller name`

A termbox-go controller for mmusicd. From it you can choose playlists,
manage their contents, select songs to play, add to upcoming (start and
end), toggle random, pause, change volume and view what is playing.

In otherwords, an interface that makes everything easier to see.