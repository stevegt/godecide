#!/bin/bash

while true
do inotifywait -e modify college.yaml
    godecide college.yaml stdout > college.dot.tmp
    mv college.dot.tmp college.dot
    sleep 1
done

# ...then start `xdot college.dot` in a separate window
