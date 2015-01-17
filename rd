./build.sh && docker run --name ha -ti quay.io/porch/k8s-haproxy /bin/bash && docker rm -f ha
