#!/bin/bash
# code-view workspace launcher
# Run from any project dir: ~/Projects/code-view/run.sh

pkill -x code-view 2>/dev/null
sleep 0.2

/home/ds0nt/Projects/code-view/code-view serve --bind 0.0.0.0 &
sleep 0.4

xdg-open http://127.0.0.1:7878 2>/dev/null &

HOST=$(hostname -I | awk '{print $1}')
echo "code-view running at http://127.0.0.1:7878"
echo "network access:     http://$HOST:7878"
echo ""
echo "Mac usage:"
echo "  CODEVIEW_HOST=http://$HOST:7878 code-view show <file>"
echo "  code-view --host $HOST show <file>"
