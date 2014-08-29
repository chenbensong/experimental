#!/bin/bash

# Read a file of graphite data and send to server using "nc".
# File can be get from MySQL database, like:
# mysql -h 173.194.104.24 -uroot -p'pass' skia -N -B -e "select CONCAT('size.libskia', file, '_a') as name, size, UNIX_TIMESTAMP(ts) as ts from sizes where ts < '2014-08-15 18:49:49'" > /Users/bensong/backfill.txt

PORT=2003
SERVER=23.236.55.44
INFILE=/home/default/backfill.txt

ts=""
echo "Start"
while read -r line
do
  arr=(${line//\t/})
  echo "${arr[0]} ${arr[1]} ${arr[2]}" | nc -q0 ${SERVER} ${PORT}
  if [ "$ts" != "${arr[2]}" ]
  then
    echo "Processed: `date -d @${arr[2]}`"
    ts=${arr[2]}
  fi 
  sleep 0.5s
done < $INFILE
