#!/bin/bash

typeset -i sig=1
while (( sig < 65 )); do
    trap "echo '$sig'" $sig 2>/dev/null 
    let sig=sig+1
done

>&2 echo "Send signals to PID $$ and type [q] when done."

while :
do 
  read -n1 input
  [ "$input" == "q" ] && break
  sleep .1
done
