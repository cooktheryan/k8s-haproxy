# k8s-haproxy

[![Build Status](https://travis-ci.org/mikedanese/k8s-haproxy.svg)](https://travis-ci.org/mikedanese/k8s-haproxy)
### Replace the kube-proxy with k8s-proxy
This still uses the old networking model
### Run it!

```
docker run --net=host -d quay.io/porch/k8s-haproxy --master="http://pilot10.dev.porch.com:8080"
```
