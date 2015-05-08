#!/bin/mksh

echo next > /tmp/mmusic-$USER_ID/in
sleep 0.1
cat /tmp/mmusic-$USER_ID/state/playing
