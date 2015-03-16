# k8s-haproxy

[![Build Status](https://travis-ci.org/mikedanese/k8s-haproxy.svg)](https://travis-ci.org/mikedanese/k8s-haproxy)
### Replace the kube-proxy with k8s-proxy
This still uses the old networking model
### Run it!

```
docker run --net=”host” --env KUBE_APISERVER_ADDR=pilot10.qa.porch.com --env KUBE_APISERVER_PORT=7080 -d quay.io/porch/k8s-haproxy
```
