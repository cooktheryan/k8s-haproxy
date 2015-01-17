#! /bin/bash
haproxy -f $1 -sf $(cat /var/run/haproxy.pid)
